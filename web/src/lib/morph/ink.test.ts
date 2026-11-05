import { describe, expect, it } from 'vitest';
import { offRegister } from './ink.ts';

describe('offRegister', () => {
	const ids = Array.from({ length: 200 }, (_, index) => `album-${String(index)}`);

	it('keeps every album 2 to 3 pixels off', () => {
		for (const id of ids) {
			const { x, y } = offRegister(id);
			const reach = Math.hypot(x, y);
			expect(reach).toBeGreaterThan(1.999);
			expect(reach).toBeLessThan(3.001);
		}
	});

	it('slips the same way every time, and differently from album to album', () => {
		expect(offRegister('album-1')).toEqual(offRegister('album-1'));
		const quadrants = new Set(
			ids.map((id) => {
				const { x, y } = offRegister(id);
				return `${String(Math.sign(x))}${String(Math.sign(y))}`;
			})
		);
		expect(quadrants.size).toBe(4);
	});
});
