/** What the next navigation should be, once a newer build may be waiting. */
export interface Freshness {
	/** A newer build is on the server ($app/state's updated.current). */
	updated: boolean;
	/** Sound is coming or on its way. */
	sounding: boolean;
	/** A queue has begun and not been reset: a reload would lose it. */
	started: boolean;
	/** The navigation's kind; a 'leave' is the page going anyway. */
	type: 'enter' | 'form' | 'leave' | 'link' | 'goto' | 'popstate';
}

/**
 * Whether to turn an in-app navigation into a full load of the new build: only when one is waiting,
 * nothing plays and no queue would be lost. A song in progress is never cut off for an upgrade.
 */
export function reloadsInto({ updated, sounding, started, type }: Freshness): boolean {
	return updated && !sounding && !started && type !== 'leave' && type !== 'enter';
}
