import { describe, expect, it } from 'vitest';
import { bestDecoded, firstSight, markDecoded, rungFor, rungForBox } from './rungs';

const big = { width: 1400, height: 1400 };

describe('rungFor', () => {
	it('keeps small places small and cards at 512', () => {
		expect(rungFor(big, 'small')).toBe(128);
		expect(rungFor(big, 'card')).toBe(512);
	});

	it('gives a stage 1024 only when the source has it to give', () => {
		expect(rungFor(big, 'stage')).toBe(1024);
		expect(rungFor({ width: 641, height: 300 }, 'stage')).toBe(1024);
		expect(rungFor({ width: 640, height: 640 }, 'stage')).toBe(512);
	});
});

describe('the decoded map', () => {
	it('only ever goes up', () => {
		markDecoded('a', 512);
		markDecoded('a', 128);
		expect(bestDecoded('a')).toBe(512);
	});

	it('fades a cover in once', () => {
		expect(firstSight('b')).toBe(true);
		expect(firstSight('b')).toBe(false);
	});
});

describe('rungForBox', () => {
	it('takes the smallest rung that covers the box, density included', () => {
		expect(rungForBox(big, 112)).toBe(128);
		expect(rungForBox(big, 224)).toBe(512);
		expect(rungForBox(big, 512)).toBe(512);
		expect(rungForBox(big, 840)).toBe(1024);
	});
	it('stops at 512 for a source with no more to give', () => {
		expect(rungForBox({ width: 500, height: 480 }, 840)).toBe(512);
	});
});
