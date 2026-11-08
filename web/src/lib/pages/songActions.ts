import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { library } from '#lib/library/library.svelte.ts';
import type { LibraryIndex } from '#lib/library/source.ts';
import { player } from '#lib/player/player.svelte.ts';
import type { Queued } from '#lib/player/queue.ts';
import { tagEdits } from './tagEdits.svelte.ts';

/** Where each song in the index is: its album, by track ID. */
export function albumsByTrack({ artists }: LibraryIndex): Map<string, string> {
	const out = new Map<string, string>();
	for (const { albums } of artists) {
		for (const { id, tracks } of albums) {
			for (const track of tracks) {
				out.set(track.id, id);
			}
		}
	}
	return out;
}

/**
 * Fetches what the player needs of songs the list only names: their albums, each once. A song
 * whose album can't be had is left out.
 */
export async function songsOf(
	trackIds: readonly string[],
	albumOf: ReadonlyMap<string, string>
): Promise<Queued[]> {
	const albumIds = [...new Set(trackIds.map((id) => albumOf.get(id)))].filter(
		(id) => id !== undefined
	);
	await Promise.all(albumIds.map((id) => library.loadAlbum(id)));
	return trackIds.flatMap((trackId) => {
		const albumId = albumOf.get(trackId);
		const album = albumId === undefined ? undefined : library.album(albumId);
		const track = album?.tracks.find(({ id }) => id === trackId);
		return album === undefined || track === undefined ? [] : [{ album, track }];
	});
}

/** What the ⋯ menu is on: one song, or the songs picked, which a right-click on one of them acts on. */
export type MenuTarget =
	| {
			kind: 'song';
			trackId: string;
			albumId: string;
			title: string;
			/** Under the title, in the mono: the album and the length. */
			line: string;
			artistId: string | undefined;
	  }
	| { kind: 'picked'; count: number; line: string; trackIds: readonly string[] };

/** What can be done with songs, one or picked: the bar's actions and the ⋯ menu's. */
export interface SongActions {
	play(): void;
	playNext(): void;
	addToQueue(): void;
	/** Hearts them all, unless every one is hearted already: then the hearts come off. */
	heart(): void;
	rate(stars: number | null): void;
	/** Set when they're all on one album the person may edit. */
	editTags: (() => void) | undefined;
}

/** The actions on some songs, in the order they should play. */
export function actionsFor(
	trackIds: readonly string[],
	albumOf: ReadonlyMap<string, string>
): SongActions {
	const albums = new Set(trackIds.map((id) => albumOf.get(id)));
	const [only] = albums;
	const editable = albums.size === 1 && only !== undefined && library.user?.canEditTags === true;
	const queued = (then: (songs: Queued[]) => void): (() => void) => {
		return () => {
			void songsOf(trackIds, albumOf).then(then);
		};
	};
	return {
		play: queued((songs) => {
			player.playSongs(songs);
		}),
		playNext: queued((songs) => {
			player.playNext(songs);
		}),
		addToQueue: queued((songs) => {
			player.addToQueue(songs);
		}),
		heart() {
			const on = !trackIds.every((id) => library.marks.hearted('tracks', id));
			for (const id of trackIds) {
				library.marks.setHeart('tracks', id, on);
			}
		},
		rate(stars) {
			for (const id of trackIds) {
				library.marks.rate(id, stars);
			}
		},
		editTags: editable
			? () => {
					tagEdits.open(only);
					void goto(resolve('/album/[id]', { id: only }));
				}
			: undefined
	};
}
