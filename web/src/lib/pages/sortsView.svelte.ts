import { library } from '#lib/library/library.svelte.ts';
import type { PrefKey, Prefs } from '#lib/library/source.ts';
import {
	defaultAlbumSort,
	defaultArtistSongs,
	defaultArtistSort,
	defaultLibrarySort,
	savedAlbumSort,
	savedArtistSongs,
	savedArtistSort,
	savedLibrarySort
} from '#lib/library/sorts.ts';

type SortKey = Extract<PrefKey, 'library.sort' | 'albums.sort' | 'artists.sort' | 'artist.songs'>;

/** One list's order: the default until the server answers, then what it has. A change shows at once. */
class SortPref<K extends SortKey> {
	value: Prefs[K] = $state.raw() as Prefs[K];
	/** Set once the server has answered, or failed to: a list in a chosen order waits, not to show two. */
	known = $state(false);
	readonly #key: K;
	readonly #initial: Prefs[K];
	readonly #read: (saved: unknown) => Prefs[K];
	#asked: Promise<void> | null = null;

	constructor(key: K, initial: Prefs[K], read: (saved: unknown) => Prefs[K]) {
		this.#key = key;
		this.#initial = initial;
		this.#read = read;
		this.value = initial;
		// An order chosen on another device follows here, as the columns do.
		library.watchPref(key, (saved) => {
			this.value = read(saved);
			this.known = true;
		});
	}

	/** Asks the server, once a session. */
	load(): Promise<void> {
		this.#asked ??= library.prefs.get(this.#key).then((saved) => {
			// A choice made while the answer was on its way is newer than it.
			if (!this.known) {
				this.value = this.#read(saved);
				this.known = true;
			}
		});
		return this.#asked;
	}

	set(next: Prefs[K]): void {
		this.value = next;
		this.known = true;
		library.prefs.put(this.#key, next);
	}

	reset(): void {
		this.value = this.#initial;
		this.known = false;
		this.#asked = null;
	}
}

/**
 * The orders a person chose for the Library, the Albums page and the Artists page, and how an
 * artist page's songs run.
 */
export const sorts = {
	library: new SortPref('library.sort', defaultLibrarySort, savedLibrarySort),
	albums: new SortPref('albums.sort', defaultAlbumSort, savedAlbumSort),
	artists: new SortPref('artists.sort', defaultArtistSort, savedArtistSort),
	artistSongs: new SortPref('artist.songs', defaultArtistSongs, savedArtistSongs),
	reset(): void {
		this.library.reset();
		this.albums.reset();
		this.artists.reset();
		this.artistSongs.reset();
	}
};
