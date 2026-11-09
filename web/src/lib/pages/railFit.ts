/**
 * How the Library's alphabet rail fits the room it has. Every letter keeps its own slot, so a finger
 * picks the exact letter under it; when the slots are too short to read, only some letters are
 * drawn, with a dot between them, the way iOS folds its index.
 */

/** A letter needs a line of the rail's mono type (12 px) to read: below this, the rail folds. */
export const letterLine = 16;

/** Folded, a letter has its line and the dot after it half another, so the drawn letters stand this far apart. */
const foldedPitch = letterLine * 1.5;

/** The letters a rail draws, by index, and its dots, each at a position counted in slots from the top. */
export interface Fold {
	shown: readonly number[];
	dots: readonly number[];
}

/**
 * Folds a rail of `count` letters whose slots are `step` px tall: every letter when they read,
 * otherwise as many as stand `foldedPitch` apart, spread evenly, the first and last always among
 * them, with a dot halfway between any two that skip something.
 */
export function fold(count: number, step: number): Fold {
	const all = Array.from({ length: count }, (_, index) => index);
	if (count <= 2 || step >= letterLine) {
		return { shown: all, dots: [] };
	}
	const every = Math.ceil(foldedPitch / Math.max(step, Number.EPSILON));
	const drawn = Math.min(count, Math.floor((count - 1) / every) + 1);
	if (drawn >= count) {
		return { shown: all, dots: [] };
	}
	const gaps = Math.max(1, drawn - 1);
	const shown = Array.from({ length: gaps + 1 }, (_, place) =>
		Math.round((place * (count - 1)) / gaps)
	);
	const dots = shown.slice(1).flatMap((to, place) => {
		const from = shown[place] ?? to;
		// The middle of the slots between: slot i's centre is at i + 0.5.
		return to - from > 1 ? [(from + to + 1) / 2] : [];
	});
	return { shown, dots };
}

/** The letter, by index, under a point `offset` px down a rail `height` px tall of `count` slots. */
export function letterAt(offset: number, height: number, count: number): number {
	if (count <= 0 || height <= 0) {
		return 0;
	}
	return Math.min(count - 1, Math.max(0, Math.floor((offset / height) * count)));
}
