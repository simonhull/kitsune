import type { Album, PlaybackRecord, PlaybackSave, PlaybackSong } from './source.ts';

/** A record the fixtures start with: saved by "another" browser, or one whose song is gone. */
export type PlaybackFixture = 'phone' | 'gone';

/** Where the fixtures keep their record, so a reload of a fixtures page restores it. */
export const playbackFixtureKey = 'kitsune:fixtures:playback';

/** The "other" browser's ID: never one this browser makes. */
const elsewhere = 'feedfacecafebeef';
const twoHoursMs = 2 * 60 * 60 * 1000;

interface Stored {
	save: PlaybackSave;
	updatedAt: number;
}

interface PlaybackFixtureOptions {
	/** The albums the fixtures hold now, by ID: what a record resolves against. */
	albums: () => ReadonlyMap<string, Album>;
	/** Where the record is kept across reloads: the test double's own memory, not the product's. */
	store?: () => Storage;
	seed?: PlaybackFixture | undefined;
	now?: () => number;
}

/** The fixtures' `GET` and `PUT /me/playback`, resolving gone songs as the server does. */
export function createPlaybackFixture({
	albums,
	store = () => sessionStorage,
	seed,
	now = Date.now
}: PlaybackFixtureOptions): {
	playback(): PlaybackRecord | null;
	savePlayback(save: PlaybackSave): void;
} {
	let held: Stored | null = read(store) ?? seeded(seed, albums(), now());

	function albumOf(trackId: string): string | undefined {
		for (const album of albums().values()) {
			if (album.tracks.some(({ id }) => id === trackId)) {
				return album.id;
			}
		}
		return undefined;
	}

	function live(ids: readonly string[] | undefined): { songs: PlaybackSong[]; at: number[] } {
		const songs: PlaybackSong[] = [];
		const at: number[] = [];
		(ids ?? []).forEach((id, place) => {
			const albumId = albumOf(id);
			if (albumId !== undefined) {
				songs.push({ id, albumId });
				at.push(place);
			}
		});
		return { songs, at };
	}

	return {
		playback() {
			if (held === null) {
				return null;
			}
			const { save, updatedAt } = held;
			const tracks = live(save.tracks);
			const index = tracks.at.findIndex((place) => place >= save.index);
			if (index < 0) {
				return null;
			}
			const moved = tracks.at[index] !== save.index;
			const later = live(save.later).songs;
			const order = live(save.shuffled?.order).songs;
			return {
				kind: save.shuffle === undefined ? save.kind : 'songs',
				tracks: tracks.songs,
				...(later.length > 0 ? { later } : {}),
				index,
				positionMs: moved ? 0 : save.positionMs,
				ordered: save.ordered,
				moved,
				...(save.continues !== undefined && albums().has(save.continues)
					? { continues: save.continues }
					: {}),
				// The fixtures' shuffles live in the tab: after a reload, its session is gone.
				repeat: save.repeat ?? 'off',
				...(save.shuffled !== undefined && order.length > 0
					? { shuffled: { seed: save.shuffled.seed, order } }
					: {}),
				clientId: save.clientId,
				device: save.device,
				updatedAt
			};
		},
		savePlayback(save) {
			held = { save: structuredClone(save), updatedAt: now() };
			try {
				store().setItem(playbackFixtureKey, JSON.stringify(held));
			} catch {
				// Storage is off: the record lasts as long as the page.
			}
		}
	};
}

function read(store: () => Storage): Stored | null {
	try {
		const raw = store().getItem(playbackFixtureKey);
		return raw === null ? null : (JSON.parse(raw) as Stored);
	} catch {
		return null;
	}
}

/** The record a seed starts with, on the newest album. */
function seeded(
	seed: PlaybackFixture | undefined,
	albums: ReadonlyMap<string, Album>,
	now: number
): Stored | null {
	const album = albums.get('glasshouse-66');
	if (seed === undefined || album === undefined) {
		return null;
	}
	const ids = album.tracks.map(({ id }) => id);
	const save: PlaybackSave =
		seed === 'phone'
			? {
					kind: 'album',
					tracks: ids,
					index: 3,
					positionMs: 133_000,
					ordered: true,
					clientId: elsewhere,
					device: 'phone'
				}
			: {
					kind: 'songs',
					tracks: ['gone-from-the-library', ...ids],
					index: 0,
					positionMs: 60_000,
					ordered: false,
					clientId: elsewhere,
					device: 'computer'
				};
	return { save, updatedAt: now - twoHoursMs };
}
