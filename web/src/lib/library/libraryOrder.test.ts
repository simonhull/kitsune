import { describe, expect, it } from 'vitest';
import {
	albumAdded,
	hasLetters,
	railFor,
	railLit,
	sortFields,
	sortIndex,
	turnedLetterTarget,
	turnedRailHere
} from './libraryOrder.ts';
import type { LibraryAlbum, LibraryArtist, LibraryIndex, LibraryTrack } from './source.ts';
import { defaultLibrarySort, type LibrarySort } from './sorts.ts';

function track(id: string, extra: Partial<LibraryTrack> = {}): LibraryTrack {
	return { id, title: id, durationMs: 1000, artists: [], hearted: false, ...extra };
}

function album(id: string, extra: Partial<LibraryAlbum> = {}): LibraryAlbum {
	return {
		id,
		title: id,
		artist: 'x',
		compilation: false,
		hearted: false,
		tracks: [track(`${id}-1`)],
		...extra
	};
}

function artist(id: string, albums: LibraryAlbum[], sortName = id): LibraryArtist {
	return { id, name: id, sortName, hearted: false, albums };
}

const added = (at: string, plays = 0): Partial<LibraryAlbum> => ({
	tracks: [track(`t-${at}`, { addedAt: at, plays })]
});

function sorted(index: LibraryIndex, sort: Partial<LibrarySort>): string[] {
	return sortIndex(index, { ...defaultLibrarySort, ...sort }).artists.map(({ id = '' }) => id);
}

function albumsOf(index: LibraryIndex, albums: LibrarySort['albums']): string[] {
	return sortIndex(index, { ...defaultLibrarySort, albums }).artists.flatMap((a) =>
		a.albums.map(({ id }) => id)
	);
}

// As the server sends it: by name, then ID; Ann's albums newest first, compilations last.
const index: LibraryIndex = {
	artists: [
		artist('ann', [
			album('Wake', { year: 2020, ...added('2026-03-01T00:00:00Z', 2) }),
			album('Bright', { year: 2011, ...added('2026-05-01T00:00:00Z', 1) }),
			album('Undated', added('2026-01-01T00:00:00Z')),
			album('Best Of', { year: 2024, compilation: true, ...added('2026-02-01T00:00:00Z') })
		]),
		artist('bob-1', [album('B', added('2026-04-01T00:00:00Z', 9))], 'Bob'),
		artist('bob-2', [album('C', added('2026-04-01T00:00:00Z', 9))], 'bob'),
		artist('zed', [album('Z', added('2025-01-01T00:00:00Z', 1))])
	]
};

describe('sortIndex', () => {
	it('gives the server’s own order back as it is, at no cost', () => {
		expect(sortIndex(index, defaultLibrarySort)).toBe(index);
	});

	it('turns the names round, keeping artists of one name in ID order', () => {
		expect(sorted(index, { artists: { by: 'name', dir: 'desc' } })).toEqual([
			'zed',
			'bob-1',
			'bob-2',
			'ann'
		]);
	});

	it('puts the artist with the newest album first, ties by name', () => {
		expect(sorted(index, { artists: { by: 'added', dir: 'desc' } })).toEqual([
			'ann',
			'bob-1',
			'bob-2',
			'zed'
		]);
		expect(sorted(index, { artists: { by: 'added', dir: 'asc' } })).toEqual([
			'zed',
			'bob-1',
			'bob-2',
			'ann'
		]);
	});

	it('puts the most played artist first, adding up its albums, ties by name', () => {
		expect(sorted(index, { artists: { by: 'plays', dir: 'desc' } })).toEqual([
			'bob-1',
			'bob-2',
			'ann',
			'zed'
		]);
		expect(sorted(index, { artists: { by: 'plays', dir: 'asc' } })).toEqual([
			'zed',
			'ann',
			'bob-1',
			'bob-2'
		]);
	});

	it('runs years either way, compilations and albums with no year after the rest', () => {
		expect(albumsOf(index, { by: 'year', dir: 'desc' }).slice(0, 4)).toEqual([
			'Wake',
			'Bright',
			'Undated',
			'Best Of'
		]);
		expect(albumsOf(index, { by: 'year', dir: 'asc' }).slice(0, 4)).toEqual([
			'Bright',
			'Wake',
			'Undated',
			'Best Of'
		]);
	});

	it('orders an artist’s albums by name and by when they arrived', () => {
		expect(albumsOf(index, { by: 'name', dir: 'asc' }).slice(0, 4)).toEqual([
			'Best Of',
			'Bright',
			'Undated',
			'Wake'
		]);
		expect(albumsOf(index, { by: 'name', dir: 'desc' }).slice(0, 4)).toEqual([
			'Wake',
			'Undated',
			'Bright',
			'Best Of'
		]);
		expect(albumsOf(index, { by: 'added', dir: 'desc' }).slice(0, 4)).toEqual([
			'Bright',
			'Wake',
			'Best Of',
			'Undated'
		]);
	});

	it('breaks a tie between albums by name, then ID', () => {
		const same = { year: 2000 };
		const twins: LibraryIndex = {
			artists: [
				artist('a', [album('2', { ...same, title: 'T' }), album('1', { ...same, title: 'T' })])
			]
		};
		expect(albumsOf(twins, { by: 'year', dir: 'asc' })).toEqual(['1', '2']);
	});

	it('puts an album whose arrival isn’t known last, either way', () => {
		const unknown: LibraryIndex = {
			artists: [artist('a', [album('none'), album('known', added('2026-01-01T00:00:00Z'))])]
		};
		expect(albumsOf(unknown, { by: 'added', dir: 'asc' })).toEqual(['known', 'none']);
		expect(albumsOf(unknown, { by: 'added', dir: 'desc' })).toEqual(['known', 'none']);
	});

	it('leaves the index it was given alone', () => {
		const before = JSON.stringify(index);
		sortIndex(index, { artists: { by: 'plays', dir: 'desc' }, albums: { by: 'name', dir: 'asc' } });
		expect(JSON.stringify(index)).toBe(before);
	});
});

describe('albumAdded', () => {
	it('is when its first song arrived, whatever the precision of the time', () => {
		const a = album('a', {
			tracks: [
				track('1', { addedAt: '2026-01-02T00:00:00.5Z' }),
				track('2', { addedAt: '2026-01-02T00:00:00Z' })
			]
		});
		expect(albumAdded(a)).toBe(Date.parse('2026-01-02T00:00:00Z'));
		expect(albumAdded(album('b'))).toBeUndefined();
	});
});

describe('sortFields', () => {
	it('asks for what an order reads, and nothing for the default', () => {
		expect(sortFields(defaultLibrarySort)).toEqual([]);
		expect(
			sortFields({ artists: { by: 'plays', dir: 'desc' }, albums: { by: 'added', dir: 'desc' } })
		).toEqual(['added', 'plays']);
		expect(
			sortFields({ artists: { by: 'added', dir: 'desc' }, albums: { by: 'year', dir: 'asc' } })
		).toEqual(['added']);
	});
});

describe('the rail', () => {
	it('has letters only under a name order', () => {
		expect(hasLetters(defaultLibrarySort)).toBe(true);
		expect(hasLetters({ ...defaultLibrarySort, artists: { by: 'name', dir: 'desc' } })).toBe(true);
		expect(hasLetters({ ...defaultLibrarySort, artists: { by: 'plays', dir: 'desc' } })).toBe(
			false
		);
		expect(hasLetters({ ...defaultLibrarySort, artists: { by: 'added', dir: 'desc' } })).toBe(
			false
		);
	});

	const rail = ['#', 'A', 'B', 'C', 'Z'];
	const turned = railFor(rail, { ...defaultLibrarySort, artists: { by: 'name', dir: 'desc' } });

	it('turns round with the names', () => {
		expect(railFor(rail, defaultLibrarySort)).toEqual(rail);
		expect(turned).toEqual(['Z', 'C', 'B', 'A', '#']);
	});

	it('jumps to a letter’s first artist on a turned list, or the next letter down', () => {
		// Z to A: zed, then the Bs, then ann; nothing under C.
		const letters = new Map([
			['Z', 0],
			['B', 1],
			['A', 3]
		]);
		expect(turnedLetterTarget(letters, turned, 'B', 4)).toBe(1);
		expect(turnedLetterTarget(letters, turned, 'C', 4)).toBe(1);
		expect(turnedLetterTarget(letters, turned, '#', 4)).toBe(3);
	});

	it('lights the letter an artist is under, or the nearest above it on a rail that skips it', () => {
		expect(turnedRailHere(turned, 'B')).toBe('B');
		expect(turnedRailHere(['Z', 'X', 'A'], 'Y')).toBe('Z');
		expect(turnedRailHere(turned, '#')).toBe('#');
	});
});

describe('railLit', () => {
	it('lights the letter, or the nearest drawn before it, either way round', () => {
		expect(railLit(['#', 'A', 'D', 'G'], 'E')).toBe('D');
		expect(railLit(['#', 'A', 'D', 'G'], '#')).toBe('#');
		expect(railLit(['G', 'D', 'A', '#'], 'E')).toBe('G');
		expect(railLit(['G', 'D', 'A', '#'], 'D')).toBe('D');
		expect(railLit(['G', 'D', 'A', '#'], '#')).toBe('#');
	});
});
