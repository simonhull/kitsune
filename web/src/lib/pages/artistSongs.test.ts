import { describe, expect, it } from 'vitest';
import type { LibraryAlbum, LibraryIndex, LibraryTrack } from '#lib/library/source.ts';
import {
	albumOrder,
	albumSongs,
	coldStart,
	flatOrder,
	headingOf,
	queueFrom,
	rankTop,
	rowWindow,
	albumsLayout,
	songsOf,
	starWeight,
	topCount,
	topScore,
	type ArtistSong
} from './artistSongs.ts';
import type { MarksView } from './libraryRows.ts';

function song(id: string, title: string, extra: Partial<LibraryTrack> = {}): LibraryTrack {
	return { id, title, durationMs: 60_000, artists: ['Ann'], ...extra };
}

function album(id: string, year: number, tracks: LibraryTrack[]): LibraryAlbum {
	return { id, title: id, artist: 'Ann', compilation: false, year, tracks };
}

const ann = { id: 'ann', name: 'Ann', sortName: 'Ann' };

/** Two of Ann's own albums, newest first as the server sends them, then one she appears on. */
const index: LibraryIndex = {
	artists: [
		{
			...ann,
			albums: [
				album('new', 2020, [
					song('n1', 'Breakwater', { plays: 3 }),
					song('n2', 'Condensation', { plays: 0 })
				]),
				album('old', 1990, [song('o1', 'Anchor', { plays: 9 }), song('o2', 'Driftwood')]),
				{
					...album('guest', 2001, [song('g1', 'Echo', { plays: 3, artists: ['Bob', 'Ann'] })]),
					artist: 'Bob'
				}
			]
		}
	]
};
const owned = new Set(['new', 'old']);

function marksOf(stars: Record<string, number> = {}): MarksView {
	return { stars: (id) => stars[id] ?? null };
}

const ids = (songs: readonly ArtistSong[]): string[] => songs.map(({ track }) => track.id);

describe('Top', () => {
	it('weighs stars on the plays', () => {
		expect([5, 4, 3, null, 2, 1].map(starWeight)).toEqual([2, 1.5, 1, 1, 0.5, 0.25]);
		expect([4.5, 3.5, 2.5, 1.5, 0.5].map(starWeight)).toEqual([1.73, 1.22, 0.71, 0.35, 0.125]);
		const marks = marksOf({ n1: 5 });
		expect(topScore(song('n1', 'x', { plays: 3 }), marks)).toBe(4 * 2);
		expect(topScore(song('n2', 'x'), marks)).toBe(1);
	});

	it('ranks a song up by its stars', () => {
		const songs = songsOf(index, owned);
		// At 4½, g1 (3 plays) is 4 × 1.73 = 6.92: past n1, still under o1's 10.
		expect(ids(rankTop(songs, marksOf({ g1: 4.5 })))).toEqual(['o1', 'g1', 'n1', 'n2', 'o2']);
	});

	it('ranks best first, a tie in the server order', () => {
		const songs = songsOf(index, owned);
		// o1: 10; n1 and g1: 4 each, n1 first in the server's order; then n2 and o2 at 1.
		expect(ids(rankTop(songs, marksOf()))).toEqual(['o1', 'n1', 'g1', 'n2', 'o2']);
		// Five stars on a song never played: 2, above those at 1 and below those played.
		expect(ids(rankTop(songs, marksOf({ o2: 5 })))).toEqual(['o1', 'n1', 'g1', 'o2', 'n2']);
	});

	it('is cold when nothing was played or rated, and then newest first', () => {
		const cold = songsOf(
			{
				artists: index.artists.map((artist) => ({
					...artist,
					albums: artist.albums.map((each) => ({
						...each,
						tracks: each.tracks.map((track) => ({ ...track, plays: 0 }))
					}))
				}))
			},
			owned
		);
		expect(coldStart(cold, marksOf())).toBe(true);
		expect(ids(rankTop(cold, marksOf()))).toEqual(['n1', 'n2', 'o1', 'o2', 'g1']);
		expect(coldStart(cold, marksOf({ g1: 3 }))).toBe(true);
		expect(coldStart(cold, marksOf({ g1: 4 }))).toBe(false);
	});
});

describe('the flat list', () => {
	const songs = songsOf(index, owned);
	const marks = marksOf({ n2: 4, o2: 4, g1: 2 });

	it('turns each order round, ties staying in the server order', () => {
		expect(ids(flatOrder(songs, { by: 'plays', dir: 'desc' }, marks))).toEqual([
			'o1',
			'n1',
			'g1',
			'n2',
			'o2'
		]);
		expect(ids(flatOrder(songs, { by: 'plays', dir: 'asc' }, marks))).toEqual([
			'n2',
			'o2',
			'n1',
			'g1',
			'o1'
		]);
		expect(ids(flatOrder(songs, { by: 'title', dir: 'asc' }, marks))).toEqual([
			'o1',
			'n1',
			'n2',
			'o2',
			'g1'
		]);
		expect(ids(flatOrder(songs, { by: 'title', dir: 'desc' }, marks))).toEqual([
			'g1',
			'o2',
			'n2',
			'n1',
			'o1'
		]);
		expect(ids(flatOrder(songs, { by: 'year', dir: 'asc' }, marks))).toEqual([
			'o1',
			'o2',
			'g1',
			'n1',
			'n2'
		]);
	});

	it('rates by stars, then by plays', () => {
		expect(ids(flatOrder(songs, { by: 'rating', dir: 'desc' }, marks))).toEqual([
			'n2',
			'o2',
			'g1',
			'o1',
			'n1'
		]);
	});

	it('puts a song with no date added last, whichever way', () => {
		const dated = songsOf(
			{
				artists: [
					{
						...ann,
						albums: [
							album('a', 2000, [
								song('a1', 'One', { addedAt: '2026-01-01T00:00:00Z' }),
								song('a2', 'Two'),
								song('a3', 'Three', { addedAt: '2026-02-01T00:00:00Z' })
							])
						]
					}
				]
			},
			new Set(['a'])
		);
		expect(ids(flatOrder(dated, { by: 'added', dir: 'desc' }, marks))).toEqual(['a3', 'a1', 'a2']);
		expect(ids(flatOrder(dated, { by: 'added', dir: 'asc' }, marks))).toEqual(['a1', 'a3', 'a2']);
	});
});

describe('by album', () => {
	it("lists the artist's own albums first, then those it appears on", () => {
		const oldest = albumOrder(index, owned, { by: 'year', dir: 'asc' });
		expect(oldest.own.map(({ id }) => id)).toEqual(['old', 'new']);
		expect(oldest.appears.map(({ id }) => id)).toEqual(['guest']);
		expect(ids(albumSongs(oldest, songsOf(index, owned)))).toEqual(['o1', 'o2', 'n1', 'n2', 'g1']);
	});
});

describe('playing', () => {
	it('plays from the song pressed to the end of the list as shown', () => {
		const ranked = rankTop(songsOf(index, owned), marksOf());
		expect(ids(queueFrom(ranked, 'n1'))).toEqual(['n1', 'g1', 'n2', 'o2']);
		expect(ids(queueFrom(ranked, 'o2'))).toEqual(['o2']);
		expect(queueFrom(ranked, 'gone')).toEqual([]);
	});

	it('goes on past the fold', () => {
		const ranked = rankTop(songsOf(index, owned), marksOf());
		const shown = topCount(false);
		// Top shows them all here; with a fold at two, the third song still follows the second.
		expect(ranked.length).toBeLessThanOrEqual(shown);
		const [, second, third] = ranked;
		expect(ids(queueFrom(ranked, second?.track.id ?? '')).at(1)).toBe(third?.track.id);
	});
});

describe('the heading', () => {
	it('says Top, Songs, or All', () => {
		expect(headingOf(false, 30, 10, false)).toBe('top');
		expect(headingOf(false, 30, 10, true)).toBe('cold');
		expect(headingOf(false, 10, 10, false)).toBe('songs');
		expect(headingOf(false, 3, 10, true)).toBe('songs');
		expect(headingOf(true, 30, 10, false)).toBe('all');
		expect(topCount(true)).toBe(5);
	});
});

describe('windowing', () => {
	it('mounts a screen either side of a flat list, and holds it until near its edge', () => {
		const first = rowWindow(800, 44, 0, 880, { first: 0, last: -1 });
		expect(first).toEqual({ first: 0, last: 40 });
		// A little scroll stays within the half screen: the same window, the same object.
		expect(rowWindow(800, 44, 200, 880, first)).toBe(first);
		expect(rowWindow(800, 44, 4400, 880, first)).toEqual({ first: 80, last: 140 });
		expect(rowWindow(800, 44, 99_999, 880, first)).toEqual({ first: 799, last: 799 });
		expect(rowWindow(0, 44, 0, 880, first)).toEqual({ first: 0, last: -1 });
	});

	it('lays the albums out own first, then the divider, then the rest', () => {
		const metrics = {
			song: 44,
			firstDisc: 36,
			nextDisc: 64,
			albumHead: 0,
			albumMin: 0,
			artistHead: 0,
			artistMin: 0,
			albumGap: 40,
			artistGap: 0
		};
		const runs = albumOrder(index, owned, { by: 'year', dir: 'desc' });
		const laid = albumsLayout(ann, runs, metrics, 72);
		expect(laid.albums.map(({ album: { id }, top }) => [id, top])).toEqual([
			['new', 0],
			['old', 128],
			['guest', 128 + 88 + 40 + 72]
		]);
		expect(laid.divider).toBe(128 + 88 + 40);
		expect(laid.rows.at(-1)).toMatchObject({ top: 328, albumIndex: 2, trackIndex: 0 });
		expect(laid.albums.at(-1)).toMatchObject({ firstRow: 4, lastRow: 4 });
		expect(laid.height).toBe(328 + 44);
		expect(albumsLayout(ann, { own: [], appears: runs.appears }, metrics, 72)).toMatchObject({
			divider: null,
			height: 44
		});
	});
});
