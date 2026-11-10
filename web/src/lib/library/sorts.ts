/**
 * The orders a person can put a list in, the way each runs, and what the menu calls them. Each list
 * keeps its choice as a pref (`library.sort`, `albums.sort`, `artists.sort`); `internal/prefs` says
 * the same orders, and the contract their shapes.
 */

/** Which way an order runs. What's unknown, such as a missing year, goes last either way. */
export type Direction = 'asc' | 'desc';

/** One order and the way it runs. */
export interface Sort<By extends string = string> {
	by: By;
	dir: Direction;
}

/** How the Library orders its artists. */
export type LibraryArtistOrder = 'name' | 'added' | 'plays';
/** How an artist's albums are ordered, on the Library and on its own page. */
export type ArtistAlbumOrder = 'year' | 'name' | 'added';
/** How the Albums page orders its albums. */
export type AlbumOrder = 'name' | 'artist' | 'year' | 'added' | 'plays' | 'rating';
/** How the Artists page orders its artists. */
export type ArtistOrder = 'name' | 'albums' | 'added' | 'plays';
/** How an artist page's full song list orders its songs, flat. */
export type ArtistSongOrder = 'top' | 'plays' | 'rating' | 'title' | 'year' | 'added';

/** How an artist page's full song list runs: flat in an order, or by album in `library.sort`'s. */
export interface ArtistSongs {
	group: 'flat' | 'album';
	sort: Sort<ArtistSongOrder>;
}

export interface LibrarySort {
	artists: Sort<LibraryArtistOrder>;
	albums: Sort<ArtistAlbumOrder>;
}

/** An order the menu offers: its name, and the way it runs when first picked. */
export interface SortOption<By extends string> {
	by: By;
	label: string;
	first: Direction;
}

const name = { label: 'Name', first: 'asc' } as const;
const added = { label: 'Recently added', first: 'desc' } as const;
const plays = { label: 'Most played', first: 'desc' } as const;

export const libraryArtistOrders: readonly SortOption<LibraryArtistOrder>[] = [
	{ by: 'name', ...name },
	{ by: 'added', ...added },
	{ by: 'plays', ...plays }
];

export const artistAlbumOrders: readonly SortOption<ArtistAlbumOrder>[] = [
	{ by: 'year', label: 'Year', first: 'desc' },
	{ by: 'name', ...name },
	{ by: 'added', ...added }
];

export const albumOrders: readonly SortOption<AlbumOrder>[] = [
	{ by: 'name', ...name },
	{ by: 'artist', label: 'Artist', first: 'asc' },
	{ by: 'year', label: 'Year', first: 'desc' },
	{ by: 'added', ...added },
	{ by: 'plays', ...plays },
	{ by: 'rating', label: 'Rating', first: 'desc' }
];

export const artistOrders: readonly SortOption<ArtistOrder>[] = [
	{ by: 'name', ...name },
	{ by: 'albums', label: 'Albums', first: 'desc' },
	{ by: 'added', ...added },
	{ by: 'plays', ...plays }
];

export const artistSongOrders: readonly SortOption<ArtistSongOrder>[] = [
	{ by: 'top', label: 'Top', first: 'desc' },
	{ by: 'plays', ...plays },
	{ by: 'rating', label: 'Rating', first: 'desc' },
	{ by: 'title', label: 'Title', first: 'asc' },
	{ by: 'year', label: 'Year', first: 'desc' },
	{ by: 'added', ...added }
];

/** Flat, best first: the order Top songs shows, so its rows stay where they are on expanding. */
export const defaultArtistSongs: ArtistSongs = { group: 'flat', sort: { by: 'top', dir: 'desc' } };

export const defaultLibrarySort: LibrarySort = {
	artists: { by: 'name', dir: 'asc' },
	albums: { by: 'year', dir: 'desc' }
};
export const defaultAlbumSort: Sort<AlbumOrder> = { by: 'name', dir: 'asc' };
export const defaultArtistSort: Sort<ArtistOrder> = { by: 'name', dir: 'asc' };

/** Picks an order: the one already chosen turns round, any other starts its own way. */
export function pick<By extends string>(
	options: readonly SortOption<By>[],
	current: Sort<By>,
	by: By
): Sort<By> {
	if (by === current.by) {
		return { by, dir: current.dir === 'asc' ? 'desc' : 'asc' };
	}
	return { by, dir: options.find((option) => option.by === by)?.first ?? 'asc' };
}

/** How each order says which way it runs, ascending then descending. */
const ways: Readonly<Record<string, readonly [string, string]>> = {
	name: ['A→Z', 'Z→A'],
	artist: ['A→Z', 'Z→A'],
	title: ['A→Z', 'Z→A'],
	top: ['BOTTOM FIRST', 'BEST FIRST'],
	year: ['OLDEST FIRST', 'NEWEST FIRST'],
	added: ['OLDEST FIRST', 'NEWEST FIRST'],
	plays: ['FEWEST FIRST', 'MOST FIRST'],
	albums: ['FEWEST FIRST', 'MOST FIRST'],
	rating: ['LOWEST FIRST', 'HIGHEST FIRST']
};

/** Words for the way an order runs, as the menu shows them. */
export function wayOf(by: string, dir: Direction): string {
	const [up, down] = ways[by] ?? ['ASCENDING', 'DESCENDING'];
	return dir === 'asc' ? up : down;
}

/** Whether a name order runs Z to A. */
export function turned(sort: Sort): boolean {
	return sort.dir === 'desc';
}

function known<By extends string>(
	options: readonly SortOption<By>[],
	saved: unknown
): Sort<By> | null {
	if (typeof saved !== 'object' || saved === null) {
		return null;
	}
	const { by, dir } = saved as Partial<Record<'by' | 'dir', unknown>>;
	const option = options.find((each) => each.by === by);
	return option !== undefined && (dir === 'asc' || dir === 'desc') ? { by: option.by, dir } : null;
}

/** A saved choice, or the default for anything a client doesn't know: a newer one may have saved it. */
export function savedLibrarySort(saved: unknown): LibrarySort {
	const parts = (typeof saved === 'object' && saved !== null ? saved : {}) as Partial<
		Record<'artists' | 'albums', unknown>
	>;
	return {
		artists: known(libraryArtistOrders, parts.artists) ?? defaultLibrarySort.artists,
		albums: known(artistAlbumOrders, parts.albums) ?? defaultLibrarySort.albums
	};
}

export function savedAlbumSort(saved: unknown): Sort<AlbumOrder> {
	return known(albumOrders, saved) ?? defaultAlbumSort;
}

export function savedArtistSort(saved: unknown): Sort<ArtistOrder> {
	return known(artistOrders, saved) ?? defaultArtistSort;
}

export function savedArtistSongs(saved: unknown): ArtistSongs {
	const parts = (typeof saved === 'object' && saved !== null ? saved : {}) as Partial<
		Record<'group' | 'sort', unknown>
	>;
	return {
		group: parts.group === 'album' ? 'album' : 'flat',
		sort: known(artistSongOrders, parts.sort) ?? defaultArtistSongs.sort
	};
}

/** Compares two numbers, `dir` deciding which comes first. */
export function byNumber(a: number, b: number, dir: Direction): number {
	return dir === 'asc' ? a - b : b - a;
}

/**
 * Compares two values that may be unknown: the unknown one goes last whichever way the order runs,
 * so turning an order round never brings albums with no year to the top.
 */
export function byKnown(a: number | undefined, b: number | undefined, dir: Direction): number {
	if (a === undefined || b === undefined) {
		return Number(a === undefined) - Number(b === undefined);
	}
	return byNumber(a, b, dir);
}

export function byText(a: string, b: string): number {
	return a < b ? -1 : Number(a > b);
}
