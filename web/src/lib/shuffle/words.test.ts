import { describe, expect, it } from 'vitest';
import {
	fabWords,
	flowWord,
	ratingsSentence,
	ratingsWord,
	settingsLine,
	songsLine
} from './words.ts';

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
			'A 5★ song comes up about 1.5 times as often as an unrated one, and a 1★ about half as often.'
		);
		expect(ratingsSentence(1)).toContain('about 2.4 times as often');
		expect(ratingsSentence(1)).toContain('a 1★ about a fifth as often');
		expect(ratingsSentence(0.25)).toContain('about 1.2 times');
		expect(ratingsSentence(0)).toBe(
			'Ratings change nothing: every song comes up as often as any other.'
		);
		expect(ratingsSentence(0.5)).toBe(
			'A 5★ song comes up about 1.5 times as often as an unrated one, and a 1★ about half as often.'
		);
	});

	it('names the Library’s Shuffle for what it shuffles', () => {
		expect(fabWords('everything')).toEqual({
			name: 'Shuffle the library',
			what: 'The library',
			tag: ''
		});
		expect(fabWords('rated')).toEqual({
			name: 'Shuffle these: 4★ and up',
			what: '4★ and up',
			tag: '4★'
		});
	});

	it('counts the songs an endless shuffle draws from', () => {
		expect(songsLine(9812)).toBe('9,812 SONGS · ENDLESS');
		expect(songsLine(1)).toBe('1 SONG · ENDLESS');
		expect(songsLine(0)).toBe('0 SONGS · ENDLESS');
	});
});
