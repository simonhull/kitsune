import { describe, expect, it } from 'vitest';
import type { HeartsSummary } from '#lib/library/source.ts';
import {
	commonLine,
	defaultStars,
	goesToo,
	hasHearts,
	heartsLine,
	loveFromValue,
	loveHint,
	rateLabel
} from './heartsCard.ts';

/** Rupert's hearts, as the board has them. */
const rupert: HeartsSummary = {
	tracksRated: 367,
	tracksUnrated: 271,
	albums: 5,
	artists: 2,
	byStars: [0, 0, 1, 185, 181],
	preview: []
};

describe('the card’s default', () => {
	it('is the most common rating among hearted, rated songs', () => {
		expect(defaultStars(rupert.byStars)).toBe(4);
		expect(defaultStars([0, 9, 0, 1, 2])).toBe(2);
	});

	it('takes the higher star on a tie', () => {
		expect(defaultStars([0, 0, 0, 3, 3])).toBe(5);
	});

	it('is 4★ when none of them is rated', () => {
		expect(defaultStars([0, 0, 0, 0, 0])).toBe(4);
		expect(defaultStars([])).toBe(4);
	});
});

describe('the card’s words', () => {
	it('say the board’s lines', () => {
		expect(heartsLine(rupert)).toBe(
			'You hearted 638 songs. 367 of them have stars of their own and keep them; only the heart goes. 271 have no stars yet.'
		);
		expect(commonLine(rupert.byStars)).toBe(
			'Most songs you hearted and rated are 4★ (185) or 5★ (181).'
		);
		expect(rateLabel(271, 4)).toBe('Rate 271 songs 4★');
		expect(goesToo(rupert)).toBe(
			'Album and artist hearts (5 and 2) go too. How much you like an artist comes from how you rate their songs.'
		);
	});

	it('fit a person with fewer kinds of heart', () => {
		const songsOnly = { ...rupert, tracksRated: 0, tracksUnrated: 1, albums: 0, artists: 0 };
		expect(heartsLine(songsOnly)).toBe('You hearted 1 song. None of them has stars yet.');
		expect(rateLabel(1, 5)).toBe('Rate 1 song 5★');
		expect(goesToo(songsOnly)).toBe('');
		expect(commonLine([0, 0, 0, 0, 0])).toBe('');
		expect(commonLine([0, 0, 0, 2, 0])).toBe('Most songs you hearted and rated are 4★ (2).');
		const allRated = { ...rupert, tracksUnrated: 0, albums: 1, artists: 0 };
		expect(heartsLine(allRated)).toBe(
			'You hearted 367 songs. All of them have stars of their own and keep them; only the heart goes.'
		);
		expect(goesToo(allRated)).toBe(
			'Album hearts (1) go too. How much you like an artist comes from how you rate their songs.'
		);
	});

	it('is drawn while any heart is left, and never after', () => {
		expect(hasHearts(rupert)).toBe(true);
		expect(hasHearts({ ...rupert, tracksRated: 0, tracksUnrated: 0, albums: 0, artists: 1 })).toBe(
			true
		);
		expect(hasHearts({ ...rupert, tracksRated: 0, tracksUnrated: 0, albums: 0, artists: 0 })).toBe(
			false
		);
	});
});

describe('the Last.fm love line', () => {
	it('says the line', () => {
		expect(loveFromValue(4)).toBe('4★ AND UP');
		expect(loveFromValue(4.5)).toBe('4½★ AND UP');
		expect(loveFromValue(0.5)).toBe('½★ AND UP');
	});

	it('counts the songs over it, and those the hearts will add', () => {
		const rule = 'Rating a song 4★ or more loves it on Last.fm; rating it lower un-loves it.';
		const kept = 'Loves already on Last.fm stay as they are.';
		expect(loveHint(4, 530, { songs: 271, stars: 4 })).toBe(
			`${rule} That's 801 songs once your hearts become stars. ${kept}`
		);
		// Under the line, the hearts' stars add nothing to it.
		expect(loveHint(4, 530, { songs: 271, stars: 3 })).toBe(
			`${rule} That's 530 songs once your hearts become stars. ${kept}`
		);
		expect(loveHint(4, 1, null)).toBe(`${rule} That's 1 song. ${kept}`);
		// Before the songs are here, it says the rule alone.
		expect(loveHint(4, null, null)).toBe(`${rule} ${kept}`);
		expect(loveHint(4.5, 2, null)).toContain('Rating a song 4½★ or more');
	});
});
