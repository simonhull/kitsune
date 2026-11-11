/**
 * "Why this song": words for the factors the server gives each shuffled song. It is one module and
 * its component, `Why.svelte`, and nothing depends on either: set `whyShown` to false and Up next
 * shows no reasons. To take it out for good, delete this file, `Why.svelte` and `why.test.ts`, and
 * the two lines in Now playing that pass it to Up next.
 */
import type { ShuffleWhy } from '#lib/library/source.ts';
import { shownHalves, starsText } from '#lib/library/stars.ts';

/** The one switch. */
export const whyShown = true;

/** A factor as the popover shows it. */
export interface WhyFactor {
	/** In words, such as "Rated 4★". */
	name: string;
	/** The multiplier, such as "×1.3". */
	times: string;
	/** How far its bar goes, from 0 to 1: half is no change. */
	bar: number;
	/** It lifted the song. */
	lifts: boolean;
}

const daysPerWeek = 7;
const daysPerMonth = 30;
const daysPerYear = 365;

/** How long ago, in the fewest words: "6 weeks", "3 months". */
export function agoWords(days: number): string {
	const counted = (n: number, one: string): string =>
		n === 1 ? `a ${one}` : `${String(n)} ${one}s`;
	if (days < 2) {
		return days < 1 ? 'today' : 'a day';
	}
	if (days < 2 * daysPerWeek) {
		return counted(days, 'day');
	}
	if (days < 2 * daysPerMonth) {
		return counted(Math.round(days / daysPerWeek), 'week');
	}
	if (days < daysPerYear) {
		return counted(Math.round(days / daysPerMonth), 'month');
	}
	return counted(Math.round(days / daysPerYear), 'year');
}

/** How long unheard before it's worth saying. */
const unheardDays = 14;
/** A skip rate factor under this says the person skips it. */
const skipsOften = 0.9;
/** Listens on average this full say the person hears it all. */
const hearsAll = 0.9;
/** A flow factor past this says it leaned toward the song. */
const leans = 1.02;

/** Stars as a short mark: "4★", or "4½★" for a half or a run between ratings. */
function starsMark(stars: number): string {
	return `${starsText(shownHalves(stars))}★`;
}

/**
 * Why a song came up, in the two facts that say the most, for the quiet line beside it: "5★ · not
 * heard in 3 months", which a wide screen sets in capitals. `songs` is how many play together.
 */
export function whyLine(why: ShuffleWhy, songs = 1): string {
	const facts: string[] = [];
	if (songs > 1) {
		facts.push(`Combined · ${String(songs)} songs`);
	}
	if (why.after !== undefined && why.flow > leans) {
		facts.push(`close to ${why.after.name}`);
	}
	// Stars a heart gave name their source, so they never claim a rating the person didn't give.
	const byHeart = why.stars > 0 && why.heartStars > 0;
	if (why.stars > 0) {
		facts.push(byHeart ? `${starsMark(why.stars)} by heart` : starsMark(why.stars));
	}
	if (why.hearted && !byHeart) {
		facts.push('hearted');
	}
	if (!why.played) {
		facts.push('never played', 'a nudge for new');
	}
	if (why.stars <= 0 && !why.hearted && why.played) {
		facts.push('unrated');
	}
	if (why.played && why.sinceDays >= unheardDays) {
		facts.push(`not heard in ${agoWords(why.sinceDays)}`);
	}
	if (why.memory && why.skipRate < skipsOften) {
		facts.push('you skip it often');
	} else if (why.memory && why.listens >= 3 && why.heard >= hearsAll) {
		facts.push('you usually hear it all');
	}
	const line = facts.slice(0, songs > 1 ? 1 : 2).join(' · ');
	return line.charAt(0).toUpperCase() + line.slice(1);
}

/** A multiplier as the popover writes it. */
function times(x: number): string {
	const near = x.toFixed(1);
	// A small lean still shows, rather than reading as no change at all.
	return `×${near === '1.0' && Math.abs(x - 1) >= 0.005 ? x.toFixed(2) : near}`;
}

/** A factor's bar: half for no change, more for a lift, less for a cut, on a log scale. */
function bar(x: number): number {
	return Math.min(1, Math.max(0.04, 0.48 + Math.log(x) / 2));
}

function factor(name: string, x: number): WhyFactor {
	return { name, times: times(x), bar: bar(x), lifts: x > leans };
}

/** When it last came up, as the popover says it. */
function heardWhen({ played, sinceDays }: ShuffleWhy): string {
	if (!played) {
		return 'Never played';
	}
	return sinceDays < 1 ? 'Heard today' : `Last heard ${agoWords(sinceDays)} ago`;
}

/** Every factor behind the song's weight, in words, for the popover. */
export function whyFactors(why: ShuffleWhy): WhyFactor[] {
	const rated =
		why.heartStars > 0
			? `Rated ${starsMark(why.stars)} · from your heart`
			: `Rated ${starsMark(why.stars)}`;
	const out = [factor(why.stars > 0 ? rated : 'Unrated', why.rating)];
	if (why.hearted) {
		out.push(factor('Hearted', why.heart));
	}
	out.push(factor(heardWhen(why), why.recency));
	if (why.memory) {
		if (why.earlySkips > 0) {
			out.push(factor('Skipped early this week', why.skipMemory));
		}
		if (why.starts > 0) {
			out.push(factor(`Skipped ${String(why.skips)} of ${String(why.starts)} times`, why.skipRate));
		}
		if (why.listens > 0) {
			const heard =
				why.heard >= hearsAll
					? 'Usually heard to the end'
					: `Usually heard ${String(Math.round(why.heard * 10) * 10)}% through`;
			out.push(factor(heard, why.listen));
		}
	}
	if (why.after !== undefined && why.flow !== 1) {
		const name = why.flow > 1 ? `Close to ${why.after.name}` : `Unlike ${why.after.name}`;
		out.push(factor(name, why.flow));
	}
	return out;
}
