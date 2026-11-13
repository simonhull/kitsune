/**
 * The You page's one-time card that turns the hearts someone kept into stars, and the Last.fm love
 * line beside it, in words. Pure, so what they say is tested without a page.
 */
import type { HeartsSummary } from '#lib/library/source.ts';

/** What the card offers when the person has rated none of the songs they hearted: 4★. */
export const fallbackStars = 4;

/** Whether there's anything left to turn into stars: with nothing, the card isn't drawn. */
export function hasHearts(h: HeartsSummary): boolean {
	return h.tracksRated + h.tracksUnrated + h.albums + h.artists > 0;
}

/** Each whole star with its count, most first; on a tie the higher star leads. */
function ranked(byStars: readonly number[]): { stars: number; songs: number }[] {
	return byStars
		.map((songs, at) => ({ stars: at + 1, songs }))
		.filter(({ songs }) => songs > 0)
		.toSorted((a, b) => b.songs - a.songs || b.stars - a.stars);
}

/**
 * The card's first choice: the person's most common rating among songs they hearted and rated,
 * to a whole star (a half rounds up, as the server counts them), else 4★. A tie goes to the higher.
 */
export function defaultStars(byStars: readonly number[]): number {
	return ranked(byStars)[0]?.stars ?? fallbackStars;
}

function songs(n: number): string {
	return `${n.toLocaleString('en-US')} ${n === 1 ? 'song' : 'songs'}`;
}

/** The card's opening line: how many songs were hearted, and what becomes of them. */
export function heartsLine({ tracksRated, tracksUnrated }: HeartsSummary): string {
	const all = tracksRated + tracksUnrated;
	if (all === 0) {
		return 'You hearted no songs, only albums and artists.';
	}
	const opening = `You hearted ${songs(all)}.`;
	if (tracksUnrated === 0) {
		return `${opening} ${all === 1 ? 'It has' : 'All of them have'} stars of their own and ${all === 1 ? 'keeps them' : 'keep them'}; only the heart goes.`;
	}
	if (tracksRated === 0) {
		return `${opening} None of them has stars yet.`;
	}
	const keep =
		tracksRated === 1 ? '1 of them has' : `${tracksRated.toLocaleString('en-US')} of them have`;
	const none = tracksUnrated === 1 ? '1 has' : `${tracksUnrated.toLocaleString('en-US')} have`;
	return `${opening} ${keep} stars of their own and ${tracksRated === 1 ? 'keeps them' : 'keep them'}; only the heart goes. ${none} no stars yet.`;
}

/** What the person tends to give a song they hearted: "Most songs you hearted and rated are 4★ (185) or 5★ (183)." */
export function commonLine(byStars: readonly number[]): string {
	const [first, second] = ranked(byStars);
	if (first === undefined) {
		return '';
	}
	const one = (r: { stars: number; songs: number }): string =>
		`${String(r.stars)}★ (${r.songs.toLocaleString('en-US')})`;
	return `Most songs you hearted and rated are ${one(first)}${second === undefined ? '' : ` or ${one(second)}`}.`;
}

/** The card's button that rates them: "Rate 271 songs 4★". */
export function rateLabel(unrated: number, stars: number): string {
	return `Rate ${songs(unrated)} ${String(stars)}★`;
}

/** The card's last line: album and artist hearts go too, and why that's fine. */
export function goesToo({ albums, artists }: HeartsSummary): string {
	if (albums + artists === 0) {
		return '';
	}
	const kinds =
		albums > 0 && artists > 0
			? `Album and artist hearts (${String(albums)} and ${String(artists)})`
			: albums > 0
				? `Album hearts (${String(albums)})`
				: `Artist hearts (${String(artists)})`;
	return `${kinds} go too. How much you like an artist comes from how you rate their songs.`;
}

/** A star count as the Last.fm line writes it: "4★ AND UP", "4½★ AND UP". */
export function loveFromValue(stars: number): string {
	const whole = Math.floor(stars);
	return `${whole > 0 ? String(whole) : ''}${stars > whole ? '½' : ''}★ AND UP`;
}

/**
 * The hint under the love line, with how many songs are at or over it now: `rated` of the person's
 * own, and, while their hearts wait to become stars, those that will once they choose `pending`
 * stars for them (null for none chosen yet).
 */
export function loveHint(
	from: number,
	rated: number | null,
	pending: { songs: number; stars: number } | null
): string {
	const line = `${String(from).replace('.5', '½')}★`;
	const rule = `Rating a song ${line} or more loves it on Last.fm; rating it lower un-loves it.`;
	const kept = 'Loves already on Last.fm stay as they are.';
	if (rated === null) {
		return `${rule} ${kept}`;
	}
	const more = pending !== null && pending.stars >= from ? pending.songs : 0;
	const count = songs(rated + more);
	const when = pending === null ? '' : ' once your hearts become stars';
	return `${rule} That's ${count}${when}. ${kept}`;
}
