import { describe, expect, it } from 'vitest';
import {
	barLift,
	barRise,
	carryOffset,
	dissolveOpacities,
	dissolveTarget,
	sharedWidth,
	unit
} from './path';

describe('barLift', () => {
	it('stays on the floor until the bar is past the buttons', () => {
		expect(barLift(0)).toBe(0);
		expect(barLift(0.5)).toBe(0);
		expect(barLift(0.85)).toBe(0);
	});

	it('ends in the track', () => {
		expect(barLift(1)).toBe(1);
	});

	it('only ever rises, so the way back is the same path', () => {
		const heights = Array.from({ length: 21 }, (_, step) => barLift(step / 20));
		expect(heights).toEqual([...heights].sort((a, b) => a - b));
	});

	it('survives a spring running past either end', () => {
		expect(barLift(-0.01)).toBe(0);
		expect(barLift(1.01)).toBe(1);
	});
});

describe('barRise', () => {
	it('runs from the floor height to the track height', () => {
		expect(barRise(0, 4, 40)).toBe(4);
		expect(barRise(1, 4, 40)).toBe(40);
	});
});

describe('sharedWidth', () => {
	it('is each picture true width at its own end', () => {
		expect(sharedWidth(120, 900, 0)).toBe(120);
		expect(sharedWidth(120, 900, 1)).toBe(900);
	});
});

describe('dissolveTarget', () => {
	it('leaves the small picture straight away on the way up', () => {
		expect(dissolveTarget(0.01, true)).toBe(0);
		expect(dissolveTarget(0.03, true)).toBe(1);
	});

	it('holds the large picture until near the end on the way down', () => {
		expect(dissolveTarget(0.5, false)).toBe(1);
		expect(dissolveTarget(0.05, false)).toBe(0);
	});
});

describe('dissolveOpacities', () => {
	it('shows one picture at each end', () => {
		expect(dissolveOpacities(0)).toEqual({ small: 1, large: 0 });
		expect(dissolveOpacities(1)).toEqual({ small: 0, large: 1 });
	});

	it('never dips: at least one picture is fully on', () => {
		for (const dissolve of [0.1, 0.3, 0.5, 0.7, 0.9]) {
			const { small, large } = dissolveOpacities(dissolve);
			expect(Math.max(small, large)).toBe(1);
		}
	});
});

describe('carryOffset', () => {
	it('is zero when the glass is at rest', () => {
		expect(carryOffset({ x: 5, y: 9 }, { x: 5, y: 9 })).toEqual({ x: 0, y: 0 });
	});

	it('follows the glass', () => {
		expect(carryOffset({ x: 25, y: 3 }, { x: 5, y: 9 })).toEqual({ x: 20, y: -6 });
	});
});

describe('unit', () => {
	it('clamps', () => {
		expect(unit(5, 0, 10)).toBe(0.5);
		expect(unit(-1, 0, 10)).toBe(0);
		expect(unit(11, 0, 10)).toBe(1);
	});
});
