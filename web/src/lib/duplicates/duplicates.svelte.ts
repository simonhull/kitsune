/**
 * What Server › Duplicates holds across its two pages: the list under its filter, the groups merged
 * from this browser while the server works (hidden at the press, back if the merge fails), and the
 * toast a merge leaves for the list. Nothing here is on the play path.
 */

import { SvelteSet } from 'svelte/reactivity';
import { library } from '#lib/library/library.svelte.ts';
import {
	DuplicatesError,
	type DuplicateFilter,
	type DuplicateList,
	type MergeRequest,
	type Merged
} from './types.ts';

/** How long the toast after a merge stays. */
const toastMs = 8000;

class Duplicates {
	filter = $state<DuplicateFilter>('all');
	list = $state<DuplicateList | null>(null);
	/** Why the list couldn't be read, in the server's words; empty while it can. */
	problem = $state('');
	loading = $state(false);
	/** Groups and pairs merged from here and not yet gone from the server's list. */
	readonly hidden = new SvelteSet<string>();
	/** What the last merge did, for the list's toast. */
	toast = $state<string | null>(null);
	#toastTimer: ReturnType<typeof setTimeout> | undefined;
	/** Which read is the latest: an older answer is let go. */
	#reading = 0;

	/** Reads the list's first page under `filter`. */
	async read(filter: DuplicateFilter = this.filter): Promise<void> {
		this.filter = filter;
		const reading = ++this.#reading;
		this.loading = true;
		try {
			const list = await library.duplicates.list(filter);
			if (reading === this.#reading) {
				this.list = list;
				this.problem = '';
			}
		} catch (error) {
			if (reading === this.#reading) {
				this.problem =
					error instanceof DuplicatesError ? error.detail : 'The list couldn’t be read.';
			}
		} finally {
			if (reading === this.#reading) {
				this.loading = false;
			}
		}
	}

	/** Reads the next page of groups onto the list. */
	async more(): Promise<void> {
		const list = this.list;
		if (list?.nextCursor === undefined || this.loading) {
			return;
		}
		const reading = ++this.#reading;
		this.loading = true;
		try {
			const next = await library.duplicates.list(list.filter, list.nextCursor);
			if (reading === this.#reading) {
				this.list = { ...next, pairs: list.pairs, groups: [...list.groups, ...next.groups] };
			}
		} catch {
			// The button stays, to try again.
		} finally {
			if (reading === this.#reading) {
				this.loading = false;
			}
		}
	}

	/**
	 * Merges `key` as `request` says: the list hides it at once, and shows it again if the server
	 * refuses, with the refusal thrown on. `words` is the toast the list shows after.
	 */
	async merge(
		key: string,
		request: MergeRequest,
		words: (merged: Merged) => string
	): Promise<Merged> {
		this.hidden.add(key);
		try {
			const merged = await library.duplicates.merge(key, request);
			this.say(words(merged));
			void this.read();
			return merged;
		} catch (error) {
			this.hidden.delete(key);
			throw error;
		}
	}

	/** Says a group or pair is not duplicates, and reads the list again. */
	async dismiss(key: string): Promise<void> {
		await library.duplicates.dismiss(key);
		await this.read();
	}

	/** Brings a dismissed group or pair back, and reads the list again. */
	async bringBack(key: string): Promise<void> {
		await library.duplicates.bringBack(key);
		await this.read();
	}

	/** Shows the toast for a while. */
	say(words: string): void {
		this.toast = words;
		clearTimeout(this.#toastTimer);
		this.#toastTimer = setTimeout(() => {
			this.toast = null;
		}, toastMs);
	}
}

export const duplicates = new Duplicates();
