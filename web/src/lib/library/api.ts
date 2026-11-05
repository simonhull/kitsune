import {
	AlbumNotFoundError,
	ArtistNotFoundError,
	EditError,
	LoginError,
	ServerError,
	UnauthorizedError,
	type Album,
	type AlbumPage,
	type AlbumSort,
	type ArtistDetail,
	type ArtistEntry,
	type EditedTrack,
	type EditFailure,
	type EditStopped,
	type LibraryFacts,
	type LibrarySource,
	type LoginFailure,
	type Me,
	type SearchResults,
	type User
} from './source.ts';

const base = '/api/v1';

/** When the server gives no usable `Retry-After`, come back in a minute. */
const defaultRetrySeconds = 60;

export interface ApiOptions {
	/** Called on every 401 except `/auth/login`'s own. Parallel calls all land here. */
	onUnauthorized: () => void;
	fetch?: typeof globalThis.fetch;
}

/** The URL of a cover rendition. */
export function artworkUrl(hash: string, size: number): string {
	return `${base}/artwork/${hash}?size=${String(size)}`;
}

/** What the server says about when to come back. */
function retrySeconds(response: Response): number {
	const seconds = Number(response.headers.get('Retry-After'));
	return seconds > 0 ? seconds : defaultRetrySeconds;
}

/** What stopped an edit. Only an edit that got as far as the files says how far it got. */
async function editFailureOf(response: Response): Promise<EditFailure> {
	switch (response.status) {
		case 404:
			return { kind: 'gone' };
		case 409:
			return { kind: 'busy', retryAfterSeconds: retrySeconds(response) };
		case 412:
			return { kind: 'stale' };
	}
	const stopped = (await response.json().catch(() => null)) as Partial<EditStopped> | null;
	const { detail = '', changed, remaining } = stopped ?? {};
	if (changed === undefined || remaining === undefined) {
		return { kind: response.status === 403 ? 'forbidden' : 'unavailable' };
	}
	const kind =
		response.status === 403 ? 'unwritable' : response.status === 422 ? 'refused' : 'failed';
	return { kind, detail, changed, remaining };
}

/** Builds a {@link LibrarySource} over the real API, on the session cookie. */
export function createApiSource({
	onUnauthorized,
	fetch: send = fetch
}: ApiOptions): LibrarySource {
	function call(path: string, init: RequestInit = {}): Promise<Response> {
		const unsafe = init.method !== undefined && init.method !== 'GET';
		return send(`${base}${path}`, {
			...init,
			credentials: 'same-origin',
			headers: {
				...init.headers,
				// Without it the server refuses cookie-authenticated writes: a form can't set a header.
				...(unsafe ? { 'X-Kitsune-Request': '1' } : {})
			}
		});
	}

	async function read<T>(path: string, init?: RequestInit): Promise<T> {
		const response = await call(path, init);
		if (response.status === 401) {
			onUnauthorized();
			throw new UnauthorizedError();
		}
		if (!response.ok) {
			throw new ServerError(response.status);
		}
		return (await response.json()) as T;
	}

	async function failureOf(response: Response): Promise<LoginFailure> {
		if (response.status === 401) {
			return { kind: 'credentials' };
		}
		if (response.status === 429) {
			return { kind: 'throttled', retryAfterSeconds: retrySeconds(response) };
		}
		return { kind: 'unavailable' };
	}

	/** Sends an edit. The ETag goes back as it came: comparison is strong, on the server. */
	async function edit<T>(path: string, etag: string, changes: object): Promise<T> {
		let response: Response;
		try {
			response = await call(path, {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json', 'If-Match': etag },
				body: JSON.stringify(changes)
			});
		} catch {
			throw new EditError({ kind: 'unavailable' });
		}
		if (response.status === 401) {
			onUnauthorized();
			throw new UnauthorizedError();
		}
		if (!response.ok) {
			throw new EditError(await editFailureOf(response));
		}
		return (await response.json()) as T;
	}

	return {
		me: () => read<Me>('/me'),

		async login(name, password) {
			let response: Response;
			try {
				response = await call('/auth/login', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ name, password, cookie: true })
				});
			} catch {
				throw new LoginError({ kind: 'unavailable' });
			}
			if (!response.ok) {
				throw new LoginError(await failureOf(response));
			}
			const { user } = (await response.json()) as { user: User };
			return user;
		},

		facts: () => read<LibraryFacts>('/library'),

		async ready() {
			return (await call('/readyz')).ok;
		},

		albumsPage(sort: AlbumSort, cursor?: string, limit?: number) {
			const query = new URLSearchParams({ sort });
			if (cursor !== undefined) {
				query.set('cursor', cursor);
			}
			if (limit !== undefined) {
				query.set('limit', String(limit));
			}
			return read<AlbumPage>(`/albums?${query.toString()}`);
		},

		async album(id) {
			try {
				return await read<Album>(`/albums/${encodeURIComponent(id)}`);
			} catch (error) {
				if (error instanceof ServerError && error.status === 404) {
					throw new AlbumNotFoundError(id);
				}
				throw error;
			}
		},

		async artists() {
			const { artists } = await read<{ artists: ArtistEntry[] }>('/artists');
			return artists;
		},

		async artist(id) {
			try {
				return await read<ArtistDetail>(`/artists/${encodeURIComponent(id)}`);
			} catch (error) {
				if (error instanceof ServerError && error.status === 404) {
					throw new ArtistNotFoundError(id);
				}
				throw error;
			}
		},

		search(query, signal) {
			const search = new URLSearchParams({ q: query });
			return read<SearchResults>(`/search?${search.toString()}`, signal && { signal });
		},

		editTrack: (id, etag, changes) =>
			edit<EditedTrack>(`/tracks/${encodeURIComponent(id)}`, etag, changes),

		editAlbum: (id, etag, changes) =>
			edit<Album>(`/albums/${encodeURIComponent(id)}`, etag, changes),

		streamUrl: (trackId) => `${base}/tracks/${encodeURIComponent(trackId)}/stream`
	};
}
