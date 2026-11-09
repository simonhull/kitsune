import { sortKey } from './albumOrder.ts';
import type { IndexField, LibraryAlbum, LibraryArtist, LibraryIndex } from './source.ts';
import {
	byKnown,
	byNumber,
	byText,
	defaultLibrarySort,
	type ArtistAlbumOrder,
	type LibraryArtistOrder,
	type LibrarySort,
	type Sort
} from './sorts.ts';

/**
 * Puts the Library's index in the order a person chose. The server sends it in the default order
 * (`orderIndex` in `internal/store`): artists by name, their albums newest first, compilations and
 * albums with no year after the rest. That order comes back as it is, the same object, so the
 * default costs nothing. Any other is worked out here, from what the client already holds.
 */
export function sortIndex(index: LibraryIndex, sort: LibrarySort): LibraryIndex {
	const artists = isDefault(sort.albums, defaultLibrarySort.albums)
		? index.artists
		: index.artists.map((artist) => ({
				...artist,
				albums: sortAlbums(artist.albums, sort.albums)
			}));
	const ordered = isDefault(sort.artists, defaultLibrarySort.artists)
		? artists
		: sortArtists(artists, sort.artists);
	return ordered === index.artists ? index : { artists: ordered };
}

function isDefault(sort: Sort, wanted: Sort): boolean {
	return sort.by === wanted.by && sort.dir === wanted.dir;
}

/** The index fields an order reads, beyond what the index always carries. */
export function sortFields(sort: LibrarySort): IndexField[] {
	const fields = new Set<IndexField>();
	if (sort.artists.by === 'added' || sort.albums.by === 'added') {
		fields.add('added');
	}
	if (sort.artists.by === 'plays') {
		fields.add('plays');
	}
	return [...fields];
}

/**
 * What each album sorts by, worked out once per album the index holds: a new index is new objects,
 * so nothing here goes stale, and a second change of order reads no date again.
 */
const albumKeys = new WeakMap<LibraryAlbum, { name: string; added: number | undefined }>();

function keysOf(album: LibraryAlbum): { name: string; added: number | undefined } {
	let keys = albumKeys.get(album);
	if (keys === undefined) {
		keys = { name: sortKey(album.title), added: firstAdded(album) };
		albumKeys.set(album, keys);
	}
	return keys;
}

/** When an album arrived: when its first song did. Undefined without the `added` field. */
export function albumAdded(album: LibraryAlbum): number | undefined {
	return keysOf(album).added;
}

function firstAdded(album: LibraryAlbum): number | undefined {
	let first: number | undefined;
	for (const { addedAt } of album.tracks) {
		const at = addedAt === undefined ? Number.NaN : Date.parse(addedAt);
		if (!Number.isNaN(at) && (first === undefined || at < first)) {
			first = at;
		}
	}
	return first;
}

/** How often an album's songs were played, by the person the index was read for. */
export function albumPlays(album: LibraryAlbum): number {
	return album.tracks.reduce((sum, { plays = 0 }) => sum + plays, 0);
}

interface KeyedAlbum {
	album: LibraryAlbum;
	name: string;
	value: number | undefined;
}

function albumValue(album: LibraryAlbum, by: ArtistAlbumOrder): number | undefined {
	switch (by) {
		case 'year':
			return album.year;
		case 'added':
			return albumAdded(album);
		case 'name':
			return undefined;
	}
}

/** An artist's albums in an order. Ties go by name, then ID. */
export function sortAlbums(
	albums: readonly LibraryAlbum[],
	{ by, dir }: Sort<ArtistAlbumOrder>
): LibraryAlbum[] {
	const keyed: KeyedAlbum[] = albums.map((album) => ({
		album,
		name: keysOf(album).name,
		value: albumValue(album, by)
	}));
	const byName = (a: KeyedAlbum, b: KeyedAlbum): number =>
		byText(a.name, b.name) || byText(a.album.id, b.album.id);
	const compare = (a: KeyedAlbum, b: KeyedAlbum): number => {
		switch (by) {
			case 'name':
				return (
					(dir === 'asc' ? byText(a.name, b.name) : byText(b.name, a.name)) ||
					byText(a.album.id, b.album.id)
				);
			case 'year':
				// Compilations go after the artist's own records, whichever way the years run.
				return (
					Number(a.album.compilation) - Number(b.album.compilation) ||
					byKnown(a.value, b.value, dir) ||
					byName(a, b)
				);
			case 'added':
				return byKnown(a.value, b.value, dir) || byName(a, b);
		}
	};
	return keyed.sort(compare).map(({ album }) => album);
}

/** What an artist sorts by, other than its name: its newest album's arrival, or its plays. */
function artistValue(
	artist: LibraryArtist,
	by: Exclude<LibraryArtistOrder, 'name'>
): number | undefined {
	if (by === 'plays') {
		return artist.albums.reduce((sum, album) => sum + albumPlays(album), 0);
	}
	let newest: number | undefined;
	for (const album of artist.albums) {
		const at = albumAdded(album);
		if (at !== undefined && (newest === undefined || at > newest)) {
			newest = at;
		}
	}
	return newest;
}

/** What makes two artists the same name to the order: their sort name, as NOCASE compares it. */
function nameKey({ sortName }: LibraryArtist): string {
	return sortName.replace(/[A-Z]/g, (letter) => letter.toLowerCase());
}

/**
 * The artists in an order. They arrive by name, ties by ID, so where each stands is its name's
 * rank: every other order breaks its ties by that. Z to A turns the names round, and keeps the
 * artists that share one in ID order.
 */
export function sortArtists(
	artists: readonly LibraryArtist[],
	{ by, dir }: Sort<LibraryArtistOrder>
): LibraryArtist[] {
	if (by === 'name') {
		return dir === 'asc' ? [...artists] : turnRuns(artists);
	}
	return artists
		.map((artist, rank) => ({ artist, rank, value: artistValue(artist, by) }))
		.sort((a, b) =>
			by === 'plays'
				? byNumber(a.value ?? 0, b.value ?? 0, dir) || a.rank - b.rank
				: byKnown(a.value, b.value, dir) || a.rank - b.rank
		)
		.map(({ artist }) => artist);
}

/** Turns the list round a name at a time: artists who share a name keep their order. */
function turnRuns(artists: readonly LibraryArtist[]): LibraryArtist[] {
	const runs: LibraryArtist[][] = [];
	let last: string | undefined;
	for (const artist of artists) {
		const key = nameKey(artist);
		if (key === last) {
			runs.at(-1)?.push(artist);
		} else {
			runs.push([artist]);
			last = key;
		}
	}
	return runs.reverse().flat();
}

/** Whether a sort keeps the artists in letters, so the rail has somewhere to jump. */
export function hasLetters(sort: LibrarySort): boolean {
	return sort.artists.by === 'name';
}

/** The rail's letters, turned round when the names are. */
export function railFor(letters: readonly string[], sort: LibrarySort): string[] {
	return sort.artists.dir === 'desc' ? letters.toReversed() : [...letters];
}

/**
 * Finds the artist a letter jumps to, on a list running Z to A: the letter's first, or the first of
 * the letter that comes next down the list, or the very last.
 */
export function turnedLetterTarget(
	letters: ReadonlyMap<string, number>,
	rail: readonly string[],
	letter: string,
	count: number
): number {
	const from = rail.indexOf(letter);
	const named = rail.slice(Math.max(from, 0)).map((each) => letters.get(each));
	return named.find((index) => index !== undefined) ?? count - 1;
}

/**
 * Finds the letter to light on a rail, either way round, for an artist under `letter`: it, or the
 * nearest the rail draws before it.
 */
export function railLit(rail: readonly string[], letter: string): string | undefined {
	const first = rail.at(0);
	const last = rail.at(-1);
	return first !== undefined && last !== undefined && first > last
		? turnedRailHere(rail, letter)
		: rail.findLast((each) => each <= letter);
}

/** Finds the letter to light on a turned rail for an artist under `letter`. */
export function turnedRailHere(rail: readonly string[], letter: string): string | undefined {
	return rail.findLast((each) => each >= letter);
}
