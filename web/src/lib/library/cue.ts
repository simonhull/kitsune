import { formatOf } from './format.ts';
import type { Album, Track } from './source.ts';

/** How many of the next tracks Now playing names; the rest are counted. */
const upNextShown = 3;

/**
 * The track the island and Now playing name. Before the first play it's only what would play; from
 * then on it follows the player's queue, and a track change may hold it back while the sound moves
 * on. Anything that means the sound itself reads the player.
 */
export interface NowPlaying {
	album: Album;
	track: Track;
	/** Its place in the queue, from 0. Track numbers can repeat or be missing. */
	index: number;
	trackCount: number;
	format: string;
	upNext: Track[];
	moreCount: number;
}

/**
 * Cues the track at `index` of `tracks`: the album's own order unless a queue says otherwise. An
 * album with no tracks can't be cued.
 */
export function cueOf(
	album: Album,
	tracks: readonly Track[] = album.tracks,
	index = 0
): NowPlaying | null {
	const track = tracks.at(index);
	if (track === undefined) {
		return null;
	}
	const rest = tracks.slice(index + 1);
	return {
		album,
		track,
		index,
		trackCount: tracks.length,
		format: formatOf(track),
		upNext: rest.slice(0, upNextShown),
		moreCount: Math.max(0, rest.length - upNextShown)
	};
}
