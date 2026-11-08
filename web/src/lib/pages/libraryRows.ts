import { isMultiDisc } from '#lib/library/format.ts';
import type {
	LibraryAlbum,
	LibraryArtist,
	LibraryIndex,
	LibraryTrack,
	MarkKind
} from '#lib/library/source.ts';

/** The heights a layout is built from: everything on the page has one known before it mounts. */
export interface RowMetrics {
	song: number;
	/** A disc's heading at the top of its album's songs, and one between two discs, with room above. */
	firstDisc: number;
	nextDisc: number;
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
	| {
			kind: 'disc';
			top: number;
			height: number;
			albumIndex: number;
			disc: number;
			/** Whether it heads the album's songs on show: then it has no disc above it. */
			first: boolean;
	  }
	| { kind: 'song'; top: number; height: number; albumIndex: number; trackIndex: number };

export interface AlbumBlock {
	/** Its artist in the layout's {@link Layout.artists}. */
	artistIndex: number;
	album: LibraryAlbum;
	top: number;
	height: number;
	firstRow: number;
	lastRow: number;
	/** How many of its songs show. */
	shown: number;
}

export interface ArtistBlock {
	artist: LibraryArtist;
	/** Where it stands in the whole index, shown or not. */
	source: number;
	top: number;
	height: number;
	/** How many of its albums show, and of their songs. */
	albums: number;
	songs: number;
}

export interface Layout {
	rows: Row[];
	albums: AlbumBlock[];
	artists: ArtistBlock[];
	/** Each letter's first artist, by its index. */
	letters: Map<string, number>;
	height: number;
	/** The songs it shows, and the songs there are. */
	songs: number;
	totalSongs: number;
}

/** What the Library list shows: everything, or what a filter keeps of it. */
export type LibraryFilter = 'everything' | 'hearted' | 'rated';

/** Whether a filter shows a song, given its album and artist. */
export type Keep = (artist: LibraryArtist, album: LibraryAlbum, track: LibraryTrack) => boolean;

/** As much of the marks as a filter reads. */
export interface MarksView {
	hearted(kind: MarkKind, id: string): boolean;
	stars(trackId: string): number | null;
}

/** The least a song is rated to show under 4 and up. */
export const goodStars = 4;

/**
 * Says what a filter keeps, or undefined to keep everything. A hearted artist keeps all its albums
 * and a hearted album all its songs; otherwise only a hearted song shows. 4 and up looks at the song
 * alone: an album's mean never shows a song nobody rated.
 */
export function keepFor(filter: LibraryFilter, marks: MarksView): Keep | undefined {
	switch (filter) {
		case 'everything':
			return undefined;
		case 'hearted':
			return (artist, album, track) =>
				(artist.id !== undefined && marks.hearted('artists', artist.id)) ||
				marks.hearted('albums', album.id) ||
				marks.hearted('tracks', track.id);
		case 'rated':
			return (_artist, _album, track) => (marks.stars(track.id) ?? 0) >= goodStars;
	}
}

/** Which rows are mounted, by index. Empty when `last` is below `first`. */
export interface RowWindow {
	first: number;
	last: number;
}

/**
 * Works out where everything goes, or everything `keep` shows: one pass, so a library this size
 * costs a few milliseconds. An album with no song kept is left out, and so is an artist with none.
 */
export function layOut(index: LibraryIndex, metrics: RowMetrics, keep?: Keep): Layout {
	const layout: Layout = {
		rows: [],
		albums: [],
		artists: [],
		letters: new Map(),
		height: 0,
		songs: 0,
		totalSongs: 0
	};
	let y = 0;
	for (const [source, artist] of index.artists.entries()) {
		const artistIndex = layout.artists.length;
		const top = y;
		let end = top + metrics.artistHead;
		let albums = 0;
		const songs = layout.songs;
		for (const album of artist.albums) {
			layout.totalSongs += album.tracks.length;
			const shown =
				keep === undefined
					? album.tracks
					: album.tracks.filter((track) => keep(artist, album, track));
			if (shown.length > 0) {
				end = layOutAlbum(
					layout,
					artistIndex,
					album,
					shown,
					end + (albums === 0 ? 0 : metrics.albumGap),
					metrics
				);
				albums += 1;
			}
		}
		if (albums === 0) {
			continue;
		}
		y = Math.max(end, top + metrics.artistMin);
		layout.artists.push({
			artist,
			source,
			top,
			height: y - top,
			albums,
			songs: layout.songs - songs
		});
		const letter = letterOf(artist.sortName);
		if (!layout.letters.has(letter)) {
			layout.letters.set(letter, artistIndex);
		}
		layout.height = y;
		y += metrics.artistGap;
	}
	return layout;
}

/** Adds the songs of an album it shows at `top`, and returns where it ends. */
function layOutAlbum(
	layout: Layout,
	artistIndex: number,
	album: LibraryAlbum,
	shown: readonly LibraryTrack[],
	top: number,
	metrics: RowMetrics
): number {
	const { rows, albums } = layout;
	const albumIndex = albums.length;
	const multiDisc = isMultiDisc(album.tracks);
	const firstRow = rows.length;
	// Everything shows unless a filter left some out: then it says which.
	const kept = shown === album.tracks ? null : new Set(shown);
	let y = top + metrics.albumHead;
	let disc: number | null = null;
	for (const [trackIndex, track] of album.tracks.entries()) {
		if (kept?.has(track) === false) {
			continue;
		}
		const on = track.discNo ?? 1;
		if (multiDisc && on !== disc) {
			const first = disc === null;
			const height = first ? metrics.firstDisc : metrics.nextDisc;
			disc = on;
			rows.push({ kind: 'disc', top: y, height, albumIndex, disc, first });
			y += height;
		}
		rows.push({ kind: 'song', top: y, height: metrics.song, albumIndex, trackIndex });
		y += metrics.song;
	}
	const height = Math.max(y - top, metrics.albumMin);
	albums.push({
		artistIndex,
		album,
		top,
		height,
		firstRow,
		lastRow: rows.length - 1,
		shown: shown.length
	});
	layout.songs += shown.length;
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
	// A window past the end is from a longer list, before a filter: it holds rows this one hasn't.
	const held = mounted.last >= mounted.first && mounted.last <= lastRow;
	if (held && near >= mounted.first && far <= mounted.last) {
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
