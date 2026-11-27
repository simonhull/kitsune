/**
 * Choose artwork, for administrators: the artist hero's ⋯ menu, the picker it reshapes into, Not
 * this artist's confirm, and the toast each change leaves. One lives in the layout, as the album
 * menu does. Nothing here is optimistic but the ring: a pin is shown once the server has it, and the
 * stage develops it in place.
 */
import { SvelteMap } from 'svelte/reactivity';
import { library } from '#lib/library/library.svelte.ts';
import type { ExtraImage } from '#lib/library/source.ts';
import {
	ArtworkError,
	type ArtistImageCandidates,
	type ArtistImageKind,
	type ImageCandidate
} from './types.ts';
import { keepOrder } from './order.ts';
import { doneWords, failedWords, type ToastPart } from './words.ts';

/** How long a toast stays, its fox line draining: as long as Trash's and a playlist's. */
export const artworkToastMs = 8000;

/** Whose menu is open, and the ⋯ it buds from. */
export interface ChooserOpen {
	artistId: string;
	name: string;
	mbid: string | undefined;
	/** The ⋯'s box on the screen when it was pressed. */
	anchor: { left: number; right: number; top: number; bottom: number };
}

/** Which face the one surface shows: the menu, the picker, or Not this artist's confirm. */
export type ChooserFace = 'menu' | 'picker' | 'confirm';

/** One kind's candidates, as far as they've come. */
export type CandidatesLoad =
	| { kind: 'loading' }
	| { kind: 'ready'; value: ArtistImageCandidates }
	| { kind: 'failed'; words: string };

export interface ArtworkToast {
	id: number;
	words: ToastPart[];
	failed: boolean;
}

/** What is being done, so its control says so and nothing else is pressed meanwhile. */
export type ChooserBusy = { what: 'pin'; id: string } | { what: 'upload' | 'automatic' | 'reject' };

class Chooser {
	current = $state.raw<ChooserOpen | null>(null);
	face = $state<ChooserFace>('menu');
	kind = $state<ArtistImageKind>('background');
	busy = $state.raw<ChooserBusy | null>(null);
	toast = $state.raw<ArtworkToast | null>(null);
	/** Each kind's candidates for the artist open, read once per opening and again after a change. */
	readonly sets = new SvelteMap<ArtistImageKind, CandidatesLoad>();
	/** Where Not this artist's confirm goes back to on Keep it. */
	#confirmFrom: ChooserFace = 'menu';
	#opener: HTMLElement | null = null;
	#toasts = 0;
	#timer: ReturnType<typeof setTimeout> | undefined;
	/** Moves on with every opening, so a read for one artist never lands on another. */
	#generation = 0;

	/** Whether the person may choose: an administrator. */
	get can(): boolean {
		return library.admin;
	}

	open(at: ChooserOpen): void {
		this.#opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		this.#generation += 1;
		this.sets.clear();
		this.face = 'menu';
		this.kind = 'background';
		this.busy = null;
		this.current = at;
	}

	close(refocus: boolean): void {
		this.current = null;
		this.busy = null;
		if (refocus) {
			this.#opener?.focus();
		}
	}

	/** Choose artwork…: the menu reshapes into the picker, on the backdrop first. */
	choose(): void {
		this.face = 'picker';
		this.show(this.kind);
	}

	/** Shows a kind's candidates, asking for them the first time. */
	show(kind: ArtistImageKind): void {
		this.kind = kind;
		if (!this.sets.has(kind)) {
			void this.#read(kind);
		}
	}

	async #read(kind: ArtistImageKind): Promise<void> {
		const open = this.current;
		if (open === null) {
			return;
		}
		const generation = this.#generation;
		if (!this.sets.has(kind)) {
			this.sets.set(kind, { kind: 'loading' });
		}
		try {
			const value = await library.artwork.candidates(open.artistId, kind);
			if (generation === this.#generation) {
				const before = this.sets.get(kind);
				this.sets.set(kind, {
					kind: 'ready',
					value:
						before?.kind === 'ready'
							? { ...value, candidates: keepOrder(before.value.candidates, value.candidates) }
							: value
				});
			}
		} catch (error) {
			if (generation === this.#generation && !(this.sets.get(kind)?.kind === 'ready')) {
				this.sets.set(kind, {
					kind: 'failed',
					words: failedOf(error, 'The candidates couldn’t be read')
				});
			}
		}
	}

	/** Pins a candidate: the ring moves to it at the press, and the stage develops it once it's here. */
	async pin(candidate: ImageCandidate): Promise<void> {
		const open = this.current;
		if (open === null || this.busy !== null || (candidate.chosen && candidate.pinned)) {
			return;
		}
		const kind = this.kind;
		this.busy = { what: 'pin', id: candidate.id };
		try {
			const image = await library.artwork.pin(open.artistId, kind, candidate.id);
			this.#landed(open, kind, image, candidate.id);
			this.#say(doneWords('pinned', open.name, kind));
		} catch (error) {
			this.#fail(error, 'Couldn’t pin it');
		} finally {
			this.busy = null;
		}
	}

	/** Pins an image from this computer. */
	async upload(file: File): Promise<void> {
		const open = this.current;
		if (open === null || this.busy !== null) {
			return;
		}
		const kind = this.kind;
		this.busy = { what: 'upload' };
		try {
			const image = await library.artwork.upload(open.artistId, kind, file);
			this.#landed(open, kind, image, null);
			this.#say(doneWords('uploaded', open.name, kind));
		} catch (error) {
			this.#fail(error, 'Couldn’t upload it');
		} finally {
			this.busy = null;
		}
	}

	/** Automatic: the pin goes, and the page reads what the folders and sources give again. */
	async automatic(): Promise<void> {
		const open = this.current;
		if (open === null || this.busy !== null) {
			return;
		}
		const kind = this.kind;
		this.busy = { what: 'automatic' };
		try {
			await library.artwork.unpin(open.artistId, kind);
			void library.loadArtist(open.artistId);
			await this.#read(kind);
			this.#say(doneWords('unpinned', open.name, kind));
		} catch (error) {
			this.#fail(error, 'Couldn’t unpin it');
		} finally {
			this.busy = null;
		}
	}

	/** Look again now: no surface of its own, the menu closes and a toast says so. */
	async lookAgain(): Promise<void> {
		const open = this.current;
		if (open === null) {
			return;
		}
		this.close(true);
		try {
			await library.artwork.refresh(open.artistId);
			this.#say(doneWords('refresh', open.name, this.kind));
		} catch (error) {
			this.#fail(error, 'Couldn’t look again');
		}
	}

	/** Not this artist…: the surface reshapes into its confirm. */
	confirm(): void {
		this.#confirmFrom = this.face === 'confirm' ? this.#confirmFrom : this.face;
		this.face = 'confirm';
	}

	/** Keep it: back to where the confirm came from. */
	keep(): void {
		this.face = this.#confirmFrom;
		if (this.face === 'picker') {
			this.show(this.kind);
		}
	}

	/** Not this artist, confirmed: the sources' art goes, the page reads again, the surface closes. */
	async reject(): Promise<void> {
		const open = this.current;
		if (open === null || this.busy !== null) {
			return;
		}
		this.busy = { what: 'reject' };
		try {
			await library.artwork.reject(open.artistId);
			void library.loadArtist(open.artistId);
			this.close(true);
			this.#say(doneWords('rejected', open.name, this.kind));
		} catch (error) {
			this.busy = null;
			this.#fail(error, 'Couldn’t hold it back');
		}
	}

	/** A pin is on the server: the page shows it now, the ring holds it, the list reads again. */
	#landed(open: ChooserOpen, kind: ArtistImageKind, image: ExtraImage, id: string | null): void {
		library.showArtistImage(open.artistId, kind, image);
		const set = this.sets.get(kind);
		if (set?.kind === 'ready' && id !== null) {
			const by = library.user?.name;
			const at = new Date().toISOString();
			this.sets.set(kind, {
				kind: 'ready',
				value: {
					...set.value,
					pinned: true,
					candidates: set.value.candidates.map((c) =>
						c.id === id
							? {
									...c,
									chosen: true,
									pinned: true,
									pinnedAt: at,
									...(by === undefined ? {} : { pinnedBy: by })
								}
							: { ...c, chosen: false, pinned: false }
					)
				}
			});
		}
		void this.#read(kind);
	}

	#say(words: ToastPart[], failed = false): void {
		clearTimeout(this.#timer);
		this.#toasts += 1;
		this.toast = { id: this.#toasts, words, failed };
		this.#timer = setTimeout(() => {
			this.toast = null;
		}, artworkToastMs);
	}

	#fail(error: unknown, fallback: string): void {
		this.#say([{ text: failedOf(error, fallback) }], true);
	}

	/** Takes the toast away. */
	dismiss(): void {
		clearTimeout(this.#timer);
		this.toast = null;
	}
}

/** What a failure says. */
function failedOf(error: unknown, fallback: string): string {
	return error instanceof ArtworkError
		? failedWords(error.status, error.detail, fallback)
		: fallback;
}

export const chooser = new Chooser();
