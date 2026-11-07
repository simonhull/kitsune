import { describe, expect, it } from 'vitest';
import type {
	LibraryAlbum,
	LibraryArtist,
	LibraryIndex,
	LibraryTrack
} from '#lib/library/source.ts';
import {
	artistAt,
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
	disc: 30,
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
			[332, 148],
			[540, 132]
		]);
		expect(layout.artists.map(({ top, height }) => [top, height])).toEqual([
			[0, 272],
			[332, 148],
			[540, 132]
		]);
		expect(layout.height).toBe(672);
	});

	it('puts each row under its album, in order', () => {
		expect(layout.rows.map(({ top }) => top)).toEqual([
			0, 44, 88, 172, 332, 362, 406, 436, 540, 584, 628
		]);
		const [first, second] = layout.albums;
		expect([first?.firstRow, first?.lastRow, second?.firstRow, second?.lastRow]).toEqual([
			0, 2, 3, 3
		]);
		expect(layout.albums.map(({ artistIndex }) => artistIndex)).toEqual([0, 0, 1, 2]);
	});

	it('gives a disc row only to an album with several discs', () => {
		expect(layout.rows.filter(({ kind }) => kind === 'disc')).toMatchObject([
			{ albumIndex: 2, disc: 1 },
			{ albumIndex: 2, disc: 2 }
		]);
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
