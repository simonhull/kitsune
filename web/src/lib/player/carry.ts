import { tick } from 'svelte';
import { whenSettled } from '#lib/morph/settled.ts';

export interface CarryParts {
	hidden(): boolean;
	onNowPlaying(): boolean;
	/** The engine's blend: `change` must set its state at once, and never with `flushSync`. */
	blend(change: () => Promise<void>): Promise<void>;
}

/** How a track change reaches the page. */
export interface Carrier {
	/**
	 * Shows a change: the sound has already moved, and only the names wait. In a hidden tab they
	 * don't, because nothing there ever settles. On Now playing the whole picture blends; anywhere
	 * else `apply` is the island's own swap. If a later change comes first, this one never happens.
	 */
	carry(apply: () => void): void;
	/** Applies what's waiting now, for a tab going hidden with a flight still in the air. */
	flush(): void;
}

export function createCarrier({ hidden, onNowPlaying, blend }: CarryParts): Carrier {
	let waiting: (() => void) | null = null;
	let cancel: () => void = () => undefined;

	function flush(): void {
		const apply = waiting;
		cancel();
		waiting = null;
		apply?.();
	}

	function carry(apply: () => void): void {
		cancel();
		waiting = apply;
		if (hidden()) {
			flush();
			return;
		}
		cancel = whenSettled(() => {
			waiting = null;
			if (onNowPlaying()) {
				void blend(async () => {
					apply();
					await tick();
				});
			} else {
				apply();
			}
		});
	}

	return { carry, flush };
}
