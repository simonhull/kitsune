import { library, type Library } from '#lib/library/library.svelte.ts';
import type { PlaybackRecord } from '#lib/library/source.ts';
import { shuffles, type Shuffles } from '#lib/shuffle/session.svelte.ts';
import { sourceOf } from '#lib/shuffle/sources.ts';
import { deviceForm, deviceId } from './device.ts';
import { player, type Heard, type Player } from './player.svelte.ts';
import type { AlbumLookup } from './queue.ts';
import { leftOf, recordOf, restored, type Client, type Left } from './remember.ts';

/** After a song first sounds, the save waits this long: past the sound, beside the listen's report. */
export const soundedSaveMs = 2000;
/** A pause saves this soon after. */
export const pausedSaveMs = 250;
/** Seeks settle this long before one save. */
export const seekedSaveMs = 1000;
/** While sound plays, where it is is saved this often. */
export const soundingSaveMs = 15_000;

/** How the island arrived: by a first play, or restored. */
export type Arrival = 'play' | 'restore';

export interface ResumeParts {
	player: Player;
	library: Library;
	shuffles: Shuffles;
	/** This browser, as a record names it. */
	client: () => Client;
}

const dueAfter: Record<Heard, number> = {
	sounded: soundedSaveMs,
	paused: pausedSaveMs,
	seeked: seekedSaveMs
};

/**
 * Where the person left off: restored, paused, when the shell loads, and saved behind the sound as
 * this browser plays. Nothing here ever waits on the play path, and a tab that has only restored
 * never writes, so it can't overwrite a newer record from another device.
 */
export class Resume {
	/** Set while a restore's island is on its way in; the island clears it once it has arrived. */
	arrival = $state<Arrival | null>(null);
	/** Where the record was left, when another browser left it or its song is gone. */
	left = $state.raw<Left | null>(null);
	/** This browser has changed something the server doesn't hold yet. */
	dirty = $state(false);

	readonly #parts: ResumeParts;
	#timer: ReturnType<typeof setTimeout> | undefined;
	#ticker: ReturnType<typeof setInterval> | undefined;
	#inflight: Promise<void> | null = null;
	/** Moves on at every reset: anything older that answers late is ignored. */
	#epoch = 0;
	/** The epoch a restore was asked for, so a shell that loads twice asks once. */
	#asked = -1;
	/** When the record restored here was saved: catching up takes only a newer one. */
	#restoredAt: number | null = null;
	/** Something has played here since the restore, or there was none. */
	#touched = false;

	constructor(parts: ResumeParts) {
		this.#parts = parts;
		parts.player.heard = (moment) => {
			this.#heard(moment);
		};
	}

	/** A queue has begun here, and the server doesn't hold where it is: a reload would lose it. */
	get unsaved(): boolean {
		return this.dirty || this.#inflight !== null;
	}

	/**
	 * Restores the signed-in person's record, paused, once per sign-in. It never blocks the page:
	 * the island arrives when the record and its current album have. Anything played first wins.
	 */
	async restore(): Promise<void> {
		if (this.#asked === this.#epoch) {
			return;
		}
		this.#asked = this.#epoch;
		const epoch = this.#epoch;
		const record = await this.#read();
		if (record !== null && this.#parts.player.queue === null && epoch === this.#epoch) {
			await this.#apply(record, epoch, 'restore');
		}
	}

	/**
	 * A tab looked at again: if it has sat paused and untouched since it restored, it takes on a
	 * newer record, saved since on another device. Nothing that sounds is ever touched.
	 */
	async catchUp(): Promise<void> {
		const { player } = this.#parts;
		if (this.#asked !== this.#epoch || !this.#untouched(this.#epoch)) {
			return;
		}
		const epoch = this.#epoch;
		const seen = this.#restoredAt;
		if (seen === null && player.queue !== null) {
			return;
		}
		const record = await this.#read();
		if (record === null || (seen !== null && record.updatedAt <= seen)) {
			return;
		}
		if (seen === null ? player.queue === null : this.#untouched(epoch)) {
			// Nothing was here to catch up on: it arrives as a restore does.
			await this.#apply(record, epoch, seen === null ? 'restore' : null);
		}
	}

	/**
	 * Saves now, if this browser has changed anything. `keepalive` lets the request outlive the page:
	 * a page going away while it sounds saves where the sound had got to.
	 */
	async flush({ keepalive = false }: { keepalive?: boolean } = {}): Promise<void> {
		clearTimeout(this.#timer);
		this.#timer = undefined;
		const { player } = this.#parts;
		if (keepalive && player.sounding) {
			this.dirty = true;
		}
		if (this.#inflight !== null && !keepalive) {
			// One at a time: this one goes after it, with the latest state.
			await this.#inflight;
			return this.flush();
		}
		const queue = player.queue;
		if (!this.dirty || queue === null || player.failure) {
			return;
		}
		const current = this.#parts.shuffles.current;
		const at = current !== null && current.ref === queue.shuffle ? current.resumePoint() : null;
		const save = recordOf(queue, player.position, at, this.#parts.client());
		this.dirty = false;
		const sent = this.#parts.library
			.savePlayback(save, { keepalive })
			.catch(() => {
				// A failed save goes again at the next moment.
				this.dirty = true;
			})
			.finally(() => {
				if (this.#inflight === sent) {
					this.#inflight = null;
				}
			});
		this.#inflight = sent;
		await sent;
	}

	/** Forgets everything, for the next person: their record is their own. */
	reset(): void {
		clearTimeout(this.#timer);
		clearInterval(this.#ticker);
		this.#timer = undefined;
		this.#ticker = undefined;
		this.#epoch += 1;
		this.#inflight = null;
		this.#restoredAt = null;
		this.#touched = false;
		this.dirty = false;
		this.arrival = null;
		this.left = null;
	}

	async #read(): Promise<PlaybackRecord | null> {
		try {
			return await this.#parts.library.playback();
		} catch {
			return null;
		}
	}

	#untouched(epoch: number): boolean {
		const { player } = this.#parts;
		return epoch === this.#epoch && !this.#touched && !this.dirty && !player.sounding;
	}

	/** Puts a record in the player, paused, once its current song's album is here. */
	async #apply(record: PlaybackRecord, epoch: number, arrival: Arrival | null): Promise<void> {
		const { player, library: host } = this.#parts;
		const current = record.tracks.at(record.index);
		if (current === undefined) {
			return;
		}
		await host.loadAlbum(current.albumId);
		const stillFirst = arrival === 'restore' ? player.queue === null : true;
		if (epoch !== this.#epoch || !stillFirst || !this.#untouched(epoch)) {
			return;
		}
		const lookup: AlbumLookup = (id) => {
			const album = host.album(id);
			if (album !== undefined) {
				return album;
			}
			return host.failed(id) || host.unavailable(id) ? 'gone' : undefined;
		};
		const queue = restored(record, lookup);
		if (queue === null) {
			return;
		}
		this.#restoredAt = record.updatedAt;
		this.#touched = false;
		this.dirty = false;
		this.left = leftOf(record, this.#parts.client());
		if (arrival !== null) {
			this.arrival = arrival;
		}
		player.restore(queue, record.positionMs / 1000);
		if (queue.shuffle !== undefined && record.shuffle !== undefined) {
			const { scope, mode, after } = record.shuffle;
			this.#parts.shuffles.resume(queue.shuffle, sourceOf(scope, mode), after);
		}
	}

	/** What the player tells: each sets a save going, later. It returns at once. */
	#heard(moment: Heard): void {
		if (this.#parts.player.failure) {
			return;
		}
		this.dirty = true;
		this.#touched = true;
		if (moment === 'sounded') {
			this.left = null;
			this.#tick();
		}
		clearTimeout(this.#timer);
		this.#timer = setTimeout(() => {
			this.#timer = undefined;
			void this.flush();
		}, dueAfter[moment]);
	}

	/** Saves every so often while the sound plays, and stops when it doesn't. */
	#tick(): void {
		if (this.#ticker !== undefined) {
			return;
		}
		this.#ticker = setInterval(() => {
			if (!this.#parts.player.sounding) {
				clearInterval(this.#ticker);
				this.#ticker = undefined;
				return;
			}
			this.dirty = true;
			void this.flush();
		}, soundingSaveMs);
	}
}

/** The one record keeper. */
export const resume = new Resume({
	player,
	library,
	shuffles,
	client: () => ({ id: deviceId(), device: deviceForm() })
});
