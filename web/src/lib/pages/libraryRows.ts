import { isMultiDisc } from '#lib/library/format.ts';
import type { LibraryAlbum, LibraryArtist, LibraryIndex } from '#lib/library/source.ts';

/** The heights a layout is built from: everything on the page has one known before it mounts. */
export interface RowMetrics {
	song: number;
	disc: number;
	/** What stands above an album's songs. Beside them, as at desktop, it's nothing. */
	albumHead: number;
	/** The least height of an album: its cover column can be taller than a short album's songs. */
	albumMin: number;
	/** What stands above an artist's albums. */
	artistHead: number;
	/** The least height of an artist: a long name needs room to reach where it pins. */
	artistMin: number;
	albumGap: number;
	artistGap: number;
}

export type Row =
	| { kind: 'disc'; top: number; height: number; albumIndex: number; disc: number }
	| { kind: 'song'; top: number; height: number; albumIndex: number; trackIndex: number };

export interface AlbumBlock {
	artistIndex: number;
	album: LibraryAlbum;
	top: number;
	height: number;
	firstRow: number;
	lastRow: number;
}

export interface ArtistBlock {
	artist: LibraryArtist;
	top: number;
	height: number;
}

export interface Layout {
	rows: Row[];
	albums: AlbumBlock[];
	artists: ArtistBlock[];
	/** Each letter's first artist, by its index. */
	letters: Map<string, number>;
	height: number;
}

/** Which rows are mounted, by index. Empty when `last` is below `first`. */
export interface RowWindow {
	first: number;
	last: number;
}

/** Works out where everything goes: one pass, so a library this size costs a few milliseconds. */
export function layOut(index: LibraryIndex, metrics: RowMetrics): Layout {
	const layout: Layout = { rows: [], albums: [], artists: [], letters: new Map(), height: 0 };
	let y = 0;
	for (const [artistIndex, artist] of index.artists.entries()) {
		const top = y;
		y += metrics.artistHead;
		for (const [position, album] of artist.albums.entries()) {
			y = layOutAlbum(
				layout,
				artistIndex,
				album,
				y + (position === 0 ? 0 : metrics.albumGap),
				metrics
			);
		}
		y = Math.max(y, top + metrics.artistMin);
		layout.artists.push({ artist, top, height: y - top });
		const letter = letterOf(artist.sortName);
		if (!layout.letters.has(letter)) {
			layout.letters.set(letter, artistIndex);
		}
		layout.height = y;
		y += metrics.artistGap;
	}
	return layout;
}

/** Adds one album at `top`, and returns where it ends. */
function layOutAlbum(
	layout: Layout,
	artistIndex: number,
	album: LibraryAlbum,
	top: number,
	metrics: RowMetrics
): number {
	const { rows, albums } = layout;
	const albumIndex = albums.length;
	const multiDisc = isMultiDisc(album.tracks);
	const firstRow = rows.length;
	let y = top + metrics.albumHead;
	let disc: number | null = null;
	for (const [trackIndex, track] of album.tracks.entries()) {
		const on = track.discNo ?? 1;
		if (multiDisc && on !== disc) {
			disc = on;
			rows.push({ kind: 'disc', top: y, height: metrics.disc, albumIndex, disc });
			y += metrics.disc;
		}
		rows.push({ kind: 'song', top: y, height: metrics.song, albumIndex, trackIndex });
		y += metrics.song;
	}
	const height = Math.max(y - top, metrics.albumMin);
	albums.push({ artistIndex, album, top, height, firstRow, lastRow: rows.length - 1 });
	return top + height;
}

/** Finds the last item that starts at or above `y`, or the first when `y` is above them all. */
function lastAbove(items: readonly { top: number }[], y: number): number {
	let low = 0;
	let high = items.length - 1;
	while (low < high) {
		const middle = Math.ceil((low + high) / 2);
		if ((items.at(middle)?.top ?? 0) <= y) {
			low = middle;
		} else {
			high = middle - 1;
		}
	}
	return low;
}

/** Finds the row at `y`, the one before it in a gap. */
export function rowAt(layout: Layout, y: number): number {
	return lastAbove(layout.rows, y);
}

/** Finds whose block `y` falls in, and the one before it in a gap. */
export function artistAt(layout: Layout, y: number): number {
	return lastAbove(layout.artists, y);
}

/** Names a song row by its track, which outlives the row: undefined for a disc label. */
export function rowKey({ rows, albums }: Layout, index: number): string | undefined {
	const row = rows.at(index);
	return row?.kind === 'song'
		? albums.at(row.albumIndex)?.album.tracks.at(row.trackIndex)?.id
		: undefined;
}

/**
 * Finds the rows to mount for a screen `height` tall at `y`: a screen either side of it. It holds
 * on to `mounted` until the screen comes within half a screen of its edge, so a scroll changes the
 * DOM now and then, not every frame. That's also why the answer can be `mounted` itself.
 */
export function windowAt(layout: Layout, y: number, height: number, mounted: RowWindow): RowWindow {
	const lastRow = layout.rows.length - 1;
	const near = rowAt(layout, y - height / 2);
	const far = rowAt(layout, y + height * 1.5);
	if (mounted.last >= mounted.first && near >= mounted.first && far <= mounted.last) {
		return mounted;
	}
	return {
		first: rowAt(layout, y - height),
		last: Math.min(lastRow, rowAt(layout, y + height * 2))
	};
}

/** The letter an artist files under: its first, with any accent off, or `#` for anything else. */
export function letterOf(name: string): string {
	const first = String.fromCodePoint(name.codePointAt(0) ?? 0).normalize('NFD');
	return /^[a-z]/i.test(first) ? first.charAt(0).toUpperCase() : '#';
}

const alphabet = [...'ABCDEFGHIJKLMNOPQRSTUVWXYZ'];
/** The phone's rail, off its board: 44 px targets leave room for about every other letter. */
const everyOther = [...'ADGJLNPRTVXZ'];

/** The letters on the rail. `#` is there only for a library with something to find under it. */
export function railLetters({ letters }: Layout, stacked: boolean): string[] {
	return [...(letters.has('#') ? ['#'] : []), ...(stacked ? everyOther : alphabet)];
}

/** Finds the artist a letter jumps to: its first, or the first after it, or the very last. */
export function letterTarget({ letters, artists }: Layout, letter: string): number {
	const from = alphabet.indexOf(letter);
	const named = [letter, ...alphabet.slice(from + 1)].map((each) => letters.get(each));
	return named.find((index) => index !== undefined) ?? artists.length - 1;
}

/** Finds the letter to light on the rail for an artist under `letter`, on a rail that may skip it. */
export function railHere(rail: readonly string[], letter: string): string | undefined {
	return rail.findLast((each) => each <= letter);
}
