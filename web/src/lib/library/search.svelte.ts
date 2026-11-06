import {
	UnauthorizedError,
	type AlbumEntry,
	type ArtistLink,
	type SearchResults,
	type TrackHit
} from './source.ts';

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
	/** The query the results on screen answer, which is behind `state.query` while typing. */
	answered = $state.raw('');

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
					this.answered = query;
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

/** The one hit a search puts first. */
export type TopHit =
	| { kind: 'artist'; artist: ArtistLink }
	| { kind: 'album'; album: AlbumEntry }
	| { kind: 'track'; track: TrackHit };

/** A search's results, with the top hit taken out of its group so nothing shows twice. */
export interface Spotlight extends SearchResults {
	top: TopHit;
}

/** A name as a person might type it. The server folds harder: this only has to recognize a match. */
function folded(name: string): string {
	return name.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase().replace(/\s+/gu, ' ').trim();
}

function nameOf(hit: TopHit): string {
	switch (hit.kind) {
		case 'artist':
			return hit.artist.name;
		case 'album':
			return hit.album.title;
		case 'track':
			return hit.track.title;
	}
}

/**
 * Picks the best hit: a name that is exactly the query, an artist before an album before a track.
 * With no exact name, the first of the server's artists, then albums, then tracks. Null with
 * nothing found.
 */
export function spotlight(results: SearchResults, query: string): Spotlight | null {
	const { artists, albums, tracks } = results;
	const hits: TopHit[] = [
		...artists.map((artist) => ({ kind: 'artist', artist }) as const),
		...albums.map((album) => ({ kind: 'album', album }) as const),
		...tracks.map((track) => ({ kind: 'track', track }) as const)
	];
	const wanted = folded(query);
	const top = hits.find((hit) => folded(nameOf(hit)) === wanted) ?? hits.at(0);
	if (top === undefined) {
		return null;
	}
	return {
		top,
		artists: top.kind === 'artist' ? artists.filter((artist) => artist !== top.artist) : artists,
		albums: top.kind === 'album' ? albums.filter((album) => album !== top.album) : albums,
		tracks: top.kind === 'track' ? tracks.filter((track) => track !== top.track) : tracks
	};
}

function credits(names: readonly string[], name: string): boolean {
	return names.some((credited) => folded(credited) === folded(name));
}

/** What a search found by this artist. An artist's own hit carries no more than a name. */
function byArtist({ albums, tracks }: SearchResults, { name }: ArtistLink): SearchResults {
	return {
		artists: [],
		albums: albums.filter(({ artist }) => credits([artist], name)),
		tracks: tracks.filter(({ artists }) => credits(artists, name))
	};
}

/**
 * The album whose colors a hit wears, for `paletteFor`. An artist has none of its own, so it borrows
 * from its first album or track in the same answer. Undefined with neither: that's the fox.
 */
export function colorSource(
	top: TopHit,
	results: SearchResults
): Pick<AlbumEntry, 'id' | 'artwork'> | undefined {
	switch (top.kind) {
		case 'album':
			return top.album;
		case 'track':
			return { id: top.track.albumId, artwork: top.track.artwork };
		case 'artist': {
			const { albums, tracks } = byArtist(results, top.artist);
			const track = tracks.at(0);
			return albums.at(0) ?? (track && { id: track.albumId, artwork: track.artwork });
		}
	}
}

/** What a search found by an artist, as the line under its name. Their whole catalog isn't known. */
export function artistLine(artist: ArtistLink, results: SearchResults): string {
	const found = foundLine(byArtist(results, artist));
	return found === '' ? '' : `${found} in this search`;
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
