import type { AlbumEntry } from './source.ts';
import {
	byKnown,
	byNumber,
	byText,
	type AlbumOrder,
	type ArtistAlbumOrder,
	type Sort
} from './sorts.ts';

/** Letters that Unicode won't take apart. `store/fold.go` keeps the same list for search. */
const whole: Readonly<Record<string, string>> = {
	ø: 'o',
	ł: 'l',
	đ: 'd',
	ð: 'd',
	ħ: 'h',
	ı: 'i',
	æ: 'ae',
	œ: 'oe',
	ß: 'ss',
	þ: 'th'
};

/**
 * Makes a name comparable: folded to plain letters, past any leading punctuation and article, so
 * T isn't a fifth of the list. Whatever doesn't start with A to Z is marked to sort first, which
 * puts the `#` bucket in one piece at the top.
 */
export function sortKey(name: string): string {
	const folded = name
		.normalize('NFKD')
		.replace(/\p{M}+/gu, '')
		.toLowerCase()
		.replace(/[øłđðħıæœßþ]/g, (letter) => whole[letter] ?? letter);
	const word = folded.replace(/^[^\p{L}\p{N}]+/u, '').replace(/^(?:the|an?)\s+(?=\S)/, '');
	return /^[a-z]/.test(word) ? word : `#${word}`;
}

/** An album with everything it sorts by worked out, once for all the sorts. */
export interface Keyed {
	album: AlbumEntry;
	name: string;
	artist: string;
	added: number;
}

export function keyed(album: AlbumEntry): Keyed {
	return {
		album,
		name: sortKey(album.sortName ?? album.title),
		artist: sortKey(album.sortArtist ?? album.artist),
		added: Date.parse(album.addedAt ?? '') || 0
	};
}

function byName(a: Keyed, b: Keyed): number {
	return byText(a.name, b.name) || byText(a.album.id, b.album.id);
}

/** What a person thinks of an album: how often they played it, and its stars. Either may be unknown. */
export interface Standing {
	plays?: number;
	/** The mean of its rated songs; absent when none is. */
	rating?: number;
}

export type StandingOf = (albumId: string) => Standing | undefined;

const nothing: StandingOf = () => undefined;

/**
 * Puts albums in an order. Ties go by name, then ID, so an order is the same every time; an
 * album with no year, no time added or no rating goes last whichever way the order runs.
 */
export function ordered(
	albums: readonly Keyed[],
	{ by, dir }: Sort<AlbumOrder>,
	standing: StandingOf = nothing
): Keyed[] {
	const compare = (a: Keyed, b: Keyed): number => {
		switch (by) {
			case 'name':
				return (
					(dir === 'asc' ? byText(a.name, b.name) : byText(b.name, a.name)) ||
					byText(a.album.id, b.album.id)
				);
			case 'artist':
				return (
					(dir === 'asc' ? byText(a.artist, b.artist) : byText(b.artist, a.artist)) || byName(a, b)
				);
			case 'year':
				return byKnown(a.album.year, b.album.year, dir) || byName(a, b);
			case 'added':
				return byKnown(a.added || undefined, b.added || undefined, dir) || byName(a, b);
			case 'plays':
				return (
					byNumber(standing(a.album.id)?.plays ?? 0, standing(b.album.id)?.plays ?? 0, dir) ||
					byName(a, b)
				);
			case 'rating':
				return (
					byKnown(standing(a.album.id)?.rating, standing(b.album.id)?.rating, dir) || byName(a, b)
				);
		}
	};
	return albums.toSorted(compare);
}

/**
 * An artist's albums in the order the Library has them (`library.sort`'s albums): by year, its own
 * records before compilations whichever way the years run.
 */
export function artistAlbums(
	albums: readonly AlbumEntry[],
	sort: Sort<ArtistAlbumOrder>
): AlbumEntry[] {
	const list = ordered(albums.map(keyed), sort);
	if (sort.by === 'year') {
		list.sort((a, b) => Number(a.album.compilation) - Number(b.album.compilation));
	}
	return list.map(({ album }) => album);
}

/** A place to jump to on the rail. `index` is where it starts in the list, and null is nowhere. */
export interface Mark {
	label: string;
	index: number | null;
}

const letters = ['#', ...'ABCDEFGHIJKLMNOPQRSTUVWXYZ'];

/** The letters in the list's order: Z to A when the names run that way. */
function letterMarks(keys: readonly string[], dir: Sort['dir']): Mark[] {
	const firsts = new Map<string, number>();
	keys.forEach((key, index) => {
		const letter = key.charAt(0).toUpperCase();
		if (!firsts.has(letter)) {
			firsts.set(letter, index);
		}
	});
	return (dir === 'asc' ? letters : letters.toReversed()).map((label) => ({
		label,
		index: firsts.get(label) ?? null
	}));
}

function decadeOf({ album: { year } }: Keyed): string {
	return year === undefined ? '–' : `${String(year - (year % 10))}s`;
}

function decadeMarks(albums: readonly Keyed[]): Mark[] {
	const marks: Mark[] = [];
	albums.forEach((album, index) => {
		const label = decadeOf(album);
		if (marks.at(-1)?.label !== label) {
			marks.push({ label, index });
		}
	});
	return marks;
}

/**
 * Reads the rail off the list as shown. Time added, plays and ratings have no steps worth naming,
 * so they have none.
 */
export function marksOf(albums: readonly Keyed[], { by, dir }: Sort<AlbumOrder>): Mark[] {
	switch (by) {
		case 'name':
			return letterMarks(
				albums.map(({ name }) => name),
				dir
			);
		case 'artist':
			return letterMarks(
				albums.map(({ artist }) => artist),
				dir
			);
		case 'year':
			return decadeMarks(albums);
		case 'added':
		case 'plays':
		case 'rating':
			return [];
	}
}

/** Finds the mark a place in the list comes under. */
export function markAt(marks: readonly Mark[], index: number): Mark | undefined {
	return marks.findLast((mark) => mark.index !== null && mark.index <= index);
}

/** A genre and how many albums show it. */
export interface GenreCount {
	name: string;
	albums: number;
}

/** Counts every genre on these albums, the most used first. */
export function genreCounts(albums: readonly AlbumEntry[]): GenreCount[] {
	const counts = new Map<string, number>();
	for (const { displayGenres = [] } of albums) {
		for (const genre of displayGenres) {
			counts.set(genre, (counts.get(genre) ?? 0) + 1);
		}
	}
	return [...counts]
		.map(([name, count]) => ({ name, albums: count }))
		.toSorted((a, b) => b.albums - a.albums || byText(a.name, b.name));
}

/** Reports whether two genres are the same one. A hand-typed URL may not match the case. */
export function sameGenre(a: string, b: string): boolean {
	return a.toLowerCase() === b.toLowerCase();
}

export function withGenre(albums: readonly Keyed[], genre: string): Keyed[] {
	return albums.filter(({ album }) => album.displayGenres?.some((g) => sameGenre(g, genre)));
}

/**
 * What an Albums page's URL asks for: a genre. The order is the person's, kept as `albums.sort`,
 * so it follows them to other devices rather than living in a link.
 */
export interface AlbumsQuery {
	genre: string | null;
}

/** Reads a query. Anything it doesn't know falls back to the plain list. */
export function albumsQueryIn({
	searchParams
}: {
	searchParams: Pick<URLSearchParams, 'get'>;
}): AlbumsQuery {
	return { genre: searchParams.get('genre') || null };
}

/** The search part of the URL for a query. The plain list has none. */
export function albumsSearch({ genre }: AlbumsQuery): string {
	const params = new URLSearchParams();
	if (genre !== null) {
		params.set('genre', genre);
	}
	const search = params.toString();
	return search === '' ? '' : `?${search}`;
}
