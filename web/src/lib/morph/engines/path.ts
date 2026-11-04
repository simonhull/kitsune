/** Clamps a value's position between two bounds to 0–1. */
export function unit(value: number, low: number, high: number): number {
	return high === low ? 1 : Math.min(1, Math.max(0, (value - low) / (high - low)));
}

function lerp(from: number, to: number, amount: number): number {
	return from + (to - from) * amount;
}

/**
 * How far the fox bar has lifted off the glass's floor, `along` its way from the island's
 * hairline (0) to the scrubber (1). It slides under everything first and only rises at the
 * very end, past the buttons. One function of progress, so the way back is the same path.
 */
export function barLift(along: number): number {
	const rise = unit(along, 0.85, 1);
	return rise * rise * (3 - 2 * rise);
}

/** Finds a bar's distance above the glass's floor, between its two resting heights. */
export function barRise(along: number, floor: number, track: number): number {
	return lerp(floor, track, barLift(along));
}

/** Gives both pictures of a title one width, so their letters line up through the dissolve. */
export function sharedWidth(small: number, large: number, grown: number): number {
	return lerp(small, large, grown);
}

/**
 * Which picture of a title should show, as 0 for small and 1 for large. It leaves the small one
 * at once on the way up, and goes back to it only near the end on the way down, so a short
 * dissolve finishes as the title lands rather than dragging through the spring's slow tail.
 */
export function dissolveTarget(grown: number, headingLarge: boolean): number {
	return (headingLarge ? grown > 0.02 : grown >= 0.08) ? 1 : 0;
}

/** Turns a dissolve into two opacities that never dip together, so the words don't flicker. */
export function dissolveOpacities(dissolve: number): { small: number; large: number } {
	const large = Math.min(1, Math.max(0, dissolve));
	return { small: Math.min(1, 2 * (1 - large)), large: Math.min(1, 2 * large) };
}

export interface Point {
	x: number;
	y: number;
}

/** Finds how far something riding the glass is from where it would sit at rest. */
export function carryOffset(glass: Point, anchor: Point): Point {
	return { x: glass.x - anchor.x, y: glass.y - anchor.y };
}
