import { describe, expect, it } from 'vitest';
import {
	landingLeft,
	moveFrame,
	movedTo,
	relaid,
	rubber,
	rubberMax,
	type Slot
} from './columnMove.ts';

const gap = 12;
/** Four columns, 100, 50, 80 and 60 wide, a gap apart. */
const slots: Slot[] = [
	{ left: 0, width: 100 },
	{ left: 112, width: 50 },
	{ left: 174, width: 80 },
	{ left: 266, width: 60 }
];

describe('moveFrame', () => {
	it('stays put with no movement', () => {
		expect(moveFrame(slots, 1, 0, gap)).toEqual({
			offset: 0,
			shifts: [0, 0, 0, 0],
			target: 1,
			holeLeft: 112
		});
	});

	it('moves a neighbour over once the centre crosses its centre, to the right', () => {
		// Held centre 137; the next column's centre is 214.
		expect(moveFrame(slots, 1, 76, gap).target).toBe(1);
		const crossed = moveFrame(slots, 1, 78, gap);
		expect(crossed.target).toBe(2);
		expect(crossed.shifts).toEqual([0, 78, -62, 0]);
		// It would end where the column it passed ended.
		expect(crossed.holeLeft).toBe(174 + 80 - 50);
	});

	it('moves neighbours over to the left', () => {
		const crossed = moveFrame(slots, 2, -170, gap);
		expect(crossed.target).toBe(0);
		expect(crossed.shifts).toEqual([92, 92, -170, 0]);
		expect(crossed.holeLeft).toBe(0);
	});

	it('rubber-bands past either end, never more than 18 px', () => {
		const left = moveFrame(slots, 0, -1000, gap);
		expect(left.offset).toBeLessThan(0);
		expect(left.offset).toBeGreaterThan(-rubberMax);
		expect(left.target).toBe(0);
		const right = moveFrame(slots, 3, 500, gap);
		expect(right.offset).toBeGreaterThan(0);
		expect(right.offset).toBeLessThan(rubberMax);
		expect(right.target).toBe(3);
		// Up to the wall, it follows the pointer exactly.
		expect(moveFrame(slots, 0, 226, gap).offset).toBe(226);
	});

	it('returns the start index for a drop in place', () => {
		expect(moveFrame(slots, 2, 10, gap).target).toBe(2);
		expect(moveFrame(slots, 2, -10, gap).target).toBe(2);
	});

	it('puts the hole at each target', () => {
		expect([0, 1, 2, 3].map((target) => landingLeft(slots, 1, target))).toEqual([0, 112, 204, 276]);
	});
});

describe('rubber', () => {
	it('gives less the further it goes', () => {
		expect(rubber(0)).toBe(0);
		expect(rubber(-5)).toBe(0);
		expect(rubber(30)).toBeLessThan(rubber(60));
		expect(rubber(60) - rubber(30)).toBeLessThan(rubber(30));
		expect(rubber(10_000)).toBeLessThanOrEqual(rubberMax);
	});
});

describe('movedTo', () => {
	it('moves one item', () => {
		expect(movedTo(['a', 'b', 'c'], 0, 2)).toEqual(['b', 'c', 'a']);
		expect(movedTo(['a', 'b', 'c'], 2, 0)).toEqual(['c', 'a', 'b']);
		expect(movedTo(['a', 'b', 'c'], 1, 9)).toEqual(['a', 'c', 'b']);
	});
});

describe('relaid', () => {
	const before = new Map(slots.map((slot, index) => [`c${String(index)}`, slot] as const));

	it('places the same widths in the new order, a gap apart, from the same start', () => {
		expect([...(relaid(before, ['c1', 'c0', 'c2', 'c3'], gap) ?? [])]).toEqual([
			['c1', 0],
			['c0', 62],
			['c2', 174],
			['c3', 266]
		]);
	});

	it('is the layout before when nothing moved', () => {
		const same = relaid(before, ['c0', 'c1', 'c2', 'c3'], gap);
		expect([...(same?.values() ?? [])]).toEqual(slots.map(({ left }) => left));
	});

	it('knows nothing of columns that weren’t there', () => {
		expect(relaid(before, ['c0', 'c1', 'c2'], gap)).toBeNull();
		expect(relaid(before, ['c0', 'c1', 'c2', 'c9'], gap)).toBeNull();
	});
});
