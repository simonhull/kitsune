import type { Track } from '#lib/library/source.ts';

/** A track shorter than this never counts, however often it plays. */
const shortestMs = 30_000;
/** Hearing this much counts, whatever the track's length. */
const enoughSeconds = 240;

/**
 * One listen to one track, and whether it has been heard enough to count: half the track or 4
 * minutes, whichever is first. Heard is how far the position moved by itself, which is the only
 * thing that measures it. The clock credits a frozen tab, and capping each step credits small seeks
 * and undercounts a throttled one, so whatever moves the position must say so through
 * {@link Listen.jumped}. A position that goes back unannounced is an output starting over on its way
 * to where it was, and nothing heard.
 */
export class Listen {
	readonly trackId: string;
	/** When it started, in Unix ms. */
	readonly startedAt: number;
	readonly #needed: number;
	#at: number;
	#heard = 0;
	#counted = false;

	constructor({ id, durationMs }: Pick<Track, 'id' | 'durationMs'>, startedAt: number, at = 0) {
		this.trackId = id;
		this.startedAt = startedAt;
		this.#needed = durationMs < shortestMs ? Infinity : Math.min(durationMs / 2000, enoughSeconds);
		this.#at = at;
	}

	/** Takes in where the position got to by itself. True once, on the tick that makes it count. */
	heard(seconds: number): boolean {
		if (seconds > this.#at) {
			this.#heard += seconds - this.#at;
			this.#at = seconds;
		}
		if (this.#counted || this.#heard < this.#needed) {
			return false;
		}
		this.#counted = true;
		return true;
	}

	/** The position was put here: nothing between there and here was heard. */
	jumped(seconds: number): void {
		this.#at = seconds;
	}
}
