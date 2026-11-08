import { SvelteSet } from 'svelte/reactivity';

/** How the header's box stands for what a list shows. */
export type BoxState = 'all' | 'some' | 'none';

/**
 * The songs a person has picked, by track ID: not by row, so it lasts through scrolling, the
 * virtual list's recycling and a change of filter. A song a filter hides stays picked.
 */
export class Selection {
	readonly #ids = new SvelteSet<string>();
	/** Where a Shift range starts: the last song picked on its own. */
	#anchor: string | null = null;

	get ids(): ReadonlySet<string> {
		return this.#ids;
	}

	get size(): number {
		return this.#ids.size;
	}

	get any(): boolean {
		return this.#ids.size > 0;
	}

	get anchor(): string | null {
		return this.#anchor;
	}

	has(id: string): boolean {
		return this.#ids.has(id);
	}

	/** A box clicked, Space, or Ctrl or Cmd with a click: that one song in or out. */
	toggle(id: string): void {
		if (!this.#ids.delete(id)) {
			this.#ids.add(id);
		}
		this.#anchor = id;
	}

	/**
	 * Shift with a click: every song from the anchor to `id` in `order`, which runs across albums,
	 * joins what's picked. With no anchor, or one the list no longer shows, it's `id` alone.
	 */
	extend(id: string, order: readonly string[]): void {
		const to = order.indexOf(id);
		const from = this.#anchor === null ? -1 : order.indexOf(this.#anchor);
		if (to < 0) {
			return;
		}
		if (from < 0) {
			this.toggle(id);
			return;
		}
		const [low, high] = from < to ? [from, to] : [to, from];
		for (const each of order.slice(low, high + 1)) {
			this.#ids.add(each);
		}
	}

	/** Shift with an arrow: the song moved to joins, and the anchor stays. */
	add(id: string): void {
		this.#ids.add(id);
		this.#anchor ??= id;
	}

	/** The header's box: everything the list shows, or nothing of it when that's all picked already. */
	toggleAll(shown: readonly string[]): void {
		const all = this.boxFor(shown) === 'all';
		for (const id of shown) {
			if (all) {
				this.#ids.delete(id);
			} else {
				this.#ids.add(id);
			}
		}
	}

	/** Picks exactly this one, as a long press starting a selection does. */
	only(id: string): void {
		this.#ids.clear();
		this.#ids.add(id);
		this.#anchor = id;
	}

	clear(): void {
		this.#ids.clear();
		this.#anchor = null;
	}

	boxFor(shown: readonly string[]): BoxState {
		const picked = shown.reduce((count, id) => count + (this.#ids.has(id) ? 1 : 0), 0);
		if (picked === 0) {
			return 'none';
		}
		return picked === shown.length ? 'all' : 'some';
	}

	/** How many picked songs the list doesn't show, for "4 selected, 1 hidden". */
	hiddenFrom(shown: ReadonlySet<string>): number {
		let hidden = 0;
		for (const id of this.#ids) {
			if (!shown.has(id)) {
				hidden += 1;
			}
		}
		return hidden;
	}

	/** The picked songs in the list's order, then any it doesn't show. */
	inOrder(order: readonly string[]): string[] {
		const shown = order.filter((id) => this.#ids.has(id));
		if (shown.length === this.#ids.size) {
			return shown;
		}
		const rest = [...this.#ids].filter((id) => !shown.includes(id));
		return [...shown, ...rest];
	}
}

/** The one selection on the Library list. It outlives the page, as the list's place does. */
export const selection = new Selection();
