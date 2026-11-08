import type { Album } from '#lib/library/source.ts';
import { onwardOf, pendingAlbum, upcoming, type Queue } from '#lib/player/queue.ts';
import type { AlbumBlock } from './libraryRows.ts';

/** What carrying on through the list needs from the rest of the app. */
export interface ContinuesHost {
	/** The album, if it's already here. */
	album(id: string): Album | undefined;
	/** Fetches an album. It never rejects: `album` afterwards says how that went. */
	loadAlbum(id: string): Promise<void>;
	/** The queue playing. */
	readonly queue: Queue | null;
	/** Says which album follows `after` in the queue, as the player's `continueList` does. */
	continueList(after: string, album: Album | null): boolean;
	/**
	 * The album after `id` in the list as it shows now, with its sort and filter: `null` at the end
	 * of the list, or when `id` isn't in it any more.
	 */
	albumAfter(id: string): string | null;
}

/** Finds the album after `id` in a list's albums: `null` past the last, or with `id` not there. */
export function albumAfter(albums: readonly AlbumBlock[], id: string): string | null {
	const at = albums.findIndex(({ album }) => album.id === id);
	return at < 0 ? null : (albums.at(at + 1)?.album.id ?? null);
}

/**
 * Keeps a queue started from the Library going on through the list, album by album, to the end of
 * it. The next album's record is fetched as the one before it starts, and its songs go in when the
 * queue's last song begins, so the change of album is a change of track: nothing waits on the
 * server. A list that changes before the album has begun puts the right one in its place.
 */
export class LibraryContinues {
	readonly #host: ContinuesHost;

	constructor(host: ContinuesHost) {
		this.#host = host;
	}

	/** Looks again at what comes after the queue: as each song begins, and when the list changes. */
	keep(): void {
		const queue = this.#host.queue;
		if (queue?.continues === undefined || onwardOf(queue) !== 'list') {
			return;
		}
		const ahead = pendingAlbum(queue);
		const after = ahead?.after ?? queue.continues.album;
		const want = this.#host.albumAfter(after);
		if (ahead === undefined && upcoming(queue) !== undefined) {
			// Songs still to play before the end: the next album's record comes now, while they do.
			if (want !== null) {
				void this.#host.loadAlbum(want);
			}
			return;
		}
		if (ahead !== undefined && want === queue.continues.album) {
			return;
		}
		const album = want === null ? undefined : this.#host.album(want);
		if (want === null || album !== undefined) {
			this.#host.continueList(after, album ?? null);
			return;
		}
		// Not here yet: whatever was put after the queue no longer follows, and this one does once
		// it arrives, if the list still says so.
		this.#host.continueList(after, null);
		void this.#host.loadAlbum(want).then(() => {
			const late = this.#host.album(want);
			if (late !== undefined && this.#host.albumAfter(after) === want) {
				this.#host.continueList(after, late);
			}
		});
	}
}
