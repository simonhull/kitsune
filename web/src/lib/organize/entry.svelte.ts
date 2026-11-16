/**
 * The ways into Organize from a song or an album: an album's ⋯ and the selection bar. Each previews
 * just those songs; when nothing would move, a toast says so in place of a trip to the page.
 */

import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { library } from '#lib/library/library.svelte.ts';
import { organizer } from './organizer.svelte.ts';
import { inPlaceWords } from './words.ts';

/** How long the toast stays. */
export const toastMs = 6000;

class OrganizeEntry {
	toast = $state<{ id: number; words: string; template: boolean } | null>(null);
	#made = 0;
	#timer: ReturnType<typeof setTimeout> | undefined;

	/** Whether the ways in show: an administrator, with tag writes on. */
	get can(): boolean {
		return library.user?.canOrganize === true;
	}

	/** Previews an album's songs. */
	album(id: string): Promise<void> {
		return this.#preview({ albums: [id] });
	}

	/** Previews picked songs. */
	songs(trackIds: readonly string[]): Promise<void> {
		return this.#preview({ tracks: trackIds });
	}

	dismiss(): void {
		clearTimeout(this.#timer);
		this.toast = null;
	}

	async #preview(scope: { albums?: readonly string[]; tracks?: readonly string[] }): Promise<void> {
		try {
			const preview = await library.organize.preview(scope);
			if (preview.counts.moves === 0) {
				this.#say(inPlaceWords(preview.counts.songs), true);
				return;
			}
			organizer.showPreview(preview);
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- Built from resolve('/server/organize').
			await goto(`${resolve('/server/organize')}?preview=${encodeURIComponent(preview.id)}`);
		} catch {
			this.#say('Those songs couldn’t be previewed. Try again from Organize.', true);
		}
	}

	#say(words: string, template: boolean): void {
		clearTimeout(this.#timer);
		this.#made += 1;
		this.toast = { id: this.#made, words, template };
		this.#timer = setTimeout(() => {
			this.toast = null;
		}, toastMs);
	}
}

export const organizeEntry = new OrganizeEntry();
