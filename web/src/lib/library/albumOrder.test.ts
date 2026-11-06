import { describe, expect, it } from 'vitest';
import {
	albumsQueryIn,
	albumsSearch,
	genreCounts,
	keyed,
	markAt,
	marksOf,
	ordered,
	sameGenre,
	sortKey,
	withGenre,
	type AlbumOrder
} from './albumOrder';
import type { AlbumEntry } from './source.ts';

function album(id: string, extra: Partial<AlbumEntry> = {}): AlbumEntry {
	return { id, title: id, artist: 'Someone', trackCount: 1, compilation: false, ...extra };
}

function ids(albums: readonly AlbumEntry[], order: AlbumOrder): string[] {
	return ordered(albums.map(keyed), order).map((entry) => entry.album.id);
}

describe('sortKey', () => {
	it.each([
		['The Wall', 'wall'],
		['the wall', 'wall'],
		['A Love Supreme', 'love supreme'],
		['An Awesome Wave', 'awesome wave'],
		['Théâtre', 'theatre'],
		['Ænima', 'aenima'],
		['Øst', 'ost'],
		['...And Justice', 'and justice'],
		['(What’s the Story)', 'what’s the story)']
	])('files %s under its words', (name, key) => {
		expect(sortKey(name)).toBe(key);
	});

	it('keeps an article that is the whole name, or part of a word', () => {
		expect(sortKey('The')).toBe('the');
		expect(sortKey('A')).toBe('a');
		expect(sortKey('A-ha')).toBe('a-ha');
		expect(sortKey('Theory')).toBe('theory');
	});

	it('marks what has no letter from A to Z, to sort first', () => {
		expect(sortKey('1984')).toBe('#1984');
		expect(sortKey('サカナ')).toBe('#サカナ');
		expect(sortKey('...')).toBe('#');
		expect(sortKey('')).toBe('#');
		expect([sortKey('Zoo'), sortKey('サカナ'), sortKey('1984')].toSorted().at(-1)).toBe('zoo');
	});
});

describe('ordered', () => {
	it('sorts by name past articles, from the sort tag when there is one', () => {
		const albums = [
			album('z'),
			album('The Middle'),
			album('Bad', { sortName: 'Tagged Late' }),
			album('Édith'),
			album('1999')
		];
		expect(ids(albums, 'name')).toEqual(['1999', 'Édith', 'The Middle', 'Bad', 'z']);
	});

	it('sorts by who owns the album, then by name', () => {
		const albums = [
			album('b', { artist: 'The Beatles', sortArtist: 'Beatles, The' }),
			album('a', { artist: 'The Beatles', sortArtist: 'Beatles, The' }),
			album('c', { artist: 'Aretha Franklin', sortArtist: 'Franklin, Aretha' }),
			album('d', { artist: 'Various' })
		];
		expect(ids(albums, 'artist')).toEqual(['a', 'b', 'c', 'd']);
	});

	it('sorts by album year, newest first, and never loses an album with none', () => {
		const albums = [album('none'), album('old', { year: 1971 }), album('new', { year: 2020 })];
		expect(ids(albums, 'year')).toEqual(['new', 'old', 'none']);
	});

	it('sorts by when an album was added, whatever the precision of the time', () => {
		const albums = [
			album('first', { addedAt: '2026-11-05T12:00:00Z' }),
			album('second', { addedAt: '2026-11-05T12:00:00.5Z' }),
			album('unknown')
		];
		expect(ids(albums, 'added')).toEqual(['second', 'first', 'unknown']);
	});

	it('leaves the list it was given alone', () => {
		const albums = [album('b'), album('a')].map(keyed);
		ordered(albums, 'name');
		expect(albums.map((entry) => entry.album.id)).toEqual(['b', 'a']);
	});
});

describe('marksOf', () => {
	const list = ordered(
		['Zed', 'The Apple', '1984', 'Ant', 'サカナ'].map((id) => keyed(album(id))),
		'name'
	);

	it('reads letters off the list as shown, with nowhere for what it lacks', () => {
		const marks = marksOf(list, 'name');
		expect(marks).toHaveLength(27);
		expect(marks.at(0)).toEqual({ label: '#', index: 0 });
		expect(marks.at(1)).toEqual({ label: 'A', index: 2 });
		expect(marks.at(2)).toEqual({ label: 'B', index: null });
		expect(marks.at(26)).toEqual({ label: 'Z', index: 4 });
	});

	it('reads the artist’s letter under the artist sort', () => {
		const albums = [album('x', { artist: 'The Kinks' })].map(keyed);
		expect(marksOf(albums, 'artist').find(({ index }) => index === 0)?.label).toBe('K');
	});

	it('steps through decades by year, with what has none last', () => {
		const albums = [
			album('n'),
			album('a', { year: 1999 }),
			album('b', { year: 1991 }),
			album('c', { year: 2001 })
		];
		expect(marksOf(ordered(albums.map(keyed), 'year'), 'year')).toEqual([
			{ label: '2000s', index: 0 },
			{ label: '1990s', index: 1 },
			{ label: '–', index: 3 }
		]);
	});

	it('has nothing to say about time added', () => {
		expect(marksOf(list, 'added')).toEqual([]);
	});

	it('finds the mark a place comes under, past the gaps', () => {
		const marks = marksOf(list, 'name');
		expect(markAt(marks, 0)?.label).toBe('#');
		expect(markAt(marks, 3)?.label).toBe('A');
		expect(markAt(marks, 4)?.label).toBe('Z');
		expect(markAt([], 4)).toBeUndefined();
	});
});

describe('genres', () => {
	const albums = [
		album('a', { displayGenres: ['Rock', 'Pop'] }),
		album('b', { displayGenres: ['Rock'] }),
		album('c', { displayGenres: ['Jazz'] }),
		album('d', { displayGenres: [] }),
		album('e')
	];

	it('counts albums, most used first, then by name', () => {
		expect(genreCounts(albums)).toEqual([
			{ name: 'Rock', albums: 2 },
			{ name: 'Jazz', albums: 1 },
			{ name: 'Pop', albums: 1 }
		]);
	});

	it('filters to albums that show a genre, whatever the case of the URL', () => {
		const found = withGenre(albums.map(keyed), 'rock');
		expect(found.map((entry) => entry.album.id)).toEqual(['a', 'b']);
		expect(sameGenre('Électronique', 'électronique')).toBe(true);
	});
});

describe('the Albums URL', () => {
	const read = (search: string) => albumsQueryIn(new URL(`http://x/albums${search}`));

	it('leaves the plain list plain', () => {
		expect(read('')).toEqual({ order: 'name', genre: null });
		expect(albumsSearch({ order: 'name', genre: null })).toBe('');
	});

	it('round-trips a sort and a genre with an awkward name', () => {
		const query = { order: 'year', genre: 'Drum & Bass' } as const;
		expect(read(albumsSearch(query))).toEqual(query);
	});

	it('takes a sort it doesn’t know, or an empty genre, as the plain list', () => {
		expect(read('?sort=mood&genre=')).toEqual({ order: 'name', genre: null });
	});
});
