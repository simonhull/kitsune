import { describe, expect, it } from 'vitest';
import {
	dealInterval,
	expectedGap,
	openingFor,
	queueCap,
	restOf,
	seedGapMs,
	shrinkingDelays,
	wobble
} from './deck';

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

describe('restOf', () => {
	it('keeps every card within 9 degrees and 14 pixels', () => {
		for (let index = 0; index < 200; index += 1) {
			const [, x, y, tilt] =
				/translate\((.+)px, (.+)px\) rotate\((.+)deg\)/.exec(restOf(`album-${String(index)}`)) ??
				[];
			expect(Math.abs(Number(x))).toBeLessThanOrEqual(14);
			expect(Math.abs(Number(y))).toBeLessThanOrEqual(14);
			expect(Math.abs(Number(tilt))).toBeLessThanOrEqual(9);
		}
	});

	it('gives the same album the same place', () => {
		expect(restOf('album-1')).toBe(restOf('album-1'));
		expect(restOf('album-1')).not.toBe(restOf('album-2'));
	});
});

describe('shrinkingDelays', () => {
	const ids = Array.from({ length: 12 }, (_, index) => `album-${String(index)}`);

	it('starts at once, and every step is shorter than the last', () => {
		const delays = shrinkingDelays(ids, 30);
		expect(delays.at(0)).toBe(0);
		expect(delays.at(1)).toBe(30);
		const steps = delays.slice(1).map((delay, index) => delay - (delays.at(index) ?? 0));
		for (const [index, step] of steps.slice(1).entries()) {
			expect(step).toBeLessThan(steps.at(index) ?? 0);
		}
	});

	it('is not uniform: the steps shrink by different amounts', () => {
		const delays = shrinkingDelays(ids, 30);
		const steps = delays.slice(1).map((delay, index) => delay - (delays.at(index) ?? 0));
		const ratios = steps.slice(1).map((step, index) => step / (steps.at(index) ?? 1));
		expect(new Set(ratios.map((ratio) => ratio.toFixed(3))).size).toBeGreaterThan(3);
	});

	it('leaves a dozen cards time to land by about 600 ms', () => {
		expect(shrinkingDelays(ids, 30).at(-1)).toBeLessThanOrEqual(220);
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
