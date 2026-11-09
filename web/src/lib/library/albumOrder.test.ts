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
	type Standing
} from './albumOrder';
import type { AlbumEntry } from './source.ts';
import type { AlbumOrder, Direction } from './sorts.ts';

function album(id: string, extra: Partial<AlbumEntry> = {}): AlbumEntry {
	return { id, title: id, artist: 'Someone', trackCount: 1, compilation: false, ...extra };
}

/** The sort `by` runs, the way it first runs unless told. */
function way(by: AlbumOrder, dir?: Direction): { by: AlbumOrder; dir: Direction } {
	return { by, dir: dir ?? (by === 'name' || by === 'artist' ? 'asc' : 'desc') };
}

function ids(
	albums: readonly AlbumEntry[],
	by: AlbumOrder,
	dir?: Direction,
	standing: Record<string, Standing> = {}
): string[] {
	return ordered(albums.map(keyed), way(by, dir), (id) => standing[id]).map(
		(entry) => entry.album.id
	);
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

	it('turns each order round, what is unknown still last and ties still by name', () => {
		const albums = [
			album('none'),
			album('b-old', { year: 1971, addedAt: '2026-01-01T00:00:00Z' }),
			album('a-old', { year: 1971, addedAt: '2026-01-01T00:00:00Z' }),
			album('new', { year: 2020, addedAt: '2026-06-01T00:00:00Z' })
		];
		expect(ids(albums, 'year', 'asc')).toEqual(['a-old', 'b-old', 'new', 'none']);
		expect(ids(albums, 'added', 'asc')).toEqual(['a-old', 'b-old', 'new', 'none']);
		expect(ids(albums, 'added', 'desc')).toEqual(['new', 'a-old', 'b-old', 'none']);
		expect(ids(albums, 'name', 'desc')).toEqual(['none', 'new', 'b-old', 'a-old']);
	});

	it('keeps albums of one name in ID order, either way', () => {
		const albums = [album('2', { title: 'Same' }), album('1', { title: 'Same' }), album('z')];
		expect(ids(albums, 'name', 'asc')).toEqual(['1', '2', 'z']);
		expect(ids(albums, 'name', 'desc')).toEqual(['z', '1', '2']);
	});

	it('turns the artist order round, an artist’s albums still by name', () => {
		const albums = [
			album('b', { sortArtist: 'Ann' }),
			album('a', { sortArtist: 'Ann' }),
			album('c', { sortArtist: 'Zed' })
		];
		expect(ids(albums, 'artist', 'desc')).toEqual(['c', 'a', 'b']);
	});

	it('sorts by plays, most first, an album nobody played as none', () => {
		const albums = [album('quiet'), album('loud'), album('some'), album('also')];
		const standing = { loud: { plays: 40 }, some: { plays: 3 }, also: { plays: 3 } };
		expect(ids(albums, 'plays', 'desc', standing)).toEqual(['loud', 'also', 'some', 'quiet']);
		expect(ids(albums, 'plays', 'asc', standing)).toEqual(['quiet', 'also', 'some', 'loud']);
	});

	it('sorts by rating, highest first, and an album nobody rated last either way', () => {
		const albums = [album('unrated'), album('good'), album('fine')];
		const standing = { good: { rating: 4.5 }, fine: { rating: 3 }, unrated: { plays: 9 } };
		expect(ids(albums, 'rating', 'desc', standing)).toEqual(['good', 'fine', 'unrated']);
		expect(ids(albums, 'rating', 'asc', standing)).toEqual(['fine', 'good', 'unrated']);
	});

	it('leaves the list it was given alone', () => {
		const albums = [album('b'), album('a')].map(keyed);
		ordered(albums, way('name'));
		expect(albums.map((entry) => entry.album.id)).toEqual(['b', 'a']);
	});
});

describe('marksOf', () => {
	const list = ordered(
		['Zed', 'The Apple', '1984', 'Ant', 'サカナ'].map((id) => keyed(album(id))),
		way('name')
	);

	it('reads letters off the list as shown, with nowhere for what it lacks', () => {
		const marks = marksOf(list, way('name'));
		expect(marks).toHaveLength(27);
		expect(marks.at(0)).toEqual({ label: '#', index: 0 });
		expect(marks.at(1)).toEqual({ label: 'A', index: 2 });
		expect(marks.at(2)).toEqual({ label: 'B', index: null });
		expect(marks.at(26)).toEqual({ label: 'Z', index: 4 });
	});

	it('reads the artist’s letter under the artist sort', () => {
		const albums = [album('x', { artist: 'The Kinks' })].map(keyed);
		expect(marksOf(albums, way('artist')).find(({ index }) => index === 0)?.label).toBe('K');
	});

	it('steps through decades by year, with what has none last', () => {
		const albums = [
			album('n'),
			album('a', { year: 1999 }),
			album('b', { year: 1991 }),
			album('c', { year: 2001 })
		];
		expect(marksOf(ordered(albums.map(keyed), way('year')), way('year'))).toEqual([
			{ label: '2000s', index: 0 },
			{ label: '1990s', index: 1 },
			{ label: '–', index: 3 }
		]);
	});

	it('turns the letters round with the names', () => {
		const turned = ordered(list, way('name', 'desc'));
		const marks = marksOf(turned, way('name', 'desc'));
		expect(marks.at(0)).toEqual({ label: 'Z', index: 0 });
		expect(marks.at(-1)).toEqual({ label: '#', index: 3 });
	});

	it('has nothing to say about time added, plays or ratings', () => {
		expect(marksOf(list, way('added'))).toEqual([]);
		expect(marksOf(list, way('plays'))).toEqual([]);
		expect(marksOf(list, way('rating'))).toEqual([]);
	});

	it('finds the mark a place comes under, past the gaps', () => {
		const marks = marksOf(list, way('name'));
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
		expect(read('')).toEqual({ genre: null });
		expect(albumsSearch({ genre: null })).toBe('');
	});

	it('round-trips a genre with an awkward name', () => {
		const query = { genre: 'Drum & Bass' } as const;
		expect(read(albumsSearch(query))).toEqual(query);
	});

	it('takes an empty genre as the plain list, and leaves the order to the person', () => {
		expect(read('?sort=year&genre=')).toEqual({ genre: null });
	});
});
