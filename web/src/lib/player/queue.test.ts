import { describe, expect, it } from 'vitest';
import type { Album, Track } from '#lib/library/source.ts';
import { currentOf, inOrder, next, previous, retagged, rewound, shuffled, upcoming } from './queue';

function album(count: number): Album {
	const tracks: Track[] = Array.from({ length: count }, (_, index) => ({
		id: `t${String(index)}`,
		title: `T${String(index)}`,
		artists: [],
		durationMs: 1000,
		format: 'FLAC',
		genres: [],
		etag: ''
	}));
	return {
		id: 'a',
		title: 'A',
		artist: 'B',
		artists: [],
		trackCount: count,
		compilation: false,
		tracks,
		durationMs: 1000,
		formats: ['FLAC'],
		addedAt: '2026-10-12T18:00:00Z',
		genres: [],
		etag: ''
	};
}

function ids(tracks: readonly Track[]): string[] {
	return tracks.map(({ id }) => id);
}

describe('a queue in order', () => {
	it('starts where it was asked to', () => {
		const queue = inOrder(album(4), 2);
		expect(currentOf(queue)?.id).toBe('t2');
		expect(queue.ordered).toBe(true);
	});

	it('steps forward to the last track and no further', () => {
		const last = next(inOrder(album(2)));
		expect(last === null ? null : currentOf(last)?.id).toBe('t1');
		expect(last === null ? null : upcoming(last)).toBeUndefined();
		expect(last === null ? 'none' : next(last)).toBeNull();
	});

	it('steps back to the first track and no further', () => {
		const first = previous(inOrder(album(2), 1));
		expect(first?.index).toBe(0);
		expect(first === null ? 'none' : previous(first)).toBeNull();
	});

	it('rewinds without losing its order', () => {
		const queue = shuffled(album(6), 3);
		const end = { ...queue, index: 5 };
		expect(rewound(end)).toEqual(queue);
	});
});

describe('a shuffled queue', () => {
	it('holds every track once', () => {
		const queue = shuffled(album(50), 7);
		expect(ids(queue.tracks).toSorted()).toEqual(ids(album(50).tracks).toSorted());
		expect(queue.ordered).toBe(false);
	});

	it('puts the chosen track first', () => {
		for (const chosen of [0, 3, 9]) {
			expect(shuffled(album(10), 11, chosen).tracks.at(0)?.id).toBe(`t${String(chosen)}`);
		}
	});

	it('repeats for a seed and differs across seeds', () => {
		expect(ids(shuffled(album(30), 5).tracks)).toEqual(ids(shuffled(album(30), 5).tracks));
		expect(ids(shuffled(album(30), 5).tracks)).not.toEqual(ids(shuffled(album(30), 6).tracks));
	});

	it('leaves the album alone', () => {
		const source = album(8);
		shuffled(source, 2);
		expect(ids(source.tracks)).toEqual(ids(album(8).tracks));
	});

	it('gives every track a fair go at every place', () => {
		const seconds = new Map<string, number>();
		for (let seed = 0; seed < 4000; seed += 1) {
			const id = shuffled(album(4), seed, 0).tracks.at(1)?.id ?? '';
			seconds.set(id, (seconds.get(id) ?? 0) + 1);
		}
		expect(seconds.size).toBe(3);
		for (const count of seconds.values()) {
			expect(count).toBeGreaterThan(1200);
			expect(count).toBeLessThan(1467);
		}
	});

	it('copes with an album of one', () => {
		expect(ids(shuffled(album(1), 1).tracks)).toEqual(['t0']);
	});
});

describe('retagged', () => {
	it('takes the new names and keeps the order and the place', () => {
		const queue = { ...shuffled(album(4), 7), index: 2 };
		const edited = {
			...album(4),
			tracks: album(4).tracks.map((track) => ({ ...track, title: `New ${track.id}` }))
		};
		const after = retagged(queue, edited);
		expect(after.album).toBe(edited);
		expect(after.index).toBe(2);
		expect(after.ordered).toBe(false);
		expect(after.tracks.map(({ id }) => id)).toEqual(queue.tracks.map(({ id }) => id));
		expect(after.tracks.every(({ title, id }) => title === `New ${id}`)).toBe(true);
	});

	it('keeps a track the album no longer holds', () => {
		const queue = inOrder(album(2));
		const after = retagged(queue, { ...album(2), tracks: [] });
		expect(after.tracks).toEqual(queue.tracks);
	});
});
