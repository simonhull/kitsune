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
	/** The album of each of {@link NowPlaying.upNext}, which can differ in a queue of picked songs. */
	upNextAlbums: Album[];
	moreCount: number;
}

/**
 * Cues the track at `index` of `tracks`: the album's own order unless a queue says otherwise. An
 * album with no tracks can't be cued.
 */
export function cueOf(
	album: Album,
	tracks: readonly Track[] = album.tracks,
	index = 0,
	albums?: ReadonlyMap<string, Album>
): NowPlaying | null {
	const track = tracks.at(index);
	if (track === undefined) {
		return null;
	}
	const rest = tracks.slice(index + 1);
	const upNext = rest.slice(0, upNextShown);
	const of = (each: Track): Album => albums?.get(each.id) ?? album;
	return {
		album: of(track),
		track,
		index,
		trackCount: tracks.length,
		format: formatOf(track),
		upNext,
		upNextAlbums: upNext.map(of),
		moreCount: Math.max(0, rest.length - upNextShown)
	};
}

/**
 * Names a cue from an album just edited, keeping its order and its place: the track is the same
 * one, whatever its number says now. Tracks not in the album stay as they were.
 */
export function retagged(cue: NowPlaying, album: Album): NowPlaying {
	const fresh = (track: Track): Track => album.tracks.find(({ id }) => id === track.id) ?? track;
	return {
		...cue,
		album: cue.album.id === album.id ? album : cue.album,
		track: fresh(cue.track),
		upNext: cue.upNext.map(fresh),
		upNextAlbums: cue.upNextAlbums.map((each) => (each.id === album.id ? album : each))
	};
}
