import { describe, expect, it } from 'vitest';
import type {
	LibraryAlbum,
	LibraryArtist,
	LibraryIndex,
	LibraryTrack
} from '#lib/library/source.ts';
import {
	artistAt,
	keepFor,
	layOut,
	letterOf,
	letterTarget,
	railHere,
	railLetters,
	rowAt,
	rowKey,
	windowAt,
	type RowMetrics
} from './libraryRows';

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

function song(id: string, discNo?: number): LibraryTrack {
	return {
		id,
		title: id,
		durationMs: 1000,
		artists: [],
		hearted: false,
		...(discNo === undefined ? {} : { discNo })
	};
}

function album(id: string, tracks: LibraryTrack[]): LibraryAlbum {
	return { id, title: id, artist: 'x', compilation: false, hearted: false, tracks };
}

function artist(name: string, albums: LibraryAlbum[]): LibraryArtist {
	return { id: name, name, sortName: name, hearted: false, albums };
}

const index: LibraryIndex = {
	artists: [
		artist('Abba', [
			album('a1', [song('a', 1), song('b', 1), song('c', 1)]),
			album('a2', [song('d')])
		]),
		artist('Alan', [album('b1', [song('e', 1), song('f', 2)])]),
		artist('Cher', [album('c1', [song('g'), song('h'), song('i')])])
	]
};

describe('layOut', () => {
	const layout = layOut(index, metrics);

	it('adds up: every block starts where the last one and its gap end', () => {
		// Abba: 3 songs is 132, a gap of 40, then one song held to the album's least height.
		expect(layout.albums.map(({ top, height }) => [top, height])).toEqual([
			[0, 132],
			[172, 100],
			[332, 178],
			[570, 132]
		]);
		expect(layout.artists.map(({ top, height }) => [top, height])).toEqual([
			[0, 272],
			[332, 178],
			[570, 132]
		]);
		expect(layout.height).toBe(702);
	});

	it('puts each row under its album, in order', () => {
		expect(layout.rows.map(({ top }) => top)).toEqual([
			0, 44, 88, 172, 332, 362, 406, 466, 570, 614, 658
		]);
		const [first, second] = layout.albums;
		expect([first?.firstRow, first?.lastRow, second?.firstRow, second?.lastRow]).toEqual([
			0, 2, 3, 3
		]);
		expect(layout.albums.map(({ artistIndex }) => artistIndex)).toEqual([0, 0, 1, 2]);
	});

	it('gives a disc row only to an album with several discs', () => {
		expect(layout.rows.filter(({ kind }) => kind === 'disc')).toMatchObject([
			{ albumIndex: 2, disc: 1, first: true, height: 30 },
			{ albumIndex: 2, disc: 2, first: false, height: 60 }
		]);
	});

	it('heads a disc as the first when a filter leaves none above it', () => {
		const kept = layOut(index, metrics, (_artist, _album, track) => track.id === 'f');
		expect(kept.rows).toMatchObject([
			{ kind: 'disc', disc: 2, first: true, top: 0, height: 30 },
			{ kind: 'song', top: 30, height: 44 }
		]);
		expect(kept.albums.at(0)?.height).toBe(100);
	});

	it('leaves room for the heads a stacked layout has', () => {
		const stacked = layOut(index, { ...metrics, albumHead: 72, albumMin: 0, artistHead: 80 });
		expect(stacked.albums.at(0)?.top).toBe(80);
		expect(stacked.rows.at(0)?.top).toBe(152);
		expect(stacked.albums.at(1)).toMatchObject({ top: 80 + 72 + 132 + 40, height: 72 + 44 });
	});

	it('gives the first artist of each letter', () => {
		expect([...layout.letters]).toEqual([
			['A', 0],
			['C', 2]
		]);
	});

	it('files an artist under its sort name, not the name on show', () => {
		const the = { ...artist('The Band', [album('t', [song('t')])]), sortName: 'Band, The' };
		expect([...layOut({ artists: [the] }, metrics).letters]).toEqual([['B', 0]]);
	});

	it('lays out nothing for an empty library', () => {
		expect(layOut({ artists: [] }, metrics)).toMatchObject({ rows: [], height: 0 });
	});
});

describe('an artist shorter than its name', () => {
	const index: LibraryIndex = {
		artists: [artist('A', [album('a', [song('a1')])]), artist('B', [album('b', [song('b1')])])]
	};

	it('stands tall enough to hold the name where it pins', () => {
		const { artists } = layOut(index, { ...metrics, artistMin: 300 });
		expect(artists[0]?.height).toBe(300);
	});

	it('moves the next artist down by as much', () => {
		const { artists, height } = layOut(index, { ...metrics, artistMin: 300 });
		expect(artists[1]?.top).toBe(300 + metrics.artistGap);
		expect(height).toBe(2 * 300 + metrics.artistGap);
	});

	it('leaves a taller artist as it is', () => {
		const { artists } = layOut(index, { ...metrics, artistMin: 50 });
		expect(artists[0]?.height).toBe(metrics.albumMin);
	});
});

describe('rowAt', () => {
	const layout = layOut(index, metrics);

	it('finds the first row at 0 and before it', () => {
		expect(rowAt(layout, 0)).toBe(0);
		expect(rowAt(layout, -50)).toBe(0);
	});

	it('finds a row from its first pixel', () => {
		expect(rowAt(layout, 43)).toBe(0);
		expect(rowAt(layout, 44)).toBe(1);
	});

	it('stays on the last row through a gap', () => {
		expect(rowAt(layout, 150)).toBe(2);
	});

	it('finds the last row past the end', () => {
		expect(rowAt(layout, 1_000_000)).toBe(layout.rows.length - 1);
	});
});

describe('artistAt', () => {
	it('finds whose block a line falls in', () => {
		const layout = layOut(index, metrics);
		expect(artistAt(layout, 0)).toBe(0);
		expect(artistAt(layout, 331)).toBe(0);
		expect(artistAt(layout, 332)).toBe(1);
		expect(artistAt(layout, 9999)).toBe(2);
	});
});

describe('rowKey', () => {
	it('names a song by its ID, wherever the layout has moved it', () => {
		const layout = layOut(index, metrics);
		expect(rowKey(layout, 5)).toBe('e');
		expect(rowKey(layout, 4)).toBeUndefined();
	});
});

describe('windowAt', () => {
	const layout = layOut(index, metrics);

	it('mounts a viewport either side of the screen', () => {
		expect(windowAt(layout, 200, 100, { first: 0, last: -1 })).toEqual({ first: 2, last: 5 });
	});

	it('stays put while the screen has room inside it', () => {
		const mounted = { first: 2, last: 5 };
		expect(windowAt(layout, 250, 100, mounted)).toBe(mounted);
	});

	it('moves once the screen comes near its edge', () => {
		expect(windowAt(layout, 520, 100, { first: 2, last: 5 })).toEqual({ first: 6, last: 10 });
	});

	it('stays put at the ends of the list, where it can go no further', () => {
		const mounted = { first: 0, last: layout.rows.length - 1 };
		expect(windowAt(layout, 0, 100, mounted)).toBe(mounted);
	});

	it('lets go of rows a shorter list no longer has', () => {
		const shorter = layOut({ artists: index.artists.slice(0, 1) }, metrics);
		expect(windowAt(shorter, 0, 100, { first: 0, last: 10 })).toEqual({ first: 0, last: 3 });
	});
});

describe('letterOf', () => {
	it.each([
		['Élan', 'E'],
		['élan', 'E'],
		['abba', 'A'],
		['Zed', 'Z'],
		['3 Doors', '#'],
		['東京', '#'],
		['Ørsted', '#'],
		['', '#']
	])('files %s under %s', (name, letter) => {
		expect(letterOf(name)).toBe(letter);
	});
});

describe('railLetters', () => {
	const layout = layOut(index, metrics);

	it('runs A to Z at desktop, with no # for a library that has none', () => {
		const letters = railLetters(layout, false);
		expect(letters).toHaveLength(26);
		expect(letters.at(0)).toBe('A');
	});

	it('shows every other letter or so on a phone', () => {
		expect(railLetters(layout, true).join('')).toBe('ADGJLNPRTVXZ');
	});

	it('puts # at the top when something files there', () => {
		const numbered = layOut({ artists: [artist('3 Doors', [album('n', [song('n')])])] }, metrics);
		expect(railLetters(numbered, false).at(0)).toBe('#');
		expect(railLetters(numbered, true).at(0)).toBe('#');
	});
});

describe('letterTarget', () => {
	const layout = layOut(index, metrics);

	it('jumps to the first artist of a letter', () => {
		expect(letterTarget(layout, 'C')).toBe(2);
	});

	it('goes on to the next letter there is: a letter on the rail is never dead', () => {
		expect(letterTarget(layout, 'B')).toBe(2);
	});

	it('stops at the last artist past the end', () => {
		expect(letterTarget(layout, 'Z')).toBe(2);
	});
});

describe('railHere', () => {
	it('lights the letter itself when the rail has it', () => {
		expect(railHere(railLetters(layOut(index, metrics), false), 'M')).toBe('M');
	});

	it('lights the letter before it on a rail that skips', () => {
		expect(railHere(['#', ...'ADGJLNPRTVXZ'], 'M')).toBe('L');
		expect(railHere(['#', ...'ADGJLNPRTVXZ'], '#')).toBe('#');
	});
});

/** Marks for the filter tests: hearts by kind and ID, and stars by track. */
function marks(hearts: string[], stars: Record<string, number> = {}) {
	return {
		hearted: (kind: string, id: string) => hearts.includes(`${kind}/${id}`),
		stars: (id: string): number | null => stars[id] ?? null
	};
}

/** What a layout shows: each artist, with each album's songs. */
function shown(index: LibraryIndex, keep: ReturnType<typeof keepFor>) {
	const layout = layOut(index, metrics, keep);
	return layout.artists.map(({ artist }) => ({
		[artist.name]: layout.albums
			.filter(({ artistIndex }) => layout.artists.at(artistIndex)?.artist === artist)
			.map(({ album, firstRow, lastRow }) =>
				layout.rows
					.slice(firstRow, lastRow + 1)
					.flatMap((row) => (row.kind === 'song' ? [album.tracks.at(row.trackIndex)?.id] : []))
			)
	}));
}

describe('the Hearted filter', () => {
	it('shows only hearted songs, under their album and artist', () => {
		const keep = keepFor('hearted', marks(['tracks/b', 'tracks/h']));
		expect(shown(index, keep)).toEqual([{ Abba: [['b']] }, { Cher: [['h']] }]);
	});

	it('shows every song of a hearted album', () => {
		const keep = keepFor('hearted', marks(['albums/a1', 'tracks/i']));
		expect(shown(index, keep)).toEqual([{ Abba: [['a', 'b', 'c']] }, { Cher: [['i']] }]);
	});

	it('shows every album of a hearted artist', () => {
		const keep = keepFor('hearted', marks(['artists/Abba']));
		expect(shown(index, keep)).toEqual([{ Abba: [['a', 'b', 'c'], ['d']] }]);
	});

	it('takes no heart from a group with nobody to link to', () => {
		const various = { ...artist('Various', [album('v', [song('v1')])]), id: undefined };
		const keep = keepFor('hearted', marks(['artists/undefined', 'artists/Various']));
		expect(layOut({ artists: [various] }, metrics, keep).artists).toEqual([]);
	});

	it('knows nothing of stars', () => {
		expect(shown(index, keepFor('hearted', marks([], { a: 5 })))).toEqual([]);
	});
});

describe('the 4 and up filter', () => {
	it('shows songs rated 4 or 5, under their album and artist', () => {
		const keep = keepFor('rated', marks([], { a: 4, b: 3, d: 5, g: 1 }));
		expect(shown(index, keep)).toEqual([{ Abba: [['a'], ['d']] }]);
	});

	it("never shows an unrated song because its neighbors were good: album means don't count", () => {
		const keep = keepFor('rated', marks(['albums/c1', 'artists/Cher'], { g: 5, h: 5 }));
		expect(shown(index, keep)).toEqual([{ Cher: [['g', 'h']] }]);
	});

	it('knows nothing of hearts', () => {
		expect(shown(index, keepFor('rated', marks(['tracks/a', 'artists/Abba'])))).toEqual([]);
	});
});

describe('a filtered layout', () => {
	const keep = keepFor('hearted', marks(['tracks/b', 'tracks/c', 'tracks/h']));
	const layout = layOut(index, metrics, keep);

	it('lays out everything with no filter', () => {
		expect(keepFor('everything', marks(['tracks/a']))).toBeUndefined();
		expect(layOut(index, metrics, undefined)).toEqual(layOut(index, metrics));
	});

	it('keeps each row on its track in the album, whatever is hidden before it', () => {
		const first = layout.albums.at(0);
		const rows = layout.rows.slice(first?.firstRow, (first?.lastRow ?? 0) + 1);
		expect(rows.map((row) => (row.kind === 'song' ? row.trackIndex : -1))).toEqual([1, 2]);
	});

	it('counts what it shows of each album and artist, and of the library', () => {
		expect(layout.albums.map(({ shown: songs, album }) => [songs, album.tracks.length])).toEqual([
			[2, 3],
			[1, 3]
		]);
		expect(layout.artists.map(({ albums, songs }) => [albums, songs])).toEqual([
			[1, 2],
			[1, 1]
		]);
		expect([layout.songs, layout.totalSongs]).toEqual([3, 9]);
	});

	it('remembers where each artist stands in the whole library', () => {
		expect(layout.artists.map(({ source }) => source)).toEqual([0, 2]);
		expect(layout.albums.map(({ artistIndex }) => artistIndex)).toEqual([0, 1]);
	});

	it('gives a letter only to an artist it shows', () => {
		expect([...layout.letters]).toEqual([
			['A', 0],
			['C', 1]
		]);
		expect(letterTarget(layout, 'B')).toBe(1);
	});

	it('lays out nothing when nothing is kept', () => {
		const none = layOut(index, metrics, keepFor('rated', marks([])));
		expect(none).toMatchObject({ rows: [], albums: [], artists: [], height: 0, songs: 0 });
		expect(none.totalSongs).toBe(9);
	});
});
