import type { ServerEventKind, ServerEvents } from './source.ts';

/** Where the stream is. */
export const eventsUrl = '/api/v1/events';

/** The first wait before opening the stream again, doubling each time to the longest. */
const firstWaitMs = 1000;
const longestWaitMs = 30_000;

/** What the live connection needs of an `EventSource`, so a test can stand in for one. */
export interface EventStream {
	readonly readyState: number;
	onopen: ((event: Event) => void) | null;
	onerror: ((event: Event) => void) | null;
	addEventListener(type: string, listener: (event: MessageEvent<string>) => void): void;
	close(): void;
}

/** `EventSource.CLOSED`: the browser has given up, and won't open it again by itself. */
const closed = 2;

/** What to do with each kind of event. `resync` is also called after a gap nothing can fill. */
export type EventHandlers = { [K in ServerEventKind]?: (data: ServerEvents[K]) => void } & {
	/** The stream broke for good: something may want to ask whether the session still holds. */
	broken?: () => void;
};

const kinds: readonly ServerEventKind[] = [
	'ready',
	'resync',
	'library',
	'scan',
	'fingerprints',
	'marks',
	'prefs',
	'shuffle-rules',
	'shuffle',
	'listening',
	'lastfm',
	'settings'
];

/**
 * One stream of the server's events, for as long as someone is signed in. The browser opens it
 * again by itself after a dropped connection, sending the last id, and the server says `ready` when
 * it filled the gap or `resync` when it couldn't. When the browser gives up, after a refusal or a
 * server that's away, it's opened again here, waiting longer each time; that new stream has no last
 * id to send, so its `ready` counts as a `resync`.
 */
export class Live {
	/** Whether events are arriving: while they are, nothing need ask the server what changed. */
	connected = false;

	readonly #open: () => EventStream;
	readonly #handlers: EventHandlers;
	#stream: EventStream | null = null;
	#retry: ReturnType<typeof setTimeout> | undefined;
	#attempts = 0;
	/** Set when the stream that's opening follows one that gave up: what was missed is lost. */
	#gap = false;

	constructor(open: () => EventStream, handlers: EventHandlers) {
		this.#open = open;
		this.#handlers = handlers;
	}

	/** Opens the stream, unless it's open. */
	start(): void {
		if (this.#stream === null && this.#retry === undefined) {
			this.#connect();
		}
	}

	/** Closes the stream, and stops opening it again. */
	stop(): void {
		clearTimeout(this.#retry);
		this.#retry = undefined;
		this.#stream?.close();
		this.#stream = null;
		this.connected = false;
		this.#attempts = 0;
		this.#gap = false;
	}

	#connect(): void {
		this.#retry = undefined;
		const stream = this.#open();
		this.#stream = stream;
		stream.onopen = () => {
			this.#attempts = 0;
		};
		stream.onerror = () => {
			this.connected = false;
			if (stream.readyState === closed && this.#stream === stream) {
				this.#giveUp(stream);
			}
		};
		for (const kind of kinds) {
			stream.addEventListener(kind, (event) => {
				if (this.#stream === stream) {
					this.#take(kind, event.data);
				}
			});
		}
	}

	/** Opens a stream again after the browser gave up on one, waiting longer each time. */
	#giveUp(stream: EventStream): void {
		stream.close();
		this.#stream = null;
		this.#gap = true;
		const wait = Math.min(firstWaitMs * 2 ** this.#attempts, longestWaitMs);
		this.#attempts += 1;
		this.#retry = setTimeout(() => {
			this.#connect();
		}, wait);
		this.#handlers.broken?.();
	}

	#take(kind: ServerEventKind, raw: string): void {
		let data: unknown;
		try {
			data = JSON.parse(raw);
		} catch {
			return; // Not the server's: the contract says one line of JSON.
		}
		this.connected = true;
		if (kind === 'ready' && this.#gap) {
			this.#gap = false;
			this.#handlers.resync?.({});
			return;
		}
		if (kind === 'resync') {
			this.#gap = false;
		}
		const handle = this.#handlers[kind] as ((value: unknown) => void) | undefined;
		handle?.(data);
	}
}
