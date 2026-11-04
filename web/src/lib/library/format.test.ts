import { describe, expect, it } from 'vitest';
import { balancedLines, clock, gain } from './format';

describe('clock', () => {
	it('pads seconds', () => {
		expect(clock(102)).toBe('1:42');
		expect(clock(61)).toBe('1:01');
	});
});

describe('gain', () => {
	it('uses a true minus sign', () => {
		expect(gain(-6.2)).toBe('−6.2');
		expect(gain(1)).toBe('+1.0');
	});
});

describe('balancedLines', () => {
	it('breaks where the board does', () => {
		expect(balancedLines('Salt in the Lungs')).toEqual(['Salt in', 'the Lungs']);
	});

	it('leaves a single word alone', () => {
		expect(balancedLines('Panes')).toEqual(['Panes', '']);
	});
});
