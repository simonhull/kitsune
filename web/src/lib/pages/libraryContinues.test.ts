import { describe, expect, it, vi } from 'vitest';
import { createFixtureSource } from '#lib/library/fixtures.ts';
import { Library } from '#lib/library/library.svelte.ts';
import { sortIndex } from '#lib/library/libraryOrder.ts';
import { defaultLibrarySort, type LibrarySort } from '#lib/library/sorts.ts';
import type { Album, LibraryAlbum, LibraryIndex, Track } from '#lib/library/source.ts';
import { SilentOutput } from '#lib/player/output.ts';
import { Player } from '#lib/player/player.svelte.ts';
import { albumAfter, LibraryContinues } from './libraryContinues.ts';
import { keepFor, layOut, type LibraryFilter, type RowMetrics } from './libraryRows.ts';

const metrics: RowMetrics = {
	song: 44,
	firstDisc: 30,
	nextDisc: 60,
	albumHead: 0,
	albumMin: 100,
	artistHead: 0,
	artistMin: 0,
	albumGap: 40,
	artistGap: 60
};

/** An album of `count` 10-second songs, `<id>0`, `<id>1`, …, each with its own album gain. */
function album(id: string, count = 2, albumGain = -1): Album {
	const tracks: Track[] = Array.from({ length: count }, (_, index) => ({
		id: `${id}${String(index)}`,
		title: `${id}${String(index)}`,
		artists: [],
		trackNo: index + 1,
		durationMs: 10_000,
		format: 'FLAC',
		genres: [],
		etag: '',
		hearted: false,
		replayGain: { albumGain, trackGain: -9 }
	}));
	return {
		id,
		title: id,
		artist: 'B',
		artists: [],
		trackCount: count,
		compilation: false,
		tracks,
		discs: [],
		durationMs: count * 10_000,
		formats: ['FLAC'],
		addedAt: '2026-10-12T18:00:00Z',
		genres: [],
		etag: '',
		hearted: false
	};
}

/** The years the albums came out: none in the order they're listed. */
const years = new Map([
	['a', 2003],
	['b', 2001],
	['c', 2004],
	['d', 2002]
]);

function listed({ id, title, tracks }: Album): LibraryAlbum {
	return {
		id,
		title,
		artist: 'B',
		compilation: false,
		year: years.get(id),
		hearted: false,
		tracks: tracks.map(({ id: track, title: name, durationMs }) => ({
			id: track,
			title: name,
			durationMs,
			artists: [],
			hearted: false
		}))
	};
}

/** Lets everything waiting on a resolved promise run. */
async function settle(): Promise<void> {
	await new Promise((done) => setTimeout(done));
}

/**
 * A player and the Library's list, with albums `a` to `d` under one artist each, in that order, or
 * all under one with `together`. The list shows them in `sort`, as the Library page does. Albums
 * are fetched as the app fetches them: `fetched` says which were asked for.
 */
function setup() {
	const records = new Map(['a', 'b', 'c', 'd'].map((id) => [id, album(id)]));
	const library = new Library({ ...createFixtureSource(), streamUrl: (id: string) => id });
	const output = new SilentOutput({ ticking: false });
	const player = new Player(library, () => output);
	const here = new Map<string, Album>();
	const fetched: string[] = [];
	const prefetched: string[] = [];
	const list = {
		order: ['a', 'b', 'c', 'd'],
		filter: 'everything' as LibraryFilter,
		hearted: new Set<string>(),
		sort: defaultLibrarySort as LibrarySort,
		together: false
	};
	const albums = (): LibraryAlbum[] =>
		list.order.flatMap((id) => {
			const record = records.get(id);
			return record === undefined ? [] : [listed(record)];
		});
	const index = (): LibraryIndex =>
		sortIndex(
			{
				artists: list.together
					? [{ id: 'x', name: 'x', sortName: 'x', hearted: false, albums: albums() }]
					: albums().map((record) => ({
							id: record.id,
							name: record.id,
							sortName: record.id,
							hearted: false,
							albums: [record]
						}))
			},
			list.sort
		);
	const marks = {
		hearted: (kind: string, id: string) => kind === 'albums' && list.hearted.has(id),
		stars: () => null
	};
	const reports = vi.spyOn(library, 'reportListen').mockImplementation(() => undefined);
	const load = (id: string): Promise<void> => {
		fetched.push(id);
		const record = records.get(id);
		if (record !== undefined) {
			here.set(id, record);
		}
		return Promise.resolve();
	};
	const continues = new LibraryContinues({
		album: (id) => here.get(id),
		loadAlbum: load,
		// The app waits a moment first, off the play path; here it's at once.
		prefetch: (id) => {
			prefetched.push(id);
			void load(id);
		},
		get queue() {
			return player.queue;
		},
		continueList: (after, next) => player.continueList(after, next),
		albumAfter: (id) => albumAfter(layOut(index(), metrics, keepFor(list.filter, marks)).albums, id)
	});
	player.onward.list = () => {
		continues.keep();
	};
	/** Plays the rest of the song playing, to its end. */
	const finish = async (): Promise<void> => {
		output.elapse(10);
		await settle();
	};
	const play = (id: string, index = 0): void => {
		const record = records.get(id);
		if (record !== undefined) {
			player.play(record, index, 'list');
		}
	};
	const ended = () =>
		reports.mock.calls.flatMap(([report]) =>
			report.ended === undefined ? [] : [`${report.trackId} ${report.ended}`]
		);
	return { player, output, list, continues, fetched, prefetched, finish, play, ended, records };
}

describe('albumAfter', () => {
	it('finds the next album, and nothing past the last or for one not there', () => {
		const albums = layOut(
			{
				artists: [
					{ name: 'x', sortName: 'x', hearted: false, albums: [listed(album('a'))] },
					{ name: 'y', sortName: 'y', hearted: false, albums: [listed(album('b'))] }
				]
			},
			metrics
		).albums;
		expect(albumAfter(albums, 'a')).toBe('b');
		expect(albumAfter(albums, 'b')).toBeNull();
		expect(albumAfter(albums, 'z')).toBeNull();
	});
});

describe('LibraryContinues', () => {
	it('goes on into the next album in the list, as a completed song, to the end of the list', async () => {
		const { player, output, play, finish, ended, prefetched } = setup();
		play('c');
		await settle();
		// The next album's record comes while the first song plays, off the play path: nothing
		// waits at the change.
		expect(prefetched).toEqual(['d']);
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['c0', 'c1']);
		await finish();
		// The last song of the album begins: the next album is put after it, and shows in Up next.
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['c0', 'c1', 'd0', 'd1']);
		expect(player.cue?.upNext.map(({ id }) => id)).toEqual(['d0', 'd1']);
		expect(player.cue?.upNextAlbums.map(({ id }) => id)).toEqual(['d', 'd']);
		await finish();
		expect(player.current?.id).toBe('d0');
		expect(player.album?.id).toBe('d');
		// Now playing counts the song in its album, not in a queue that grows album by album.
		expect(player.cue).toMatchObject({ index: 0, trackCount: 2 });
		expect(output.url).toBe('d0');
		expect(player.status).toBe('playing');
		await finish();
		await finish();
		// The end of the list: at rest, as at the end of any queue.
		expect(player.finished).toBe(true);
		expect(player.status).toBe('paused');
		expect(ended()).toEqual(['c0 completed', 'c1 completed', 'd0 completed', 'd1 completed']);
	});

	it('plays each album at its own album gain', async () => {
		const { player, output, play, finish, records } = setup();
		records.set('d', album('d', 2, -4));
		play('c');
		await settle();
		expect(output.gain).toBe(-1);
		await finish();
		await finish();
		expect(player.current?.id).toBe('d0');
		expect(player.queue?.ordered).toBe(true);
		expect(output.gain).toBe(-4);
	});

	it('preloads the next album’s first song, as it does the next song', async () => {
		const { output, play, finish } = setup();
		play('a');
		await settle();
		await finish();
		output.elapse(1);
		expect(output.preloaded).toBe('b0');
	});

	it('follows the filter the list shows', async () => {
		const { player, list, play, finish } = setup();
		list.filter = 'hearted';
		list.hearted = new Set(['a', 'c']);
		play('a');
		await settle();
		await finish();
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['a0', 'a1', 'c0', 'c1']);
		await finish();
		await finish();
		await finish();
		expect(player.finished).toBe(true);
	});

	it('goes on from the album playing when the list’s order changes', async () => {
		const { player, list, play, finish } = setup();
		play('a');
		await settle();
		list.order = ['c', 'a', 'd', 'b'];
		await finish();
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['a0', 'a1', 'd0', 'd1']);
	});

	it('goes on in the order the list is sorted in: artists Z to A', async () => {
		const { player, list, play, finish } = setup();
		list.sort = { ...defaultLibrarySort, artists: { by: 'name', dir: 'desc' } };
		play('c');
		await settle();
		await finish();
		// The list reads d, c, b, a: after c comes b, not d.
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['c0', 'c1', 'b0', 'b1']);
		await finish();
		expect(player.album?.id).toBe('b');
		await finish();
		await finish();
		expect(player.album?.id).toBe('a');
	});

	it('goes on in the order the list is sorted in: an artist’s albums oldest first', async () => {
		const { player, list, play, finish } = setup();
		list.together = true;
		list.sort = { ...defaultLibrarySort, albums: { by: 'year', dir: 'asc' } };
		play('d');
		await settle();
		await finish();
		// By year the artist's albums read b (2001), d (2002), a (2003), c (2004).
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['d0', 'd1', 'a0', 'a1']);
		await finish();
		expect(player.album?.id).toBe('a');
		await finish();
		await finish();
		expect(player.album?.id).toBe('c');
	});

	it('swaps an album put after the queue that hasn’t begun, for a list that changed', async () => {
		const { player, list, continues, play, finish } = setup();
		play('a');
		await settle();
		await finish();
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['a0', 'a1', 'b0', 'b1']);
		list.order = ['a', 'c', 'b', 'd'];
		continues.keep();
		await settle();
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['a0', 'a1', 'c0', 'c1']);
		await finish();
		expect(player.album?.id).toBe('c');
		// Once it has begun, it's the album playing: a change now is followed from it.
		list.order = ['c', 'a', 'b', 'd'];
		continues.keep();
		await finish();
		expect(player.queue?.tracks.slice(-2).map(({ id }) => id)).toEqual(['a0', 'a1']);
	});

	it('comes to rest when the album playing leaves the list', async () => {
		const { player, list, continues, play, finish } = setup();
		play('a');
		await settle();
		await finish();
		list.order = ['b', 'c', 'd'];
		continues.keep();
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['a0', 'a1']);
		await finish();
		expect(player.finished).toBe(true);
	});

	it('keeps songs put next in front of the album that follows', async () => {
		const { player, play, finish, records } = setup();
		play('a');
		await settle();
		await finish();
		const d = records.get('d');
		const [first] = d?.tracks ?? [];
		if (d !== undefined && first !== undefined) {
			player.playNext([{ album: d, track: first }]);
		}
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['a0', 'a1', 'd0', 'b0', 'b1']);
		await finish();
		await finish();
		expect(player.album?.id).toBe('b');
	});

	it('plays songs added to the queue before the list goes on', async () => {
		const { player, play, finish, records } = setup();
		play('a');
		await settle();
		await finish();
		const d = records.get('d');
		const [first] = d?.tracks ?? [];
		if (d !== undefined && first !== undefined) {
			player.addToQueue([{ album: d, track: first }]);
		}
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['a0', 'a1', 'd0', 'b0', 'b1']);
		await finish();
		expect(player.current?.id).toBe('d0');
		await finish();
		expect(player.album?.id).toBe('b');
	});

	it('doesn’t go on in a shuffle, which owns what comes next', async () => {
		const { player, play, finish, records, fetched } = setup();
		play('a');
		await settle();
		const a = records.get('a');
		const [first] = a?.tracks ?? [];
		if (a !== undefined && first !== undefined) {
			player.playSongs([{ album: a, track: first }], { id: 'shuffle' });
		}
		fetched.length = 0;
		await finish();
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['a0']);
		expect(player.finished).toBe(true);
		expect(fetched).toEqual([]);
	});

	it('doesn’t go on for an album played anywhere but the list', async () => {
		const { player, play, finish, records, fetched } = setup();
		const b = records.get('b');
		play('a');
		await settle();
		if (b !== undefined) {
			player.play(b);
		}
		fetched.length = 0;
		await finish();
		await finish();
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['b0', 'b1']);
		expect(player.finished).toBe(true);
		expect(fetched).toEqual([]);
	});

	it('plays what it holds going round again after the end, and goes no further', async () => {
		const { player, list, play, finish } = setup();
		list.order = ['c', 'd'];
		play('c', 1);
		await settle();
		await finish();
		await finish();
		await finish();
		expect(player.finished).toBe(true);
		expect(player.queue?.continues).toBeUndefined();
		player.toggle();
		await settle();
		expect(player.current?.id).toBe('c0');
	});
});
