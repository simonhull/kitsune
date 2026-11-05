import type { Album, AlbumEntry, ArtistDetail, Track } from './source.ts';

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
export function isMultiDisc(tracks: readonly Track[]): boolean {
	return new Set(tracks.map((track) => track.discNo ?? 1)).size > 1;
}

/** A track's number as tagged: `03`, `2-03` on a multi-disc album, or a dash with no number. */
export function trackLabel({ discNo, trackNo }: Track, multiDisc: boolean): string {
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
export function credit({ artists }: Track, albumArtist: string): string | undefined {
	const named = artists.filter((artist) => artist !== '');
	const guests = named.filter((artist) => artist !== albumArtist);
	if (guests.length === 0) {
		return undefined;
	}
	return guests.length < named.length ? `feat. ${guests.join(', ')}` : guests.join(', ');
}

/** The track's ReplayGain in dB, or null when the file has none. Other RG fields don't count. */
export function trackGain({ replayGain }: Track): number | null {
	return replayGain?.trackGain ?? null;
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

/** How many artists a list holds. */
export function artistCount(count: number): string {
	return plural(count, 'artist', 'artists');
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
