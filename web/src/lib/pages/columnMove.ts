/**
 * A column header dragged sideways: where it is, which neighbours have made room, and where it
 * would land, from where the columns stood when it lifted and how far the pointer has gone. Pure
 * geometry, with no DOM: SongHeader reads the slots once at the lift and writes what this returns as
 * custom properties on the list, so a pointer move changes properties and no row.
 */

/** A column's place in the header when the drag began, in CSS pixels from the header's start. */
export interface Slot {
	left: number;
	width: number;
}

/** How far past either end a column gives ground, at most, and how soon it stiffens. */
export const rubberMax = 18;
const rubberSoftness = 60;

/** The rubber band at the walls: the further past, the less it gives, never more than 18 px. */
export function rubber(over: number): number {
	return over <= 0 ? 0 : rubberMax * (1 - Math.exp(-over / rubberSoftness));
}

/** One frame of a drag. */
export interface MoveFrame {
	/** How far the held column is drawn from its own slot, rubber band included. */
	offset: number;
	/** How far each column is drawn from its slot, by index: the held one's is `offset`. */
	shifts: number[];
	/** The index it would land at. */
	target: number;
	/** Where its slot would start: the hole's left. */
	holeLeft: number;
}

/** Where the held column would start if it landed at `target`. */
export function landingLeft(slots: readonly Slot[], held: number, target: number): number {
	const own = slots.at(held);
	const there = slots.at(target);
	if (own === undefined || there === undefined) {
		return own?.left ?? 0;
	}
	if (target > held) {
		return there.left + there.width - own.width;
	}
	return target < held ? there.left : own.left;
}

/**
 * The drag of the column at `held` by `dx`. Columns live between the first slot and the last: past
 * either, it rubber-bands. A neighbour makes room once the held column's centre crosses its
 * centre, and moves over by the held column's width and one gap.
 */
export function moveFrame(
	slots: readonly Slot[],
	held: number,
	dx: number,
	gap: number
): MoveFrame {
	const own = slots.at(held);
	const first = slots.at(0);
	const last = slots.at(-1);
	if (own === undefined || first === undefined || last === undefined) {
		return { offset: 0, shifts: slots.map(() => 0), target: held, holeLeft: own?.left ?? 0 };
	}
	const min = first.left;
	const max = last.left + last.width - own.width;
	let left = own.left + dx;
	if (left < min) {
		left = min - rubber(min - left);
	} else if (left > max) {
		left = max + rubber(left - max);
	}
	const offset = left - own.left;
	const centre = left + own.width / 2;
	const room = own.width + gap;
	let target = held;
	const shifts = slots.map((slot, index) => {
		const middle = slot.left + slot.width / 2;
		if (index > held && centre > middle) {
			target += 1;
			return -room;
		}
		if (index < held && centre < middle) {
			target -= 1;
			return room;
		}
		return index === held ? offset : 0;
	});
	return { offset, shifts, target, holeLeft: landingLeft(slots, held, target) };
}

/** `ids` with the one at `from` moved to `to`. */
export function movedTo<T>(ids: readonly T[], from: number, to: number): T[] {
	const out = [...ids];
	const [item] = out.splice(from, 1);
	if (item === undefined) {
		return out;
	}
	out.splice(Math.max(0, Math.min(to, out.length)), 0, item);
	return out;
}
