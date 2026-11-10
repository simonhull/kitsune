/**
 * An artist page's songs: what Top ranks them by, the orders the full list offers, and what plays
 * from a press. Pure: the page holds the index (`GET /library/index?artist=`) and the marks.
 */
import { sortAlbums } from '#lib/library/libraryOrder.ts';
import type {
	LibraryAlbum,
	LibraryArtist,
	LibraryIndex,
	LibraryTrack
} from '#lib/library/source.ts';
import {
	byKnown,
	byNumber,
	byText,
	type ArtistAlbumOrder,
	type ArtistSongOrder,
	type Sort
} from '#lib/library/sorts.ts';
import { sortKey } from '#lib/library/albumOrder.ts';
import {
	layOut,
	type Layout,
	type MarksView,
	type RowMetrics,
	type RowWindow
} from './libraryRows.ts';

/** A song of the list, with its album, whether the album is the artist's own, and its place. */
export interface ArtistSong {
	track: LibraryTrack;
	album: LibraryAlbum;
	own: boolean;
	/**
	 * Where it stands in the server's order: the artist's own albums, newest first, then those it
	 * appears on, each in disc and track order. Every tie in every order falls back to this.
	 */
	at: number;
}

/** Every song of an artist's index, in the server's order. `owned` names the albums it owns. */
export function songsOf(index: LibraryIndex, owned: ReadonlySet<string>): ArtistSong[] {
	const songs: ArtistSong[] = [];
	for (const artist of index.artists) {
		for (const album of artist.albums) {
			const own = owned.has(album.id);
			for (const track of album.tracks) {
				songs.push({ track, album, own, at: songs.length });
			}
		}
	}
	return songs;
}

/** How much a song's stars count toward Top: 5 doubles it, unrated counts as 3, and 1 quarters it. */
export function starWeight(stars: number | null): number {
	switch (stars) {
		case 5:
			return 2;
		case 4:
			return 1.5;
		case 2:
			return 0.5;
		case 1:
			return 0.25;
		default:
			return 1;
	}
}

/** How much a heart counts toward Top. */
export const heartWeight = 1.25;

/**
 * A song's place in Top: `(plays + 1) × w(stars) × 1.25 if hearted`, from the person's own plays
 * and marks only. A song nobody has played, rated or hearted scores 1.
 */
export function topScore(track: LibraryTrack, marks: MarksView): number {
	const plays = track.plays ?? 0;
	const hearted = marks.hearted('tracks', track.id);
	return (plays + 1) * starWeight(marks.stars(track.id)) * (hearted ? heartWeight : 1);
}

/** Whether nothing has been played, rated or hearted, so a ranking would only be noise. */
export function coldStart(songs: readonly ArtistSong[], marks: MarksView): boolean {
	return songs.every(({ track }) => topScore(track, marks) === 1);
}

/** Top: best first, ties in the server's order. Cold, that is the newest album's songs first. */
export function rankTop(songs: readonly ArtistSong[], marks: MarksView): ArtistSong[] {
	return flatOrder(songs, { by: 'top', dir: 'desc' }, marks);
}

/** When a song arrived, from the index's `added` field. */
function addedOf({ track }: ArtistSong): number | undefined {
	const at = track.addedAt === undefined ? Number.NaN : Date.parse(track.addedAt);
	return Number.isNaN(at) ? undefined : at;
}

/** The songs flat in an order: each turns round with `dir`, and ties go in the server's order. */
export function flatOrder(
	songs: readonly ArtistSong[],
	{ by, dir }: Sort<ArtistSongOrder>,
	marks: MarksView
): ArtistSong[] {
	const keyed = songs.map((song) => ({ song, key: keyOf(song, by, marks) }));
	const compare = (a: (typeof keyed)[number], b: (typeof keyed)[number]): number => {
		const first = (() => {
			if (typeof a.key === 'string' && typeof b.key === 'string') {
				return dir === 'asc' ? byText(a.key, b.key) : byText(b.key, a.key);
			}
			const [x, y] = [a.key as number | undefined, b.key as number | undefined];
			return by === 'year' || by === 'added' ? byKnown(x, y, dir) : byNumber(x ?? 0, y ?? 0, dir);
		})();
		return first || secondOf(a.song, b.song, by) || a.song.at - b.song.at;
	};
	return keyed.sort(compare).map(({ song }) => song);
}

function keyOf(
	song: ArtistSong,
	by: ArtistSongOrder,
	marks: MarksView
): number | string | undefined {
	switch (by) {
		case 'top':
			return topScore(song.track, marks);
		case 'plays':
			return song.track.plays ?? 0;
		case 'rating':
			return marks.stars(song.track.id) ?? 0;
		case 'title':
			return sortKey(song.track.title);
		case 'year':
			return song.track.year ?? song.album.year;
		case 'added':
			return addedOf(song);
	}
}

/** Rating's ties go to the song played more, as a person would rank two of the same stars. */
function secondOf(a: ArtistSong, b: ArtistSong, by: ArtistSongOrder): number {
	return by === 'rating' ? (b.track.plays ?? 0) - (a.track.plays ?? 0) : 0;
}

/**
 * The albums by album, in `library.sort`'s order for an artist's albums: the artist's own first,
 * then those it appears on, as the shelves have them.
 */
export function albumOrder(
	index: LibraryIndex,
	owned: ReadonlySet<string>,
	sort: Sort<ArtistAlbumOrder>
): { own: LibraryAlbum[]; appears: LibraryAlbum[] } {
	const albums = index.artists.flatMap((artist) => artist.albums);
	return {
		own: sortAlbums(
			albums.filter(({ id }) => owned.has(id)),
			sort
		),
		appears: sortAlbums(
			albums.filter(({ id }) => !owned.has(id)),
			sort
		)
	};
}

/** The songs in the order By album lists them. */
export function albumSongs(
	albums: { own: readonly LibraryAlbum[]; appears: readonly LibraryAlbum[] },
	songs: readonly ArtistSong[]
): ArtistSong[] {
	const byTrack = new Map(songs.map((song) => [song.track.id, song]));
	return [...albums.own, ...albums.appears].flatMap((album) =>
		album.tracks.flatMap((track) => byTrack.get(track.id) ?? [])
	);
}

/** What a press on a song plays: it, then the rest of the list as shown, to the end. */
export function queueFrom(list: readonly ArtistSong[], trackId: string): ArtistSong[] {
	const at = list.findIndex(({ track }) => track.id === trackId);
	return at < 0 ? [] : list.slice(at);
}

/** How many songs Top shows: ten, five on a phone. */
export function topCount(phone: boolean): number {
	return phone ? 5 : 10;
}

/** What the section's heading says, from how many songs there are and whether any was played. */
export type Heading = 'top' | 'songs' | 'cold' | 'all';

/**
 * The heading: Top songs, or for an artist with no more songs than Top shows, or with nothing
 * played, rated or hearted, just Songs (with NOTHING PLAYED YET · NEWEST FIRST for the second); the
 * full list says All songs.
 */
export function headingOf(expanded: boolean, count: number, shown: number, cold: boolean): Heading {
	if (expanded) {
		return 'all';
	}
	if (count <= shown) {
		return 'songs';
	}
	return cold ? 'cold' : 'top';
}

/**
 * The rows of a flat list to mount, every row `rowHeight` tall, for a screen `height` tall at `y`
 * into the list, as `windowAt` finds them: a screen either side, held until the screen comes within
 * half a screen of its edge. One height for every row makes it a division.
 */
export function rowWindow(
	count: number,
	rowHeight: number,
	y: number,
	height: number,
	mounted: RowWindow
): RowWindow {
	if (count === 0) {
		return { first: 0, last: -1 };
	}
	const last = count - 1;
	const at = (place: number): number =>
		Math.max(0, Math.min(last, Math.floor(place / Math.max(rowHeight, 1))));
	const held = mounted.last >= mounted.first && mounted.last <= last;
	if (held && at(y - height / 2) >= mounted.first && at(y + height * 1.5) <= mounted.last) {
		return mounted;
	}
	return { first: at(y - height), last: at(y + height * 2) };
}

/** The list by album, laid out: the artist's own albums, then the divider, then the rest. */
export interface AlbumsLayout extends Layout {
	/** Where the Appears on divider stands, or null with nothing before it to divide from. */
	divider: number | null;
}

/**
 * Lays the albums out with the Library's own maths (`layOut`): the artist's own first, then,
 * `dividerHeight` below them, those it appears on. A run with nothing in it takes no room.
 */
export function albumsLayout(
	heading: Omit<LibraryArtist, 'albums'>,
	albums: { own: readonly LibraryAlbum[]; appears: readonly LibraryAlbum[] },
	metrics: RowMetrics,
	dividerHeight: number
): AlbumsLayout {
	const of = (run: readonly LibraryAlbum[]): Layout =>
		layOut({ artists: run.length === 0 ? [] : [{ ...heading, albums: [...run] }] }, metrics);
	const own = of(albums.own);
	const appears = of(albums.appears);
	const divided = own.albums.length > 0 && appears.albums.length > 0;
	const divider = divided ? own.height + metrics.albumGap : null;
	const offset = divider === null ? 0 : divider + dividerHeight;
	return {
		rows: [
			...own.rows,
			...appears.rows.map((row) => ({
				...row,
				top: row.top + offset,
				albumIndex: row.albumIndex + own.albums.length
			}))
		],
		albums: [
			...own.albums,
			...appears.albums.map((block) => ({
				...block,
				top: block.top + offset,
				firstRow: block.firstRow + own.rows.length,
				lastRow: block.lastRow + own.rows.length
			}))
		],
		artists: [],
		letters: new Map(),
		height: appears.albums.length === 0 ? own.height : offset + appears.height,
		songs: own.songs + appears.songs,
		totalSongs: own.totalSongs + appears.totalSongs,
		divider
	};
}
