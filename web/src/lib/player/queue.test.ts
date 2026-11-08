import { describe, expect, it } from 'vitest';
import type { Album, Track } from '#lib/library/source.ts';
import {
	albumOf,
	appended,
	currentOf,
	holds,
	inOrder,
	next,
	onDisc,
	picked,
	playingNext,
	previous,
	retagged,
	rewound,
	shuffled,
	upcoming
} from './queue';

function album(count: number): Album {
	const tracks: Track[] = Array.from({ length: count }, (_, index) => ({
		id: `t${String(index)}`,
		title: `T${String(index)}`,
		artists: [],
		durationMs: 1000,
		format: 'FLAC',
		genres: [],
		etag: '',
		hearted: false
	}));
	return {
		id: 'a',
		title: 'A',
		artist: 'B',
		artists: [],
		trackCount: count,
		compilation: false,
		tracks,
		discs: [],
		durationMs: 1000,
		formats: ['FLAC'],
		addedAt: '2026-10-12T18:00:00Z',
		genres: [],
		etag: '',
		hearted: false
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

	it('queues one disc in order, an unnumbered song on disc 1', () => {
		const two = album(5);
		const tracks = two.tracks.map((track, index) =>
			index === 0 ? track : { ...track, discNo: index < 3 ? 1 : 2 }
		);
		const discs = { ...two, tracks };
		expect(ids(onDisc(discs, 1).tracks)).toEqual(['t0', 't1', 't2']);
		const second = onDisc(discs, 2);
		expect(ids(second.tracks)).toEqual(['t3', 't4']);
		expect(second.index).toBe(0);
		expect(second.ordered).toBe(true);
		expect(onDisc(discs, 3).tracks).toEqual([]);
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

/** Another album, with tracks of its own. */
function other(id: string, count: number): Album {
	const base = album(count);
	return {
		...base,
		id,
		tracks: base.tracks.map((track) => ({ ...track, id: `${id}${track.id}` }))
	};
}

describe('a queue of picked songs', () => {
	const a = album(3);
	const b = other('b', 3);
	const songs = (from: Album, ...at: number[]) =>
		at.flatMap((index) => {
			const track = from.tracks.at(index);
			return track === undefined ? [] : [{ album: from, track }];
		});

	it('plays them in the order given, each with its album, at track gain', () => {
		const queue = picked([...songs(b, 2), ...songs(a, 0, 1)]);
		expect(queue === null ? [] : ids(queue.tracks)).toEqual(['bt2', 't0', 't1']);
		expect(queue?.ordered).toBe(false);
		const second = queue?.tracks.at(1);
		expect(queue && second && albumOf(queue, second).id).toBe('a');
		expect(picked([])).toBeNull();
	});

	it('puts songs next, after the one playing', () => {
		const queue = playingNext({ ...inOrder(a), index: 1 }, songs(b, 0, 1));
		expect(ids(queue.tracks)).toEqual(['t0', 't1', 'bt0', 'bt1', 't2']);
		expect(queue.ordered).toBe(false);
		const added = queue.tracks.at(2);
		expect(added && albumOf(queue, added).id).toBe('b');
		const own = queue.tracks.at(4);
		expect(own && albumOf(queue, own).id).toBe('a');
	});

	it('adds songs at the end', () => {
		const queue = appended(inOrder(a), songs(b, 2));
		expect(ids(queue.tracks)).toEqual(['t0', 't1', 't2', 'bt2']);
		expect(holds(queue, 'b')).toBe(true);
		expect(holds(inOrder(a), 'b')).toBe(false);
	});

	it('changes nothing with nothing to add', () => {
		const queue = inOrder(a);
		expect(appended(queue, [])).toBe(queue);
	});

	it('takes in an edit of any album it holds', () => {
		const queue = appended(inOrder(a), songs(b, 0));
		const edited = { ...b, tracks: b.tracks.map((track) => ({ ...track, title: 'New' })) };
		const after = retagged(queue, edited);
		const last = after.tracks.at(-1);
		expect(last?.title).toBe('New');
		expect(last && albumOf(after, last)).toBe(edited);
		expect(after.album).toBe(a);
	});
});
