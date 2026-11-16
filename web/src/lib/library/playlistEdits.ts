/**
 * What a change to a playlist does to the copy of it here, worked out before the server answers:
 * pure, so each is a test, and `playlists.svelte.ts` only keeps the state.
 */
import type { PlaylistDetail, PlaylistEntry, PlaylistPlace, PlaylistSummary } from './source.ts';

/** Entries taken out, as Undo puts them back: each run of them before the entry that followed it. */
export interface TakenOut {
	runs: { before: string | undefined; entries: PlaylistEntry[] }[];
}

/** Now, as the server writes a time. */
export function nowIso(): string {
	return new Date().toISOString();
}

/** How many songs there are among these, each once. */
export function distinct(trackIds: readonly string[]): number {
	return new Set(trackIds).size;
}

/** A detail's counts and mosaic after its entries changed, from what it holds now. */
export function recount(detail: PlaylistDetail): PlaylistDetail {
	const order = [...new Set(detail.entries.map(({ albumId }) => albumId))];
	return {
		...detail,
		songCount: detail.entries.length,
		albumCount: order.length,
		durationMs: detail.entries.reduce((sum, { track }) => sum + track.durationMs, 0),
		mosaic: order.slice(0, 4).flatMap((id) => detail.albums.find((album) => album.id === id) ?? [])
	};
}

/** The summary part of a detail. */
export function summaryOf(detail: PlaylistDetail): PlaylistSummary {
	// eslint-disable-next-line @typescript-eslint/no-unused-vars -- The summary is the rest.
	const { entries, albums, missing, ...summary } = detail;
	return summary;
}

/** What taking these entries out leaves, and how Undo puts them back. */
export function takenFrom(
	entries: readonly PlaylistEntry[],
	entryIds: readonly string[]
): { left: PlaylistEntry[]; taken: TakenOut } {
	const taking = new Set(entryIds);
	const taken: TakenOut = { runs: [] };
	let run: PlaylistEntry[] = [];
	for (const entry of entries) {
		if (taking.has(entry.id)) {
			run.push(entry);
		} else if (run.length > 0) {
			taken.runs.push({ before: entry.id, entries: run });
			run = [];
		}
	}
	if (run.length > 0) {
		taken.runs.push({ before: undefined, entries: run });
	}
	return { left: entries.filter(({ id }) => !taking.has(id)), taken };
}

/** The entries with what a take-out took back where it was: before the entry that followed it. */
export function putBackInto(entries: readonly PlaylistEntry[], taken: TakenOut): PlaylistEntry[] {
	let out = [...entries];
	for (const run of taken.runs) {
		const at = run.before === undefined ? -1 : out.findIndex(({ id }) => id === run.before);
		out = at < 0 ? [...out, ...run.entries] : out.toSpliced(at, 0, ...run.entries);
	}
	return out;
}

/**
 * An entry moved to index `to` of the order, and where the server is told it went: after the entry
 * it now follows, or before the first. Null when it doesn't move.
 */
export function movedTo(
	entries: readonly PlaylistEntry[],
	entryId: string,
	to: number
): { order: PlaylistEntry[]; place: PlaylistPlace } | null {
	const from = entries.findIndex(({ id }) => id === entryId);
	const moving = entries[from];
	if (moving === undefined || from === to || to < 0 || to >= entries.length) {
		return null;
	}
	const order = entries.toSpliced(from, 1).toSpliced(to, 0, moving);
	const after = order[to - 1];
	const before = order[to + 1];
	if (after !== undefined) {
		return { order, place: { after: after.id } };
	}
	return before === undefined ? null : { order, place: { before: before.id } };
}
