import {
	AlbumNotFoundError,
	ArtistNotFoundError,
	EditError,
	LoginError,
	ServerError,
	SettingsError,
	SetupError,
	UnauthorizedError,
	type Album,
	type AlbumEdit,
	type AlbumPage,
	type AlbumSort,
	type AlbumTags,
	type ArtistDetail,
	type ArtistEntry,
	type EditedTracks,
	type EditFailure,
	type EditStopped,
	type LastfmStatus,
	type LastfmSwitches,
	type LibraryFacts,
	type LibraryIndex,
	type Listening,
	type LibrarySource,
	type LoginFailure,
	type MarkKind,
	type Me,
	type ListenReport,
	type PlayReport,
	type Prefs,
	type ScanIssues,
	type ScanStatus,
	type SearchResults,
	type SetupFailure,
	type FolderListing,
	type ServerSettings,
	type SettingKey,
	type SetupStatus,
	type ShufflePage,
	type ShuffleRules,
	type ShuffleRun,
	type TracksEdit,
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
	const stopped = (await response.json().catch(() => null)) as
		(Partial<EditStopped> & { album?: string }) | null;
	const { detail = '', changed, remaining, album } = stopped ?? {};
	if (response.status === 412 && album !== undefined) {
		return { kind: 'stale', album };
	}
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

/**
 * Why the first account wasn't made. A 400 names its rule in the detail, which comes from the
 * auth service's fixed messages, never from the input.
 */
async function setupFailureOf(response: Response): Promise<SetupFailure> {
	switch (response.status) {
		case 403:
			return { kind: 'code' };
		case 409:
			return { kind: 'done' };
		case 429:
			return { kind: 'throttled', retryAfterSeconds: retrySeconds(response) };
		case 400: {
			const { detail = '' } = ((await response.json().catch(() => null)) ?? {}) as {
				detail?: string;
			};
			if (detail.startsWith('invalid user name')) {
				return { kind: 'name' };
			}
			if (detail.startsWith('password')) {
				return { kind: 'password' };
			}
		}
	}
	return { kind: 'unavailable' };
}

/** The keys a settings refusal can name, as its detail begins. */
const settingKeys: readonly SettingKey[] = [
	'lastfm.api_key',
	'lastfm.shared_secret',
	'library.roots',
	'library.tag_writes',
	'library.quick_scan',
	'library.missing_grace',
	'log.level',
	'server.base_url',
	'server.trusted_proxies'
];

/** Reads a refused settings change: its detail is `key: why`. */
async function settingsErrorOf(response: Response): Promise<SettingsError> {
	const { detail = '' } = ((await response.json().catch(() => null)) ?? {}) as {
		detail?: string;
	};
	const key = settingKeys.find((each) => detail.startsWith(`${each}: `));
	return new SettingsError(key === undefined ? detail : detail.slice(key.length + 2), key);
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

		reportListen: (listen: ListenReport) =>
			read<undefined>(
				'/me/listens',
				{
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(listen)
				},
				quietly
			),

		setHeart: (kind: MarkKind, id: string, on: boolean) =>
			read<undefined>(`/me/hearts/${kind}/${encodeURIComponent(id)}`, {
				method: on ? 'PUT' : 'DELETE'
			}),

		setStars: (trackId: string, stars: number | null) =>
			read<undefined>(
				`/me/ratings/tracks/${encodeURIComponent(trackId)}`,
				stars === null
					? { method: 'DELETE' }
					: {
							method: 'PUT',
							headers: { 'Content-Type': 'application/json' },
							body: JSON.stringify({ stars })
						}
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

		setupStatus: () => read<SetupStatus>('/setup'),

		async setup(name, password, code) {
			let response: Response;
			try {
				response = await call('/setup', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(code === undefined ? { name, password } : { name, password, code })
				});
			} catch {
				throw new SetupError({ kind: 'unavailable' });
			}
			if (!response.ok) {
				throw new SetupError(await setupFailureOf(response));
			}
			const { user } = (await response.json()) as { user: User };
			return user;
		},

		logout: () => read<undefined>('/auth/logout', { method: 'POST' }),

		listening: () => read<Listening>('/me/listening'),

		lastfm: () => read<LastfmStatus>('/me/lastfm'),

		async connectLastfm() {
			const { url } = await read<{ url: string }>('/me/lastfm/connect', { method: 'POST' });
			return url;
		},

		setLastfm: (patch: LastfmSwitches) =>
			read<LastfmStatus>('/me/lastfm', {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(patch)
			}),

		disconnectLastfm: () => read<undefined>('/me/lastfm', { method: 'DELETE' }),

		nowPlaying: (trackId: string) =>
			read<undefined>(
				'/me/now-playing',
				{
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ trackId })
				},
				quietly
			),

		facts: () => read<LibraryFacts>('/library'),

		libraryIndex: (fields = []) =>
			read<LibraryIndex>(
				fields.length === 0 ? '/library/index' : `/library/index?fields=${fields.join(',')}`
			),

		libraryIndexOf: (albums, fields = []) => {
			const query = new URLSearchParams({ albums: albums.join(',') });
			if (fields.length > 0) {
				query.set('fields', fields.join(','));
			}
			return read<LibraryIndex>(`/library/index?${query.toString().replaceAll('%2C', ',')}`);
		},

		async pref(key) {
			try {
				return await read<Prefs[typeof key]>(`/me/prefs/${encodeURIComponent(key)}`);
			} catch (error) {
				if (error instanceof ServerError && error.status === 404) {
					return null;
				}
				throw error;
			}
		},

		setPref: (key, value) =>
			read<undefined>(`/me/prefs/${encodeURIComponent(key)}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(value)
			}),

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

		async editTracks(edit: TracksEdit) {
			const response = await forEdit('/edits', editFailureOf, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(edit)
			});
			return (await response.json()) as EditedTracks;
		},

		shuffleRules: (trackIds) =>
			read<ShuffleRules>(`/me/shuffle/rules?tracks=${trackIds.map(encodeURIComponent).join(',')}`),

		startShuffle: (start) =>
			read<ShufflePage>('/me/shuffles', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(start)
			}),

		nextShuffle: (id, after) =>
			read<ShufflePage>(
				`/me/shuffles/${encodeURIComponent(id)}/next?after=${String(after)}`,
				undefined,
				quietly
			),

		setRun: (trackIds) =>
			read<ShuffleRun>('/me/shuffle/runs', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ tracks: trackIds })
			}),

		deleteRun: (id) =>
			read<undefined>(`/me/shuffle/runs/${encodeURIComponent(id)}`, { method: 'DELETE' }),

		setExcluded: (trackIds, on) =>
			read<undefined>('/me/shuffle/excluded', {
				method: on ? 'PUT' : 'DELETE',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ tracks: trackIds })
			}),

		async trackAlbum(trackId) {
			const response = await forEdit(`/tracks/${encodeURIComponent(trackId)}`, tagsFailureOf);
			return ((await response.json()) as { albumId: string }).albumId;
		},

		streamUrl: (trackId) => `${base}/tracks/${encodeURIComponent(trackId)}/stream`,

		startScan: (mode) =>
			read<ScanStatus>('/admin/library/scan', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ mode })
			}),

		issues: () => read<ScanIssues>('/admin/library/issues'),

		settings: () => read<ServerSettings>('/admin/settings'),

		folders: (path) =>
			read<FolderListing>(`/admin/folders?${new URLSearchParams({ path }).toString()}`),

		async changeSettings(change) {
			const response = await call('/admin/settings', {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(change)
			});
			if (response.status === 401) {
				onUnauthorized();
				throw new UnauthorizedError();
			}
			if (response.status === 400) {
				throw await settingsErrorOf(response);
			}
			if (!response.ok) {
				throw new ServerError(response.status);
			}
			return (await response.json()) as ServerSettings;
		}
	};
}
