/** What an album page's song list needs that the album itself says differently. */
import { discHeads } from '#lib/library/format.ts';
import type { Album, Disc, LibraryAlbum } from '#lib/library/source.ts';

/**
 * An album's songs as song rows show them, from the album itself: what a row can show before the
 * index's own rows for it, with the columns' fields, have come.
 */
export function rowsOf(album: Album): LibraryAlbum {
	return {
		id: album.id,
		title: album.title,
		artist: album.artist,
		compilation: album.compilation,
		year: album.year,
		artwork: album.artwork,
		discs: album.discs,
		tracks: album.tracks.map((track) => ({
			id: track.id,
			title: track.title,
			durationMs: track.durationMs,
			artists: track.artists,
			discNo: track.discNo,
			trackNo: track.trackNo,
			stars: track.stars,
			year: track.year,
			format: track.format,
			sampleRate: track.sampleRate,
			bitDepth: track.bitDepth,
			trackGain: track.replayGain?.trackGain,
			trackGainSource:
				track.replayGain?.trackGain === undefined ? undefined : track.replayGain.source
		}))
	};
}

/** One disc of an album as its page lists it: its heading, if it has one, and its songs. */
export interface DiscGroup {
	/** Absent for an album on one disc, which has no heading. */
	disc: Disc | undefined;
	/** The songs' places in the album, in order. */
	indexes: number[];
}

/** An album's songs, a disc at a time, in album order. One disc with no name is one group. */
export function discGroups(album: Pick<LibraryAlbum, 'tracks' | 'discs'>): DiscGroup[] {
	const heads = discHeads({ tracks: album.tracks, discs: album.discs ?? [] });
	const groups: DiscGroup[] = [];
	for (const index of album.tracks.keys()) {
		const head = heads.get(index);
		const last = groups.at(-1);
		if (head !== undefined || last === undefined) {
			groups.push({ disc: head, indexes: [index] });
		} else {
			last.indexes.push(index);
		}
	}
	return groups;
}
