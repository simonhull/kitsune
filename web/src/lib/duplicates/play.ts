import type { Album, Track } from '#lib/library/source.ts';
import type { Queued } from '#lib/player/queue.ts';
import type { DuplicateCopy } from './types.ts';

/**
 * A copy as the player takes it: the song alone, on an album of one, so Play on a copy plays that
 * very file at once, with nothing fetched first.
 */
export function queuedCopy(copy: DuplicateCopy, artist: string): Queued {
	const track: Track = {
		id: copy.track,
		title: copy.title,
		artists: artist === '' ? [] : [artist],
		...(copy.trackNo === undefined ? {} : { trackNo: copy.trackNo }),
		...(copy.disc === undefined ? {} : { discNo: copy.disc }),
		durationMs: copy.durationMs,
		format: copy.format,
		...(copy.sampleRate === undefined ? {} : { sampleRate: copy.sampleRate }),
		...(copy.bitDepth === undefined ? {} : { bitDepth: copy.bitDepth }),
		genres: [],
		etag: copy.track,
		...(copy.stars > 0 ? { stars: copy.stars } : {})
	};
	const album: Album = {
		id: copy.album.id,
		title: copy.album.title,
		artist: copy.album.artist,
		...(copy.album.year === undefined ? {} : { year: copy.album.year }),
		...(copy.album.artwork === undefined ? {} : { artwork: copy.album.artwork }),
		trackCount: 1,
		compilation: false,
		artists: [],
		tracks: [track],
		durationMs: copy.durationMs,
		formats: [copy.format],
		addedAt: copy.addedAt,
		genres: [],
		etag: copy.track,
		discs: [{ songs: 1, durationMs: copy.durationMs }]
	};
	return { album, track };
}
