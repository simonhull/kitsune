import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FakeStream } from './fakeStream';
import { Live, type EventHandlers } from './live';

describe('the live connection', () => {
	let streams: FakeStream[];
	const open = (): FakeStream => {
		const stream = new FakeStream();
		streams.push(stream);
		return stream;
	};

	beforeEach(() => {
		streams = [];
		vi.useFakeTimers();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	function started(handlers: EventHandlers): Live {
		const live = new Live(open, handlers);
		live.start();
		return live;
	}

	it('hands each event to its handler, and opens one stream however often it starts', () => {
		const marks = vi.fn();
		const live = started({ marks });
		live.start();
		expect(streams).toHaveLength(1);
		streams[0]?.open();
		streams[0]?.send('ready', {});
		streams[0]?.send('marks', { kind: 'tracks', id: 't1' });
		expect(marks).toHaveBeenCalledWith({ kind: 'tracks', id: 't1' });
		expect(live.connected).toBe(true);
	});

	it('leaves a dropped stream to the browser, which says ready or resync itself', () => {
		const resync = vi.fn();
		const live = started({ resync });
		streams[0]?.send('ready', {});
		streams[0]?.fail(false);
		expect(live.connected).toBe(false);
		streams[0]?.send('ready', {});
		expect(resync).not.toHaveBeenCalled();
		streams[0]?.send('resync', {});
		expect(resync).toHaveBeenCalledTimes(1);
		expect(streams).toHaveLength(1);
	});

	it('opens a stream the browser gave up on again, later each time, and resyncs on it', () => {
		const resync = vi.fn();
		const broken = vi.fn();
		started({ resync, broken });
		streams[0]?.fail(true);
		expect(streams[0]?.closed).toBe(true);
		expect(broken).toHaveBeenCalledTimes(1);
		vi.advanceTimersByTime(999);
		expect(streams).toHaveLength(1);
		vi.advanceTimersByTime(1);
		expect(streams).toHaveLength(2);
		streams[1]?.fail(true);
		vi.advanceTimersByTime(1999);
		expect(streams).toHaveLength(2);
		vi.advanceTimersByTime(1);
		expect(streams).toHaveLength(3);
		streams[2]?.open();
		streams[2]?.send('ready', {});
		expect(resync).toHaveBeenCalledTimes(1);
		streams[2]?.send('ready', {});
		expect(resync).toHaveBeenCalledTimes(1);
	});

	it('stops for good, and hears nothing from a stream it closed', () => {
		const library = vi.fn();
		const live = started({ library });
		const first = streams[0];
		first?.fail(true);
		live.stop();
		vi.advanceTimersByTime(60_000);
		expect(streams).toHaveLength(1);
		first?.send('library', { albums: [], all: false });
		expect(library).not.toHaveBeenCalled();
		live.start();
		expect(streams).toHaveLength(2);
	});

	it('ignores data that is not JSON', () => {
		const library = vi.fn();
		started({ library });
		streams[0]?.sendRaw('library', 'not json');
		expect(library).not.toHaveBeenCalled();
		streams[0]?.send('library', { albums: ['a'], all: false });
		expect(library).toHaveBeenCalledTimes(1);
	});
});
