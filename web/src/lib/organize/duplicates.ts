/**
 * The way to Duplicates (plan 069), from the Organize · Duplicates tabs and a conflict card's
 * "Compare in Duplicates ›". Until it is built both are marked SOON; plan 069 sets
 * `duplicatesReady` and makes `openDuplicates` go to its page, and nothing else changes here.
 */

/** Whether the Duplicates page is built. */
export const duplicatesReady: boolean = false;

/** Opens Duplicates, on the group holding these songs when given. Plan 069 fills this in. */
export function openDuplicates(tracks: readonly string[] = []): void {
	void tracks;
}
