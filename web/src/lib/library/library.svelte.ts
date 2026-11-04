import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { createApiSource } from './api.ts';
import { cueOf, type NowPlaying } from './cue.ts';
import { createFixtureSource } from './fixtures.ts';
import {
	AlbumNotFoundError,
	UnauthorizedError,
	type Album,
	type AlbumEntry,
	type LibraryFacts,
	type LibrarySource,
	type User
} from './source.ts';

/** The feature tile and the five beside it. */
const recentCount = 6;
/** How long to leave a server that isn't answering, or a library still empty. */
const retryMs = 5000;

export type Session = 'signed-in' | 'signed-out' | 'down';

export interface LibraryOptions {
	/** Holds the cue on this album, for the lab: a hero condensing into the island needs another. */
	pinnedAlbumId?: string;
}

/** The client's view of the library. List entries seed the first frame of the next page. */
export class Library {
	user = $state.raw<User | null>(null);
	facts = $state.raw<LibraryFacts | null>(null);
	/** Newest first: the feature album, then what's beside it. */
	recent = $state.raw<AlbumEntry[]>([]);
	/** Every album loaded so far, by name. */
	albums = $state.raw<AlbumEntry[]>([]);
	/** Whether the server has more albums than {@link Library.albums} holds. */
	hasMoreAlbums = $state(false);
	/** Set once a first load has failed for any reason but the session. */
	down = $state(false);
	/** Whether the first scan is still running; only known, and only asked, while there's nothing. */
	scanning = $state(false);
	playing = $state.raw<NowPlaying | null>(null);
	/** Sends the app to the door. Set by the shell once it can navigate. */
	onExpired: (() => Promise<void>) | null = null;

	readonly #source: LibrarySource;
	readonly #pinnedAlbumId: string | undefined;
	readonly #entries = new SvelteMap<string, AlbumEntry>();
	readonly #albums = new SvelteMap<string, Album>();
	readonly #missing = new SvelteSet<string>();
	readonly #unavailable = new SvelteSet<string>();
	readonly #loading = new SvelteSet<string>();
	#session: Promise<Session> | null = null;
	#shell: Promise<void> | null = null;
	#cursor: string | undefined;
	#more: Promise<void> | null = null;
	#retry: ReturnType<typeof setTimeout> | undefined;
	#expiring = false;

	constructor(
		source: LibrarySource | ((expired: () => void) => LibrarySource),
		options: LibraryOptions = {}
	) {
		this.#source = typeof source === 'function' ? source(() => void this.expire()) : source;
		this.#pinnedAlbumId = options.pinnedAlbumId;
	}

	/** Returns what's known about an album right now, without waiting. */
	entry(id: string): AlbumEntry | undefined {
		return this.#albums.get(id) ?? this.#entries.get(id);
	}

	album(id: string): Album | undefined {
		return this.#albums.get(id);
	}

	/** Reports whether the server says this album doesn't exist. */
	failed(id: string): boolean {
		return this.#missing.has(id);
	}

	/** Reports whether the album couldn't be loaded this time; {@link Library.loadAlbum} retries. */
	unavailable(id: string): boolean {
		return this.#unavailable.has(id);
	}

	/** Asks who's signed in, once: every navigation must not wait for this. */
	checkSession(): Promise<Session> {
		this.#session ??= this.#askSession();
		return this.#session;
	}

	async #askSession(): Promise<Session> {
		try {
			this.user = await this.#source.me();
			return 'signed-in';
		} catch (error) {
			return error instanceof UnauthorizedError ? 'signed-out' : 'down';
		}
	}

	/** Exchanges a name and password for a session. Rejects with `LoginError`. */
	async login(name: string, password: string): Promise<void> {
		this.user = await this.#source.login(name, password);
		this.#session = Promise.resolve('signed-in');
	}

	/** Loads everything the shell shows. Failures surface through {@link Library.down}. */
	loadShell(): Promise<void> {
		this.#shell ??= this.#loadShell().finally(() => {
			this.#shell = null;
		});
		return this.#shell;
	}

	async #loadShell(): Promise<void> {
		clearTimeout(this.#retry);
		try {
			this.user ??= await this.#source.me();
			const [facts, recent, first] = await Promise.all([
				this.#source.facts(),
				this.#source.albumsPage('added', undefined, recentCount),
				this.#source.albumsPage('name')
			]);
			this.#remember([...recent.albums, ...first.albums]);
			this.recent = recent.albums;
			this.albums = first.albums;
			this.#cursor = first.nextCursor;
			this.hasMoreAlbums = first.nextCursor !== undefined;
			await this.#cueOpening();
			this.facts = facts;
			this.down = false;
			if (facts.albums === 0) {
				this.scanning = facts.roots > 0 && !(await this.#source.ready().catch(() => true));
				this.#retryShell();
			}
		} catch (error) {
			if (!(error instanceof UnauthorizedError)) {
				this.down = true;
				this.#retryShell();
			}
		}
	}

	#retryShell(): void {
		clearTimeout(this.#retry);
		this.#retry = setTimeout(() => void this.loadShell(), retryMs);
	}

	#remember(entries: AlbumEntry[]): void {
		for (const entry of entries) {
			this.#entries.set(entry.id, entry);
		}
	}

	/** Cues something for the island's first frame, without holding up the page if it can't. */
	async #cueOpening(): Promise<void> {
		const id = this.#pinnedAlbumId ?? this.recent.at(0)?.id;
		if (this.playing !== null || id === undefined) {
			return;
		}
		await this.loadAlbum(id);
		this.#cue(this.#albums.get(id));
	}

	#cue(album: Album | undefined): void {
		this.playing = album === undefined ? null : cueOf(album);
	}

	/** Loads the next page of {@link Library.albums}. Only one page is ever in flight. */
	loadMoreAlbums(): Promise<void> {
		this.#more ??= this.#loadMoreAlbums().finally(() => {
			this.#more = null;
		});
		return this.#more;
	}

	async #loadMoreAlbums(): Promise<void> {
		if (this.#cursor === undefined) {
			return;
		}
		try {
			const page = await this.#source.albumsPage('name', this.#cursor);
			this.#remember(page.albums);
			this.albums = [...this.albums, ...page.albums];
			this.#cursor = page.nextCursor;
			this.hasMoreAlbums = page.nextCursor !== undefined;
		} catch {
			// The next scroll asks again; a lost session has already gone to the door.
		}
	}

	/** Streams an album's tracks in. Failures surface through {@link Library.failed}. */
	async loadAlbum(id: string): Promise<void> {
		if (this.#albums.has(id) || this.#loading.has(id)) {
			return;
		}
		this.#loading.add(id);
		this.#unavailable.delete(id);
		try {
			const album = await this.#source.album(id);
			this.#albums.set(id, album);
		} catch (error) {
			if (error instanceof AlbumNotFoundError) {
				this.#missing.add(id);
			} else if (!(error instanceof UnauthorizedError)) {
				this.#unavailable.add(id);
			}
		} finally {
			this.#loading.delete(id);
		}
	}

	/** Reacts to a lost session. Parallel calls are one event: the app leaves once. */
	async expire(): Promise<void> {
		if (this.#expiring || this.onExpired === null) {
			return;
		}
		this.#expiring = true;
		try {
			await this.onExpired();
			this.reset();
		} finally {
			this.#expiring = false;
		}
	}

	/** Forgets everything the last session saw. */
	reset(): void {
		clearTimeout(this.#retry);
		this.user = null;
		this.facts = null;
		this.recent = [];
		this.albums = [];
		this.hasMoreAlbums = false;
		this.down = false;
		this.scanning = false;
		this.playing = null;
		this.#cursor = undefined;
		this.#session = Promise.resolve('signed-out');
		this.#entries.clear();
		this.#albums.clear();
		this.#missing.clear();
		this.#unavailable.clear();
	}
}

function fromQuery(): Library {
	// Tests and the build's route analysis import this with no window: they get the API source.
	const query = new URLSearchParams(typeof location === 'undefined' ? '' : location.search);
	if (query.get('source') !== 'fixtures') {
		return new Library((expired) => createApiSource({ onUnauthorized: expired }));
	}
	return new Library(createFixtureSource({ latencyMs: Number(query.get('latency') ?? 0) }), {
		pinnedAlbumId: query.get('playing') ?? undefined
	});
}

/** Chosen once, from the URL the app opened on, and held here for every later navigation. */
export const library = fromQuery();
