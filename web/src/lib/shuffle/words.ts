import { starsText } from '#lib/library/stars.ts';

/** A person's shuffle settings, as the `shuffle.*` preferences hold them. */
export interface ShuffleSettings {
	/** How much ratings and hearts matter, from 0 to 1. */
	ratings: number;
	/** How much the next song leans toward one that suits the last, from 0 to 1. */
	flow: number;
	/** Whether skips and how much of a song is heard count. */
	memory: boolean;
}

/** What someone who never changed a thing has: the server's defaults. */
export const defaultShuffleSettings: ShuffleSettings = { ratings: 0.5, flow: 0.3, memory: true };

/** A 5★ and a 1★ at "how much ratings matter" 1, against an unrated song: the server's curve. */
const fiveStar = 2.4;
const oneStar = 0.2;

/** How much ratings matter, in a word, as the setting's label says it. */
export function ratingsWord(ratings: number): string {
	if (ratings <= 0) {
		return 'NOT AT ALL';
	}
	if (ratings < 0.35) {
		return 'A LITTLE';
	}
	if (ratings < 0.65) {
		return 'GENTLY';
	}
	return ratings < 0.9 ? 'A LOT' : 'STRONGLY';
}

/** How much flow leans, in a word, as the setting's label says it. */
export function flowWord(flow: number): string {
	if (flow <= 0) {
		return 'OFF';
	}
	if (flow < 0.45) {
		return 'A LITTLE';
	}
	return flow < 0.8 ? 'SOME' : 'A LOT';
}

/** The settings in one quiet line, under a Shuffle button. */
export function settingsLine({ ratings, flow, memory }: ShuffleSettings): string {
	const rated = ratings <= 0 ? 'RATINGS OFF' : `RATINGS ${ratingsWord(ratings)}`;
	const flowing = flow <= 0 ? 'NO FLOW' : `${flowWord(flow)} FLOW`;
	return [rated, flowing, memory ? 'LEARNS FROM SKIPS' : 'IGNORES SKIPS'].join(' · ');
}

/** Says a share as words a person reads at a glance: "half", "a third". */
function share(part: number): string {
	const shares: readonly [number, string][] = [
		[1, 'just as often'],
		[3 / 4, 'three quarters as often'],
		[2 / 3, 'two thirds as often'],
		[1 / 2, 'half as often'],
		[1 / 3, 'a third as often'],
		[1 / 4, 'a quarter as often'],
		[1 / 5, 'a fifth as often']
	];
	const [, words] = shares.reduce((best, each) =>
		Math.abs(Math.log(each[0] / part)) < Math.abs(Math.log(best[0] / part)) ? each : best
	);
	return words;
}

/**
 * What "how much ratings matter" does, at its value, in a sentence; with `heartStars` above 0, what
 * an unrated heart counts as too.
 */
export function ratingsSentence(ratings: number, heartStars = 0): string {
	if (ratings <= 0) {
		return 'Ratings and hearts change nothing: every song comes up as often as any other.';
	}
	const five = fiveStar ** ratings;
	const one = oneStar ** ratings;
	const times = five.toFixed(1).replace(/\.0$/, '');
	const hearts = heartStars > 0 ? ` An unrated heart counts as ${starsText(heartStars)}★.` : '';
	return `A 5★ song comes up about ${times} times as often as an unrated one, and a 1★ about ${share(one)}. Hearts give a small lift.${hearts}`;
}
