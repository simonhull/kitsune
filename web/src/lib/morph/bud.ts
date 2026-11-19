/**
 * The shapes a surface in the page goes through (M4's Bud), worked out without a document: where it
 * is born, how its box clips its words, and how one shape is weighed against another.
 */

/** A box on screen in viewport pixels, with its corners: a surface's true size and radius. */
export interface Rect {
	left: number;
	top: number;
	width: number;
	height: number;
	radius: number;
}

/** A corner can't be rounder than half the box's shorter side. */
export function cornerOf(radius: number, width: number, height: number): number {
	return Math.max(0, Math.min(radius, width / 2, height / 2));
}

/**
 * Where a surface with no control left on screen is born: the floor, its own width, its top edge
 * rising from its foot, which stands on the island's top edge or the selection bar's.
 */
export function floorRect(rest: Rect): Rect {
	return { ...rest, top: rest.top + rest.height, height: 0 };
}

/**
 * Where a surface whose control is unknown is born, as a keyboard shortcut opens one: the line of
 * its own top edge, its own width, so it still grows rather than appearing.
 */
export function edgeRect(rest: Rect): Rect {
	return { ...rest, height: 0 };
}

/**
 * The `clip-path` that shows exactly `box` of an element laid out at `rest`: a box that grows reveals
 * the words, which never move, and a box that reshapes cuts them. A box that misses the element
 * altogether clips everything away.
 */
export function clipInset(box: Rect, rest: Rect): string {
	const top = box.top - rest.top;
	const left = box.left - rest.left;
	const bottom = rest.top + rest.height - (box.top + box.height);
	const right = rest.left + rest.width - (box.left + box.width);
	const radius = cornerOf(box.radius, box.width, box.height);
	if (box.width <= 0 || box.height <= 0) {
		return 'inset(50% 50% 50% 50%)';
	}
	return `inset(${px(top)} ${px(right)} ${px(bottom)} ${px(left)} round ${px(radius)})`;
}

function px(value: number): string {
	return `${value.toFixed(2)}px`;
}

/** Reports whether two shapes are the same to the eye, so a change of nothing animates nothing. */
export function sameRect(a: Rect, b: Rect): boolean {
	return (
		Math.abs(a.left - b.left) < 0.5 &&
		Math.abs(a.top - b.top) < 0.5 &&
		Math.abs(a.width - b.width) < 0.5 &&
		Math.abs(a.height - b.height) < 0.5 &&
		Math.abs(a.radius - b.radius) < 0.5
	);
}
