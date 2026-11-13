import { describe, expect, it } from 'vitest';
import { roamTo } from './roving.ts';

const plain = { slider: false, shift: false };

describe('roamTo', () => {
	it('moves along with ← and →, stopping at either end, and jumps with Home and End', () => {
		expect(roamTo('ArrowRight', 2, 7, plain)).toBe(3);
		expect(roamTo('ArrowLeft', 2, 7, plain)).toBe(1);
		expect(roamTo('ArrowLeft', 0, 7, plain)).toBe(0);
		expect(roamTo('ArrowRight', 6, 7, plain)).toBe(6);
		expect(roamTo('Home', 4, 7, plain)).toBe(0);
		expect(roamTo('End', 1, 7, plain)).toBe(6);
		expect(roamTo('Tab', 1, 7, plain)).toBeNull();
		expect(roamTo('Enter', 1, 7, plain)).toBeNull();
	});

	it('leaves a slider its arrows, Home and End, and hands Tab on to the item beside it', () => {
		const slider = { slider: true, shift: false };
		for (const key of ['ArrowLeft', 'ArrowRight', 'Home', 'End']) {
			expect(roamTo(key, 1, 7, slider)).toBeNull();
		}
		expect(roamTo('Tab', 1, 7, slider)).toBe(2);
		expect(roamTo('Tab', 1, 7, { slider: true, shift: true })).toBe(0);
		// Past either end, Tab leaves the toolbar as it would anywhere.
		expect(roamTo('Tab', 6, 7, slider)).toBeNull();
		expect(roamTo('Tab', 0, 7, { slider: true, shift: true })).toBeNull();
	});
});
