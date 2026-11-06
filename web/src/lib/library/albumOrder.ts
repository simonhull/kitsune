import type { AlbumEntry } from './source.ts';

/** What the Albums page sorts by. */
export type AlbumOrder = 'name' | 'artist' | 'year' | 'added';

export const albumOrders: readonly { order: AlbumOrder; label: string }[] = [
	{ order: 'name', label: 'Name' },
	{ order: 'artist', label: 'Artist' },
	{ order: 'year', label: 'Year' },
	{ order: 'added', label: 'Added' }
];

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

function byText(a: string, b: string): number {
	return a < b ? -1 : Number(a > b);
}

function byName(a: Keyed, b: Keyed): number {
	return byText(a.name, b.name) || byText(a.album.id, b.album.id);
}

/** Newest first. An album with no year goes last, not missing. */
function byYear(a: Keyed, b: Keyed): number {
	return (b.album.year ?? 0) - (a.album.year ?? 0) || byName(a, b);
}

const comparers: Readonly<Record<AlbumOrder, (a: Keyed, b: Keyed) => number>> = {
	name: byName,
	artist: (a, b) => byText(a.artist, b.artist) || byName(a, b),
	year: byYear,
	added: (a, b) => b.added - a.added || byText(b.album.id, a.album.id)
};

export function ordered(albums: readonly Keyed[], order: AlbumOrder): Keyed[] {
	return albums.toSorted(comparers[order]);
}

/** A place to jump to on the rail. `index` is where it starts in the list, and null is nowhere. */
export interface Mark {
	label: string;
	index: number | null;
}

const letters = ['#', ...'ABCDEFGHIJKLMNOPQRSTUVWXYZ'];

function letterMarks(keys: readonly string[]): Mark[] {
	const firsts = new Map<string, number>();
	keys.forEach((key, index) => {
		const letter = key.charAt(0).toUpperCase();
		if (!firsts.has(letter)) {
			firsts.set(letter, index);
		}
	});
	return letters.map((label) => ({ label, index: firsts.get(label) ?? null }));
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

/** Reads the rail off the list as shown. Time added has no steps worth naming, so it has none. */
export function marksOf(albums: readonly Keyed[], order: AlbumOrder): Mark[] {
	switch (order) {
		case 'name':
			return letterMarks(albums.map(({ name }) => name));
		case 'artist':
			return letterMarks(albums.map(({ artist }) => artist));
		case 'year':
			return decadeMarks(albums);
		case 'added':
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

/** What an Albums page's URL asks for. */
export interface AlbumsQuery {
	order: AlbumOrder;
	genre: string | null;
}

/** Reads a query. Anything it doesn't know falls back to the plain list. */
export function albumsQueryIn({
	searchParams
}: {
	searchParams: Pick<URLSearchParams, 'get'>;
}): AlbumsQuery {
	const asked = searchParams.get('sort');
	const known = albumOrders.find(({ order }) => order === asked);
	return { order: known?.order ?? 'name', genre: searchParams.get('genre') || null };
}

/** The search part of the URL for a query. The plain list has none. */
export function albumsSearch({ order, genre }: AlbumsQuery): string {
	const params = new URLSearchParams();
	if (order !== 'name') {
		params.set('sort', order);
	}
	if (genre !== null) {
		params.set('genre', genre);
	}
	const search = params.toString();
	return search === '' ? '' : `?${search}`;
}
