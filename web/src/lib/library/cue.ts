import { formatOf } from './format.ts';
import type { Album, AlbumEntry, Track } from './source.ts';

/** How many of the next tracks Now playing names; the rest are counted. */
const upNextShown = 3;

/**
 * The track in the player. until streaming lands this is only ever cued: it is what would
 * play, never what is playing, so nothing that reads it may claim sound or progress.
 */
export interface NowPlaying {
	album: AlbumEntry;
	track: Track;
	/** Its place in the album, from 0. Track numbers can repeat or be missing. */
	index: number;
	trackCount: number;
	format: string;
	upNext: Track[];
	moreCount: number;
}

/** Cues an album's first track. An album with no tracks can't be cued. */
export function cueOf(album: Album): NowPlaying | null {
	const [track, ...rest] = album.tracks;
	if (track === undefined) {
		return null;
	}
	return {
		album,
		track,
		index: 0,
		trackCount: album.tracks.length,
		format: formatOf(track),
		upNext: rest.slice(0, upNextShown),
		moreCount: Math.max(0, rest.length - upNextShown)
	};
}
