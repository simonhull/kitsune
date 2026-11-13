import type { Repeat } from '#lib/library/source.ts';
import type { LibraryFilter } from '#lib/pages/libraryRows.ts';

/** A person's shuffle settings, as the `shuffle.*` preferences hold them. */
export interface ShuffleSettings {
	/** How much ratings matter, from 0 to 1. */
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

/** The settings in one quiet line, on the Library Shuffle's card. */
export function settingsLine({ ratings, flow, memory }: ShuffleSettings): string {
	const rated = ratings <= 0 ? 'RATINGS OFF' : `RATINGS ${ratingsWord(ratings)}`;
	const flowing = flow <= 0 ? 'NO FLOW' : `${flowWord(flow)} FLOW`;
	return [rated, flowing, memory ? 'LEARNS FROM SKIPS' : 'IGNORES SKIPS'].join(' · ');
}

/** What each Library filter shuffles, in the island's capitals: a shuffle source's label. */
export const libraryLabels: Readonly<Record<LibraryFilter, string>> = {
	everything: 'THE LIBRARY',
	rated: '4★ AND UP'
};

/** The Library's Shuffle button, in words: its accessible name, its card's heading and its tag. */
export interface FabWords {
	/** "Shuffle the library", or "Shuffle these: …" under a filter. */
	name: string;
	/** What the card says it shuffles: "The library", "4★ and up". */
	what: string;
	/** The filter chip's mark, stamped on the disc; empty for the whole library. */
	tag: string;
}

/** What the Library's Shuffle says under `filter`. */
export function fabWords(filter: LibraryFilter): FabWords {
	const words = libraryLabels[filter].toLowerCase();
	const tags = { everything: '', rated: '4★' };
	return {
		name: filter === 'everything' ? `Shuffle ${words}` : `Shuffle these: ${words}`,
		what: `${words.charAt(0).toUpperCase()}${words.slice(1)}`,
		tag: tags[filter]
	};
}

/**
 * Now playing's line under the controls, in the island's capitals: what's next and how it goes on.
 * `shuffled` is the album the player's own shuffle is of, `true` for songs from several, or
 * `false` when it's off: "UP NEXT · SHUFFLED FROM DEMON DAYS · THEN AGAIN", "REPEATING THIS SONG".
 */
export function upNextLine(shuffled: string | boolean, repeat: Repeat): string {
	if (repeat === 'one') {
		return 'REPEATING THIS SONG';
	}
	const from =
		typeof shuffled === 'string' ? `SHUFFLED FROM ${shuffled.toUpperCase()}` : 'SHUFFLED';
	return [
		'UP NEXT',
		...(shuffled === false ? [] : [from]),
		...(repeat === 'all' ? ['THEN AGAIN'] : [])
	].join(' · ');
}

/** How many songs an endless shuffle draws from, on one line. */
export function songsLine(count: number): string {
	return `${count.toLocaleString('en-US')} ${count === 1 ? 'SONG' : 'SONGS'} · ENDLESS`;
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

/** What "how much ratings matter" does, at its value, in a sentence. */
export function ratingsSentence(ratings: number): string {
	if (ratings <= 0) {
		return 'Ratings change nothing: every song comes up as often as any other.';
	}
	const five = fiveStar ** ratings;
	const one = oneStar ** ratings;
	const times = five.toFixed(1).replace(/\.0$/, '');
	return `A 5★ song comes up about ${times} times as often as an unrated one, and a 1★ about ${share(one)}.`;
}
