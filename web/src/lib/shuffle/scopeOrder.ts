/**
 * What a shuffle session's scope is in its own order, for leaving the shuffle there: the Library's
 * order for the library and its filter, an album's, an artist page's. Pure: the caller says what it
 * holds.
 */
import type { Album, LibraryIndex, ShuffleScope } from '#lib/library/source.ts';
import { keepFor, type Keep, type MarksView } from '#lib/pages/libraryRows.ts';
import type { ListEntry } from '#lib/player/queue.ts';

/** What the scope's order is read from, as far as it's here: null for what isn't yet. */
export interface ScopeHolds {
	/** The Library's index. */
	index: LibraryIndex | null;
	album(id: string): Album | undefined;
	/** An artist page's songs. */
	artistSongs(id: string): LibraryIndex | null;
	marks: MarksView;
}

/** Every song of an index `keep` keeps, in the index's order, as list entries. */
function entriesOf(index: LibraryIndex, keep: Keep = () => true): ListEntry[] {
	return index.artists.flatMap((artist) =>
		artist.albums.flatMap((album) =>
			album.tracks.flatMap((track) =>
				keep(artist, album, track) ? [{ trackId: track.id, albumId: album.id }] : []
			)
		)
	);
}

/** A scope's songs in its own order, or null while what holds that order isn't here. */
export function scopeOrder(scope: ShuffleScope, holds: ScopeHolds): ListEntry[] | null {
	switch (scope.kind) {
		case 'library':
		case 'rated': {
			const keep = keepFor(scope.kind === 'library' ? 'everything' : 'rated', holds.marks);
			return holds.index === null ? null : entriesOf(holds.index, keep);
		}
		case 'tracks': {
			const picked = new Set(scope.tracks);
			return holds.index === null
				? null
				: entriesOf(holds.index, (_artist, _album, track) => picked.has(track.id));
		}
		case 'album': {
			const album = holds.album(scope.id);
			return album === undefined
				? null
				: album.tracks.map(({ id }) => ({ trackId: id, albumId: album.id }));
		}
		case 'artist': {
			const songs = holds.artistSongs(scope.id);
			return songs === null ? null : entriesOf(songs);
		}
	}
}

/**
 * The songs after `trackId` in an order: what plays on from it. A song the order doesn't hold (a
 * run's song from outside the scope) goes on from the order's start.
 */
export function entriesAfter(
	order: readonly ListEntry[],
	trackId: string | undefined
): ListEntry[] {
	const at = order.findIndex((entry) => entry.trackId === trackId);
	return order.slice(at + 1);
}
