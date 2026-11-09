import { describe, expect, it } from 'vitest';
import { fold, letterAt, letterLine } from './railFit';

const alphabet = [...'ABCDEFGHIJKLMNOPQRSTUVWXYZ'];

function drawn(step: number, letters = alphabet): string {
	const { shown } = fold(letters.length, step);
	return shown.map((index) => letters[index]).join('');
}

describe('fold', () => {
	it('draws every letter while each has a line', () => {
		expect(drawn(letterLine)).toBe(alphabet.join(''));
		expect(fold(26, 44).dots).toEqual([]);
	});

	it('draws every other letter just short of a line, with a dot in each gap', () => {
		const { shown, dots } = fold(26, 15);
		expect(drawn(15)).toBe('ACEGIKNPRTVXZ');
		expect(dots).toHaveLength(shown.length - 1);
		// Between A (slot 0) and C (slot 2): B's slot, whose centre is 1.5.
		expect(dots.at(0)).toBe(1.5);
	});

	it('draws every third letter when slots are shorter still', () => {
		expect(drawn(8)).toBe('ADGJNQTWZ');
	});

	it('always keeps the first and last letters', () => {
		for (const step of [15, 11, 8, 5, 3, 1, 0]) {
			const { shown } = fold(27, step);
			expect(shown.at(0)).toBe(0);
			expect(shown.at(-1)).toBe(26);
		}
	});

	it('never draws two letters closer than a folded pitch', () => {
		for (const step of [15, 12, 9, 7, 4]) {
			const { shown } = fold(27, step);
			const gaps = shown.slice(1).map((to, place) => to - (shown[place] ?? 0));
			expect(Math.min(...gaps) * step).toBeGreaterThanOrEqual(letterLine * 1.5);
		}
	});

	it('puts a dot only between letters that skip something', () => {
		for (const step of [15, 9, 4]) {
			const { shown, dots } = fold(27, step);
			for (const dot of dots) {
				const before = shown.findLast((index) => index + 0.5 < dot);
				const after = shown.find((index) => index + 0.5 > dot);
				expect(before).toBeDefined();
				expect(after).toBeDefined();
				expect((after ?? 0) - (before ?? 0)).toBeGreaterThan(1);
			}
		}
	});

	it('keeps a tiny rail to its ends', () => {
		expect(fold(26, 1).shown).toEqual([0, 25]);
		expect(fold(2, 1).shown).toEqual([0, 1]);
		expect(fold(1, 1).shown).toEqual([0]);
		expect(fold(0, 1).shown).toEqual([]);
	});
});

describe('letterAt', () => {
	it('picks the slot under the point', () => {
		expect(letterAt(0, 260, 26)).toBe(0);
		expect(letterAt(9.9, 260, 26)).toBe(0);
		expect(letterAt(10, 260, 26)).toBe(1);
		expect(letterAt(125, 260, 26)).toBe(12);
		expect(letterAt(259, 260, 26)).toBe(25);
	});

	it('picks letters a folded rail does not draw', () => {
		// At 8 px a slot the rail draws A, D, G…; the point over B's slot is still B.
		expect(fold(26, 8).shown).not.toContain(1);
		expect(letterAt(12, 208, 26)).toBe(1);
	});

	it('holds to the ends past the rail', () => {
		expect(letterAt(-40, 260, 26)).toBe(0);
		expect(letterAt(400, 260, 26)).toBe(25);
		expect(letterAt(260, 260, 26)).toBe(25);
	});

	it('is safe on an empty or unmeasured rail', () => {
		expect(letterAt(10, 0, 26)).toBe(0);
		expect(letterAt(10, 100, 0)).toBe(0);
	});
});
