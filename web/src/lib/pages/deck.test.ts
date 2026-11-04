import { describe, expect, it } from 'vitest';
import { dealInterval, expectedGap, openingFor, queueCap, seedGapMs, wobble } from './deck';

describe('dealInterval', () => {
	it('spreads a burst over the gap, within a quarter either side', () => {
		const even = seedGapMs / 60;
		for (const id of ['a', 'b', 'c', 'd', 'e']) {
			const interval = dealInterval(60, seedGapMs, id);
			expect(interval).toBeGreaterThanOrEqual(even * 0.75);
			expect(interval).toBeLessThanOrEqual(even * 1.25);
		}
	});

	it('never rushes a big burst or stalls on a small one', () => {
		expect(dealInterval(1000, seedGapMs, 'a')).toBeGreaterThanOrEqual(180 * 0.75);
		expect(dealInterval(1, seedGapMs, 'a')).toBeLessThanOrEqual(3000 * 1.25);
	});

	it('survives an empty queue', () => {
		expect(Number.isFinite(dealInterval(0, seedGapMs, 'a'))).toBe(true);
	});

	it('is the same for the same album, and differs between albums', () => {
		expect(dealInterval(60, seedGapMs, 'a')).toBe(dealInterval(60, seedGapMs, 'a'));
		const spread = new Set(['a', 'b', 'c', 'd'].map((id) => dealInterval(60, seedGapMs, id)));
		expect(spread.size).toBeGreaterThan(1);
	});
});

describe('wobble', () => {
	it('stays between -1 and 1', () => {
		for (const id of ['a', 'scan-12', 'night-ferry-94', '']) {
			expect(Math.abs(wobble(id))).toBeLessThanOrEqual(1);
		}
	});

	it('gives each salt its own value', () => {
		expect(wobble('album', 'tilt')).not.toBe(wobble('album', 'x'));
	});
});

describe('expectedGap', () => {
	it('moves three tenths of the way to what was measured', () => {
		expect(expectedGap(27_000, 37_000)).toBeCloseTo(30_000);
		expect(expectedGap(27_000, 27_000)).toBe(27_000);
	});
});

describe('queueCap', () => {
	it('holds at least 40 and at most 100', () => {
		expect(queueCap(3000)).toBe(40);
		expect(queueCap(seedGapMs)).toBe(40);
		expect(queueCap(180_000)).toBe(60);
		expect(queueCap(3_000_000)).toBe(100);
	});
});

describe('openingFor', () => {
	const fresh = { remembered: false, waited: false };

	it('opens the deck on an empty library being scanned', () => {
		expect(openingFor({ albums: 0, roots: 1 }, false, fresh)).toBe('deck');
	});

	it('keeps the plain line when there is nothing to scan', () => {
		expect(openingFor({ albums: 0, roots: 0 }, false, fresh)).toBe('home');
	});

	it('opens Home once ready, whatever the counts say', () => {
		expect(openingFor({ albums: 0, roots: 1 }, true, fresh)).toBe('home');
		expect(openingFor({ albums: 9, roots: 1 }, true, fresh)).toBe('home');
	});

	it('waits a moment before believing a library with albums is still on its first scan', () => {
		expect(openingFor({ albums: 9, roots: 1 }, false, fresh)).toBe('wait');
		expect(openingFor({ albums: 9, roots: 1 }, false, { ...fresh, waited: true })).toBe('deck');
	});

	it('takes a browser that has seen this library ready at its word: this is a restart', () => {
		expect(openingFor({ albums: 9, roots: 1 }, false, { ...fresh, remembered: true })).toBe('home');
	});
});
