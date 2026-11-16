/** The organize routes' types, by the contract's names: see `api/openapi.yaml`. */

import type { Artwork } from '#lib/library/source.ts';
import type { Song } from './template.ts';

/** Which songs a preview's page lists. `all` is every move: renames and new folders. */
export type OrganizeFilter = 'all' | 'rename' | 'move' | 'conflict' | 'skipped' | 'inPlace';

/** What a preview does with a song. */
export type FileKind = 'rename' | 'move' | 'conflict' | 'skipped' | 'inPlace';

/** Why a song is in conflict or skipped, in a preview, or left where it was by a run. */
export type LeftReason =
	| 'claimed'
	| 'unknownFile'
	| 'noTags'
	| 'tooLong'
	| 'otherFilesystem'
	| 'changed'
	| 'missing'
	| 'failed'
	| 'movedSince'
	| 'taken';

/** What to preview: the saved templates and the whole library, unless said otherwise. */
export interface OrganizeRequest {
	template?: string;
	compilations?: string;
	tracks?: readonly string[];
	albums?: readonly string[];
}

export interface OrganizeCounts {
	songs: number;
	inPlace: number;
	/** Renames and new folders together. */
	moves: number;
	renames: number;
	newFolders: number;
	conflicts: number;
	skipped: number;
	/** How many folders the moves land in. */
	folders: number;
}

export interface OrganizeClaimant {
	track: string;
	title: string;
	album: string;
	albumId: string;
	path: string;
	root: number;
	format: string;
	bitrate?: number;
	durationMs: number;
	plays: number;
	stars: number;
	addedAt: string;
	/** At the path already. */
	here: boolean;
}

export interface OrganizeConflict {
	self: OrganizeClaimant;
	with: OrganizeClaimant[];
	sameRecording: boolean;
	why?: 'digest' | 'recording' | 'tags';
	/** The group of Duplicates the songs are in, for its page. */
	group?: string;
}

export interface OrganizeFile {
	track: string;
	title: string;
	/** Relative to its library folder. */
	from: string;
	to?: string;
	kind: FileKind;
	caseOnly?: boolean;
	reason?: 'claimed' | 'unknownFile' | 'noTags' | 'tooLong';
	/** The file in the way, by name. */
	unknown?: string;
	conflict?: OrganizeConflict;
}

export interface OrganizeAlbum {
	id: string;
	title: string;
	artist: string;
	artwork?: Artwork;
	root: number;
	kind: FileKind | 'mixed';
	songs: number;
	fromFolder?: string;
	toFolder?: string;
	caseOnly?: boolean;
	files: OrganizeFile[];
	companions: { from: string; to: string }[];
}

export interface OrganizePreview {
	id: string;
	template: string;
	compilations: string;
	counts: OrganizeCounts;
	scope?: { songs: number; albums: number; album?: string };
	filter: OrganizeFilter;
	matching?: number;
	albums: OrganizeAlbum[];
	nextCursor?: string;
}

/** A song to show a template on. */
export interface OrganizeExample {
	track: string;
	title: string;
	artist: string;
	album: string;
	/** Where the file is now, relative to its library folder. */
	from: string;
	root: number;
	song: Song;
}

export type RunState = 'running' | 'done' | 'stopped' | 'undone';

export interface OrganizeRun {
	id: string;
	kind: 'run' | 'undo' | 'edit';
	state: RunState;
	startedAt: string;
	finishedAt?: string;
	by?: string;
	template: string;
	total: number;
	done: number;
	moved: number;
	folders: number;
	conflicts: number;
	left: number;
	failed: number;
	companions: number;
	playlists?: number;
	album?: string;
	stopping?: boolean;
	undoable: boolean;
	undoOf?: string;
	undoneBy?: string;
}

export interface OrganizeLeft {
	track: string;
	title: string;
	from: string;
	to?: string;
	root: number;
	reason: LeftReason;
}

export interface OrganizeRunDetail {
	run: OrganizeRun;
	left: OrganizeLeft[];
}

/** `organize` on `GET /events`: a run's progress, to administrators. */
export interface OrganizeEvent {
	run: string;
	state: RunState;
	done: number;
	total: number;
	/** Albums finished, in the preview's order. */
	albumsDone: number;
	album?: string;
}

/** What starts a run: the preview, and only these of its songs or albums when given. */
export interface OrganizeRunRequest {
	previewId: string;
	tracks?: readonly string[];
	albums?: readonly string[];
}

/** A refusal: `field` and `at` for a template that doesn't parse, `detail` the server's words. */
export class OrganizeError extends Error {
	override name = 'OrganizeError';
	/** 0 when no answer came. */
	readonly status: number;
	readonly detail: string;
	readonly field: 'template' | 'compilations' | undefined;
	readonly at: number | undefined;

	constructor(
		status: number,
		detail: string,
		template?: { field: 'template' | 'compilations'; at: number }
	) {
		super(detail === '' ? `organize: the server answered ${String(status)}` : detail);
		this.status = status;
		this.detail = detail;
		this.field = template?.field;
		this.at = template?.at;
	}
}

/** The organize routes. Every one is for administrators. Each rejects with {@link OrganizeError}. */
export interface OrganizeSource {
	preview(request: OrganizeRequest): Promise<OrganizePreview>;
	/** A page of a kept preview: a 404 when it's gone. */
	page(
		id: string,
		page: { filter: OrganizeFilter; q?: string; cursor?: string }
	): Promise<OrganizePreview>;
	examples(count: number, seed: number): Promise<OrganizeExample[]>;
	runs(): Promise<OrganizeRun[]>;
	run(id: string): Promise<OrganizeRunDetail>;
	start(request: OrganizeRunRequest): Promise<OrganizeRun>;
	stop(id: string): Promise<OrganizeRun>;
	undo(id: string): Promise<OrganizeRun>;
	/**
	 * Hears runs' progress where no server sends it: the fixtures run in the page. The real source
	 * leaves it out, and the events arrive on the one stream.
	 */
	listen?(heard: (event: OrganizeEvent) => void): () => void;
}
