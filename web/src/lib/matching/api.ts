import { library } from '#lib/library/library.svelte.ts';
import type { MatchList, MatchQueue, MatchReview, MatchState, MatchTerms } from './types.ts';

const base = '/api/v1';

/** Why a match couldn't be had. */
export type MatchFailure =
	/** MusicBrainz can't be reached, or matching isn't set up: try again later. */
	| { kind: 'unavailable'; retryAfterSeconds: number }
	/** The album or release is gone. */
	| { kind: 'gone' }
	/** Only administrators match. */
	| { kind: 'forbidden' }
	/** The session ended: the shell takes the person to the door. */
	| { kind: 'signedOut' };

export class MatchError extends Error {
	constructor(readonly failure: MatchFailure) {
		super(failure.kind);
		this.name = 'MatchError';
	}
}

/** When the server gives no usable `Retry-After`, come back in ten seconds. */
const defaultRetrySeconds = 10;

async function call<T>(path: string, init: RequestInit = {}): Promise<T> {
	const unsafe = init.method !== undefined && init.method !== 'GET';
	let response: Response;
	try {
		response = await fetch(`${base}${path}`, {
			...init,
			credentials: 'same-origin',
			headers: {
				...init.headers,
				...(init.body === undefined ? {} : { 'Content-Type': 'application/json' }),
				...(unsafe ? { 'X-Kitsune-Request': '1' } : {})
			}
		});
	} catch {
		throw new MatchError({ kind: 'unavailable', retryAfterSeconds: defaultRetrySeconds });
	}
	switch (response.status) {
		case 200:
			return (await response.json()) as T;
		case 204:
			return undefined as T;
		case 401:
			void library.onExpired?.();
			throw new MatchError({ kind: 'signedOut' });
		case 403:
			throw new MatchError({ kind: 'forbidden' });
		case 404:
			throw new MatchError({ kind: 'gone' });
	}
	const seconds = Number(response.headers.get('Retry-After'));
	throw new MatchError({
		kind: 'unavailable',
		retryAfterSeconds: seconds > 0 ? seconds : defaultRetrySeconds
	});
}

/** The releases an album may be, best first. `prefetch` asks ahead, behind anything a person waits on. */
export function findMatches(
	albumId: string,
	terms: MatchTerms = {},
	prefetch = false
): Promise<MatchList> {
	return call<MatchList>(`/albums/${encodeURIComponent(albumId)}/matches`, {
		method: 'POST',
		body: JSON.stringify(prefetch ? { ...terms, prefetch } : terms)
	});
}

/** One candidate, closely, against its album. */
export function reviewMatch(candidateId: string): Promise<MatchReview> {
	return call<MatchReview>(`/matches/${encodeURIComponent(candidateId)}`);
}

/** The albums to match. */
export function matchQueue(): Promise<MatchQueue> {
	return call<MatchQueue>('/matching/queue');
}

/** Says where an album stands in the queue. */
export function setMatchState(albumId: string, state: MatchState): Promise<void> {
	return call<undefined>(`/matching/albums/${encodeURIComponent(albumId)}`, {
		method: 'PUT',
		body: JSON.stringify({ state })
	});
}
