import { describe, expect, it } from 'vitest';
import {
	fillOf,
	halfAt,
	keyed,
	pressed,
	shownHalves,
	starAt,
	starsLine,
	starsText,
	starsWords,
	tapped
} from './stars';

describe('halfAt', () => {
	it('takes the half under the pointer: the left of the fourth star is 3½, its right 4', () => {
		expect(halfAt(0.65)).toBe(3.5);
		expect(halfAt(0.7)).toBe(3.5);
		expect(halfAt(0.71)).toBe(4);
		expect(halfAt(0.8)).toBe(4);
	});

	it('changes at every boundary, a tenth of the way, and nowhere else', () => {
		for (let step = 1; step <= 10; step++) {
			expect(halfAt(step / 10)).toBe(step / 2);
			expect(halfAt(step / 10 - 0.001)).toBe(step / 2);
			if (step < 10) {
				expect(halfAt(step / 10 + 0.001)).toBe((step + 1) / 2);
			}
		}
	});

	it('keeps to ½ and 5 at and past the edges', () => {
		expect(halfAt(0)).toBe(0.5);
		expect(halfAt(-0.2)).toBe(0.5);
		expect(halfAt(1)).toBe(5);
		expect(halfAt(1.3)).toBe(5);
	});
});

describe('starAt', () => {
	it('takes the whole star under a finger', () => {
		expect(starAt(0)).toBe(1);
		expect(starAt(0.19)).toBe(1);
		expect(starAt(0.21)).toBe(2);
		expect(starAt(0.99)).toBe(5);
		expect(starAt(1.4)).toBe(5);
	});
});

describe('pressed', () => {
	it('sets the half pressed, and clears the value a song already has', () => {
		expect(pressed(null, 3.5)).toBe(3.5);
		expect(pressed(4, 3.5)).toBe(3.5);
		expect(pressed(3.5, 3.5)).toBeNull();
		expect(pressed(4, 4)).toBeNull();
	});
});

describe('tapped', () => {
	it('cycles a star: k, then k less a half, then nothing, then k again', () => {
		let stars: number | null = null;
		const seen: (number | null)[] = [];
		for (let tap = 0; tap < 4; tap++) {
			stars = tapped(stars, 4);
			seen.push(stars);
		}
		expect(seen).toEqual([4, 3.5, null, 4]);
	});

	it('goes straight to another star from anything', () => {
		expect(tapped(4, 2)).toBe(2);
		expect(tapped(3.5, 5)).toBe(5);
		expect(tapped(1, 1)).toBe(0.5);
		expect(tapped(0.5, 1)).toBeNull();
	});
});

describe('keyed', () => {
	it('moves a half with the arrows, and clears below ½', () => {
		expect(keyed(3, 'ArrowRight')).toBe(3.5);
		expect(keyed(3, 'ArrowUp')).toBe(3.5);
		expect(keyed(null, 'ArrowRight')).toBe(0.5);
		expect(keyed(5, 'ArrowRight')).toBe(5);
		expect(keyed(3.5, 'ArrowLeft')).toBe(3);
		expect(keyed(1, 'ArrowDown')).toBe(0.5);
		expect(keyed(0.5, 'ArrowLeft')).toBeNull();
		expect(keyed(null, 'ArrowLeft')).toBeNull();
	});

	it('sets whole stars with the numbers, five with End, none with Home or Delete', () => {
		expect(keyed(null, '4')).toBe(4);
		expect(keyed(2.5, '1')).toBe(1);
		expect(keyed(2, 'End')).toBe(5);
		for (const key of ['Home', 'Delete', 'Backspace']) {
			expect(keyed(4.5, key)).toBeNull();
		}
	});

	it('leaves every other key alone', () => {
		for (const key of ['0', '6', 'a', 'Enter', ' ', 'Tab']) {
			expect(keyed(3, key)).toBeUndefined();
		}
	});
});

describe('drawing', () => {
	it('fills each star full, by half or not at all', () => {
		expect([1, 2, 3, 4, 5].map((place) => fillOf(3.5, place))).toEqual([
			'full',
			'full',
			'full',
			'half',
			''
		]);
		expect([1, 2, 3, 4, 5].map((place) => fillOf(0.5, place))).toEqual(['half', '', '', '', '']);
		expect([1, 2, 3, 4, 5].map((place) => fillOf(null, place))).toEqual(['', '', '', '', '']);
	});

	it('draws a mean to the nearest half', () => {
		expect(shownHalves(4.4)).toBe(4.5);
		expect(shownHalves(4.2)).toBe(4);
		expect(shownHalves(4.25)).toBe(4.5);
		expect(shownHalves(3.8)).toBe(4);
		expect(shownHalves(0.5)).toBe(0.5);
	});

	it('writes a rating as text and as one line', () => {
		expect(starsText(4.5)).toBe('4½');
		expect(starsText(0.5)).toBe('½');
		expect(starsText(5)).toBe('5');
		expect(starsLine(4.5)).toBe('★★★★½');
		expect(starsLine(0.5)).toBe('½');
		expect(starsLine(3)).toBe('★★★');
		expect(starsWords(4.5)).toBe('4½ stars');
		expect(starsWords(1)).toBe('1 star');
		expect(starsWords(0.5)).toBe('½ star');
		expect(starsWords(null)).toBe('Unrated');
	});
});
