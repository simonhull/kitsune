/** The ReplayGain routes' types, by the contract's names: see `api/openapi.yaml`. */

import type { Artwork } from '#lib/library/source.ts';

/** `running`, `done`, `stopped` (by Stop, or by the server stopping), or `undone`. */
export type GainState = 'running' | 'done' | 'stopped' | 'undone';

/** An album with songs a run writes, or wrote; `songs` counts those. */
export interface GainAlbum {
	id: string;
	title: string;
	artist: string;
	year?: number;
	artwork?: Artwork;
	formats: string[];
	songs: number;
}

/** A run, or an undo of one (`undoOf`). */
export interface GainRun {
	id: string;
	kind: 'run' | 'undo';
	state: GainState;
	startedAt: string;
	finishedAt?: string;
	by?: string;
	/** Songs it takes on, and how many it has dealt with. */
	total: number;
	done: number;
	/** Files written (for an undo, put back). */
	written: number;
	/** Songs left as they were. */
	left: number;
	/** Songs whose write failed. */
	failed: number;
	albums: number;
	albumsDone: number;
	/** The album in hand. */
	album?: string;
	stopping?: boolean;
	/** While an undo would take something out. */
	undoable: boolean;
	undoOf?: string;
	undoneBy?: string;
}

/** `GET /admin/replaygain`: what can be written now, what waits, the run going and the last runs. */
export interface ReplayGainStatus {
	songs: number;
	albums: number;
	formats: string[];
	/** The albums with the most songs to write, at most four. */
	biggest: GainAlbum[];
	/** Songs the loudness pass hasn't measured, or pooled, yet. */
	waiting: number;
	unmeasurable: number;
	run?: GainRun;
	runs: GainRun[];
}

/** An album of a run, and how far the run got with it. */
export interface GainRunAlbum extends GainAlbum {
	state: 'written' | 'writing' | 'waiting' | 'left';
	written: number;
	left: number;
}

/** Why a run left a song as it was. */
export type GainLeftReason =
	| 'changed'
	| 'unwritable'
	| 'stale'
	| 'trashed'
	| 'missing'
	| 'retagged'
	| 'interrupted'
	| 'failed';

/** A song a run left as it was, and why. */
export interface GainLeft {
	track: string;
	title: string;
	/** Relative to its library folder. */
	path: string;
	root: number;
	reason: GainLeftReason;
}

/** `GET /admin/replaygain/runs/{id}`. */
export interface GainRunDetail {
	run: GainRun;
	albums: GainRunAlbum[];
	left: GainLeft[];
}

/** `replaygain` on `GET /events`: a run's progress, to administrators. */
export interface ReplayGainEvent {
	run: string;
	state: GainState;
	done: number;
	total: number;
	albumsDone: number;
	album?: string;
}

/** A refusal: `detail` is the server's words. */
export class ReplayGainError extends Error {
	override name = 'ReplayGainError';
	/** 0 when no answer came. */
	readonly status: number;
	readonly detail: string;

	constructor(status: number, detail: string) {
		super(detail === '' ? `replaygain: the server answered ${String(status)}` : detail);
		this.status = status;
		this.detail = detail;
	}
}

/** The ReplayGain routes, for administrators. Each rejects with {@link ReplayGainError}. */
export interface ReplayGainSource {
	status(): Promise<ReplayGainStatus>;
	/** Writes every album that can be, or just these. */
	start(albums?: readonly string[]): Promise<GainRun>;
	run(id: string): Promise<GainRunDetail>;
	stop(id: string): Promise<GainRun>;
	undo(id: string): Promise<GainRun>;
	/**
	 * Hears runs' progress where no server sends it: the fixtures run in the page. The real source
	 * leaves it out, and the events arrive on the one stream.
	 */
	listen?(heard: (event: ReplayGainEvent) => void): () => void;
	/**
	 * Hears which albums a run changed, as the server's `library` events would say. Only the
	 * fixtures have it.
	 */
	changed?(heard: (albums: string[]) => void): () => void;
}
