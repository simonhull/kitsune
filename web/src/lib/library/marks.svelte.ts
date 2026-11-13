import { SvelteMap } from 'svelte/reactivity';
import {
	UnauthorizedError,
	type Album,
	type LibraryIndex,
	type LibrarySource,
	type MarksEvent
} from './source.ts';

/** What the marks need of the server. */
export type MarkServer = Pick<LibrarySource, 'setStars'>;

/** A rating the server didn't take, which has been put back. */
export interface MarkFailure {
	id: string;
}

/**
 * One kind of mark, by ID. What's shown changes at once. Each item has at most one request out, and
 * a press made meanwhile waits as its newest wish: older wishes are dropped, because only the last
 * press says what's wanted. What the server has confirmed is what a failure goes back to.
 */
class Lane<T> {
	/** Only what differs from `none` is held, so a library of unmarked songs costs nothing. */
	readonly #shown = new SvelteMap<string, T>();
	readonly #confirmed = new Map<string, T>();
	readonly #wishes = new Map<string, T>();
	readonly #flying = new Set<string>();
	readonly #none: T;
	readonly #send: (id: string, value: T) => Promise<void>;
	readonly #failed: (id: string, error: unknown) => void;
	/** Moves on at every reset: an answer for an older session has no business here. */
	#epoch = 0;

	constructor(
		none: T,
		send: (id: string, value: T) => Promise<void>,
		failed: (id: string, error: unknown) => void
	) {
		this.#none = none;
		this.#send = send;
		this.#failed = failed;
	}

	get(id: string): T {
		return this.#shown.has(id) ? (this.#shown.get(id) as T) : this.#none;
	}

	/** Every value shown that isn't `none`. */
	shown(): IterableIterator<T> {
		return this.#shown.values();
	}

	/** Takes what the server said. An item with a change out keeps it: the answer will speak for it. */
	seed(id: string, value: T): void {
		if (this.#flying.has(id)) {
			return;
		}
		this.#hold(this.#confirmed, id, value);
		if (this.get(id) !== value) {
			this.#hold(this.#shown, id, value);
		}
	}

	/** Shows `value` now, and sends it when nothing for this item is out. */
	want(id: string, value: T): void {
		this.#hold(this.#shown, id, value);
		if (this.#flying.has(id)) {
			this.#wishes.set(id, value);
		} else if (value !== this.#confirmedOf(id)) {
			void this.#fly(id, value);
		}
	}

	reset(): void {
		this.#epoch += 1;
		this.#shown.clear();
		this.#confirmed.clear();
		this.#wishes.clear();
		this.#flying.clear();
	}

	async #fly(id: string, first: T): Promise<void> {
		const epoch = this.#epoch;
		this.#flying.add(id);
		let next: T | undefined = first;
		while (next !== undefined) {
			const value: T = next;
			try {
				await this.#send(id, value);
				if (epoch !== this.#epoch) {
					return;
				}
				this.#hold(this.#confirmed, id, value);
			} catch (error) {
				if (epoch !== this.#epoch) {
					return;
				}
				// A newer press is waiting: it says what's wanted, and goes next.
				if (!this.#wishes.has(id)) {
					this.#hold(this.#shown, id, this.#confirmedOf(id));
					this.#failed(id, error);
				}
			}
			const wish = this.#wishes.get(id);
			this.#wishes.delete(id);
			next = wish !== undefined && wish !== this.#confirmedOf(id) ? wish : undefined;
		}
		this.#flying.delete(id);
	}

	#confirmedOf(id: string): T {
		return this.#confirmed.has(id) ? (this.#confirmed.get(id) as T) : this.#none;
	}

	#hold(map: Map<string, T>, id: string, value: T): void {
		if (value === this.#none) {
			map.delete(id);
		} else {
			map.set(id, value);
		}
	}
}

/**
 * The signed-in person's stars, by track ULID: the one place every page reads them from, so a song
 * rated in one place shows everywhere at once. The index and album pages seed it.
 */
export class Marks {
	/** The last rating the server didn't take. Cleared by the next press. */
	failure = $state.raw<MarkFailure | null>(null);

	readonly #stars: Lane<number | null>;

	constructor(server: MarkServer) {
		this.#stars = new Lane<number | null>(
			null,
			(id, stars) => server.setStars(id, stars),
			(id, error) => {
				// A lost session has gone to the door: there's nobody to tell.
				if (!(error instanceof UnauthorizedError)) {
					this.failure = { id };
				}
			}
		);
	}

	/** A track's stars, ½ to 5 in halves, or null when it's unrated. */
	stars(trackId: string): number | null {
		return this.#stars.get(trackId);
	}

	/** How many songs are rated `stars` or more: those over the Last.fm love line, say. */
	ratedAtLeast(stars: number): number {
		let n = 0;
		for (const rated of this.#stars.shown()) {
			if (rated !== null && rated >= stars) {
				n += 1;
			}
		}
		return n;
	}

	rate(trackId: string, stars: number | null): void {
		this.failure = null;
		this.#stars.want(trackId, stars);
	}

	/**
	 * Takes in a rating the person gave on another device, or their hearts became, as the server
	 * reports it. A press of this one's still out keeps what it shows: its answer speaks for it.
	 */
	heard({ id, stars }: MarksEvent): void {
		this.#stars.seed(id, stars === 0 ? null : stars);
	}

	seedIndex({ artists }: LibraryIndex): void {
		for (const artist of artists) {
			for (const album of artist.albums) {
				for (const track of album.tracks) {
					this.#stars.seed(track.id, track.stars ?? null);
				}
			}
		}
	}

	seedAlbum(album: Album): void {
		for (const track of album.tracks) {
			this.#stars.seed(track.id, track.stars ?? null);
		}
	}

	reset(): void {
		this.failure = null;
		this.#stars.reset();
	}
}
