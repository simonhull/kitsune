import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { library } from '#lib/library/library.svelte.ts';
import type { Album, AlbumEntry } from '#lib/library/source.ts';
import { matchAlbum } from '#lib/matching/hooks.ts';
import { player } from '#lib/player/player.svelte.ts';
import { shuffles } from '#lib/shuffle/session.svelte.ts';
import { albumSource } from '#lib/shuffle/sources.ts';
import { tagEdits } from './tagEdits.svelte.ts';

/** What can be done with a whole album, in album order: its ⋯ menu's actions. */
export interface AlbumActions {
	play(): void;
	playNext(): void;
	addToQueue(): void;
	shuffle(): void;
	/** Set for a person who may edit tags. */
	editTags: (() => void) | undefined;
	/** Opens the match sheet for the album's release. Set for a person who may edit tags. */
	findMatches: (() => void) | undefined;
	/** Set once the album says who it's by. */
	goToArtist: (() => void) | undefined;
}

/** The songs of an album, ready to queue. */
function songsOf(album: Album): { album: Album; track: Album['tracks'][number] }[] {
	return album.tracks.map((track) => ({ album, track }));
}

/**
 * The actions on an album. `album` is the album itself, once it's here: each action fetches it if
 * it isn't, and acts when it comes.
 */
export function albumActions(id: string, album: Album | undefined): AlbumActions {
	const withAlbum = (then: (held: Album) => void): (() => void) => {
		return () => {
			if (album !== undefined) {
				then(album);
				return;
			}
			void library.loadAlbum(id).then(() => {
				const held = library.album(id);
				if (held !== undefined) {
					then(held);
				}
			});
		};
	};
	const artistId = album?.artists.at(0)?.id;
	return {
		play: withAlbum((held) => {
			player.play(held);
		}),
		playNext: withAlbum((held) => {
			player.playNext(songsOf(held));
		}),
		addToQueue: withAlbum((held) => {
			player.addToQueue(songsOf(held));
		}),
		shuffle: withAlbum((held) => {
			shuffles.start(albumSource(held));
		}),
		editTags:
			library.user?.canEditTags === true
				? () => {
						tagEdits.open(id);
						void goto(resolve('/album/[id]', { id }));
					}
				: undefined,
		findMatches:
			library.user?.canEditTags === true
				? () => {
						matchAlbum(id);
					}
				: undefined,
		goToArtist:
			artistId === undefined
				? undefined
				: () => {
						void goto(resolve('/artists/[id]', { id: artistId }));
					}
	};
}

/**
 * The one album menu: the hero's ⋯, and a right-click (or ⋯ on a phone) on any album card. It
 * lives once in the layout, so a card holds nothing for it.
 */
class AlbumMenu {
	current = $state.raw<{ x: number; y: number; entry: AlbumEntry } | null>(null);
	#opener: HTMLElement | null = null;

	open(entry: AlbumEntry, x: number, y: number): void {
		this.#opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		this.current = { x, y, entry };
		// Its songs and who it's by, for the menu's line and Go to artist.
		void library.loadAlbum(entry.id);
	}

	close(refocus: boolean): void {
		this.current = null;
		if (refocus) {
			this.#opener?.focus();
		}
	}
}

export const albumMenu = new AlbumMenu();
