/**
 * ReplayGain in the files (plan 070): what can be written, the run in hand and the last runs, kept
 * here so a run outlives Health and the album ⋯ that started it. Health's card draws it, the nav
 * notes a run going, and an album's run ends in a toast with Undo. Nothing is optimistic: writes
 * are on disk, so everything shows the run's progress as its `replaygain` events tell it.
 */

import { library } from '#lib/library/library.svelte.ts';
import {
	ReplayGainError,
	type GainLeft,
	type GainRun,
	type GainRunAlbum,
	type ReplayGainEvent,
	type ReplayGainStatus
} from './types.ts';
import { problemWords, tookOutToast, wroteToast } from './words.ts';

/** How often a run with no events is asked after. */
const pollMs = 2000;
/** How long the album toast stays. */
export const toastMs = 8000;

/** What the card shows: what can be written, a run going, or what a run did. */
export type GainView = 'status' | 'run' | 'done';

/** The toast an album's run leaves: what it did, and the run Undo takes back. */
export interface GainToast {
	id: number;
	words: string;
	album: string;
	/** The run Undo undoes; absent when there's nothing to take back. */
	undo?: string;
	failed: boolean;
}

/** An album's own run, followed for its toast. */
interface AlbumRun {
	title: string;
	undo: boolean;
}

class ReplayGain {
	status = $state.raw<ReplayGainStatus | null>(null);
	view = $state<GainView>('status');
	run = $state.raw<GainRun | null>(null);
	/** The run's albums in its order, with how far it got. */
	albums = $state.raw<GainRunAlbum[]>([]);
	albumsDone = $state(0);
	left = $state.raw<GainLeft[]>([]);
	/** When this browser first heard of the run, for its pace. */
	heardSince = $state(0);
	problem = $state('');
	/** A write or an undo asked for, until the server answers. */
	asking = $state(false);
	toast = $state<GainToast | null>(null);

	#stopHearing: (() => void) | undefined;
	#poll: ReturnType<typeof setInterval> | undefined;
	#toastTimer: ReturnType<typeof setTimeout> | undefined;
	#toasts = 0;
	#reading = 0;
	readonly #albumRuns = new Map<string, AlbumRun>();

	/** Whether this person may write: an administrator, with tag writes on. */
	get can(): boolean {
		return library.user?.canTrash === true;
	}

	/** The run going, for the nav's note. */
	get going(): GainRun | null {
		return this.run?.state === 'running' ? this.run : null;
	}

	/** Starts hearing runs' progress, and finds a run going after a reload. Idempotent. */
	hear(): void {
		if (this.#stopHearing !== undefined) {
			return;
		}
		this.#stopHearing = library.hearReplayGain((event) => {
			this.heard(event);
		});
		void this.refresh();
	}

	/** Stops hearing, and forgets, at sign-out. */
	deafen(): void {
		this.#stopHearing?.();
		this.#stopHearing = undefined;
		clearInterval(this.#poll);
		this.#poll = undefined;
		this.status = null;
		this.run = null;
		this.view = 'status';
		this.albums = [];
		this.left = [];
		this.#albumRuns.clear();
	}

	/** Reads what can be written, and follows a run going that this browser didn't know of. */
	async refresh(): Promise<void> {
		const read = ++this.#reading;
		try {
			const status = await library.replayGain.status();
			if (read !== this.#reading) {
				return;
			}
			this.status = status;
			const going = status.run;
			if (going !== undefined && going.state === 'running' && this.run?.id !== going.id) {
				this.follow(going);
			}
		} catch {
			// The card stays as it was, or away: nothing waits on it.
		}
	}

	/** Writes every album that can be. */
	async start(): Promise<void> {
		await this.#begin(() => library.replayGain.start());
	}

	/** Writes one album's measured songs, from its ⋯; its run ends in a toast. */
	async writeAlbum(id: string, title: string): Promise<boolean> {
		try {
			const run = await library.replayGain.start([id]);
			this.#albumRuns.set(run.id, { title, undo: false });
			this.follow(run);
			return true;
		} catch (error) {
			this.#say({
				words: problemWords(detailOf(error), 'ReplayGain wasn’t written. Try again.'),
				album: '',
				failed: true
			});
			return false;
		}
	}

	async stop(): Promise<void> {
		const run = this.run;
		if (run === null) {
			return;
		}
		try {
			this.run = await library.replayGain.stop(run.id);
		} catch (error) {
			this.problem = problemWords(detailOf(error), 'The run didn’t stop. Try again.');
		}
	}

	/** Takes out what a run wrote: a run of its own, under the same head. */
	async undo(id: string, album?: string): Promise<void> {
		await this.#begin(async () => {
			const undo = await library.replayGain.undo(id);
			if (album !== undefined) {
				// An album's undo, from its toast, ends in a toast of its own.
				this.#albumRuns.set(undo.id, { title: album, undo: true });
			}
			return undo;
		});
	}

	/** Undo from the toast. */
	undoToast(): void {
		const id = this.toast?.undo;
		const album = this.toast?.album;
		this.dismiss();
		if (id !== undefined) {
			void this.undo(id, album).then(() => {
				if (this.problem !== '') {
					this.#say({ words: this.problem, album: '', failed: true });
					this.problem = '';
				}
			});
		}
	}

	/** Shows a finished run, from the runs list, so it can be undone. */
	async show(run: GainRun): Promise<void> {
		this.run = run;
		this.view = 'done';
		this.albums = [];
		this.left = [];
		try {
			const detail = await library.replayGain.run(run.id);
			if (this.run.id === run.id) {
				this.run = detail.run;
				this.albums = detail.albums;
				this.left = detail.left;
			}
		} catch {
			// The row's own facts are enough to undo it.
		}
	}

	dismiss(): void {
		clearTimeout(this.#toastTimer);
		this.toast = null;
	}

	/** Follows a run: its head, its albums, its events, and a poll where no events come. */
	follow(run: GainRun): void {
		this.run = run;
		this.albumsDone = run.albumsDone;
		this.heardSince = Date.now();
		this.left = [];
		this.problem = '';
		this.view = run.state === 'running' ? 'run' : 'done';
		void this.#readAlbums(run.id);
		this.#watch();
	}

	/** A `replaygain` event: the run in hand moves on, or ends. */
	heard(event: ReplayGainEvent): void {
		const run = this.run;
		if (run === null || run.id !== event.run) {
			if (event.state === 'running') {
				// A run started elsewhere: the nav notes it, and the card follows it.
				this.follow({ ...blankRun(event.run), done: event.done, total: event.total });
			}
			return;
		}
		const moved = event.albumsDone !== this.albumsDone;
		this.run = {
			...run,
			state: event.state,
			done: event.done,
			total: event.total,
			albumsDone: event.albumsDone,
			...(event.album === undefined ? {} : { album: event.album })
		};
		this.albumsDone = event.albumsDone;
		if (event.state !== 'running') {
			void this.ended(event.run);
		} else if (
			moved &&
			event.album !== undefined &&
			!this.albums.some((a) => a.id === event.album)
		) {
			void this.#readAlbums(event.run);
		}
	}

	/** Reads what a run did once it ended, what can be written now, and says so for an album's run. */
	async ended(id: string): Promise<void> {
		clearInterval(this.#poll);
		this.#poll = undefined;
		let finished: GainRun | null = this.run?.id === id ? this.run : null;
		try {
			const detail = await library.replayGain.run(id);
			finished = detail.run;
			if (this.run?.id === id) {
				this.run = detail.run;
				this.albums = detail.albums;
				this.left = detail.left;
			}
		} catch {
			// The head keeps what the events said.
		}
		if (this.run?.id === id) {
			this.view = 'done';
		}
		const album = this.#albumRuns.get(id);
		this.#albumRuns.delete(id);
		if (album !== undefined && finished !== null) {
			this.#toastFor(finished, album);
		}
		await this.refresh();
	}

	async #begin(ask: () => Promise<GainRun>): Promise<void> {
		this.problem = '';
		this.asking = true;
		try {
			this.follow(await ask());
		} catch (error) {
			this.problem = problemWords(detailOf(error), 'That didn’t start. Try again.');
		} finally {
			this.asking = false;
		}
	}

	async #readAlbums(id: string): Promise<void> {
		try {
			const detail = await library.replayGain.run(id);
			const run = this.run;
			if (run?.id === id) {
				this.albums = detail.albums;
				// What an event can't say: an undo, when it began and how many albums it takes.
				const { kind, startedAt, albums, undoOf } = detail.run;
				this.run = { ...run, kind, startedAt, albums, ...(undoOf === undefined ? {} : { undoOf }) };
				if (run.state !== 'running') {
					this.left = detail.left;
				}
			}
		} catch {
			// The head goes on without its albums.
		}
	}

	#toastFor(run: GainRun, album: AlbumRun): void {
		if (album.undo) {
			this.#say({ words: tookOutToast(run.written), album: album.title, failed: false });
			return;
		}
		this.#say({
			words: wroteToast(run.written, run.left + run.failed),
			album: album.title,
			failed: run.written === 0,
			...(run.undoable && this.can ? { undo: run.id } : {})
		});
	}

	#say(toast: Omit<GainToast, 'id'>): void {
		clearTimeout(this.#toastTimer);
		this.#toasts += 1;
		this.toast = { ...toast, id: this.#toasts };
		this.#toastTimer = setTimeout(() => {
			this.toast = null;
		}, toastMs);
	}

	/** While a run goes and no events arrive, asks how it's going every couple of seconds. */
	#watch(): void {
		clearInterval(this.#poll);
		this.#poll = undefined;
		if (library.replayGain.listen !== undefined || this.run?.state !== 'running') {
			return; // The fixtures tell as they go.
		}
		this.#poll = setInterval(() => {
			const run = this.run;
			if (library.live || run?.state !== 'running') {
				return;
			}
			void library.replayGain
				.status()
				.then((status) => {
					const now =
						status.run?.id === run.id ? status.run : status.runs.find((each) => each.id === run.id);
					if (now !== undefined) {
						this.heard({
							run: now.id,
							state: now.state,
							done: now.done,
							total: now.total,
							albumsDone: now.albumsDone,
							...(now.album === undefined ? {} : { album: now.album })
						});
					}
				})
				.catch(() => undefined);
		}, pollMs);
	}
}

/** The server's words for a refusal, when it gave any. */
function detailOf(error: unknown): string {
	return error instanceof ReplayGainError ? error.detail : '';
}

/** A run known only by an event, until its detail is read. */
function blankRun(id: string): GainRun {
	return {
		id,
		kind: 'run',
		state: 'running',
		startedAt: new Date().toISOString(),
		total: 0,
		done: 0,
		written: 0,
		left: 0,
		failed: 0,
		albums: 0,
		albumsDone: 0,
		undoable: false
	};
}

export const replayGain = new ReplayGain();
