import { UnauthorizedError, type SearchResults } from './source.ts';

/** How long an answer may take before anyone is told it's taking a while. */
const patienceMs = 200;

export type SearchState =
	| { kind: 'idle' }
	/** `shown` stays up until its answer lands, so typing never blanks the page. */
	| { kind: 'looking'; query: string; shown: SearchResults | null; slow: boolean }
	| { kind: 'found'; query: string; results: SearchResults }
	| { kind: 'failed'; query: string };

/** Asks a question that can be taken back. */
export type Ask = (query: string, signal: AbortSignal) => Promise<SearchResults>;

/** The results on screen in a state, if any. */
export function resultsOf(state: SearchState): SearchResults | null {
	switch (state.kind) {
		case 'looking':
			return state.shown;
		case 'found':
			return state.results;
		default:
			return null;
	}
}

/**
 * One search at a time. Starting another takes the last one back, so a stale answer can't win.
 * It outlives the page, so Back lands on results that are already there.
 */
export class Search {
	state = $state.raw<SearchState>({ kind: 'idle' });

	readonly #ask: Ask;
	#question: AbortController | null = null;
	#patience: ReturnType<typeof setTimeout> | undefined;

	constructor(ask: Ask) {
		this.#ask = ask;
	}

	/** Looks for `query`. Nothing to look for is the page's resting state, not a search. */
	run(query: string): void {
		this.stop();
		if (query === '') {
			this.state = { kind: 'idle' };
			return;
		}
		const shown = resultsOf(this.state);
		this.state = { kind: 'looking', query, shown, slow: false };
		const question = new AbortController();
		const { signal } = question;
		this.#question = question;
		this.#patience = setTimeout(() => {
			this.state = { kind: 'looking', query, shown, slow: true };
		}, patienceMs);
		this.#ask(query, signal).then(
			(results) => {
				if (!signal.aborted) {
					this.#land({ kind: 'found', query, results });
				}
			},
			(error: unknown) => {
				// A lost session has already gone to the door.
				if (!signal.aborted && !(error instanceof UnauthorizedError)) {
					this.#land({ kind: 'failed', query });
				}
			}
		);
	}

	#land(state: SearchState): void {
		clearTimeout(this.#patience);
		this.state = state;
	}

	/** Takes back whatever is in flight. What's on screen stays. */
	stop(): void {
		clearTimeout(this.#patience);
		this.#question?.abort();
		this.#question = null;
	}

	/** Forgets everything, for the next session. */
	reset(): void {
		this.stop();
		this.state = { kind: 'idle' };
	}
}

function count(value: number, one: string, many: string): string[] {
	return value === 0 ? [] : [`${value.toLocaleString('en-US')} ${value === 1 ? one : many}`];
}

/** What a search found, as a fact line. */
export function foundLine({ artists, albums, tracks }: SearchResults): string {
	return [
		...count(artists.length, 'artist', 'artists'),
		...count(albums.length, 'album', 'albums'),
		...count(tracks.length, 'track', 'tracks')
	].join(' · ');
}

/** The line under the search box. A count is a fact; anything else is the app talking. */
export interface SearchLine {
	text: string;
	fact: boolean;
}

function fact(results: SearchResults | null): SearchLine {
	return { text: results === null ? '' : foundLine(results), fact: true };
}

function words(text: string): SearchLine {
	return { text, fact: false };
}

/** Says where a search has got to. A quick answer never gets to say it's looking. */
export function searchLine(state: SearchState): SearchLine {
	switch (state.kind) {
		case 'idle':
			return words('Artists, albums, tracks. Typos are fine.');
		case 'failed':
			return words("The search didn't come back.");
		case 'looking':
			return state.slow ? words('Looking…') : fact(state.shown);
		case 'found': {
			const found = fact(state.results);
			return found.text === '' ? words(`Nothing in the library goes by “${state.query}”.`) : found;
		}
	}
}
