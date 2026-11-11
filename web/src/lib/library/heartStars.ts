/**
 * What a hearted song with no stars of its own counts as: the person's `ratings.heart_stars`, Off
 * (0) or 1 to 5 in halves. Nothing is written. A song's own stars always win, and only a heart on the
 * song counts: a hearted album or artist gives its songs nothing. The server's twin is
 * `prefs.HeartStars.Of`, which shuffle reads, so the two must agree.
 */
import { starsText } from './stars.ts';

/** The stars a song counts as, and whether they came from its heart rather than the person. */
export interface Counted {
	stars: number;
	byHeart: boolean;
}

/** The least a heart may count as: under 1★, a heart would count against a song. */
export const leastHeartStars = 1;

/** Off: a heart is just a heart. */
export const heartStarsOff = 0;

/** The song's own stars, else its heart's when it's hearted and hearts count, else null. */
export function counted(own: number | null, hearted: boolean, heartStars: number): Counted | null {
	if (own !== null) {
		return { stars: own, byHeart: false };
	}
	return hearted && heartStars > 0 ? { stars: heartStars, byHeart: true } : null;
}

/** Whether a value can be the heart stars: Off, or 1 to 5 in halves. */
export function isHeartStars(value: unknown): value is number {
	return (
		typeof value === 'number' &&
		(value === heartStarsOff ||
			(value >= leastHeartStars && value <= 5 && Number.isInteger(value * 2)))
	);
}

/** What the You page's value says: "OFF", or "4½★ WHEN UNRATED". */
export function heartStarsValue(heartStars: number): string {
	return heartStars === heartStarsOff ? 'OFF' : `${starsText(heartStars)}★ WHEN UNRATED`;
}

/** Stars from a heart, for a screen reader and the tooltip. */
export function byHeartWords(stars: number): string {
	return `${starsText(stars)} ${stars === 1 ? 'star' : 'stars'} from your heart. Rate it yourself to change it.`;
}
