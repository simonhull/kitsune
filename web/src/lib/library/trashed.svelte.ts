import { SvelteMap } from 'svelte/reactivity';
import type { Load } from './library.svelte.ts';
import type { Album, LibraryAlbum, LibraryIndex } from './source.ts';

/** A song on its way to Trash: the album it was on, and whether the server has taken it. */
interface Hidden {
	albumId: string;
	confirmed: boolean;
}

/**
 * What this browser has moved to Trash, which every list leaves out at once, before the server has
 * answered, and which an Undo or a failure gives back in place. The Library reads every list through
 * it; with nothing on its way, each one is the very object the server gave.
 */
export class Trashed {
	/** By track ID. */
	readonly #songs = new SvelteMap<string, Hidden>();
	/** Albums moved whole, by ID: the grids leave them out too, before their songs are known. */
	readonly #albums = new SvelteMap<string, boolean>();
	/** Moves on at every change, so what was worked out for the last one is let go. */
	#version = $state(0);
	readonly #memo = new WeakMap<object, { version: number; out: unknown }>();

	/** Whether nothing is on its way to Trash. */
	get empty(): boolean {
		return this.#songs.size === 0 && this.#albums.size === 0;
	}

	/** Whether a song is left out of the lists. */
	hides(trackId: string, albumId?: string): boolean {
		return this.#songs.has(trackId) || (albumId !== undefined && this.#albums.has(albumId));
	}

	/** Whether an album is left out of the grids: moved whole. */
	hidesAlbum(albumId: string): boolean {
		return this.#albums.has(albumId);
	}

	/** Leaves songs, and albums moved whole, out of every list, until the server says otherwise. */
	hide(
		songs: readonly { trackId: string; albumId: string }[],
		albums: readonly string[] = []
	): void {
		for (const { trackId, albumId } of songs) {
			this.#songs.set(trackId, { albumId, confirmed: false });
		}
		for (const id of albums) {
			this.#albums.set(id, false);
		}
		this.#version += 1;
	}

	/** The server took them: the next change it tells of for their albums lets them go. */
	confirm(change: { tracks: readonly string[]; albums: readonly string[] }, whole: boolean): void {
		const [first] = change.albums;
		for (const id of change.tracks) {
			const held = this.#songs.get(id);
			this.#songs.set(id, { albumId: held?.albumId ?? first ?? '', confirmed: true });
		}
		if (whole) {
			for (const id of change.albums) {
				this.#albums.set(id, true);
			}
		}
		this.#version += 1;
	}

	/** Gives songs and albums back to the lists, where they were: an Undo, a failure, a restore. */
	show(trackIds: readonly string[], albums: readonly string[] = []): void {
		for (const id of trackIds) {
			this.#songs.delete(id);
		}
		for (const id of albums) {
			this.#albums.delete(id);
		}
		this.#version += 1;
	}

	/**
	 * The server has told of these albums since, and the lists hold what it said: what it took off
	 * them needs leaving out no more, and one put back on another device shows again.
	 */
	settled(albumIds: readonly string[], all = false): void {
		const settledOn = (id: string): boolean => all || albumIds.includes(id);
		let changed = false;
		for (const [id, { albumId, confirmed }] of this.#songs) {
			if (confirmed && settledOn(albumId)) {
				this.#songs.delete(id);
				changed = true;
			}
		}
		for (const [id, confirmed] of this.#albums) {
			if (confirmed && settledOn(id)) {
				this.#albums.delete(id);
				changed = true;
			}
		}
		if (changed) {
			this.#version += 1;
		}
	}

	/** Forgets everything: a sign-out. */
	clear(): void {
		this.#songs.clear();
		this.#albums.clear();
		this.#version += 1;
	}

	/** An index less what's on its way, with albums and headings left empty gone. */
	index(load: Load<LibraryIndex>): Load<LibraryIndex> {
		if (load.kind !== 'ready' || this.#nothingOn()) {
			return load;
		}
		return this.#remembered(load, () => {
			const artists = load.value.artists
				.map((artist) => {
					const albums = artist.albums.flatMap((album) => {
						const left = this.rows(album);
						return left.tracks.length === 0 ? [] : [left];
					});
					return albums.length === artist.albums.length &&
						albums.every((album, at) => album === artist.albums[at])
						? artist
						: { ...artist, albums };
				})
				.filter(({ albums }) => albums.length > 0);
			return { kind: 'ready', value: { artists } };
		});
	}

	/** An album's song rows less what's on its way. */
	rows(album: LibraryAlbum): LibraryAlbum {
		return this.#without(album);
	}

	/** An album less its songs on their way. */
	album(album: Album): Album {
		return this.#without(album);
	}

	#without<T extends { id: string; tracks: readonly { id: string }[] }>(album: T): T {
		if (this.#nothingOn()) {
			return album;
		}
		const whole = this.#albums.has(album.id);
		if (!whole && !album.tracks.some(({ id }) => this.#songs.has(id))) {
			return album;
		}
		return this.#remembered(album, () => ({
			...album,
			tracks: whole ? [] : album.tracks.filter(({ id }) => !this.#songs.has(id))
		}));
	}

	/** Whether nothing is on its way; reading it ties a caller to every change. */
	#nothingOn(): boolean {
		return this.#version >= 0 && this.empty;
	}

	/** What `make` works out for `from`, once per change: a list read twice is the same object. */
	#remembered<T extends object>(from: T, make: () => T): T {
		const held = this.#memo.get(from);
		if (held !== undefined && held.version === this.#version) {
			return held.out as T;
		}
		const out = make();
		this.#memo.set(from, { version: this.#version, out });
		return out;
	}
}
