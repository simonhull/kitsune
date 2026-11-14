import { SvelteSet } from 'svelte/reactivity';
import type { TransitionConfig } from 'svelte/transition';
import { library, type Load } from '#lib/library/library.svelte.ts';
import { TrashError, type Trash, type TrashChange, type TrashEmpty } from '#lib/library/source.ts';
import { player } from '#lib/player/player.svelte.ts';
import { dropEverywhere } from './selection.svelte.ts';
import { toastWords, type ToastWords, type TrashedWhat } from './trashWords.ts';

/** How long a toast stays, its fox line draining all the while. */
export const toastMs = 8000;
/** How long the rows take to fold away: `--morph-room` and a little, for its long tail. */
const foldMs = 400;

/** The toast above the player: what moved, and the Undo for exactly that. */
export interface TrashToast {
	/** Moves on with every toast, so one replaced starts its line again. */
	id: number;
	words: ToastWords;
	/** `failed`: the server refused, and the songs are back; `words` says why. */
	state: 'moved' | 'failed';
}

/** What one move took, for its Undo: the songs, and the album when it went whole. */
interface Moved {
	ids: readonly string[];
	album: string | undefined;
	/** The server's answer, once it's come. */
	change: Promise<TrashChange | null>;
}

/** Why a move or an Undo didn't happen, as the toast says it. */
export function problemWords(
	error: unknown,
	doing: 'move' | 'undo' | 'restore' | 'empty'
): ToastWords {
	const what = {
		move: "Couldn't move to Trash",
		undo: "Couldn't put them back",
		restore: "Couldn't restore",
		empty: "Couldn't empty Trash"
	}[doing];
	if (error instanceof TrashError && error.detail !== '') {
		return { name: null, rest: `${what}: ${error.detail}` };
	}
	if (error instanceof TrashError && error.status === 0) {
		return { name: null, rest: `${what}: the server isn't answering` };
	}
	return { name: null, rest: `${what}. Try again in a moment` };
}

/**
 * Moving songs to Trash, for every list: the rows fold away at once and the toast comes up, and only
 * then is the server asked. Nothing on the play path waits on it. Administrators only, on a server
 * with tag writes on ({@link TrashBin.can}).
 */
class TrashBin {
	toast = $state.raw<TrashToast | null>(null);
	/** Songs folding out of their lists right now: their rows leave with the fold, the rest at once. */
	readonly folding = new SvelteSet<string>();
	#moved: Moved | null = null;
	#toasts = 0;
	#timer: ReturnType<typeof setTimeout> | undefined;

	/** What's in Trash, as the Server page counts it and the Trash page lists it. */
	contents = $state.raw<Load<Trash>>({ kind: 'loading' });
	/** The latest read: an older answer that comes late is let go. */
	#reads = 0;

	/** Whether the signed-in person may move songs to Trash. */
	get can(): boolean {
		return library.user?.canTrash === true;
	}

	/** Reads what's in Trash. A failure leaves what was there, or says it can't be read. */
	async read(): Promise<void> {
		this.#reads += 1;
		const run = this.#reads;
		try {
			const trash = await library.trashList();
			if (run === this.#reads) {
				this.contents = { kind: 'ready', value: trash };
			}
		} catch {
			if (run === this.#reads && this.contents.kind !== 'ready') {
				this.contents = { kind: 'unavailable' };
			}
		}
	}

	/**
	 * Puts songs in Trash back where they were, from the Trash page: they leave its list at once and
	 * come back to every other. It resolves with what to say when the server refused, else null.
	 */
	async restore(trackIds: readonly string[]): Promise<string | null> {
		const before = this.contents;
		this.#leaveContents(trackIds);
		try {
			const change = await library.restoreTrash(trackIds);
			library.trashed.show(change.tracks, change.albums);
			void this.read();
			return null;
		} catch (error) {
			this.contents = before;
			return problemWords(error, 'restore').rest;
		}
	}

	/** Deletes songs in Trash for good: these, or every one. Resolves as {@link TrashBin.restore}. */
	async empty(target: TrashEmpty): Promise<string | null> {
		const before = this.contents;
		this.#leaveContents('all' in target ? null : target.tracks);
		try {
			await library.emptyTrash(target);
			void this.read();
			return null;
		} catch (error) {
			this.contents = before;
			return problemWords(error, 'empty').rest;
		}
	}

	/** Reads Trash again, if it's been read: a page shows its count. */
	#recount(): void {
		if (this.contents.kind !== 'loading') {
			void this.read();
		}
	}

	/** Takes songs off the Trash list, or every one for null, keeping its counts true. */
	#leaveContents(trackIds: readonly string[] | null): void {
		if (this.contents.kind !== 'ready') {
			return;
		}
		const { retentionDays, albums } = this.contents.value;
		const left = albums
			.map((album) => ({
				...album,
				tracks: trackIds === null ? [] : album.tracks.filter(({ id }) => !trackIds.includes(id))
			}))
			.filter(({ tracks }) => tracks.length > 0);
		const tracks = left.flatMap((album) => album.tracks);
		this.contents = {
			kind: 'ready',
			value: {
				count: tracks.length,
				bytes: tracks.reduce((sum, { size }) => sum + size, 0),
				retentionDays,
				albums: left
			}
		};
	}

	/** Moves songs to Trash. `albumOf` says where each one is, for the lists to leave it out. */
	moveSongs(
		trackIds: readonly string[],
		albumOf: (trackId: string) => string | undefined,
		title?: string
	): void {
		if (!this.can || trackIds.length === 0) {
			return;
		}
		const ids = [...new Set(trackIds)];
		const songs = ids.map((trackId) => ({ trackId, albumId: albumOf(trackId) ?? '' }));
		this.#fold(ids);
		library.trashed.hide(songs);
		this.#leaveQueue(ids);
		const what: TrashedWhat =
			ids.length === 1 && title !== undefined
				? { kind: 'song', title }
				: { kind: 'songs', songs: ids.length };
		this.#send(ids, undefined, what, library.trash({ tracks: ids }));
	}

	/** Moves an album's songs to Trash, all of them, and the album leaves the grids. */
	moveAlbum(id: string, title: string, songs: number): void {
		if (!this.can) {
			return;
		}
		const ids = (library.album(id)?.tracks ?? []).map((track) => track.id);
		this.#fold(ids);
		library.trashed.hide(
			ids.map((trackId) => ({ trackId, albumId: id })),
			[id]
		);
		this.#leaveQueue(ids);
		this.#send(ids, id, { kind: 'album', title, songs }, library.trash({ albums: [id] }));
	}

	/** Puts back exactly what the toast moved, where it was. */
	undo(): void {
		const moved = this.#moved;
		if (moved === null) {
			return;
		}
		this.#moved = null;
		this.dismiss();
		library.trashed.show(moved.ids, moved.album === undefined ? [] : [moved.album]);
		void moved.change.then(async (change) => {
			if (change === null) {
				return; // It never went.
			}
			library.trashed.show(change.tracks);
			try {
				await library.restoreTrash(change.tracks);
				this.#recount();
			} catch (error) {
				// They're still in Trash: the lists leave them out again, and the toast says why.
				library.trashed.hide(
					change.tracks.map((trackId) => ({ trackId, albumId: change.albums.at(0) ?? '' })),
					moved.album === undefined ? [] : [moved.album]
				);
				library.trashed.confirm(change, moved.album !== undefined);
				this.#show(problemWords(error, 'undo'), 'failed');
			}
		});
	}

	/** Takes the toast away. What it moved stays in Trash, to be restored from there. */
	dismiss(): void {
		clearTimeout(this.#timer);
		this.toast = null;
	}

	/** Shows a toast for a move, and asks the server; a refusal gives the songs back. */
	#send(
		ids: readonly string[],
		album: string | undefined,
		what: TrashedWhat,
		sent: Promise<TrashChange>
	): void {
		const moved: Moved = { ids, album, change: Promise.resolve(null) };
		moved.change = sent.then(
			(answer) => {
				library.trashed.confirm(answer, album !== undefined);
				this.#recount();
				return answer;
			},
			(error: unknown) => {
				library.trashed.show(ids, album === undefined ? [] : [album]);
				if (this.#moved === moved) {
					this.#moved = null;
					this.#show(problemWords(error, 'move'), 'failed');
				}
				return null;
			}
		);
		this.#moved = moved;
		this.#show(toastWords(what), 'moved');
	}

	#show(words: ToastWords, state: TrashToast['state']): void {
		clearTimeout(this.#timer);
		this.#toasts += 1;
		this.toast = { id: this.#toasts, words, state };
		this.#timer = setTimeout(() => {
			this.toast = null;
			this.#moved = null;
		}, toastMs);
	}

	/** Lets the rows of these songs fold out of whatever list they're in. */
	#fold(ids: readonly string[]): void {
		for (const id of ids) {
			this.folding.add(id);
		}
		// A list that places its own rows lets them glide up into the room, for as long as the fold.
		document.documentElement.dataset['folding'] = '';
		setTimeout(() => {
			for (const id of ids) {
				this.folding.delete(id);
			}
			if (this.folding.size === 0) {
				delete document.documentElement.dataset['folding'];
			}
		}, foldMs);
	}

	/** Takes the songs out of what's picked and what's still to play. The one playing plays out. */
	#leaveQueue(ids: readonly string[]): void {
		dropEverywhere(ids);
		const playing = library.playing?.track.id;
		player.removeAhead(ids.filter((id) => id !== playing));
	}
}

export const trashBin = new TrashBin();

/**
 * A song row's way out: one moved to Trash folds shut (its height to nothing, fading, on
 * `--morph-room`), and the rows after it close up with it; any other goes at once, as a scrolled
 * list's rows must.
 */
export function foldOut(node: HTMLElement): TransitionConfig {
	const id = node.dataset['track'];
	if (id === undefined || !trashBin.folding.has(id)) {
		return { duration: 0 };
	}
	const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	const style = getComputedStyle(node);
	const height = node.offsetHeight;
	return {
		duration: still ? 0 : parseFloat(style.getPropertyValue('--morph-room')) || 160,
		easing: roomCurve,
		css: (t) => `height: ${String(t * height)}px; opacity: ${String(t * t)}; overflow: hidden;`
	};
}

/** `--curve-room`, cubic-bezier(0.2, 0.9, 0.3, 1): fast out, a long tail, no overshoot. */
function roomCurve(t: number): number {
	// Solved for x by Newton's method, as a browser does for a cubic Bézier timing function.
	const [x1, y1, x2, y2] = [0.2, 0.9, 0.3, 1];
	const at = (p1: number, p2: number, s: number): number =>
		3 * (1 - s) * (1 - s) * s * p1 + 3 * (1 - s) * s * s * p2 + s * s * s;
	let s = t;
	for (let step = 0; step < 6; step += 1) {
		const slope = 3 * (1 - s) * (1 - s) * x1 + 6 * (1 - s) * s * (x2 - x1) + 3 * s * s * (1 - x2);
		if (slope === 0) {
			break;
		}
		s -= (at(x1, x2, s) - t) / slope;
	}
	return at(y1, y2, Math.min(1, Math.max(0, s)));
}
