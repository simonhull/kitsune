import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { HtmlOutput, mediaTypeOf, type HtmlOutputParts, type OutputEvents } from './output';

class FakeAudio extends EventTarget {
	preload = '';
	src = '';
	currentTime = 0;
	paused = true;
	ended = false;
	error: { code: number } | null = null;
	played = 0;
	rejection: Error | null = null;

	canPlayType(type: string): string {
		return type === 'audio/aiff' ? '' : 'maybe';
	}

	play(): Promise<void> {
		this.played += 1;
		if (this.rejection !== null) {
			return Promise.reject(this.rejection);
		}
		this.paused = false;
		return Promise.resolve();
	}

	pause(): void {
		this.paused = true;
	}

	removeAttribute(): void {
		this.src = '';
	}

	load(): void {
		this.currentTime = 0;
	}

	fire(name: string): void {
		this.dispatchEvent(new Event(name));
	}
}

class FakeContext extends EventTarget {
	state: AudioContextState = 'running';
	destination = {};
	sources: unknown[] = [];
	gains: { gain: { value: number } }[] = [];
	resumes = 0;
	resumed: Promise<void> = Promise.resolve();

	resume(): Promise<void> {
		this.resumes += 1;
		return this.resumed;
	}

	createGain() {
		const node = { gain: { value: 1 }, connect: (next: unknown) => next };
		this.gains.push(node);
		return node;
	}

	createMediaElementSource(element: unknown) {
		this.sources.push(element);
		return { connect: (next: unknown) => next };
	}

	become(state: AudioContextState): void {
		this.state = state;
		this.dispatchEvent(new Event('statechange'));
	}
}

function setup() {
	const elements: FakeAudio[] = [];
	const contexts: FakeContext[] = [];
	const context = new FakeContext();
	const parts = {
		audio: () => {
			const element = new FakeAudio();
			elements.push(element);
			return element;
		},
		context: () => {
			contexts.push(context);
			return context;
		}
	} as unknown as HtmlOutputParts;
	const output = new HtmlOutput(parts);
	const events: OutputEvents = {
		time: vi.fn(),
		running: vi.fn(),
		stopped: vi.fn(),
		ended: vi.fn(),
		failed: vi.fn()
	};
	output.listen(events);
	const [first, second] = elements as [FakeAudio, FakeAudio];
	return { output, events, first, second, context, contexts };
}

const one = { url: '/one', seconds: 100 };
const two = { url: '/two', seconds: 100 };

describe('HtmlOutput', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('builds no context before the first play, and one after any number', async () => {
		const { output, contexts, context } = setup();
		expect(contexts).toHaveLength(0);
		output.load(one);
		await output.play();
		await output.play();
		output.stop();
		output.load(two);
		await output.play();
		expect(contexts).toHaveLength(1);
		expect(context.sources).toHaveLength(2);
	});

	it('asks the context to resume on every play, and never waits for that first', async () => {
		const { output, first, context } = setup();
		context.resumed = new Promise(() => undefined);
		output.load(one);
		const playing = output.play();
		expect(first.played).toBe(1);
		expect(context.resumes).toBe(1);
		await vi.runAllTimersAsync();
		await playing;
	});

	it('reads a suspended context as stopped, not playing', async () => {
		const { output, events, first, context } = setup();
		context.state = 'suspended';
		context.resumed = new Promise(() => undefined);
		output.load(one);
		const playing = output.play();
		await vi.runAllTimersAsync();
		await playing;
		expect(events.stopped).toHaveBeenCalledOnce();
		expect(first.paused).toBe(true);
	});

	it('lets a context that comes up in time carry on', async () => {
		const { output, events, first, context } = setup();
		context.state = 'suspended';
		context.resume = () => {
			context.state = 'running';
			return Promise.resolve();
		};
		output.load(one);
		await output.play();
		expect(events.stopped).not.toHaveBeenCalled();
		expect(first.paused).toBe(false);
	});

	it('stops when the context goes down under it, and stays down when it comes back', async () => {
		const { output, events, first, context } = setup();
		output.load(one);
		await output.play();
		context.become('suspended');
		expect(events.stopped).toHaveBeenCalledOnce();
		expect(first.paused).toBe(true);
		context.become('running');
		expect(first.paused).toBe(true);
		expect(first.played).toBe(1);
	});

	it('hands over to what was preloaded', async () => {
		const { output, first, second } = setup();
		output.load(one);
		await output.play();
		output.preload(two);
		expect(second.src).toBe('/two');
		output.load(two);
		await output.play();
		expect(second.played).toBe(1);
		expect(first.src).toBe('');
		expect(first.paused).toBe(true);
	});

	it('fetches afresh rather than hand over to a preload that failed', async () => {
		const { output, first, second } = setup();
		output.load(one);
		output.preload(two);
		second.error = { code: 4 };
		output.load(two);
		expect(first.src).toBe('/two');
	});

	it('hears only the track in play', async () => {
		const { output, events, first, second } = setup();
		output.load(one);
		await output.play();
		output.preload(two);
		second.fire('error');
		second.fire('ended');
		expect(events.failed).not.toHaveBeenCalled();
		expect(events.ended).not.toHaveBeenCalled();
		first.fire('ended');
		expect(events.ended).toHaveBeenCalledOnce();
	});

	it('says nothing once it has let go', async () => {
		const { output, events, first } = setup();
		output.load(one);
		await output.play();
		output.stop();
		first.fire('error');
		expect(events.failed).not.toHaveBeenCalled();
	});

	it('reports a pause it did not ask for, but not the one that comes with finishing', async () => {
		const { output, events, first } = setup();
		output.load(one);
		await output.play();
		first.paused = true;
		first.ended = true;
		first.fire('pause');
		expect(events.stopped).not.toHaveBeenCalled();
		first.ended = false;
		first.fire('pause');
		expect(events.stopped).toHaveBeenCalledOnce();
	});

	it('ignores a pause already undone', async () => {
		const { output, events, first } = setup();
		output.load(one);
		await output.play();
		first.fire('pause');
		expect(events.stopped).not.toHaveBeenCalled();
	});

	it('applies gain to the channel in play, including one set before anything is built', async () => {
		const { output, context } = setup();
		output.setGain(-20);
		output.load(one);
		await output.play();
		expect(context.gains.at(0)?.gain.value).toBeCloseTo(0.1, 10);
		output.preload(two);
		output.load(two);
		expect(context.gains.at(1)?.gain.value).toBeCloseTo(0.1, 10);
		output.setGain(0);
		expect(context.gains.at(1)?.gain.value).toBe(1);
	});

	it('passes a refused play on', async () => {
		const { output, first } = setup();
		first.rejection = new DOMException('refused', 'NotAllowedError');
		output.load(one);
		await expect(output.play()).rejects.toThrow('refused');
	});

	it('asks the element what it can play', () => {
		const { output } = setup();
		expect(output.canPlay('audio/flac')).toBe(true);
		expect(output.canPlay('audio/aiff')).toBe(false);
	});
});

describe('mediaTypeOf', () => {
	it.each([
		['FLAC', 'audio/flac'],
		['MP3', 'audio/mpeg'],
		['M4A', 'audio/mp4'],
		['M4B', 'audio/mp4'],
		['Ogg Vorbis', 'audio/ogg'],
		['Opus', 'audio/ogg'],
		['WAV', 'audio/wav'],
		['AIFF', 'audio/aiff']
	])('maps %s', (format, type) => {
		expect(mediaTypeOf(format)).toBe(type);
	});

	it('vouches for nothing it does not know', () => {
		expect(mediaTypeOf('Unknown')).toBeUndefined();
	});
});
