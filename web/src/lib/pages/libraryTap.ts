import { discOf } from '#lib/library/format.ts';
import type { Album } from '#lib/library/source.ts';

/** What a tap needs from the rest of the app. */
export interface TapHost {
	/** The album, if it's already here. */
	album(id: string): Album | undefined;
	/** Fetches an album. It never rejects: `album` afterwards says how that went. */
	load(id: string): Promise<void>;
	play(album: Album, index: number): void;
	playDisc(album: Album, disc: number): void;
	/** Says the album or song couldn't be played. The queue stays as it was. */
	failed(albumId: string): void;
}

/** What a tap plays: from a song on through its album, or one disc of it. */
export type TapFrom = { track: string } | { disc: number };

/**
 * Makes what a row's tap calls. The list has no tracks to queue, only their IDs, so a song plays
 * through its album, fetched first. An album already here plays in the tap itself: a browser may
 * only allow sound inside one. A later tap outranks any fetch still out.
 */
export function createTapper(host: TapHost): (albumId: string, from: TapFrom) => void {
	let latest = 0;

	function start(albumId: string, from: TapFrom): void {
		const album = host.album(albumId);
		if ('disc' in from) {
			const has = album?.tracks.some(({ discNo }) => discOf(discNo) === from.disc) ?? false;
			if (album === undefined || !has) {
				host.failed(albumId);
			} else {
				host.playDisc(album, from.disc);
			}
			return;
		}
		const index = album?.tracks.findIndex(({ id }) => id === from.track) ?? -1;
		if (album === undefined || index < 0) {
			host.failed(albumId);
		} else {
			host.play(album, index);
		}
	}

	return (albumId, from) => {
		latest += 1;
		const tap = latest;
		if (host.album(albumId) !== undefined) {
			start(albumId, from);
			return;
		}
		void host.load(albumId).then(() => {
			if (tap === latest) {
				start(albumId, from);
			}
		});
	};
}
