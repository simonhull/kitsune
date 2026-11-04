import { fnv1a } from '#lib/cover/cover.ts';
import type { LibraryFacts } from '#lib/library/source.ts';

/** What the gap between bursts is taken to be before two have been seen: the real library's mean. */
export const seedGapMs = 27_000;
/** How much of the expected gap one new measurement moves. */
const gapWeight = 0.3;
const fastestMs = 180;
const slowestMs = 3000;
/** How far either side of even a card may land, so a long wait never ticks like a clock. */
const jitter = 0.25;
const smallestQueue = 40;
const largestQueue = 100;

function clamp(value: number, low: number, high: number): number {
	return Math.min(high, Math.max(low, value));
}

/** A stable number from -1 to 1 for this album: the pile looks the same on every reload. */
export function wobble(id: string, salt = ''): number {
	return ((fnv1a(`${salt}${id}`) % 2001) - 1000) / 1000;
}

/** Folds one measured gap between bursts into the expected one. */
export function expectedGap(gapMs: number, measuredMs: number): number {
	return gapMs + (measuredMs - gapMs) * gapWeight;
}

/**
 * How long to wait after this card: a burst's cards spread over the time until the next burst, so
 * the pile keeps moving through the silence. `queued` is the queue as the burst left it. Taken from
 * the queue as it drains, the spacing would shrink with every card and the pile would run out early.
 */
export function dealInterval(queued: number, gapMs: number, id: string): number {
	const even = clamp(gapMs / Math.max(1, queued), fastestMs, slowestMs);
	return even * (1 + jitter * wobble(id));
}

/** How many cards may wait. Past it the oldest go: cards are a sample, and the count is the truth. */
export function queueCap(gapMs: number): number {
	return clamp(Math.ceil(gapMs / slowestMs), smallestQueue, largestQueue);
}

/** What Home opens on: the deck, Home, or a moment's wait to tell a first scan from a restart. */
export type Opening = 'deck' | 'home' | 'wait';

export interface OpeningContext {
	/** Whether this browser has ever seen this library ready, so a scan under way is a restart's. */
	remembered: boolean;
	/** Whether the moment's wait has already been spent. */
	waited: boolean;
}

/**
 * Decides what Home opens on. Not ready can't mean first run by itself: every restart has a short
 * spell of it, and a failed ping reads the same.
 */
export function openingFor(
	{ albums, roots }: Pick<LibraryFacts, 'albums' | 'roots'>,
	ready: boolean,
	{ remembered, waited }: OpeningContext
): Opening {
	if (ready || (albums === 0 && roots === 0)) {
		return 'home';
	}
	if (albums === 0) {
		return 'deck';
	}
	if (remembered) {
		return 'home';
	}
	return waited ? 'deck' : 'wait';
}
