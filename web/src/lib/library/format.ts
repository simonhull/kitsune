import type { Album, AlbumEntry, ArtistDetail, Disc, Track } from './source.ts';

/** Formats seconds as m:ss. */
export function clock(totalSeconds: number): string {
	const minutes = Math.floor(totalSeconds / 60);
	const seconds = Math.floor(totalSeconds % 60);
	return `${String(minutes)}:${String(seconds).padStart(2, '0')}`;
}

/** Formats ReplayGain the way the boards do, with a true minus sign. */
export function gain(decibels: number): string {
	const magnitude = Math.abs(decibels).toFixed(1);
	return decibels < 0 ? `−${magnitude}` : `+${magnitude}`;
}

/** Splits a title into two lines of similar length, at a word boundary. */
export function balancedLines(title: string): [string, string] {
	const words = title.split(' ');
	let best: [string, string] = [title, ''];
	let bestLongest = title.length;
	for (let cut = 1; cut < words.length; cut += 1) {
		const first = words.slice(0, cut).join(' ');
		const second = words.slice(cut).join(' ');
		const longest = Math.max(first.length, second.length);
		if (longest < bestLongest) {
			best = [first, second];
			bestLongest = longest;
		}
	}
	return best;
}

/** Reports whether an album's tracks span more than one disc. An unset disc counts as disc 1. */
export function isMultiDisc(tracks: readonly Pick<Track, 'discNo'>[]): boolean {
	return new Set(tracks.map((track) => track.discNo ?? 1)).size > 1;
}

/**
 * An album's discs, as the server works them out: in the order the tracks first reach each, named
 * by the first track on it with a name. Only a stand-in for the server needs it.
 */
export function discsOf(
	tracks: readonly Pick<Track, 'discNo' | 'discSubtitle' | 'durationMs'>[]
): Disc[] {
	const discs: Disc[] = [];
	for (const { discNo, discSubtitle, durationMs } of tracks) {
		const no = Math.max(discNo ?? 1, 1);
		let disc = discs.find((each) => each.no === no);
		if (disc === undefined) {
			disc = { no, songs: 0, durationMs: 0 };
			discs.push(disc);
		}
		disc.songs += 1;
		disc.durationMs += durationMs;
		if (disc.name === undefined && discSubtitle !== undefined && discSubtitle !== '') {
			disc.name = discSubtitle;
		}
	}
	return discs;
}

/**
 * Where an album's disc headings go: each disc by the index of its first track. Only an album on
 * more than one disc has any; one disc stays as it is, named or not.
 */
export function discHeads({
	tracks,
	discs
}: {
	tracks: readonly Pick<Track, 'discNo'>[];
	discs: readonly Disc[];
}): Map<number, Disc> {
	const heads = new Map<number, Disc>();
	if (discs.length < 2) {
		return heads;
	}
	const byNo = new Map(discs.map((disc) => [disc.no, disc]));
	let last: number | undefined;
	for (const [index, { discNo }] of tracks.entries()) {
		const no = Math.max(discNo ?? 1, 1);
		const disc = byNo.get(no);
		if (no !== last && disc !== undefined) {
			heads.set(index, disc);
		}
		last = no;
	}
	return heads;
}

/** What a disc's heading says beside its name: `5 SONGS` and `19 MIN`, never under a minute. */
export function discFacts({ songs, durationMs }: Pick<Disc, 'songs' | 'durationMs'>): {
	songs: string;
	length: string;
} {
	const minutes = Math.max(1, Math.round(durationMs / 60_000));
	return {
		songs: plural(songs, 'SONG', 'SONGS'),
		length: `${minutes.toLocaleString('en-US')} MIN`
	};
}

/** A track's number as tagged: `03`, `2-03` on a multi-disc album, or a dash with no number. */
export function trackLabel(
	{ discNo, trackNo }: Pick<Track, 'discNo' | 'trackNo'>,
	multiDisc: boolean
): string {
	if (trackNo === undefined) {
		return '–';
	}
	const number = String(trackNo).padStart(2, '0');
	return multiDisc ? `${String(discNo ?? 1)}-${number}` : number;
}

/**
 * Who a track credits beyond the album. A guest is a track artist beside the album artist; a track
 * that doesn't name the album artist at all, as on a compilation, shows its own artists plainly.
 */
export function credit(
	{ artists }: Pick<Track, 'artists'>,
	albumArtist: string
): string | undefined {
	const named = artists.filter((artist) => artist !== '');
	const guests = named.filter((artist) => artist !== albumArtist);
	// An album shared by several artists shows them joined: a song by all of them adds nothing.
	if (guests.length === 0 || named.join(', ') === albumArtist) {
		return undefined;
	}
	return guests.length < named.length ? `feat. ${guests.join(', ')}` : guests.join(', ');
}

/** The track's ReplayGain in dB, or null when it has none. Other RG fields don't count. */
export function trackGain({ replayGain }: Track): number | null {
	return replayGain?.trackGain ?? null;
}

/** Whether the track's ReplayGain is Kitsune's own measure rather than its file's tags. */
export function gainMeasured({ replayGain }: Track): boolean {
	return replayGain?.source === 'measured';
}

/** A track's format: `FLAC 24/96`, `FLAC 16/44.1`, or `MP3` where depth means nothing. */
export function formatOf({ format, bitDepth, sampleRate }: Track): string {
	if (bitDepth === undefined || bitDepth === 0 || sampleRate === undefined) {
		return format;
	}
	return `${format} ${String(bitDepth)}/${String(sampleRate / 1000)}`;
}

/** An album's format: its one format, or its codecs when tracks differ, or the first with `+`. */
export function formatLine(tracks: readonly Track[]): string {
	const exact = new Set(tracks.map(formatOf));
	if (exact.size <= 1) {
		return [...exact].at(0) ?? '';
	}
	const codecs = [...new Set(tracks.map((track) => track.format))].sort();
	return codecs.length <= 2 ? codecs.join(' · ') : `${codecs.at(0) ?? ''} +`;
}

function plural(count: number, one: string, many: string): string {
	return `${count.toLocaleString('en-US')} ${count === 1 ? one : many}`;
}

/** The album's fact line. A missing year drops its segment. */
export function albumFacts(entry: AlbumEntry, album: Album): string {
	const minutes = Math.max(1, Math.round(album.durationMs / 60_000));
	return [
		entry.compilation ? 'COMPILATION' : 'ALBUM',
		...(album.year === undefined ? [] : [String(album.year)]),
		plural(album.tracks.length, 'TRACK', 'TRACKS'),
		`${minutes.toLocaleString('en-US')} MIN`
	].join(' · ');
}

/** The line over the Library page: the letter you're at, and how much there is. */
export function libraryLine(
	letter: string,
	artists: number,
	songs: number,
	of: number = songs
): string {
	return `${letter} · ${plural(artists, 'ARTIST', 'ARTISTS')} · ${partOf(songs, of, 'SONG', 'SONGS', ' OF ')}`;
}

/** Counts `shown` of `total`, or just the total when a filter hides none of them. */
function partOf(shown: number, total: number, one: string, many: string, of: string): string {
	return shown === total
		? plural(total, one, many)
		: `${shown.toLocaleString('en-US')}${of}${plural(total, one, many)}`;
}

/** What an artist has in the Library, under its name, and how much of it a filter shows. */
export function artistHoldings(
	albums: readonly { tracks: readonly unknown[] }[],
	shown?: { albums: number; songs: number }
): string {
	const songs = albums.reduce((sum, { tracks }) => sum + tracks.length, 0);
	const { albums: albumsShown = albums.length, songs: songsShown = songs } = shown ?? {};
	return `${partOf(albumsShown, albums.length, 'ALBUM', 'ALBUMS', ' OF ')} · ${partOf(songsShown, songs, 'SONG', 'SONGS', ' OF ')}`;
}

/** An album's line in the Library: a missing year drops its segment, and a filter says what it hides. */
export function albumHoldings(year: number | undefined, songs: number, of: number = songs): string {
	return [
		...(year === undefined ? [] : [String(year)]),
		partOf(songs, of, 'song', 'songs', ' of ')
	].join(' · ');
}

/** How many artists a list holds. */
export function artistCount(count: number): string {
	return plural(count, 'artist', 'artists');
}

/** The Albums page's fact line: what's there, or what a genre leaves of it. */
export function albumsLine(
	shown: number,
	total: number,
	genres: number,
	genre: string | null
): string {
	if (genre !== null) {
		return `${shown.toLocaleString('en-US')} OF ${plural(total, 'ALBUM', 'ALBUMS')} · ${genre.toUpperCase()}`;
	}
	const albums = plural(total, 'ALBUM', 'ALBUMS');
	return genres === 0 ? albums : `${albums} · ${plural(genres, 'GENRE', 'GENRES')}`;
}

/** An artist's fact line. A guest owns nothing, so it counts where it appears instead. */
export function artistFacts({ albumCount, appearsOn }: ArtistDetail): string {
	const standing =
		albumCount > 0
			? `${plural(albumCount, 'ALBUM', 'ALBUMS')} IN YOUR LIBRARY`
			: `GUEST ON ${plural(appearsOn.length, 'ALBUM', 'ALBUMS')}`;
	return `ARTIST · ${standing}`;
}

const addedFormat = new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium' });

/** When an album was added, as `12 Oct 2026`. */
export function addedOn(addedAt: string): string {
	return addedFormat.format(new Date(addedAt));
}
