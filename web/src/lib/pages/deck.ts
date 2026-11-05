import { fnv1a } from '#lib/cover/cover.ts';
import type { LibraryFacts } from '#lib/library/source.ts';

/** How far apart cards go down when they're coming as they're found. */
const throwGapMs = 70;
/** How far apart they go down once a backlog has built up. */
const hurriedGapMs = 45;
/** How many cards waiting counts as a full backlog. */
const backlogCards = 24;
/** How far either side of that a card may go, so a run never ticks like a clock. */
const jitter = 0.2;
/** How many cards may wait. Only past this do the oldest go, which is a last resort. */
export const mostQueued = 200;
/** How many covers decode at once, ahead of the throw. */
export const decodesAhead = 6;
/** A pile is loose: wider than the 4 degrees of the one cover at rest anywhere else. */
const tiltDegrees = 9;
const offsetPixels = 14;
const spinDegrees = 7;

/** A stable number from -1 to 1 for this album: the pile looks the same on every reload. */
export function wobble(id: string, salt = ''): number {
	return ((fnv1a(`${salt}${id}`) % 2001) - 1000) / 1000;
}

/** Which way a thrown card is turning as it lands: a little past its rest, on its way there. */
export function spinOf(id: string): number {
	return spinDegrees * wobble(id, 'spin');
}

/** Where a card lies on the pile, always the same place for the same album. */
export function restOf(id: string): string {
	const x = (offsetPixels * wobble(id, 'x')).toFixed(1);
	const y = (offsetPixels * wobble(id, 'y')).toFixed(1);
	return `translate(${x}px, ${y}px) rotate(${(tiltDegrees * wobble(id, 'tilt')).toFixed(2)}deg)`;
}

/**
 * Spaces a run of cards, returning when each one goes, from 0. Every step is shorter than the one
 * before by its own amount: the run gathers pace, and never ticks.
 */
export function shrinkingDelays(ids: readonly string[], firstStepMs: number): number[] {
	let at = 0;
	let step = firstStepMs;
	return ids.map((id) => {
		const delay = at;
		at += step;
		step *= 0.875 + 0.075 * wobble(id, 'step');
		return delay;
	});
}

/** How long to leave after this card before the next: less, the more are waiting behind it. */
export function throwGap(waiting: number, id: string): number {
	const hurry = Math.min(1, waiting / backlogCards);
	const gap = throwGapMs + (hurriedGapMs - throwGapMs) * hurry;
	return gap * (1 + jitter * wobble(id));
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
