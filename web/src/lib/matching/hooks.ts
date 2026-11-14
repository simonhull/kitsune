/**
 * Where the match sheet opens from, each as one call, so the pages that offer it hold only a line.
 */
import type { EditSession } from '#lib/pages/tagEditor.svelte.ts';
import { matchSheet } from './sheet.svelte.ts';

/** From the album page: the songs are written, and the page follows the library. */
export function matchAlbum(albumId: string): void {
	matchSheet.open({ albumId, from: 'album' });
}

/**
 * From the tag editor's Matches tab, while every ticked song is on one album: only they start
 * ticked, and Apply fills the editor's fields, before anything is written.
 */
export function matchInEditor(session: EditSession): (() => void) | undefined {
	const albumId = session.oneAlbum;
	if (albumId === null || !session.canTag) {
		return undefined;
	}
	return () => {
		matchSheet.open({
			albumId,
			from: 'editor',
			only: session.tickedIds,
			fill: (edit) => {
				session.fill(albumId, edit);
			}
		});
	};
}
