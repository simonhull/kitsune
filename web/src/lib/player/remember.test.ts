import { describe, expect, it } from 'vitest';
import type { Album, PlaybackRecord, Track } from '#lib/library/source.ts';
import {
	inOrder,
	listed,
	picked,
	shuffled,
	shuffledOff,
	throughList,
	type AlbumLookup
} from './queue.ts';
import {
	ageWords,
	kindOf,
	leftNote,
	leftOf,
	leftSpoken,
	mostRemembered,
	recordOf,
	restored,
	tagWords,
	type Client
} from './remember.ts';

function track(album: string, index: number): Track {
	return {
		id: `${album}${String(index)}`,
		title: `T${String(index)}`,
		artists: [],
		durationMs: 200_000,
		format: 'FLAC',
		genres: [],
		etag: ''
	};
}

function album(id: string, count: number): Album {
	return {
		id,
		title: id,
		artist: 'B',
		artists: [],
		trackCount: count,
		compilation: false,
		tracks: Array.from({ length: count }, (_, index) => track(id, index)),
		discs: [],
		durationMs: 1000,
		formats: ['FLAC'],
		addedAt: '2026-10-12T18:00:00Z',
		genres: [],
		etag: ''
	};
}

const here: Client = { id: '0123456789abcdef', device: 'computer' };
const a = album('a', 4);
const b = album('b', 3);
const songsOf = (from: Album): { album: Album; track: Track }[] =>
	from.tracks.map((each) => ({ album: from, track: each }));
/** Song `at` of album a, to queue. */
const songOfA = (at: number): { album: Album; track: Track } => ({
	album: a,
	track: track('a', at)
});

describe('kindOf', () => {
	it('names each queue', () => {
		expect(kindOf(inOrder(a))).toBe('album');
		expect(kindOf(shuffled(a, 7))).toBe('album');
		expect(kindOf(throughList(inOrder(a)))).toBe('album');
		expect(kindOf(picked(songsOf(a)) ?? inOrder(a))).toBe('songs');
		expect(
			kindOf(
				listed(
					songsOf(a)[0] ?? { album: a, track: track('a', 0) },
					[{ trackId: 'x', albumId: 'z' }],
					() => undefined
				)
			)
		).toBe('list');
		expect(kindOf({ ...inOrder(a), shuffle: { id: 'S' } })).toBe('shuffle');
	});
});

describe('recordOf', () => {
	it('keeps an album in the order playing, with where it goes on from', () => {
		const queue = { ...throughList(shuffled(a, 3)), index: 2 };
		const save = recordOf(queue, 133.4, null, here);
		expect(save).toEqual({
			kind: 'album',
			tracks: queue.tracks.map(({ id }) => id),
			index: 2,
			positionMs: 133_400,
			ordered: false,
			continues: 'a',
			// Its own shuffle, from the song playing on: what turning it off goes back to.
			shuffled: { seed: 3, order: ['a1', 'a2', 'a3'] },
			clientId: here.id,
			device: 'computer'
		});
	});

	it("keeps a list's songs taken in and those it only names", () => {
		const queue = listed(songOfA(1), [{ trackId: 'z0', albumId: 'z' }], () => undefined);
		const save = recordOf(queue, 0, null, here);
		expect(save.kind).toBe('list');
		expect(save.tracks).toEqual(['a1']);
		expect(save.later).toEqual(['z0']);
	});

	it('keeps only the current song or run of a shuffle, and where its session goes on', () => {
		const queue = {
			...(picked([...songsOf(a), ...songsOf(b)]) ?? inOrder(a)),
			index: 2,
			shuffle: { id: 'S1' }
		};
		const save = recordOf(queue, 10, { after: 7, songs: ['a2', 'a3'] }, here);
		expect(save).toMatchObject({
			kind: 'shuffle',
			tracks: ['a2', 'a3'],
			index: 0,
			shuffle: { id: 'S1', after: 7 }
		});
	});

	it('saves a shuffle the server has not answered as its songs', () => {
		const queue = { ...(picked(songsOf(a)) ?? inOrder(a)), shuffle: { id: 'pending-x' } };
		const save = recordOf(queue, 0, null, here);
		expect(save.kind).toBe('songs');
		expect(save.shuffle).toBeUndefined();
		expect(save.tracks).toHaveLength(4);
	});

	it('trims at 1,000: up to 100 behind, the rest ahead', () => {
		const big = album('n', 1500);
		const save = recordOf({ ...inOrder(big), index: 300 }, 0, null, here);
		expect(save.tracks).toHaveLength(1000);
		expect(save.tracks[0]).toBe('n200');
		expect(save.index).toBe(100);
		expect(save.tracks[save.index]).toBe('n300');
		// Too little ahead to fill it: behind takes the rest.
		const late = recordOf({ ...inOrder(big), index: 1400 }, 0, null, here);
		expect(late.tracks).toHaveLength(1000);
		expect(late.tracks.at(-1)).toBe('n1499');
		expect(late.tracks[late.index]).toBe('n1400');
		const early = recordOf({ ...inOrder(big), index: 30 }, 0, null, here);
		expect(early.index).toBe(30);
		expect(early.tracks.at(-1)).toBe('n999');
	});

	it('trims a list across its songs and those it names', () => {
		const queue = listed(
			songOfA(0),
			Array.from({ length: 1200 }, (_, at) => ({ trackId: `z${String(at)}`, albumId: 'z' })),
			() => undefined
		);
		const save = recordOf(queue, 0, null, here);
		expect(save.tracks.length + (save.later?.length ?? 0)).toBe(1000);
	});

	it('holds the position to a day', () => {
		expect(recordOf(inOrder(a), -3, null, here).positionMs).toBe(0);
		expect(recordOf(inOrder(a), 90_000, null, here).positionMs).toBe(86_400_000);
	});
});

function record(extra: Partial<PlaybackRecord>): PlaybackRecord {
	return {
		kind: 'album',
		tracks: a.tracks.map(({ id }) => ({ id, albumId: 'a' })),
		index: 1,
		positionMs: 133_000,
		ordered: true,
		moved: false,
		clientId: here.id,
		device: 'computer',
		updatedAt: 0,
		...extra
	};
}

const both: AlbumLookup = (id) => (id === 'a' ? a : id === 'b' ? b : undefined);

describe('restored', () => {
	it('gives an album back as the same queue', () => {
		const queue = restored(record({ continues: 'a' }), both);
		expect(queue?.albums).toBeUndefined();
		expect(queue?.tracks).toEqual(a.tracks);
		expect(queue?.index).toBe(1);
		expect(queue?.ordered).toBe(true);
		expect(queue?.continues).toEqual({ album: 'a' });
	});

	it('needs the current song here', () => {
		expect(restored(record({}), () => undefined)).toBeNull();
		expect(restored(record({}), () => 'gone')).toBeNull();
		expect(restored(record({ tracks: [{ id: 'nope', albumId: 'a' }], index: 0 }), both)).toBeNull();
	});

	it('waits for the albums ahead that are not here, as a list does', () => {
		const tracks = [
			{ id: 'a0', albumId: 'a' },
			{ id: 'b0', albumId: 'b' },
			{ id: 'c0', albumId: 'c' },
			{ id: 'a1', albumId: 'a' }
		];
		const queue = restored(record({ kind: 'songs', tracks, index: 0, ordered: false }), both);
		expect(queue?.tracks.map(({ id }) => id)).toEqual(['a0', 'b0']);
		expect(queue?.later).toEqual([
			{ trackId: 'c0', albumId: 'c' },
			{ trackId: 'a1', albumId: 'a' }
		]);
	});

	it('leaves out the songs behind whose albums are not here', () => {
		const tracks = [
			{ id: 'c0', albumId: 'c' },
			{ id: 'b0', albumId: 'b' },
			{ id: 'a2', albumId: 'a' }
		];
		const queue = restored(record({ kind: 'songs', tracks, index: 2, ordered: false }), both);
		expect(queue?.tracks.map(({ id }) => id)).toEqual(['b0', 'a2']);
		expect(queue?.index).toBe(1);
		expect(queue?.albums?.get('b0')?.id).toBe('b');
	});

	it("keeps a list's later, and a shuffle's session", () => {
		const list = restored(record({ kind: 'list', later: [{ id: 'b1', albumId: 'b' }] }), both);
		expect(list?.later).toEqual([{ trackId: 'b1', albumId: 'b' }]);
		const shuffle = restored(
			record({
				kind: 'shuffle',
				index: 0,
				shuffle: { id: 'S1', after: 3, scope: { kind: 'library' }, mode: 'endless' }
			}),
			both
		);
		expect(shuffle?.shuffle).toEqual({ id: 'S1', endless: true });
	});

	it('comes back off and off from a record without repeat or shuffled', () => {
		const queue = restored(record({ kind: 'album' }), both);
		expect(queue?.shuffled).toBeUndefined();
	});

	it("keeps the player's own shuffle, with its order to go back to", () => {
		const tracks = [
			{ id: 'a2', albumId: 'a' },
			{ id: 'a0', albumId: 'a' },
			{ id: 'a1', albumId: 'a' },
			{ id: 'a3', albumId: 'a' }
		];
		const order = [
			{ id: 'a0', albumId: 'a' },
			{ id: 'a1', albumId: 'a' },
			{ id: 'a2', albumId: 'a' },
			{ id: 'a3', albumId: 'a' }
		];
		const queue = restored(
			record({ kind: 'album', tracks, index: 1, ordered: false, shuffled: { seed: 9, order } }),
			both
		);
		expect(queue?.shuffled?.seed).toBe(9);
		expect(queue?.shuffled?.ordered).toBe(true);
		const off = queue === null || queue === undefined ? null : shuffledOff(queue);
		// From a0, the album goes on in order: a1, then a3, since a2 has played.
		expect(off?.tracks.slice((off.index ?? 0) + 1).map(({ id }) => id)).toEqual(['a1', 'a3']);
		expect(off?.ordered).toBe(true);
	});
});

describe('recordOf and restored', () => {
	it('round-trips repeat and the shuffle', () => {
		const queue = { ...shuffled(a, 5), index: 1 };
		const save = recordOf(queue, 10, null, here, 'one');
		expect(save.repeat).toBe('one');
		const order = save.shuffled?.order.map((id) => ({ id, albumId: 'a' })) ?? [];
		const back = restored(
			record({
				kind: save.kind,
				tracks: save.tracks.map((id) => ({ id, albumId: 'a' })),
				index: save.index,
				ordered: save.ordered,
				repeat: save.repeat ?? 'off',
				...(save.shuffled === undefined ? {} : { shuffled: { seed: save.shuffled.seed, order } })
			}),
			both
		);
		expect(back?.tracks.map(({ id }) => id)).toEqual(queue.tracks.map(({ id }) => id));
		expect(back?.shuffled?.order.map(({ trackId }) => trackId)).toEqual(
			queue.shuffled?.order
				.slice(queue.shuffled.order.findIndex(({ trackId }) => trackId === queue.tracks[1]?.id))
				.map(({ trackId }) => trackId)
		);
	});

	it('trims a shuffled queue and its order to the cap together', () => {
		const big: Album = {
			...a,
			tracks: Array.from({ length: 900 }, (_, n) => ({ ...songOfA(0).track, id: `x${String(n)}` }))
		};
		const queue = shuffled(big, 1, 0);
		const save = recordOf(queue, 0, null, here);
		const count =
			save.tracks.length + (save.later?.length ?? 0) + (save.shuffled?.order.length ?? 0);
		expect(count).toBeLessThanOrEqual(mostRemembered);
		expect(save.shuffled?.order.length).toBe(mostRemembered / 2);
		expect(save.tracks.at(save.index)).toBe(queue.tracks.at(0)?.id);
	});
});

describe('the words', () => {
	const hour = 3_600_000;
	const now = Date.UTC(2026, 10, 11, 12);

	it.each([
		[0, 'JUST NOW'],
		[59_000, 'JUST NOW'],
		[60_000, '1 MIN AGO'],
		[12 * 60_000, '12 MIN AGO'],
		[hour - 1, '59 MIN AGO'],
		[hour, '1 H AGO'],
		[23 * hour, '23 H AGO'],
		[24 * hour, 'YESTERDAY'],
		[47 * hour, 'YESTERDAY']
	])('%i ms ago reads %s', (ago, words) => {
		expect(ageWords(now - ago, now)).toBe(words);
	});

	it('gives a date past yesterday', () => {
		expect(ageWords(Date.UTC(2026, 10, 3, 12), now)).toBe('3 NOV');
	});

	it('says nothing of a record this browser saved', () => {
		expect(leftOf(record({}), here)).toBeNull();
	});

	it('names the form of another browser, or says it is another of the same', () => {
		const phone = leftOf(
			record({ clientId: 'feedfacecafebeef', device: 'phone', updatedAt: now - 2 * hour }),
			here
		);
		expect(phone === null ? '' : tagWords(phone, now)).toBe('FROM YOUR PHONE · 2 H AGO');
		expect(phone === null ? '' : leftSpoken(phone, now)).toBe(
			'Left off on your phone, 2 hours ago.'
		);
		expect(phone === null ? '' : leftNote(phone, now)).toBe(
			'Your phone left this at 2:13, 2 h ago.'
		);
		const same = leftOf(
			record({ clientId: 'feedfacecafebeef', updatedAt: now - 12 * 60_000 }),
			here
		);
		expect(same === null ? '' : tagWords(same, now)).toBe('FROM ANOTHER BROWSER · 12 MIN AGO');
		expect(same === null ? '' : leftNote(same, now)).toBe(
			'Another browser left this at 2:13, 12 min ago.'
		);
	});

	it('says a song is gone, whoever saved it', () => {
		const gone = leftOf(record({ moved: true, positionMs: 0 }), here);
		expect(gone === null ? '' : tagWords(gone, now)).toBe('THAT ONE’S GONE · NEXT IN LINE');
		expect(gone === null ? '' : leftNote(gone, now)).toBe(
			'The song you left on is gone from the library. This is the one after it.'
		);
	});
});
