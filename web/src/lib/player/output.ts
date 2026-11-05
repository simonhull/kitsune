import { linear } from './gain.ts';

/** What a track needs to sound: where it is, and how long the scan says it runs. */
export interface Playable {
	url: string;
	seconds: number;
}

/** What an output says back. Only the track in play speaks: what's preloading has no voice. */
export interface OutputEvents {
	/** The position moved, in seconds. */
	time(seconds: number): void;
	/** Sound is coming out. */
	running(): void;
	/** Sound stopped, and nobody here asked for that. */
	stopped(): void;
	ended(): void;
	/** The media failed. It can't say why: a 401, a 404 and a bad file all read the same. */
	failed(): void;
}

/** Where sound goes: the browser, or nowhere at all. */
export interface AudioOutput {
	listen(events: OutputEvents): void;
	canPlay(mediaType: string): boolean;
	/** Makes this the track in play. What's preloaded takes over at once. */
	load(track: Playable): void;
	/** Fetches what comes next, on the side. */
	preload(track: Playable): void;
	/** Rejects as `HTMLMediaElement.play` does. */
	play(): Promise<void>;
	pause(): void;
	seek(seconds: number): void;
	setGain(decibels: number): void;
	/** Silences everything and lets go of both tracks, keeping whatever was built to play them. */
	stop(): void;
}

const mediaTypes = new Map([
	['FLAC', 'audio/flac'],
	['MP3', 'audio/mpeg'],
	['M4A', 'audio/mp4'],
	['M4B', 'audio/mp4'],
	['Ogg Vorbis', 'audio/ogg'],
	['Opus', 'audio/ogg'],
	['WAV', 'audio/wav'],
	['AIFF', 'audio/aiff']
]);

/**
 * The media type of a track's format, as the server sends it, or undefined for a format nobody can
 * vouch for. `Track` has no codec, so ALAC passes for AAC here and fails later, at decode.
 */
export function mediaTypeOf(format: string): string | undefined {
	return mediaTypes.get(format);
}

/** How long a play may wait for the context to come up before it counts as stopped. */
const resumeBoundMs = 1000;

/** Where an element's sound goes, once there's a context to send it through. */
interface Channel {
	element: HTMLAudioElement;
	url: string;
	gain: GainNode | null;
}

export interface HtmlOutputParts {
	audio(): HTMLAudioElement;
	context(): AudioContext;
}

const browserParts: HtmlOutputParts = {
	audio: () => new Audio(),
	context: () => new AudioContext({ latencyHint: 'playback' })
};

/**
 * Two elements taking turns, each through its own gain. They are never in the DOM, so a picture of
 * the page can't clone one. The nodes are built on the first play and kept for the page's lifetime:
 * an element only ever accepts one source node, whatever the context.
 */
export class HtmlOutput implements AudioOutput {
	readonly #parts: HtmlOutputParts;
	#active: Channel;
	#spare: Channel;
	#context: AudioContext | null = null;
	#events: OutputEvents | null = null;
	#gain = 1;
	/** Whether the context has been up, so that it going down means something. */
	#wasRunning = false;

	constructor(parts: HtmlOutputParts = browserParts) {
		this.#parts = parts;
		this.#active = this.#channel();
		this.#spare = this.#channel();
	}

	#channel(): Channel {
		const element = this.#parts.audio();
		element.preload = 'auto';
		const channel: Channel = { element, url: '', gain: null };
		const whenActive = (speak: () => void) => () => {
			if (channel === this.#active && channel.url !== '') {
				speak();
			}
		};
		element.addEventListener(
			'timeupdate',
			whenActive(() => this.#events?.time(element.currentTime))
		);
		element.addEventListener(
			'playing',
			whenActive(() => this.#events?.running())
		);
		// Finishing pauses too. So does a pause undone before this got here.
		element.addEventListener(
			'pause',
			whenActive(() => {
				if (element.paused && !element.ended) {
					this.#events?.stopped();
				}
			})
		);
		element.addEventListener(
			'ended',
			whenActive(() => this.#events?.ended())
		);
		element.addEventListener(
			'error',
			whenActive(() => this.#events?.failed())
		);
		return channel;
	}

	listen(events: OutputEvents): void {
		this.#events = events;
	}

	canPlay(mediaType: string): boolean {
		return this.#active.element.canPlayType(mediaType) !== '';
	}

	load({ url }: Playable): void {
		if (this.#spare.url === url && this.#spare.element.error === null) {
			const finished = this.#active;
			this.#active = this.#spare;
			this.#spare = finished;
			clear(finished);
		} else {
			aim(this.#active, url);
		}
		this.#applyGain();
	}

	preload({ url }: Playable): void {
		if (this.#spare.url !== url) {
			aim(this.#spare, url);
		}
	}

	async play(): Promise<void> {
		const context = this.#wired();
		// Never waited for first: outside a gesture this stays pending, and the element must go now.
		const resumed = context.resume().catch(() => undefined);
		const { element } = this.#active;
		await element.play();
		await Promise.race([resumed, new Promise((done) => setTimeout(done, resumeBoundMs))]);
		// A stopped context raises no error: the element claims to play, and the clock stands still.
		if (context.state !== 'running' && element === this.#active.element) {
			this.#halt();
		}
	}

	pause(): void {
		this.#active.element.pause();
	}

	seek(seconds: number): void {
		this.#active.element.currentTime = seconds;
	}

	setGain(decibels: number): void {
		this.#gain = linear(decibels);
		this.#applyGain();
	}

	stop(): void {
		clear(this.#active);
		clear(this.#spare);
	}

	#applyGain(): void {
		if (this.#active.gain !== null) {
			this.#active.gain.gain.value = this.#gain;
		}
	}

	#wired(): AudioContext {
		if (this.#context !== null) {
			return this.#context;
		}
		const context = this.#parts.context();
		for (const channel of [this.#active, this.#spare]) {
			channel.gain = context.createGain();
			context
				.createMediaElementSource(channel.element)
				.connect(channel.gain)
				.connect(context.destination);
		}
		context.addEventListener('statechange', () => {
			this.#heard(context.state);
		});
		this.#context = context;
		this.#wasRunning = context.state === 'running';
		this.#applyGain();
		return context;
	}

	/** Sleep and wake land here. Whoever comes back finds it paused, not pretending to play. */
	#heard(state: AudioContextState): void {
		const running = state === 'running';
		if (this.#wasRunning && !running) {
			this.#halt();
		}
		this.#wasRunning = running;
	}

	#halt(): void {
		const { element } = this.#active;
		if (!element.paused) {
			element.pause();
			this.#events?.stopped();
		}
	}
}

function aim(channel: Channel, url: string): void {
	channel.url = url;
	channel.element.src = url;
}

/** Lets a track go: the only way to stop an element holding on to what it fetched. */
function clear(channel: Channel): void {
	channel.url = '';
	channel.element.pause();
	channel.element.removeAttribute('src');
	channel.element.load();
}

export interface SilentOutputOptions {
	/** Runs the clock on a timer. Tests leave it off and call {@link SilentOutput.elapse}. */
	ticking?: boolean;
}

const tickMs = 250;

/** A simulated clock: the fixtures and the tests get everything but the sound. */
export class SilentOutput implements AudioOutput {
	readonly #ticking: boolean;
	#events: OutputEvents | null = null;
	#timer: ReturnType<typeof setInterval> | undefined;
	#lastTick = 0;
	#seconds = 0;
	/** The URL in play. */
	url = '';
	/** The URL fetched on the side. */
	preloaded = '';
	position = 0;
	gain = 0;
	playing = false;

	constructor({ ticking = true }: SilentOutputOptions = {}) {
		this.#ticking = ticking;
	}

	protected get events(): OutputEvents | null {
		return this.#events;
	}

	listen(events: OutputEvents): void {
		this.#events = events;
	}

	canPlay(): boolean {
		return true;
	}

	load({ url, seconds }: Playable): void {
		this.#halt();
		this.url = url;
		this.#seconds = seconds;
		this.position = 0;
		if (this.preloaded === url) {
			this.preloaded = '';
		}
	}

	preload({ url }: Playable): void {
		this.preloaded = url;
	}

	play(): Promise<void> {
		this.playing = true;
		if (this.#ticking && this.#timer === undefined) {
			this.#lastTick = performance.now();
			this.#timer = setInterval(() => {
				const now = performance.now();
				this.elapse((now - this.#lastTick) / 1000);
				this.#lastTick = now;
			}, tickMs);
		}
		this.#events?.running();
		return Promise.resolve();
	}

	pause(): void {
		this.#halt();
	}

	seek(seconds: number): void {
		this.position = seconds;
		this.#events?.time(seconds);
	}

	setGain(decibels: number): void {
		this.gain = decibels;
	}

	stop(): void {
		this.#halt();
		this.url = '';
		this.preloaded = '';
		this.position = 0;
	}

	/** Moves the clock on, if anything is playing. */
	elapse(seconds: number): void {
		if (!this.playing) {
			return;
		}
		this.position = Math.min(this.position + seconds, this.#seconds);
		this.#events?.time(this.position);
		if (this.position >= this.#seconds) {
			this.#halt();
			this.#events?.ended();
		}
	}

	#halt(): void {
		this.playing = false;
		clearInterval(this.#timer);
		this.#timer = undefined;
	}
}
