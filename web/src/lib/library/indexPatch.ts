import type { LibraryAlbum, LibraryArtist, LibraryIndex } from './source.ts';

/** What tells one heading from another: its artist, or for one with nobody to link to, its name. */
function headingKey({ id, name }: LibraryArtist): string {
	return id ?? `name:${name}`;
}

/**
 * Puts what a `library` event changed into an index, in place of what was there: the albums in
 * `ids` come out, and those `part` holds go in where each one's `after` says. A heading left with
 * no album goes. It returns null when `part` names an album to follow that the index doesn't have:
 * the index has fallen behind, and only reading it whole puts it right.
 */
export function patchIndex(
	index: LibraryIndex,
	ids: readonly string[],
	part: LibraryIndex
): LibraryIndex | null {
	const changed = new Set(ids);
	// A heading nothing changed in stays the same object, and so does every album that stays.
	let artists = index.artists
		.map((artist) =>
			artist.albums.some(({ id }) => changed.has(id))
				? { ...artist, albums: artist.albums.filter(({ id }) => !changed.has(id)) }
				: artist
		)
		.filter(({ albums }) => albums.length > 0);
	for (const heading of part.artists) {
		for (const album of heading.albums) {
			const placed = place(artists, heading, album);
			if (placed === null) {
				return null;
			}
			artists = placed;
		}
	}
	return { artists };
}

/** Puts one album right after the album its `after` names, under its own heading. */
function place(
	artists: LibraryArtist[],
	heading: LibraryArtist,
	{ after, ...album }: LibraryAlbum
): LibraryArtist[] | null {
	let at = -1;
	let position = 0;
	if (after !== undefined && after !== '') {
		at = artists.findIndex((artist) => artist.albums.some(({ id }) => id === after));
		if (at < 0) {
			return null;
		}
		position = (artists[at]?.albums.findIndex(({ id }) => id === after) ?? 0) + 1;
	}
	const key = headingKey(heading);
	const here = artists[at];
	const next = artists[at + 1];
	const out = [...artists];
	if (here !== undefined && headingKey(here) === key) {
		out[at] = { ...here, albums: here.albums.toSpliced(position, 0, album) };
	} else if (next !== undefined && headingKey(next) === key) {
		out[at + 1] = { ...next, albums: [album, ...next.albums] };
	} else {
		out.splice(at + 1, 0, { ...heading, albums: [album] });
	}
	return out;
}

/**
 * The part of a whole index a `library` event's albums make, as the server answers it: each album
 * in `ids` that's there, under its heading, with the album before it in the whole index as `after`.
 */
export function partOf(index: LibraryIndex, ids: readonly string[]): LibraryIndex {
	const wanted = new Set(ids);
	const artists: LibraryArtist[] = [];
	let before = '';
	for (const artist of index.artists) {
		const albums: LibraryAlbum[] = [];
		for (const album of artist.albums) {
			if (wanted.has(album.id)) {
				albums.push({ ...album, after: before });
			}
			before = album.id;
		}
		if (albums.length > 0) {
			artists.push({ ...artist, albums });
		}
	}
	return { artists };
}
