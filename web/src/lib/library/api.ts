import {
	AlbumNotFoundError,
	ArtistNotFoundError,
	EditError,
	LoginError,
	ServerError,
	UnauthorizedError,
	type Album,
	type AlbumEdit,
	type AlbumPage,
	type AlbumSort,
	type AlbumTags,
	type ArtistDetail,
	type ArtistEntry,
	type EditFailure,
	type EditStopped,
	type LibraryFacts,
	type LibraryIndex,
	type LibrarySource,
	type LoginFailure,
	type Me,
	type PlayReport,
	type ScanIssues,
	type ScanStatus,
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
	}
	const stopped = (await response.json().catch(() => null)) as Partial<EditStopped> | null;
	const { detail = '', changed, remaining } = stopped ?? {};
	if (changed === undefined || remaining === undefined) {
		switch (response.status) {
			case 403:
				return { kind: 'forbidden' };
			case 412:
				return { kind: 'stale' };
		}
		return { kind: 'unavailable' };
	}
	switch (response.status) {
		case 403:
			return { kind: 'unwritable', detail, changed, remaining };
		case 412:
			return { kind: 'stale', detail, changed, remaining };
		case 422:
			return { kind: 'refused', detail, changed, remaining };
	}
	return { kind: 'failed', detail, changed, remaining };
}

/** Why the tags couldn't be read. */
function tagsFailureOf({ status }: Response): EditFailure {
	switch (status) {
		case 403:
			return { kind: 'forbidden' };
		case 404:
			return { kind: 'gone' };
		case 409:
			return { kind: 'missing' };
	}
	return { kind: 'unavailable' };
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

	async function read<T>(
		path: string,
		init?: RequestInit,
		unauthorized = onUnauthorized
	): Promise<T> {
		const response = await call(path, init);
		if (response.status === 401) {
			unauthorized();
			throw new UnauthorizedError();
		}
		if (!response.ok) {
			throw new ServerError(response.status);
		}
		return (response.status === 204 ? undefined : await response.json()) as T;
	}

	/** Work in the background mustn't take the music away: the next thing asked for finds out. */
	const quietly = (): void => undefined;

	async function failureOf(response: Response): Promise<LoginFailure> {
		if (response.status === 401) {
			return { kind: 'credentials' };
		}
		if (response.status === 429) {
			return { kind: 'throttled', retryAfterSeconds: retrySeconds(response) };
		}
		return { kind: 'unavailable' };
	}

	/** Asks for something an edit depends on. A 401 goes to the door, as everywhere. */
	async function forEdit(
		path: string,
		failureFor: (response: Response) => EditFailure | Promise<EditFailure>,
		init?: RequestInit
	): Promise<Response> {
		let response: Response;
		try {
			response = await call(path, init);
		} catch {
			throw new EditError({ kind: 'unavailable' });
		}
		if (response.status === 401) {
			onUnauthorized();
			throw new UnauthorizedError();
		}
		if (!response.ok) {
			throw new EditError(await failureFor(response));
		}
		return response;
	}

	return {
		me: () => read<Me>('/me'),

		sessionUser: () => read<User>('/me', undefined, quietly),

		reportPlay: (play: PlayReport) =>
			read<undefined>(
				'/me/plays',
				{
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(play)
				},
				quietly
			),

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

		libraryIndex: () => read<LibraryIndex>('/library/index'),

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

		async albumTags(id) {
			const response = await forEdit(`/albums/${encodeURIComponent(id)}/tags`, tagsFailureOf);
			return (await response.json()) as AlbumTags;
		},

		// The ETag goes back as it came: comparison is strong, on the server.
		async editAlbum(id, etag, edit: AlbumEdit) {
			const response = await forEdit(`/albums/${encodeURIComponent(id)}/edits`, editFailureOf, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json', 'If-Match': etag },
				body: JSON.stringify(edit)
			});
			return (await response.json()) as Album;
		},

		streamUrl: (trackId) => `${base}/tracks/${encodeURIComponent(trackId)}/stream`,

		startScan: (mode) =>
			read<ScanStatus>('/admin/library/scan', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ mode })
			}),

		issues: () => read<ScanIssues>('/admin/library/issues')
	};
}
