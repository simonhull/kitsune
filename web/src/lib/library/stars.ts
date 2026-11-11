/**
 * A rating, as the app sets and draws it: ½ to 5 stars in halves, or null for none. Everything here
 * is pure, so the slider's behavior is tested without a page.
 */

/** The highest rating. */
export const maxStars = 5;

/** The lowest rating, ½. */
export const minStars = 0.5;

/** How a rating changed: by a pointer or a finger, which is done, or by a key, which may go on. */
export type StarsBy = 'pointer' | 'key';

/** How a star is drawn: filled, its left half filled, or empty. */
export type StarFill = 'full' | 'half' | '';

/** How the star at `place` (1 to 5) draws for a rating, null or 0 being none. */
export function fillOf(stars: number | null, place: number): StarFill {
	const value = stars ?? 0;
	return value >= place ? 'full' : value >= place - 0.5 ? 'half' : '';
}

/** A number of stars to the nearest half: an album's mean, drawn. */
export function shownHalves(mean: number): number {
	return Math.round(mean * 2) / 2;
}

/**
 * The half under a pointer `fraction` of the way across the five stars: the left half of the
 * fourth is 3½, its right half 4. From ½ to 5, so the very edges still rate.
 */
export function halfAt(fraction: number): number {
	return Math.min(maxStars, Math.max(minStars, Math.ceil(fraction * maxStars * 2) / 2));
}

/** The star under a finger `fraction` of the way across: 1 to 5. */
export function starAt(fraction: number): number {
	return Math.min(maxStars, Math.max(1, Math.ceil(fraction * maxStars)));
}

/** What a click on the half `at` does to a rating: it sets it, or clears the one it already has. */
export function pressed(stars: number | null, at: number): number | null {
	return stars === at ? null : at;
}

/**
 * What a tap on star `star` does to a rating, on touch: the star, then the star less a half, then
 * nothing, then the star again.
 */
export function tapped(stars: number | null, star: number): number | null {
	if (stars === star) {
		return star - 0.5;
	}
	return stars === star - 0.5 ? null : star;
}

/**
 * What a key does to a focused rating: a half up or down (below ½ clears), a whole number of
 * stars, nothing, or all five. Undefined for a key that isn't the rating's.
 */
export function keyed(stars: number | null, key: string): number | null | undefined {
	const value = stars ?? 0;
	switch (key) {
		case 'ArrowRight':
		case 'ArrowUp':
			return Math.min(maxStars, value + 0.5);
		case 'ArrowLeft':
		case 'ArrowDown':
			return value - 0.5 < minStars ? null : value - 0.5;
		case 'Home':
		case 'Delete':
		case 'Backspace':
			return null;
		case 'End':
			return maxStars;
		default:
			return /^[1-5]$/.test(key) ? Number(key) : undefined;
	}
}

/** A rating as a person reads it, without the stars: "4½", "½", "5". */
export function starsText(stars: number): string {
	const whole = Math.floor(stars);
	const half = stars - whole >= 0.5 ? '½' : '';
	return whole === 0 ? half : `${String(whole)}${half}`;
}

/** A rating in words, for a screen reader: "4½ stars", "1 star", "Unrated". */
export function starsWords(stars: number | null): string {
	if (stars === null || stars === 0) {
		return 'Unrated';
	}
	return `${starsText(stars)} ${stars === 1 || stars === minStars ? 'star' : 'stars'}`;
}

/** A rating as one line of text, in one ink: "★★★★½". */
export function starsLine(stars: number): string {
	return '★'.repeat(Math.floor(stars)) + (stars % 1 >= 0.5 ? '½' : '');
}
