/**
 * Adding songs to playlists and taking them out, for every menu and page: the toast comes up at
 * once, saying what the add did as far as this browser knows, and the server's answer corrects it.
 * The toast's Add it again appends what was skipped, and Undo takes out exactly what the add put
 * in, or puts back exactly what a take-out took. Nothing on the play path waits on any of it.
 */
import { library } from '#lib/library/library.svelte.ts';
import type { TakenOut } from '#lib/library/playlists.svelte.ts';
import { distinct } from '#lib/library/playlistEdits.ts';
import type { PlaylistAdded } from '#lib/library/source.ts';
import {
	addedWords,
	againWords,
	nameFor,
	removedWords,
	undoneWords,
	type ToastPart
} from './playlistWords.ts';

/** How long a toast stays, its fox line draining all the while: as long as Trash's. */
export const playlistToastMs = 8000;

/** The toast above the player. */
export interface PlaylistToast {
	/** Moves on with every toast, so one replaced starts its line again. */
	id: number;
	words: ToastPart[];
	/** Add it again's words, when the add skipped songs. */
	again: string | null;
	/**
	 * `failed`: the server refused; the words say so, and there's nothing to undo. `undone`: Undo
	 * took it back, and the same toast says so in place.
	 */
	state: 'done' | 'failed' | 'undone';
}

/** What the toast's buttons act on. */
type Undoable =
	| {
			kind: 'add';
			playlistId: string;
			trackIds: readonly string[];
			answer: Promise<PlaylistAdded | null>;
	  }
	| { kind: 'takeOut'; playlistId: string; taken: TakenOut; saved: Promise<unknown> };

class PlaylistActions {
	toast = $state.raw<PlaylistToast | null>(null);
	#last: Undoable | null = null;
	#toasts = 0;
	#timer: ReturnType<typeof setTimeout> | undefined;

	/**
	 * Adds songs to a playlist, in the order given, and shows what that did: those it holds already
	 * are skipped unless `again`.
	 */
	add(playlistId: string, trackIds: readonly string[], again = false): void {
		const name = library.playlists.summary(playlistId)?.name ?? 'the playlist';
		const holds = again ? 0 : (library.playlists.holds(playlistId, trackIds) ?? 0);
		const songs = again ? trackIds.length : distinct(trackIds);
		const answer: Promise<PlaylistAdded | null> = library.playlists
			.add(playlistId, trackIds, again)
			.then(
				(done) => {
					if (this.#last?.kind === 'add' && this.#last.answer === answer) {
						this.#show(
							addedWords(done.added.length, done.skipped.length, name),
							againWords(done.skipped.length),
							true
						);
					}
					return done;
				},
				() => {
					if (this.#last?.kind === 'add' && this.#last.answer === answer) {
						this.#last = null;
						this.#fail(`Couldn't add to ${name}. Try again in a moment`);
					}
					return null;
				}
			);
		this.#last = { kind: 'add', playlistId, trackIds, answer };
		this.#show(addedWords(songs - holds, holds, name), againWords(holds));
	}

	/** Makes a playlist named as typed, or New playlist N for none, holding these songs. */
	async create(typed: string, trackIds: readonly string[]): Promise<string | null> {
		const name = nameFor(
			typed,
			library.playlists.all.map((p) => p.name)
		);
		try {
			const made = library.playlists.create(name, trackIds);
			if (trackIds.length > 0) {
				this.#show(addedWords(distinct(trackIds), 0, name), null);
			}
			const done = await made;
			if (trackIds.length > 0) {
				const answer = Promise.resolve(done);
				this.#last = { kind: 'add', playlistId: done.playlist.id, trackIds, answer };
				this.#show(addedWords(done.added.length, 0, name), null, true);
			}
			return done.playlist.id;
		} catch {
			this.#fail(`Couldn't make ${name}. Try again in a moment`);
			return null;
		}
	}

	/** Takes entries out of a playlist, never the songs, with an Undo that puts them back. */
	takeOut(playlistId: string, entryIds: readonly string[]): void {
		if (entryIds.length === 0) {
			return;
		}
		const name = library.playlists.summary(playlistId)?.name ?? 'the playlist';
		const { taken, saved } = library.playlists.takeOut(playlistId, entryIds);
		const titles = taken.runs.flatMap(({ entries }) => entries.map(({ track }) => track.title));
		this.#last = { kind: 'takeOut', playlistId, taken, saved };
		saved.catch(() => {
			if (this.#last?.kind === 'takeOut' && this.#last.saved === saved) {
				this.#last = null;
				this.#fail(`Couldn't take that out of ${name}. Try again in a moment`);
			}
		});
		this.#show(removedWords(titles.length > 0 ? titles : entryIds, name), null);
	}

	/** Adds what the last add skipped, after all. */
	again(): void {
		const last = this.#last;
		if (last?.kind !== 'add') {
			return;
		}
		this.dismiss();
		void last.answer.then((done) => {
			if (done !== null && done.skipped.length > 0) {
				this.add(last.playlistId, done.skipped, true);
			}
		});
	}

	/** Undoes exactly what the toast says was done. */
	undo(): void {
		const last = this.#last;
		this.#last = null;
		if (last === null) {
			this.dismiss();
			return;
		}
		this.#undone(last);
		if (last.kind === 'add') {
			void last.answer.then((done) => {
				if (done !== null && done.added.length > 0) {
					library.playlists.takeOut(
						last.playlistId,
						done.added.map(({ id }) => id)
					);
				}
			});
		} else {
			void last.saved.then(
				() => library.playlists.putBack(last.playlistId, last.taken),
				() => undefined
			);
		}
	}

	/** Takes the toast away; what it did stays done. */
	dismiss(): void {
		clearTimeout(this.#timer);
		this.toast = null;
	}

	/** The toast showing becomes what Undo did, in place: the same toast, its line started again. */
	#undone(last: Undoable): void {
		const id = this.toast?.id;
		if (id === undefined) {
			return;
		}
		const name = library.playlists.summary(last.playlistId)?.name ?? 'the playlist';
		const count =
			last.kind === 'add'
				? distinct(last.trackIds)
				: last.taken.runs.reduce((sum, { entries }) => sum + entries.length, 0);
		clearTimeout(this.#timer);
		this.toast = { id, words: undoneWords(last.kind, count, name), again: null, state: 'undone' };
		this.#timer = setTimeout(() => {
			this.toast = null;
		}, playlistToastMs);
	}

	#fail(words: string): void {
		this.#show([{ text: words }], null, false, 'failed');
	}

	/** Shows a toast, or, for `update`, changes the words of the one showing without starting it again. */
	#show(
		words: ToastPart[],
		again: string | null,
		update = false,
		state: PlaylistToast['state'] = 'done'
	): void {
		if (update && this.toast !== null) {
			this.toast = { ...this.toast, words, again };
			return;
		}
		if (update) {
			return;
		}
		clearTimeout(this.#timer);
		this.#toasts += 1;
		this.toast = { id: this.#toasts, words, again, state };
		this.#timer = setTimeout(() => {
			this.toast = null;
			this.#last = null;
		}, playlistToastMs);
	}
}

/** The one place songs go in and out of playlists from. */
export const playlistActions = new PlaylistActions();
