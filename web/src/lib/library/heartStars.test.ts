import { describe, expect, it } from 'vitest';
import { byHeartWords, counted, heartStarsValue, isHeartStars } from './heartStars.ts';

describe('counted', () => {
	it.each([
		// own, hearted, heart stars: what the song counts as
		[null, true, 0, null],
		[3.5, true, 0, { stars: 3.5, byHeart: false }],
		[null, true, 4, { stars: 4, byHeart: true }],
		[null, true, 4.5, { stars: 4.5, byHeart: true }],
		[null, true, 1, { stars: 1, byHeart: true }],
		[null, false, 5, null],
		[0.5, true, 5, { stars: 0.5, byHeart: false }],
		[3.5, true, 5, { stars: 3.5, byHeart: false }],
		[5, false, 4, { stars: 5, byHeart: false }]
	] as const)('own %s, hearted %s, hearts at %s', (own, hearted, heartStars, want) => {
		expect(counted(own, hearted, heartStars)).toEqual(want);
	});
});

describe('isHeartStars', () => {
	it('takes Off, and 1 to 5 in halves', () => {
		for (const ok of [0, 1, 1.5, 2.5, 4, 4.5, 5]) {
			expect(isHeartStars(ok)).toBe(true);
		}
		for (const bad of [0.5, 5.5, 4.25, -1, Number.NaN, '4', null, undefined]) {
			expect(isHeartStars(bad)).toBe(false);
		}
	});
});

describe('the words', () => {
	it('say the value as the You page does', () => {
		expect(heartStarsValue(0)).toBe('OFF');
		expect(heartStarsValue(4.5)).toBe('4½★ WHEN UNRATED');
		expect(heartStarsValue(1)).toBe('1★ WHEN UNRATED');
	});

	it('say where the stars came from, for a screen reader', () => {
		expect(byHeartWords(4)).toBe('4 stars from your heart. Rate it yourself to change it.');
		expect(byHeartWords(1)).toBe('1 star from your heart. Rate it yourself to change it.');
		expect(byHeartWords(4.5)).toBe('4½ stars from your heart. Rate it yourself to change it.');
	});
});
