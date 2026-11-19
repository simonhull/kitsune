import { describe, expect, it } from 'vitest';
import { clipInset, cornerOf, edgeRect, floorRect, sameRect, type Rect } from './bud.ts';

const menu: Rect = { left: 100, top: 200, width: 300, height: 491, radius: 20 };
const dots: Rect = { left: 380, top: 150, width: 44, height: 44, radius: 22 };

describe('clipInset', () => {
	it('shows the whole surface once its box is its own shape', () => {
		expect(clipInset(menu, menu)).toBe('inset(0.00px 0.00px 0.00px 0.00px round 20.00px)');
	});

	it('shows only what the box covers while it grows from its control', () => {
		const box: Rect = { left: 300, top: 180, width: 120, height: 100, radius: 22 };
		expect(clipInset(box, menu)).toBe('inset(-20.00px -20.00px 411.00px 200.00px round 22.00px)');
	});

	it('clips everything away while the box is still the control, beside the surface', () => {
		expect(clipInset(dots, menu)).toBe('inset(-50.00px -24.00px 497.00px 280.00px round 22.00px)');
	});

	it('clips everything away for a box with no size', () => {
		expect(clipInset(floorRect(menu), menu)).toBe('inset(50% 50% 50% 50%)');
	});

	it('never rounds a corner past half the box', () => {
		const box: Rect = { ...menu, height: 10 };
		expect(clipInset(box, menu)).toContain('round 5.00px');
	});
});

describe('where a surface is born', () => {
	it('is born at the floor at its own width, its top edge rising from its foot', () => {
		expect(floorRect(menu)).toEqual({ ...menu, top: 691, height: 0 });
	});

	it('is born at its own top edge when no control is known', () => {
		expect(edgeRect(menu)).toEqual({ ...menu, height: 0 });
	});
});

describe('cornerOf', () => {
	it('keeps a corner inside half the shorter side', () => {
		expect(cornerOf(999, 44, 44)).toBe(22);
		expect(cornerOf(12, 284, 40)).toBe(12);
		expect(cornerOf(-3, 10, 10)).toBe(0);
	});
});

describe('sameRect', () => {
	it('reads under half a pixel as the same shape', () => {
		expect(sameRect(menu, { ...menu, left: 100.3 })).toBe(true);
		expect(sameRect(menu, { ...menu, width: 301 })).toBe(false);
	});
});
