import { SvelteMap } from 'svelte/reactivity';
import { library, type Library } from '#lib/library/library.svelte.ts';
import { newClientId } from '#lib/library/outbox.ts';
import type {
	AlbumEntry,
	ShuffleMode,
	ShufflePage,
	ShufflePick,
	ShuffleScope,
	ShuffleWhy,
	TrackHit
} from '#lib/library/source.ts';
import { player, type Player } from '#lib/player/player.svelte.ts';
import { albumOf, type Queued, type ShuffleRef } from '#lib/player/queue.ts';
import { pickUnit, type Unit } from './pick.ts';
import { shuffleSettings } from './settings.svelte.ts';

/** How many songs the player always has queued past the one playing, so Next never waits. */
const keepAhead = 3;
/** With this few picks left to queue, the next page is fetched. */
const refillAt = 10;
/** The longest a failed request waits before it's tried again. */
const mostRetryMs = 30_000;
/** How many songs Up next names. */
export const upNextShown = 30;

/** What a Shuffle button shuffles, and what it picks the first song from. */
export interface ShuffleSource {
	/** Names the source: a pick made ahead for one key is used for the same key. */
	key: string;
	scope: ShuffleScope;
	mode: ShuffleMode;
	/** What Now playing says it shuffles, such as `THE LIBRARY`. */
	label: string;
	/** The units here to pick the first from: worked out when they're needed. */
	units: () => Unit[] | Promise<Unit[]>;
}

/** One song or combined run coming up, as Up next shows it. */
export interface UpNextItem {
	key: string;
	/** Each song's title, in the order they play. */
	titles: string[];
	artist: string;
	/** The album of its first song, for its cover. */
	album: AlbumEntry;
	/** Its songs, in the order they play, and how long they last together. */
	trackIds: string[];
	durationMs: number;
	/** In the player's queue already, rather than a pick still to queue. */
	queued: boolean;
	/** Why it came up, when the server has said. */
	why?: ShuffleWhy;
}

/** The songs of a unit, ready to queue, if every album they're on is here already. */
function queuedNow(host: Library, unit: Unit): Queued[] | null {
	if (unit.songs.some(({ albumId }) => host.album(albumId) === undefined)) {
		return null;
	}
	return unit.songs.flatMap(({ id, albumId }) => {
		const album = host.album(albumId);
		const track = album?.tracks.find((each) => each.id === id);
		return album === undefined || track === undefined ? [] : [{ album, track }];
	});
}

/** Fetches the albums of `songs`, each once, and returns what can be queued of them. */
async function queuedLater(
	host: Library,
	songs: readonly { id: string; albumId: string }[]
): Promise<Queued[]> {
	await Promise.all(
		[...new Set(songs.map(({ albumId }) => albumId))].map((id) => host.loadAlbum(id))
	);
	return songs.flatMap(({ id, albumId }) => {
		const album = host.album(albumId);
		const track = album?.tracks.find((each) => each.id === id);
		return album === undefined || track === undefined ? [] : [{ album, track }];
	});
}

/** An album entry for a song the server named, before its album is here: enough for a cover. */
function entryOf(hit: TrackHit): AlbumEntry {
	return {
		id: hit.albumId,
		title: hit.album,
		artist: hit.artists.at(0) ?? '',
		trackCount: 0,
		compilation: false,
		artwork: hit.artwork
	};
}

/**
 * One shuffle under way. Its first song is already playing; the server's session gives the rest,
 * a page at a time, and the player is kept a few songs ahead of where it is.
 */
export class Shuffle {
	/** The client's ID at first, the server's session once it answers: what listens name. */
	readonly ref: ShuffleRef = { id: `pending-${newClientId()}` };
	readonly source: ShuffleSource;
	/** Picks the server gave that aren't queued yet, in order. */
	picks = $state.raw<ShufflePick[]>([]);
	/** How many songs it picks from, once the server has said. */
	songs = $state<number | null>(null);
	done = $state(false);
	/** Why each queued song came up, by track ID, and which songs came up together. */
	readonly #why = new SvelteMap<string, ShuffleWhy>();
	readonly #unit = new SvelteMap<string, string>();
	readonly #host: Library;
	readonly #player: Player;
	readonly #first: string;
	#to: number | null = null;
	#fetching: Promise<void> | null = null;
	#topping = false;
	#retryMs = 1000;

	constructor(host: Library, playing: Player, source: ShuffleSource, first: Unit) {
		this.#host = host;
		this.#player = playing;
		this.source = source;
		this.#first = first.songs[0]?.id ?? '';
		for (const { id } of first.songs) {
			this.#unit.set(id, this.#first);
		}
	}

	/** Whether the player is still playing this shuffle. */
	get active(): boolean {
		return this.#player.queue?.shuffle === this.ref;
	}

	/** Asks the server for the session, from the song already playing. Failures retry, quietly. */
	async begin(): Promise<void> {
		while (this.active && this.#to === null) {
			try {
				const page = await this.#host.startShuffle({
					scope: this.source.scope,
					mode: this.source.mode,
					first: this.#first
				});
				this.ref.id = page.id;
				this.#take(page, true);
			} catch {
				await this.#wait();
			}
		}
		await this.topUp();
	}

	/** Takes in a page: the first page's lead is the song already playing. */
	#take(page: ShufflePage, first: boolean): void {
		const lead = page.picks.at(0)?.songs.at(0)?.id;
		const picks = first && lead === this.#first ? page.picks.slice(1) : page.picks;
		if (first && lead === this.#first) {
			const why = page.picks.at(0)?.why;
			for (const id of this.#unit.keys()) {
				if (why !== undefined) {
					this.#why.set(id, why);
				}
			}
		}
		this.picks = [...this.picks, ...picks];
		this.songs = page.songs;
		this.done = page.done;
		this.#to = page.to;
	}

	async #wait(): Promise<void> {
		await new Promise((done) => setTimeout(done, this.#retryMs));
		this.#retryMs = Math.min(this.#retryMs * 2, mostRetryMs);
	}

	/** Fetches the next page, once at a time. */
	#fetch(): Promise<void> {
		this.#fetching ??= this.#fetchNext().finally(() => {
			this.#fetching = null;
		});
		return this.#fetching;
	}

	async #fetchNext(): Promise<void> {
		const after = this.#to;
		if (after === null || this.done || !this.active) {
			return;
		}
		try {
			const page = await this.#host.nextShuffle(this.ref.id, after);
			if (page.from !== after) {
				// Behind another device's pages: these say where the session stands.
				this.picks = [];
			}
			this.#take(page, false);
			this.#retryMs = 1000;
		} catch {
			await this.#wait();
		}
	}

	/**
	 * Keeps the player a few songs ahead: picks go into the queue as their albums arrive, and the
	 * next page is fetched well before the picks run out.
	 */
	async topUp(): Promise<void> {
		if (this.#topping) {
			return;
		}
		this.#topping = true;
		try {
			while (this.active && this.#ahead() < keepAhead) {
				const pick = this.picks.at(0);
				if (pick === undefined) {
					if (this.done || this.#to === null) {
						break;
					}
					await this.#fetch();
					if (this.picks.length === 0) {
						break;
					}
					continue;
				}
				const songs = await queuedLater(this.#host, pick.songs);
				if (!this.active) {
					break;
				}
				this.picks = this.picks.slice(1);
				const unit = pick.songs.at(0)?.id ?? '';
				for (const { track } of songs) {
					this.#why.set(track.id, pick.why);
					this.#unit.set(track.id, unit);
				}
				this.#player.extendShuffle(this.ref, songs);
			}
			if (this.#refills()) {
				const had = this.picks.length;
				void this.#fetch().then(() => {
					if (this.picks.length > had) {
						void this.topUp();
					}
				});
			}
		} finally {
			this.#topping = false;
		}
	}

	/** Whether the next page is due: the session has begun, goes on, and is running out of picks. */
	#refills(): boolean {
		return this.active && this.#to !== null && !this.done && this.picks.length < refillAt;
	}

	/** How many songs the player has queued past the one playing. */
	#ahead(): number {
		const queue = this.#player.queue;
		return queue === null ? 0 : queue.tracks.length - queue.index - 1;
	}

	/** What comes up next: the songs queued past the one playing, then the picks still to queue. */
	get upNext(): UpNextItem[] {
		const queue = this.#player.queue;
		if (queue === null || queue.shuffle !== this.ref) {
			return [];
		}
		const items: UpNextItem[] = [];
		for (const track of queue.tracks.slice(queue.index + 1)) {
			const unit = this.#unit.get(track.id) ?? track.id;
			const last = items.at(-1);
			if (last?.key === unit) {
				last.titles.push(track.title);
				last.trackIds.push(track.id);
				last.durationMs += track.durationMs;
				continue;
			}
			const album = albumOf(queue, track);
			items.push({
				key: unit,
				titles: [track.title],
				artist: track.artists.at(0) ?? album.artist,
				album,
				trackIds: [track.id],
				durationMs: track.durationMs,
				queued: true,
				why: this.#why.get(track.id)
			});
		}
		for (const pick of this.picks) {
			const [first] = pick.songs;
			if (items.length >= upNextShown || first === undefined) {
				break;
			}
			items.push({
				key: first.id,
				titles: pick.songs.map(({ title }) => title),
				artist: first.artists.at(0) ?? '',
				album: entryOf(first),
				trackIds: pick.songs.map(({ id }) => id),
				durationMs: pick.songs.reduce((sum, { durationMs }) => sum + durationMs, 0),
				queued: false,
				why: pick.why
			});
		}
		return items.slice(0, upNextShown);
	}

	/** Takes songs out of what comes up, queued or still to queue, and keeps the queue topped up. */
	remove(trackIds: readonly string[]): void {
		this.#player.removeAhead(trackIds);
		this.picks = this.picks.filter((pick) => !pick.songs.some(({ id }) => trackIds.includes(id)));
		void this.topUp();
	}

	/**
	 * Moves what comes up one place up or down. Within the queue it's the queue's order; within the
	 * picks still to queue, theirs. Across the two, the pick is queued in the other's place, and the
	 * other goes back to the picks, first.
	 */
	async move(key: string, by: -1 | 1): Promise<void> {
		const items = this.upNext;
		const at = items.findIndex((item) => item.key === key);
		const other = items.at(at + by);
		const item = items.at(at);
		if (at < 0 || at + by < 0 || item === undefined || other === undefined) {
			return;
		}
		const [upper, lower] = by < 0 ? [other, item] : [item, other];
		if (upper.queued && lower.queued) {
			const ahead = items.filter((each) => each.queued);
			const swapped = ahead.flatMap((each) =>
				each === upper ? lower.trackIds : each === lower ? upper.trackIds : each.trackIds
			);
			this.#player.reorderAhead(swapped);
			return;
		}
		if (!upper.queued && !lower.queued) {
			const from = this.picks.findIndex((pick) => pick.songs.at(0)?.id === upper.key);
			const picks = [...this.picks];
			const [a, b] = [picks[from], picks[from + 1]];
			if (from < 0 || a === undefined || b === undefined) {
				return;
			}
			picks.splice(from, 2, b, a);
			this.picks = picks;
			return;
		}
		// The last song queued and the first pick trade places.
		const pick = this.picks.at(0);
		const queue = this.#player.queue;
		if (pick === undefined || queue === null) {
			return;
		}
		const songs = await queuedLater(this.#host, pick.songs);
		if (!this.active) {
			return;
		}
		const back: ShufflePick = {
			why: this.#why.get(upper.trackIds[0] ?? '') ?? pick.why,
			songs: upper.trackIds.flatMap((id) => {
				const track = queue.tracks.find((each) => each.id === id);
				if (track === undefined) {
					return [];
				}
				const album = albumOf(queue, track);
				return [
					{
						id,
						title: track.title,
						artists: track.artists,
						albumId: album.id,
						album: album.title,
						durationMs: track.durationMs,
						artwork: album.artwork
					}
				];
			})
		};
		this.#player.removeAhead(upper.trackIds);
		for (const { track } of songs) {
			this.#why.set(track.id, pick.why);
			this.#unit.set(track.id, pick.songs.at(0)?.id ?? '');
		}
		this.#player.extendShuffle(this.ref, songs);
		this.picks = [back, ...this.picks.slice(1)];
	}
}

/** Starts shuffles, and keeps the one playing going. */
export class Shuffles {
	current = $state.raw<Shuffle | null>(null);
	readonly #host: Library;
	readonly #player: Player;
	#primed: { key: string; unit: Unit | undefined } | null = null;

	constructor(host: Library, playing: Player) {
		this.#host = host;
		this.#player = playing;
		// Each song that begins in a shuffle tops it up, so the queue stays ahead of the player.
		playing.began = (queue) => {
			const shuffle = this.current;
			if (shuffle !== null && queue.shuffle === shuffle.ref) {
				void shuffle.topUp();
			}
		};
	}

	/**
	 * Picks a source's first song ahead of a press and fetches its album, so the press plays at
	 * once. A pick made ahead is used once.
	 */
	async prime(source: ShuffleSource): Promise<void> {
		void shuffleSettings.load();
		if (this.#primed?.key === source.key) {
			return;
		}
		const unit = pickUnit(await source.units(), shuffleSettings.current.ratings);
		this.#primed = { key: source.key, unit };
		if (unit !== undefined) {
			await queuedLater(this.#host, unit.songs);
		}
	}

	/**
	 * Shuffles a source. The first song comes from what's here, and plays inside the press when its
	 * album is too; the server's session is asked for at the same time, and leads with that song.
	 */
	start(source: ShuffleSource): void {
		void shuffleSettings.load();
		const primed = this.#primed?.key === source.key ? this.#primed : null;
		this.#primed = null;
		if (primed !== null) {
			this.#play(source, primed.unit);
			return;
		}
		const units = source.units();
		if (Array.isArray(units)) {
			this.#play(source, pickUnit(units, shuffleSettings.current.ratings));
		} else {
			void units.then((each) => {
				this.#play(source, pickUnit(each, shuffleSettings.current.ratings));
			});
		}
	}

	#play(source: ShuffleSource, unit: Unit | undefined): void {
		if (unit === undefined) {
			return;
		}
		const shuffle = new Shuffle(this.#host, this.#player, source, unit);
		this.current = shuffle;
		const now = queuedNow(this.#host, unit);
		if (now !== null && now.length > 0) {
			this.#player.playSongs(now, shuffle.ref);
			void shuffle.begin();
			return;
		}
		void queuedLater(this.#host, unit.songs).then((songs) => {
			if (this.current === shuffle && songs.length > 0) {
				this.#player.playSongs(songs, shuffle.ref);
				void shuffle.begin();
			}
		});
	}

	/** Forgets the shuffle, and any pick made ahead: the next person's are their own. */
	reset(): void {
		this.current = null;
		this.#primed = null;
	}
}

/** The one shuffler. */
export const shuffles = new Shuffles(library, player);
