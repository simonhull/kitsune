/**
 * Where the match sheet opens from, each as one call, so the pages that offer it hold only a line.
 */
import { bulkEdits, type BulkSession } from '#lib/pages/bulkEdits.svelte.ts';
import { tagEdits } from '#lib/pages/tagEdits.svelte.ts';
import { matchSheet } from './sheet.svelte.ts';

/** From the album page: the songs are written, and the page follows the library. */
export function matchAlbum(albumId: string): void {
	matchSheet.open({ albumId, from: 'album' });
}

/** From the tag editor, while it holds nothing unsaved: it reads the files again after Apply. */
export function matchFromEditor(albumId: string): void {
	matchSheet.open({
		albumId,
		from: 'editor',
		applied: (landed) => {
			tagEdits.close(albumId);
			tagEdits.open(landed);
		}
	});
}

/** From a selection's Edit N songs: only when its songs are on one album, and only they start ticked. */
export function matchFromBulk(session: BulkSession): (() => void) | undefined {
	const [albumId, ...others] = session.albumIds;
	if (albumId === undefined || others.length > 0) {
		return undefined;
	}
	return () => {
		matchSheet.open({
			albumId,
			from: 'bulk',
			only: session.pickedIds,
			applied: () => {
				bulkEdits.close();
			}
		});
	};
}
