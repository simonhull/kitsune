import { describe, expect, it } from 'vitest';
import { emWidth } from './fit';

describe('emWidth', () => {
	it('falls back to a fair average with no canvas', () => {
		expect(emWidth('Phenomenon', 900, -0.07)).toBeCloseTo(5.8);
		expect(emWidth('', 900, -0.07)).toBe(0);
	});

	it('grows with the text', () => {
		expect(emWidth('Long Summer', 800, -0.05)).toBeGreaterThan(emWidth('Panes', 800, -0.05));
	});
});
