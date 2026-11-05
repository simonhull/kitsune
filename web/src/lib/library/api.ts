import {
	AlbumNotFoundError,
	LoginError,
	ServerError,
	UnauthorizedError,
	type Album,
	type AlbumPage,
	type AlbumSort,
	type LibraryFacts,
	type LibrarySource,
	type LoginFailure,
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

	async function read<T>(path: string): Promise<T> {
		const response = await call(path);
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
			const seconds = Number(response.headers.get('Retry-After'));
			return {
				kind: 'throttled',
				retryAfterSeconds: seconds > 0 ? seconds : defaultRetrySeconds
			};
		}
		return { kind: 'unavailable' };
	}

	return {
		me: () => read<User>('/me'),

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

		streamUrl: (trackId) => `${base}/tracks/${encodeURIComponent(trackId)}/stream`
	};
}
