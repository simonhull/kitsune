/** What the next navigation should be, once a newer build may be waiting. */
export interface Freshness {
	/** A newer build is on the server ($app/state's updated.current). */
	updated: boolean;
	/** Sound is coming or on its way. */
	sounding: boolean;
	/**
	 * A queue has begun and the server doesn't hold where it is (a change not saved yet, or a save
	 * on its way): a reload would lose it.
	 */
	unsaved: boolean;
	/** The navigation's kind; a 'leave' is the page going anyway. */
	type: 'enter' | 'form' | 'leave' | 'link' | 'goto' | 'popstate';
}

/**
 * Whether to turn an in-app navigation into a full load of the new build: only when one is waiting,
 * nothing plays and nothing would be lost. A paused player the server holds comes back after the
 * load, as from any reload. A song in progress is never cut off for an upgrade.
 */
export function reloadsInto({ updated, sounding, unsaved, type }: Freshness): boolean {
	return updated && !sounding && !unsaved && type !== 'leave' && type !== 'enter';
}
