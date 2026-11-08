import { describe, expect, it } from 'vitest';
import { flowWord, ratingsSentence, ratingsWord, settingsLine } from './words.ts';

describe('shuffle words', () => {
	it('says the board’s line at the defaults', () => {
		expect(settingsLine({ ratings: 0.5, flow: 0.3, memory: true })).toBe(
			'RATINGS GENTLY · A LITTLE FLOW · LEARNS FROM SKIPS'
		);
		expect(settingsLine({ ratings: 0, flow: 0, memory: false })).toBe(
			'RATINGS OFF · NO FLOW · IGNORES SKIPS'
		);
	});

	it('labels each setting', () => {
		expect([0, 0.2, 0.5, 0.8, 1].map(ratingsWord)).toEqual([
			'NOT AT ALL',
			'A LITTLE',
			'GENTLY',
			'A LOT',
			'STRONGLY'
		]);
		expect([0, 0.3, 0.6, 1].map(flowWord)).toEqual(['OFF', 'A LITTLE', 'SOME', 'A LOT']);
	});

	it('says what ratings do at each value, from the server’s curve', () => {
		expect(ratingsSentence(0.5)).toBe(
			'A 5★ song comes up about 1.5 times as often as an unrated one, and a 1★ about half as often. Hearts give a small lift.'
		);
		expect(ratingsSentence(1)).toContain('about 2.4 times as often');
		expect(ratingsSentence(1)).toContain('a 1★ about a fifth as often');
		expect(ratingsSentence(0.25)).toContain('about 1.2 times');
		expect(ratingsSentence(0)).toBe(
			'Ratings and hearts change nothing: every song comes up as often as any other.'
		);
	});
});
