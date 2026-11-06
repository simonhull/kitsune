import type { Album } from '#lib/library/source.ts';

/** What a tap needs from the rest of the app. */
export interface TapHost {
	/** The album, if it's already here. */
	album(id: string): Album | undefined;
	/** Fetches an album. It never rejects: `album` afterwards says how that went. */
	load(id: string): Promise<void>;
	play(album: Album, index: number): void;
	/** Says the album or song couldn't be played. The queue stays as it was. */
	failed(albumId: string): void;
}

/**
 * Makes what a row's tap calls. The list has no tracks to queue, only their IDs, so a song plays
 * through its album, fetched first. An album already here plays in the tap itself: a browser may
 * only allow sound inside one. A later tap outranks any fetch still out.
 */
export function createTapper(host: TapHost): (albumId: string, trackId: string) => void {
	let latest = 0;

	function start(albumId: string, trackId: string): void {
		const album = host.album(albumId);
		const index = album?.tracks.findIndex(({ id }) => id === trackId) ?? -1;
		if (album === undefined || index < 0) {
			host.failed(albumId);
		} else {
			host.play(album, index);
		}
	}

	return (albumId, trackId) => {
		latest += 1;
		const tap = latest;
		if (host.album(albumId) !== undefined) {
			start(albumId, trackId);
			return;
		}
		void host.load(albumId).then(() => {
			if (tap === latest) {
				start(albumId, trackId);
			}
		});
	};
}
