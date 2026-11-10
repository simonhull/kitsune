import { describe, expect, it } from 'vitest';
import { noDisc } from '#lib/library/format.ts';
import type { Album, Track } from '#lib/library/source.ts';
import {
	albumOf,
	albumRun,
	appended,
	extended,
	continued,
	currentOf,
	holds,
	inOrder,
	next,
	onDisc,
	onwardOf,
	pendingAlbum,
	picked,
	playingNext,
	playsAlbum,
	previous,
	retagged,
	rewound,
	shuffled,
	throughList,
	reordered,
	upcoming,
	withdrawn,
	without
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

	it('queues one disc in order, and the songs with no disc number as one more', () => {
		const two = album(6);
		const tracks = two.tracks.map((track, index) =>
			index >= 4 ? track : { ...track, discNo: index < 2 ? 1 : 2 }
		);
		const discs = { ...two, tracks };
		expect(ids(onDisc(discs, 1).tracks)).toEqual(['t0', 't1']);
		const second = onDisc(discs, 2);
		expect(ids(second.tracks)).toEqual(['t2', 't3']);
		expect(second.index).toBe(0);
		expect(second.ordered).toBe(true);
		const none = onDisc(discs, noDisc);
		expect(ids(none.tracks)).toEqual(['t4', 't5']);
		expect(none.ordered).toBe(true);
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

	it('puts songs next while shuffling, ahead of its picks, and the shuffle carries on after', () => {
		const shuffle = { id: 'session' };
		// Playing t0; the shuffle has queued t1 and t2 ahead of it.
		const shuffling = { ...inOrder(a), index: 0, ordered: false, shuffle };
		const queue = playingNext(shuffling, songs(b, 0));
		expect(ids(queue.tracks)).toEqual(['t0', 'bt0', 't1', 't2']);
		expect(queue.shuffle).toBe(shuffle);
		// The shuffle's next picks still go on the end, after what was played next.
		const carried = extended(queue, songs(b, 2));
		expect(ids(carried.tracks)).toEqual(['t0', 'bt0', 't1', 't2', 'bt2']);
		expect(carried.shuffle).toBe(shuffle);
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

describe('a queue going on through a list', () => {
	const other = {
		...album(2),
		id: 'b',
		tracks: album(2).tracks.map((t) => ({ ...t, id: `b${t.id}` }))
	};
	/** A song from elsewhere, to put in the queue. */
	const stray = (id: string) => ({ album: album(1), track: { ...other.tracks[0], id } as Track });

	it('decides what comes after it in one place: a shuffle first, then a list, else rest', () => {
		const queue = inOrder(album(2));
		expect(onwardOf(queue)).toBe('rest');
		expect(onwardOf(throughList(queue))).toBe('list');
		expect(onwardOf({ ...throughList(queue), shuffle: { id: 's' } })).toBe('shuffle');
	});

	it('takes the next album in whole, in order, keeping album gain', () => {
		const queue = continued(throughList(inOrder(album(2), 1)), other);
		expect(ids(queue.tracks)).toEqual(['t0', 't1', 'bt0', 'bt1']);
		expect(queue.ordered).toBe(true);
		expect(queue.tracks.slice(2).map((track) => albumOf(queue, track).id)).toEqual(['b', 'b']);
		expect(queue.continues).toEqual({ album: 'b', ahead: { at: 2, count: 2, after: 'a' } });
		expect(pendingAlbum(queue)).toEqual({ at: 2, count: 2, after: 'a' });
		expect(pendingAlbum({ ...queue, index: 2 })).toBeUndefined();
	});

	it('gives back an album that hasn’t begun, and keeps one that has', () => {
		const queue = continued(throughList(inOrder(album(2), 1)), other);
		const back = withdrawn(queue);
		expect(ids(back.tracks)).toEqual(['t0', 't1']);
		expect(back.continues).toEqual({ album: 'a' });
		expect(withdrawn({ ...queue, index: 2 }).tracks).toHaveLength(4);
		expect(ids(withdrawn(appended(queue, [stray('y')])).tracks)).toEqual(['t0', 't1', 'y']);
	});

	it('puts songs someone queues before the album ahead, and keeps it where its songs are', () => {
		const queue = continued(throughList(inOrder(album(2), 1)), other);
		expect(playingNext(queue, [stray('x')]).continues?.ahead?.at).toBe(3);
		const added = appended(queue, [stray('y')]);
		expect(ids(added.tracks)).toEqual(['t0', 't1', 'y', 'bt0', 'bt1']);
		expect(added.continues?.ahead?.at).toBe(3);
		expect(ids(appended({ ...added, index: 3 }, [stray('z')]).tracks).at(-1)).toBe('z');
	});

	it('says where a song is in its album, among the albums around it', () => {
		const queue = continued(throughList(inOrder(album(2), 1)), other);
		expect(albumRun(queue)).toEqual({ index: 1, trackCount: 2 });
		expect(albumRun({ ...queue, index: 2 })).toEqual({ index: 0, trackCount: 2 });
	});

	it('says which album a cover’s play pauses: the queue’s own, and from the list the one sounding', () => {
		const queue = continued(throughList(inOrder(album(2), 1)), other);
		expect(playsAlbum(null, 'a', 'list')).toBe(false);
		expect(playsAlbum(throughList(inOrder(album(2))), 'a', 'elsewhere')).toBe(true);
		// Once the next album is in, the queue is more than one album: only the list knows it.
		expect(playsAlbum(queue, 'a', 'list')).toBe(true);
		expect(playsAlbum(queue, 'a', 'elsewhere')).toBe(false);
		expect(playsAlbum(queue, 'b', 'list')).toBe(false);
		expect(playsAlbum({ ...queue, index: 2 }, 'b', 'list')).toBe(true);
		expect(playsAlbum({ ...queue, index: 2 }, 'a', 'list')).toBe(false);
		expect(playsAlbum({ ...queue, index: 2 }, 'b', 'elsewhere')).toBe(false);
		// Songs picked from that album, not going through a list, aren't the album playing.
		const songs = picked([{ album: other, track: other.tracks[0] as Track }]);
		expect(playsAlbum(songs, 'b', 'list')).toBe(false);
	});

	it('comes to rest at the end, going no further', () => {
		expect(rewound(continued(throughList(inOrder(album(2), 1)), other)).continues).toBeUndefined();
	});

	it('keeps the album ahead where its songs are when songs are taken out of Up next', () => {
		const queue = appended(continued(throughList(inOrder(album(2), 0)), other), [stray('y')]);
		expect(ids(queue.tracks)).toEqual(['t0', 't1', 'y', 'bt0', 'bt1']);
		const fewer = without(queue, ['t1', 'bt0']);
		expect(ids(fewer.tracks)).toEqual(['t0', 'y', 'bt1']);
		expect(fewer.continues).toEqual({ album: 'b', ahead: { at: 2, count: 1, after: 'a' } });
		expect(ids(withdrawn(fewer).tracks)).toEqual(['t0', 'y']);
		const gone = without(queue, ['bt0', 'bt1']);
		expect(ids(gone.tracks)).toEqual(['t0', 't1', 'y']);
		expect(gone.continues).toEqual({ album: 'b' });
	});

	it('keeps the album ahead while a reorder leaves it whole, and lets it go once it doesn’t', () => {
		const queue = appended(continued(throughList(inOrder(album(2), 0)), other), [stray('y')]);
		const moved = reordered(queue, ['bt0', 'bt1', 't1', 'y']);
		expect(ids(moved.tracks)).toEqual(['t0', 'bt0', 'bt1', 't1', 'y']);
		expect(moved.continues).toEqual({ album: 'b', ahead: { at: 1, count: 2, after: 'a' } });
		expect(ids(withdrawn(moved).tracks)).toEqual(['t0', 't1', 'y']);
		const split = reordered(queue, ['bt0', 't1', 'y', 'bt1']);
		expect(split.continues).toEqual({ album: 'b' });
		expect(withdrawn(split)).toBe(split);
		const turned = reordered(queue, ['t1', 'y', 'bt1', 'bt0']);
		expect(turned.continues).toEqual({ album: 'b' });
	});
});
