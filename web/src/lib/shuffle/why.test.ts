import { describe, expect, it } from 'vitest';
import type { ShuffleWhy } from '#lib/library/source.ts';
import { agoWords, whyFactors, whyLine } from './why.ts';

const plain: ShuffleWhy = {
	stars: 0,
	rating: 1,
	heartStars: 0,
	hearted: false,
	heart: 1,
	played: true,
	sinceDays: 3,
	recency: 0.7,
	memory: true,
	earlySkips: 0,
	skipMemory: 1,
	starts: 0,
	skips: 0,
	skipRate: 1,
	listens: 0,
	heard: 0,
	listen: 1,
	flow: 1
};

describe('stars a heart gives', () => {
	const byHeart = { ...plain, stars: 4.5, heartStars: 1, hearted: true, heart: 1.25 };

	it('name their source on the line, in place of "hearted"', () => {
		expect(whyLine({ ...byHeart, sinceDays: 92 })).toBe('4½★ by heart · not heard in 3 months');
		expect(whyLine({ ...byHeart, stars: 4 })).toBe('4★ by heart');
	});

	it('say so in the popover, and keep the heart as its own factor', () => {
		const names = whyFactors({ ...byHeart, rating: 1.4 }).map(({ name }) => name);
		expect(names.slice(0, 2)).toEqual(['Rated 4½★ · from your heart', 'Hearted']);
	});

	it('leave a rating of the person’s own as it was', () => {
		expect(whyLine({ ...plain, stars: 4.5, hearted: true, heart: 1.25 })).toBe('4½★ · hearted');
	});
});

describe('whyLine', () => {
	it('says the board’s lines', () => {
		expect(whyLine({ ...plain, stars: 5, rating: 1.55, sinceDays: 92, recency: 1 })).toBe(
			'5★ · not heard in 3 months'
		);
		expect(
			whyLine({
				...plain,
				stars: 4,
				flow: 1.1,
				after: { id: 'a', name: 'Low Tide Choir' },
				similarity: 0.8
			})
		).toBe('Close to Low Tide Choir · 4★');
		expect(whyLine({ ...plain, hearted: true, heart: 1.25, listens: 9, heard: 0.97 })).toBe(
			'Hearted · you usually hear it all'
		);
		expect(whyLine({ ...plain, played: false, recency: 1.2 })).toBe(
			'Never played · a nudge for new'
		);
		expect(whyLine({ ...plain, sinceDays: 36 })).toBe('Unrated · not heard in 5 weeks');
		expect(whyLine({ ...plain, stars: 3 }, 2)).toBe('Combined · 2 songs');
	});

	it('says a skip, unless memory is off', () => {
		expect(whyLine({ ...plain, stars: 2, skipRate: 0.6, skips: 4, starts: 5 })).toBe(
			'2★ · you skip it often'
		);
		expect(whyLine({ ...plain, stars: 2, skipRate: 0.6, memory: false })).toBe('2★');
	});

	it('writes a half as ½', () => {
		expect(whyLine({ ...plain, stars: 4.5, rating: 1.4 })).toBe('4½★');
		expect(whyLine({ ...plain, stars: 0.5, rating: 0.3 })).toBe('½★');
		// A run's mean, to the nearest half.
		expect(whyLine({ ...plain, stars: 4.33, rating: 1.4 })).toBe('4½★');
	});
});

describe('agoWords', () => {
	it('rounds to the unit a person would say', () => {
		expect([0, 1, 5, 13, 14, 42, 59, 60, 92, 400, 800].map(agoWords)).toEqual([
			'today',
			'a day',
			'5 days',
			'13 days',
			'2 weeks',
			'6 weeks',
			'8 weeks',
			'2 months',
			'3 months',
			'a year',
			'2 years'
		]);
	});
});

describe('whyFactors', () => {
	it('lists every factor in words, with its multiplier and bar', () => {
		const factors = whyFactors({
			...plain,
			stars: 4,
			rating: 1.3,
			sinceDays: 42,
			recency: 1,
			starts: 9,
			listens: 9,
			heard: 0.95,
			flow: 1.1,
			after: { id: 'a', name: 'Low Tide Choir' }
		});
		expect(factors.map(({ name, times }) => `${name} ${times}`)).toEqual([
			'Rated 4★ ×1.3',
			'Last heard 6 weeks ago ×1.0',
			'Skipped 0 of 9 times ×1.0',
			'Usually heard to the end ×1.0',
			'Close to Low Tide Choir ×1.1'
		]);
		expect(factors[0]?.bar).toBeCloseTo(0.61, 2);
		expect(factors[0]?.lifts).toBe(true);
		expect(factors[1]?.bar).toBeCloseTo(0.48, 2);
		expect(factors[1]?.lifts).toBe(false);
	});

	it('shows a small lean, and a lean away, for what it is', () => {
		const names = whyFactors({ ...plain, flow: 0.97, after: { id: 'a', name: 'Ostra' } }).map(
			({ name, times }) => `${name} ${times}`
		);
		expect(names.at(-1)).toBe('Unlike Ostra ×0.97');
		expect(names).not.toContain('Skipped 0 of 0 times ×1.0');
	});

	it('leaves skips and listening out when memory is off', () => {
		const names = whyFactors({ ...plain, memory: false, hearted: true, heart: 1.25 }).map(
			({ name }) => name
		);
		expect(names).toEqual(['Unrated', 'Hearted', 'Last heard 3 days ago']);
	});
});
