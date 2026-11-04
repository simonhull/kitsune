import { SvelteMap } from 'svelte/reactivity';
import { createFixtureSource } from './fixtures.ts';
import type { Album, AlbumEntry, Home, LibrarySource, NowPlaying } from './source.ts';

/** The client's view of the library. List entries seed the first frame of the next page. */
class Library {
	home = $state<Home | null>(null);
	playing = $state<NowPlaying | null>(null);

	readonly #source: LibrarySource;
	readonly #entries = new SvelteMap<string, AlbumEntry>();
	readonly #albums = new SvelteMap<string, Album>();
	readonly #missing = new SvelteMap<string, unknown>();
	readonly #loading = new Set<string>();

	constructor(source: LibrarySource) {
		this.#source = source;
	}

	/** Returns what's known about an album right now, without waiting. */
	entry(id: string): AlbumEntry | undefined {
		return this.#albums.get(id) ?? this.#entries.get(id);
	}

	album(id: string): Album | undefined {
		return this.#albums.get(id);
	}

	/** Reports whether the album failed to load. */
	failed(id: string): boolean {
		return this.#missing.has(id);
	}

	async loadShell(): Promise<void> {
		const [home, playing] = await Promise.all([this.#source.home(), this.#source.nowPlaying()]);
		const { continueListening, recentlyAdded, onRepeat } = home;
		for (const entry of [continueListening.album, ...recentlyAdded, ...onRepeat, playing.album]) {
			this.#entries.set(entry.id, entry);
		}
		this.home = home;
		this.playing = playing;
	}

	/** Streams an album's tracks in. Failures surface through {@link Library.failed}. */
	async loadAlbum(id: string): Promise<void> {
		if (this.#albums.has(id) || this.#loading.has(id)) {
			return;
		}
		this.#loading.add(id);
		try {
			this.#albums.set(id, await this.#source.album(id));
		} catch (error) {
			this.#missing.set(id, error);
		} finally {
			this.#loading.delete(id);
		}
	}
}

function fromQuery(): LibrarySource {
	const query = new URLSearchParams(location.search);
	return createFixtureSource({
		latencyMs: Number(query.get('latency') ?? 0),
		playingAlbumId: query.get('playing') ?? undefined
	});
}

export const library = new Library(fromQuery());
