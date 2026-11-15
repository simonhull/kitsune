/**
 * The fixtures' playlists: `/me/playlists` and its routes, as the server answers them, over the
 * fixtures' albums. A song out of the library (in Trash, or gone) leaves every answer and keeps its
 * entries, as the server's do. They're kept in the session's storage, so a reload of a fixtures
 * page finds a drag where it left it.
 */
import { evaluate, type SongFacts } from './smartEval.ts';
import {
	ServerError,
	type Album,
	type LibrarySource,
	type PlaylistAdd,
	type PlaylistAdded,
	type PlaylistAlbum,
	type PlaylistDetail,
	type PlaylistEntry,
	type PlaylistPlace,
	type PlaylistSummary,
	type SmartPreview,
	type SmartPreviewRequest,
	type SmartRules,
	type Track
} from './source.ts';

/** Where the fixtures keep their playlists, so a reload of a fixtures page restores them. */
export const playlistsFixtureKey = 'kitsune:fixtures:playlists';

/** The most entries a playlist holds, as the server says. */
const mostEntries = 10_000;

interface StoredPlaylist {
	id: string;
	name: string;
	source: 'kitsune' | 'navidrome';
	createdAt: number;
	updatedAt: number;
	addedAt?: number;
	entries: { id: string; track: string }[];
	/** A smart playlist's rules, and the seed its random choices follow. */
	rules?: SmartRules;
	seed?: number;
}

interface Stored {
	playlists: StoredPlaylist[];
	/** The last ID handed out, of an entry or a playlist: never used twice. */
	made: number;
}

interface PlaylistsFixtureOptions {
	/** The albums the fixtures hold now, by ID. */
	albums: () => ReadonlyMap<string, Album>;
	/** Whether a song is out of the library: in Trash, or deleted from it. */
	gone: (trackId: string) => boolean;
	/** The person's stars for a song, as the index carries them. */
	stars: (trackId: string) => number | undefined;
	/** Every live song, in the Library's order, with what a smart rule reads of it. */
	facts?: () => SongFacts[];
	/** When "in the last 7 days" counts back from: the fixtures' plays have their own now. */
	factsNow?: () => number;
	store?: () => Storage;
	now?: () => number;
}

type PlaylistMethods = Pick<
	LibrarySource,
	| 'playlists'
	| 'playlist'
	| 'createPlaylist'
	| 'renamePlaylist'
	| 'deletePlaylist'
	| 'addToPlaylist'
	| 'removeFromPlaylist'
	| 'movePlaylistEntry'
	| 'createSmartPlaylist'
	| 'changePlaylist'
	| 'previewSmart'
	| 'reseedPlaylist'
	| 'copyPlaylist'
>;

/** The most songs a preview lists. */
const previewSongs = 40;

/**
 * The smart board's playlist: Night Ferry's songs at 3★ and up, not played in a week, and either
 * from 2011 or before or played 10 times, the 50 least recently played.
 */
export const restedRules: SmartRules = {
	match: 'all',
	rules: [
		{ field: 'albumArtist', op: 'is', value: 'Low Tide Choir' },
		{ field: 'rating', op: 'atLeast', value: 6 },
		{ field: 'lastPlayed', op: 'notInLast', value: 7 },
		{
			match: 'any',
			rules: [
				{ field: 'year', op: 'atMost', value: 2011 },
				{ field: 'plays', op: 'atLeast', value: 10 }
			]
		}
	],
	limit: { count: 50, unit: 'songs', by: 'leastRecent' },
	order: 'chosen'
};

/** The board's playlists: one brought over from Navidrome, one made here. */
function seeded(now: number): Stored {
	const day = 86_400_000;
	let made = 0;
	const entries = (tracks: readonly string[]): StoredPlaylist['entries'] =>
		tracks.map((track) => {
			made += 1;
			return { id: String(made), track };
		});
	const falling = [
		...[1, 2, 3, 4, 5, 6, 7, 8].map((n) => `night-ferry-94-${String(n)}`),
		'orbitals-14-1',
		'orbitals-14-2',
		'orbitals-14-3',
		'quiet-machines-24-2',
		'harbour-lights-6-5',
		'paper-suns-16-1',
		'night-ferry-94-2'
	];
	const mornings = ['tidewater-12-1', 'tidewater-12-2', 'tidewater-12-3', 'after-hours-19-3'];
	return {
		playlists: [
			{
				id: 'falling-up',
				name: 'Falling Up',
				source: 'navidrome',
				createdAt: now - 40 * day,
				updatedAt: now - 3 * day,
				entries: entries(falling)
			},
			{
				id: 'slow-mornings',
				name: 'Slow mornings',
				source: 'kitsune',
				createdAt: now - 10 * day,
				updatedAt: now - day,
				addedAt: now - day,
				entries: entries(mornings)
			},
			{
				id: 'rested-and-rated',
				name: 'Rested and rated',
				source: 'kitsune',
				createdAt: now - 6 * day,
				updatedAt: now - 2 * day,
				entries: [],
				rules: restedRules,
				seed: 7
			}
		],
		made
	};
}

function read(store: () => Storage): Stored | null {
	try {
		const raw = store().getItem(playlistsFixtureKey);
		return raw === null ? null : (JSON.parse(raw) as Stored);
	} catch {
		return null;
	}
}

/** The fixtures' playlist routes. */
export function createPlaylistsFixture({
	albums,
	gone,
	stars,
	facts = () => [],
	store = () => sessionStorage,
	now = Date.now,
	factsNow = now
}: PlaylistsFixtureOptions): PlaylistMethods & { trackIdsOf(id: string): string[] } {
	const held: Stored = read(store) ?? seeded(now());

	function save(): void {
		try {
			store().setItem(playlistsFixtureKey, JSON.stringify(held));
		} catch {
			// A private window: the playlists last until the tab does.
		}
	}

	function nextId(): string {
		held.made += 1;
		return String(held.made);
	}

	/** Each live song, with its album, by track ID. */
	function songs(): Map<string, { album: Album; track: Track }> {
		const out = new Map<string, { album: Album; track: Track }>();
		for (const album of albums().values()) {
			for (const track of album.tracks) {
				if (!gone(track.id)) {
					out.set(track.id, { album, track });
				}
			}
		}
		return out;
	}

	function own(id: string): StoredPlaylist {
		const found = held.playlists.find((p) => p.id === id);
		if (found === undefined) {
			throw new ServerError(404);
		}
		return found;
	}

	function albumOf({ id, title, artist, year, compilation, artwork }: Album): PlaylistAlbum {
		return {
			id,
			title,
			artist,
			compilation,
			...(year === undefined ? {} : { year }),
			...(artwork === undefined ? {} : { artwork })
		};
	}

	/** The songs a playlist holds now, by track ID: a smart one's as its rules choose them. */
	function heldBy(id: string, depth = 0): ReadonlySet<string> | undefined {
		const p = held.playlists.find((each) => each.id === id);
		if (p === undefined) {
			return undefined;
		}
		if (p.rules === undefined) {
			return new Set(p.entries.map(({ track }) => track));
		}
		if (depth >= 3) {
			return new Set();
		}
		return new Set(chosen(p.rules, p.seed ?? 0, depth + 1).kept.map(({ id: track }) => track));
	}

	function chosen(rules: SmartRules, seed: number, depth = 0): ReturnType<typeof evaluate> {
		return evaluate(rules, facts(), {
			now: factsNow(),
			seed,
			playlist: (id) => heldBy(id, depth)
		});
	}

	function smartDetail(p: StoredPlaylist, rules: SmartRules): PlaylistDetail {
		const live = songs();
		const seen = new Map<string, PlaylistAlbum>();
		const entries: PlaylistEntry[] = [];
		for (const [i, song] of chosen(rules, p.seed ?? 0).kept.entries()) {
			const found = live.get(song.id);
			if (found === undefined) {
				continue;
			}
			const { album, track } = found;
			if (!seen.has(album.id)) {
				seen.set(album.id, albumOf(album));
			}
			entries.push({
				id: String(i + 1),
				albumId: album.id,
				track: {
					id: track.id,
					title: track.title,
					durationMs: track.durationMs,
					artists: track.artists,
					...(track.discNo === undefined ? {} : { discNo: track.discNo }),
					...(track.trackNo === undefined ? {} : { trackNo: track.trackNo }),
					...(song.halves > 0 ? { stars: song.halves / 2 } : {}),
					...(song.plays > 0 ? { plays: song.plays } : {}),
					...(song.lastPlayed === null
						? {}
						: { lastPlayedAt: new Date(song.lastPlayed).toISOString() }),
					addedAt: new Date(song.added).toISOString(),
					...(song.year === undefined ? {} : { year: song.year }),
					format: song.format,
					...(song.bitrate === undefined ? {} : { bitrate: song.bitrate })
				}
			});
		}
		const albumList = [...seen.values()];
		return {
			id: p.id,
			name: p.name,
			source: p.source,
			smart: true,
			rules,
			songCount: entries.length,
			albumCount: albumList.length,
			durationMs: entries.reduce((sum, { track }) => sum + track.durationMs, 0),
			mosaic: albumList.slice(0, 4),
			createdAt: new Date(p.createdAt).toISOString(),
			updatedAt: new Date(p.updatedAt).toISOString(),
			entries,
			albums: albumList
		};
	}

	function preview({ rules, id, seed }: SmartPreviewRequest): SmartPreview {
		const stored = id === undefined ? undefined : own(id);
		const { matched, kept } = chosen(rules, stored?.seed ?? seed ?? 0);
		const all = albums();
		const order = [...new Set(kept.map(({ albumId }) => albumId))];
		return {
			songCount: kept.length,
			matchCount: matched.length,
			albumCount: order.length,
			durationMs: kept.reduce((sum, { durationMs }) => sum + durationMs, 0),
			mosaic: order.slice(0, 4).flatMap((albumId) => {
				const album = all.get(albumId);
				return album === undefined ? [] : [albumOf(album)];
			}),
			songs: kept.slice(0, previewSongs).map((song) => ({
				id: song.id,
				albumId: song.albumId,
				title: song.title,
				...(song.halves > 0 ? { stars: song.halves / 2 } : {}),
				...(song.plays > 0 ? { plays: song.plays } : {}),
				...(song.lastPlayed === null
					? {}
					: { lastPlayedAt: new Date(song.lastPlayed).toISOString() })
			}))
		};
	}

	function detail(p: StoredPlaylist): PlaylistDetail {
		if (p.rules !== undefined) {
			return smartDetail(p, p.rules);
		}
		const live = songs();
		const entries: PlaylistEntry[] = [];
		const seen = new Map<string, PlaylistAlbum>();
		let durationMs = 0;
		for (const { id, track: trackId } of p.entries) {
			const song = live.get(trackId);
			if (song === undefined) {
				continue;
			}
			const { album, track } = song;
			if (!seen.has(album.id)) {
				seen.set(album.id, albumOf(album));
			}
			durationMs += track.durationMs;
			const rated = stars(track.id);
			entries.push({
				id,
				albumId: album.id,
				track: {
					id: track.id,
					title: track.title,
					durationMs: track.durationMs,
					artists: track.artists,
					...(track.discNo === undefined ? {} : { discNo: track.discNo }),
					...(track.trackNo === undefined ? {} : { trackNo: track.trackNo }),
					...(rated === undefined ? {} : { stars: rated })
				}
			});
		}
		const albumList = [...seen.values()];
		return {
			id: p.id,
			name: p.name,
			source: p.source,
			smart: false,
			songCount: entries.length,
			albumCount: albumList.length,
			durationMs,
			mosaic: albumList.slice(0, 4),
			createdAt: new Date(p.createdAt).toISOString(),
			updatedAt: new Date(p.updatedAt).toISOString(),
			...(p.addedAt === undefined ? {} : { addedAt: new Date(p.addedAt).toISOString() }),
			entries,
			albums: albumList
		};
	}

	function summary(p: StoredPlaylist, holding?: ReadonlySet<string>): PlaylistSummary {
		// eslint-disable-next-line @typescript-eslint/no-unused-vars -- A summary is the detail without these.
		const { entries, albums: _albums, ...rest } = detail(p);
		if (holding === undefined) {
			return rest;
		}
		const holds = new Set(
			entries.filter(({ track }) => holding.has(track.id)).map(({ track }) => track.id)
		);
		return { ...rest, holds: holds.size };
	}

	/** A regular playlist's own: a smart one has no entries to change. */
	function regular(id: string): StoredPlaylist {
		const p = own(id);
		if (p.rules !== undefined) {
			throw new ServerError(409);
		}
		return p;
	}

	function named(name: string): string {
		const trimmed = name.trim();
		if (trimmed === '' || [...trimmed].length > 200) {
			throw new ServerError(400);
		}
		return trimmed;
	}

	function add(p: StoredPlaylist, { tracks, again = false, before }: PlaylistAdd): PlaylistAdded {
		const live = songs();
		const missing = tracks.filter((id) => !live.has(id));
		if (missing.length > 0) {
			throw new ServerError(422);
		}
		const there = new Set(p.entries.map(({ track }) => track));
		const going: string[] = [];
		const skipped: string[] = [];
		for (const id of tracks) {
			if (!again && there.has(id)) {
				if (!skipped.includes(id)) {
					skipped.push(id);
				}
				continue;
			}
			there.add(id);
			going.push(id);
		}
		if (p.entries.length + going.length > mostEntries) {
			throw new ServerError(422);
		}
		const at =
			before === undefined ? p.entries.length : p.entries.findIndex(({ id }) => id === before);
		if (at < 0) {
			throw new ServerError(404);
		}
		const made = going.map((track) => ({ id: nextId(), track }));
		p.entries.splice(at, 0, ...made);
		if (made.length > 0) {
			p.updatedAt = now();
			p.addedAt = p.updatedAt;
		}
		save();
		return { playlist: summary(p), added: made, skipped };
	}

	async function answer<T>(make: () => T): Promise<T> {
		await Promise.resolve();
		return structuredClone(make());
	}

	return {
		trackIdsOf: (id) => [...(heldBy(id) ?? [])],
		playlists: (holding) =>
			answer(() =>
				held.playlists
					// Asked what they hold, smart ones aren't listed: nothing is added to one.
					.filter((p) => holding === undefined || p.rules === undefined)
					.map((p) => summary(p, holding === undefined ? undefined : new Set(holding)))
					.toSorted((a, b) => a.name.localeCompare(b.name, 'en', { sensitivity: 'base' }))
			),
		playlist: (id) => answer(() => detail(own(id))),
		createPlaylist: (name, tracks) =>
			answer(() => {
				const trimmed = name.trim();
				if (trimmed === '' || [...trimmed].length > 200) {
					throw new ServerError(400);
				}
				const at = now();
				const p: StoredPlaylist = {
					id: `playlist-${nextId()}`,
					name: trimmed,
					source: 'kitsune',
					createdAt: at,
					updatedAt: at,
					entries: []
				};
				held.playlists.push(p);
				return add(p, { tracks });
			}),
		renamePlaylist: (id, name) =>
			answer(() => {
				const p = own(id);
				const trimmed = name.trim();
				if (trimmed === '' || [...trimmed].length > 200) {
					throw new ServerError(400);
				}
				p.name = trimmed;
				p.updatedAt = now();
				save();
				return summary(p);
			}),
		deletePlaylist: (id) =>
			answer(() => {
				own(id);
				held.playlists = held.playlists.filter((p) => p.id !== id);
				save();
			}),
		addToPlaylist: (id, request) => answer(() => add(regular(id), request)),
		removeFromPlaylist: (id, entries) =>
			answer(() => {
				const p = regular(id);
				const taking = new Set(entries);
				const taken = p.entries
					.filter(({ id: entry }) => taking.has(entry))
					.map(({ id: entry }) => entry);
				p.entries = p.entries.filter(({ id: entry }) => !taking.has(entry));
				if (taken.length > 0) {
					p.updatedAt = now();
				}
				save();
				return taken;
			}),
		movePlaylistEntry: (id, entry, place: PlaylistPlace) =>
			answer(() => {
				const p = regular(id);
				const anchor = 'before' in place ? place.before : place.after;
				const from = p.entries.findIndex(({ id: each }) => each === entry);
				if (from < 0 || !p.entries.some(({ id: each }) => each === anchor)) {
					throw new ServerError(404);
				}
				if (entry === anchor) {
					return;
				}
				const [moving] = p.entries.splice(from, 1);
				const to =
					p.entries.findIndex(({ id: each }) => each === anchor) + ('after' in place ? 1 : 0);
				if (moving !== undefined) {
					p.entries.splice(to, 0, moving);
				}
				p.updatedAt = now();
				save();
			}),
		createSmartPlaylist: (name, rules, seed) =>
			answer(() => {
				const at = now();
				const p: StoredPlaylist = {
					id: `playlist-${nextId()}`,
					name: named(name),
					source: 'kitsune',
					createdAt: at,
					updatedAt: at,
					entries: [],
					rules: structuredClone(rules),
					seed
				};
				held.playlists.push(p);
				save();
				return summary(p);
			}),
		changePlaylist: (id, change) =>
			answer(() => {
				const p = own(id);
				if (change.rules !== undefined && p.rules === undefined) {
					throw new ServerError(409);
				}
				if (change.name !== undefined) {
					p.name = named(change.name);
				}
				if (change.rules !== undefined) {
					p.rules = structuredClone(change.rules);
				}
				p.updatedAt = now();
				save();
				return summary(p);
			}),
		previewSmart: (request, signal) =>
			answer(() => {
				signal?.throwIfAborted();
				return preview(request);
			}),
		reseedPlaylist: (id) =>
			answer(() => {
				const p = own(id);
				if (p.rules === undefined) {
					throw new ServerError(409);
				}
				p.seed = ((p.seed ?? 0) * 7919 + 104_729) % 2 ** 31;
				p.updatedAt = now();
				save();
				return summary(p);
			}),
		copyPlaylist: (id) =>
			answer(() => {
				const from = detail(own(id));
				const at = now();
				const p: StoredPlaylist = {
					id: `playlist-${nextId()}`,
					name: `${from.name} (copy)`.slice(0, 200),
					source: 'kitsune',
					createdAt: at,
					updatedAt: at,
					...(from.entries.length > 0 ? { addedAt: at } : {}),
					entries: from.entries.map(({ track }) => ({ id: nextId(), track: track.id }))
				};
				held.playlists.push(p);
				save();
				return summary(p);
			})
	};
}
