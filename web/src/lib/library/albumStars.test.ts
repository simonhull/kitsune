import { describe, expect, it } from 'vitest';
import { albumStars, starsLabel } from './albumStars';

function rated(stars: Record<string, number>) {
	return { stars: (id: string): number | null => stars[id] ?? null };
}

const tracks = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j'].map((id) => ({ id }));

describe('albumStars', () => {
	it('is the mean of the rated songs, to one place, with how many', () => {
		// Board A's Glasshouse: 5, 3, 4, 2, 5, 4 is 23 over 6, 3.83.
		const marks = rated({ a: 5, b: 3, e: 4, f: 2, g: 5, i: 4 });
		expect(albumStars(tracks, marks)).toEqual({ mean: 3.8, rated: 6, of: 10 });
	});

	it('rounds a mean half way up', () => {
		expect(albumStars(tracks.slice(0, 4), rated({ a: 4, b: 5, c: 4, d: 4 }))).toEqual({
			mean: 4.3,
			rated: 4,
			of: 4
		});
		// 3.25 and 3.75 sit on the line between two places.
		expect(albumStars(tracks, rated({ a: 3, b: 3, c: 3, d: 4 }))?.mean).toBe(3.3);
		expect(albumStars(tracks, rated({ a: 4, b: 4, c: 4, d: 3 }))?.mean).toBe(3.8);
	});

	it('leaves the unrated out of the mean, not in as zeroes', () => {
		expect(albumStars(tracks, rated({ c: 2 }))).toEqual({ mean: 2, rated: 1, of: 10 });
	});

	it('is nothing for an album with nothing rated', () => {
		expect(albumStars(tracks, rated({}))).toBeNull();
		expect(albumStars([], rated({}))).toBeNull();
	});
});

describe('albumStars with halves', () => {
	it('takes halves into the mean, to one place', () => {
		// 4½, 4 and 3½ are 12 over 3: 4.
		expect(albumStars(tracks, rated({ a: 4.5, b: 4, c: 3.5 }))?.mean).toBe(4);
		// 4½ and 5 are 4.75, which reads as 4.8.
		expect(albumStars(tracks, rated({ a: 4.5, b: 5 }))?.mean).toBe(4.8);
		// ½ alone is ½.
		expect(albumStars(tracks, rated({ a: 0.5 }))).toEqual({ mean: 0.5, rated: 1, of: 10 });
		// 4½, 4½ and 4 are 4.33, which reads as 4.3.
		expect(albumStars(tracks, rated({ a: 4.5, b: 4.5, c: 4 }))?.mean).toBe(4.3);
	});
});

describe('starsLabel', () => {
	it('says the mean and how many it is from', () => {
		expect(starsLabel({ mean: 3.8, rated: 6, of: 10 })).toBe(
			'Averages 3.8 of 5 from 6 rated songs'
		);
		expect(starsLabel({ mean: 4, rated: 1, of: 3 })).toBe('Averages 4 of 5 from 1 rated song');
	});
});
