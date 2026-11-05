import { describe, expect, it } from 'vitest';
import { openingFor, restOf, shrinkingDelays, spinOf, throwGap, wobble } from './deck';

describe('throwGap', () => {
	const ids = Array.from({ length: 50 }, (_, index) => `album-${String(index)}`);

	it('leaves about 70 ms with nothing waiting', () => {
		for (const id of ids) {
			expect(throwGap(0, id)).toBeGreaterThanOrEqual(70 * 0.8);
			expect(throwGap(0, id)).toBeLessThanOrEqual(70 * 1.2);
		}
	});

	it('hurries to about 45 ms under a backlog, and no further however long it gets', () => {
		for (const id of ids) {
			expect(throwGap(200, id)).toBeGreaterThanOrEqual(45 * 0.8);
			expect(throwGap(200, id)).toBeLessThanOrEqual(45 * 1.2);
			expect(throwGap(200, id)).toBe(throwGap(24, id));
		}
	});

	it('hurries gradually', () => {
		expect(throwGap(12, 'a')).toBeLessThan(throwGap(0, 'a'));
		expect(throwGap(12, 'a')).toBeGreaterThan(throwGap(24, 'a'));
	});

	it('is the same for the same album, and differs between albums', () => {
		expect(throwGap(0, 'a')).toBe(throwGap(0, 'a'));
		expect(new Set(ids.map((id) => throwGap(0, id))).size).toBeGreaterThan(1);
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

describe('spinOf', () => {
	it('stays small, and turns both ways', () => {
		const spins = Array.from({ length: 100 }, (_, index) => spinOf(`album-${String(index)}`));
		expect(Math.max(...spins.map(Math.abs))).toBeLessThanOrEqual(7);
		expect(Math.min(...spins)).toBeLessThan(0);
		expect(Math.max(...spins)).toBeGreaterThan(0);
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
