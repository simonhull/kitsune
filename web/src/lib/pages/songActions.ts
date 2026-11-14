import { clock } from '#lib/library/format.ts';
import { library } from '#lib/library/library.svelte.ts';
import type { LibraryIndex } from '#lib/library/source.ts';
import { player } from '#lib/player/player.svelte.ts';
import { shuffles } from '#lib/shuffle/session.svelte.ts';
import { selectionSource } from '#lib/shuffle/sources.ts';
import type { Queued } from '#lib/player/queue.ts';
import { tagEditor } from './tagEditor.svelte.ts';
import { trashBin } from './trashBin.svelte.ts';

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

/** How many albums some songs are on, and how long they last together. */
export interface PickedFacts {
	albums: number;
	durationMs: number;
}

/** What a list knows of a song it shows: its album and its length. */
export type SongFacts = (trackId: string) => { albumId: string; durationMs: number } | undefined;

export function pickedFacts(trackIds: readonly string[], facts: SongFacts): PickedFacts {
	const known = trackIds.flatMap((id) => facts(id) ?? []);
	return {
		albums: new Set(known.map(({ albumId }) => albumId)).size,
		durationMs: known.reduce((sum, { durationMs }) => sum + durationMs, 0)
	};
}

/** The facts as the menu and the bar say them: `1 ALBUM · 28:58`. */
export function factsLine({ albums, durationMs }: PickedFacts): string {
	return `${albums.toLocaleString('en-US')} ${albums === 1 ? 'ALBUM' : 'ALBUMS'} · ${clock(durationMs / 1000)}`;
}

/**
 * What the ⋯ menu opens on, from a row: the picked songs when it's one of several picked, or else
 * the song alone. `song` says what the menu needs of a song the list holds. `picked` is asked for
 * only when it's needed: in the Library it runs over every song.
 */
export function menuTarget(
	trackId: string,
	selection: { has(id: string): boolean; readonly size: number },
	pickedOf: () => readonly string[],
	facts: SongFacts,
	song: (
		trackId: string
	) => Omit<Extract<MenuTarget, { kind: 'song' }>, 'kind' | 'trackId'> | undefined
): MenuTarget | undefined {
	if (selection.size > 1 && selection.has(trackId)) {
		const picked = pickedOf();
		return {
			kind: 'picked',
			count: picked.length,
			line: factsLine(pickedFacts(picked, facts)),
			trackIds: picked
		};
	}
	const one = song(trackId);
	return one === undefined ? undefined : { kind: 'song', trackId, ...one };
}

/** An item of a song's ⋯ menu. */
export type RowMenuItem =
	| 'play'
	| 'playNext'
	| 'addToQueue'
	| 'rating'
	| 'editTags'
	| 'remove'
	| 'goToAlbum'
	| 'goToArtist'
	| 'fileInfo'
	| 'trash';

/**
 * What a song's ⋯ menu holds, in order. One song: Play next and Add to queue, never Play, which a
 * click on the row does. Songs picked: Play first. Then the marks and Edit tags…, Remove from queue
 * in Up next, and where to go, less the page it's opened on. Last, after a rule, Move to Trash, for
 * an administrator on a server that may change its files.
 */
export function rowMenuItems(
	target: Pick<MenuTarget, 'kind'> & { artistId?: string | undefined },
	context: {
		here?: 'album' | 'artist' | undefined;
		editable: boolean;
		removable: boolean;
		trashable?: boolean;
	}
): RowMenuItem[] {
	const items: RowMenuItem[] = [];
	if (target.kind === 'picked') {
		items.push('play');
	}
	items.push('playNext', 'addToQueue', 'rating');
	if (context.editable) {
		items.push('editTags');
	}
	if (context.removable) {
		items.push('remove');
	}
	if (target.kind === 'song') {
		if (context.here !== 'album') {
			items.push('goToAlbum');
		}
		if (target.artistId !== undefined && context.here !== 'artist') {
			items.push('goToArtist');
		}
		items.push('fileInfo');
	}
	if (context.trashable === true) {
		items.push('trash');
	}
	return items;
}

/** A song's title, from whatever this browser holds of its album. */
function titleOf(trackId: string, albumId: string | undefined): string | undefined {
	if (albumId === undefined) {
		return undefined;
	}
	const find = (tracks: readonly { id: string; title: string }[] | undefined) =>
		tracks?.find(({ id }) => id === trackId)?.title;
	return find(library.album(albumId)?.tracks) ?? find(library.albumRows(albumId, [])?.tracks);
}

/** What can be done with songs, one or picked: the bar's actions and the ⋯ menu's. */
export interface SongActions {
	play(): void;
	/** Shuffles them, each once, by the person's rules: the first plays at once. */
	shuffle(): void;
	playNext(): void;
	addToQueue(): void;
	rate(stars: number | null): void;
	/** Opens the tag editor on them. Set for a person who may edit tags, once each song's album is known. */
	editTags: (() => void) | undefined;
	/** Moves them to Trash. Set for an administrator on a server that may change its files. */
	trash: (() => void) | undefined;
}

/**
 * The actions on some songs, in the order they should play. `index` is the list they're in, which
 * a shuffle of them picks its first from: the Library's, unless a page holds its own.
 */
export function actionsFor(
	trackIds: readonly string[],
	albumOf: ReadonlyMap<string, string>,
	index?: () => LibraryIndex
): SongActions {
	const editable =
		trackIds.length > 0 &&
		library.user?.canEditTags === true &&
		trackIds.every((id) => albumOf.has(id));
	const queued = (then: (songs: Queued[]) => void): (() => void) => {
		return () => {
			void songsOf(trackIds, albumOf).then(then);
		};
	};
	return {
		play: queued((songs) => {
			player.playSongs(songs);
		}),
		shuffle() {
			shuffles.start(selectionSource(trackIds, index));
		},
		playNext: queued((songs) => {
			player.playNext(songs);
		}),
		addToQueue: queued((songs) => {
			player.addToQueue(songs);
		}),
		rate(stars) {
			for (const id of trackIds) {
				library.marks.rate(id, stars);
			}
		},
		editTags: editable
			? () => {
					const songs = trackIds.map((id) => ({ id, albumId: albumOf.get(id) ?? '', title: '' }));
					const [one] = songs;
					// One song opens on its album, it alone ticked and open to every tag.
					tagEditor.open(
						songs.length === 1 && one !== undefined
							? { kind: 'album', albumId: one.albumId, ticked: [one.id], open: one.id }
							: { kind: 'songs', songs },
						true
					);
				}
			: undefined,
		trash: trashBin.can
			? () => {
					const [first] = trackIds;
					const title =
						trackIds.length === 1 && first !== undefined
							? titleOf(first, albumOf.get(first))
							: undefined;
					trashBin.moveSongs(trackIds, (id) => albumOf.get(id), title);
				}
			: undefined
	};
}
