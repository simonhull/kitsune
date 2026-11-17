/** The Duplicates routes' types, by the contract's names: see `api/openapi.yaml`. */

import type { Artwork, TrashChange } from '#lib/library/source.ts';

/** How sure a group is: the same audio, one MusicBrainz recording, or artist, title and length. */
export type Confidence = 'audio' | 'recording' | 'probably';

/** Which groups the list shows. `all` folds a pair's groups into its card. */
export type DuplicateFilter = 'all' | Confidence | 'pairs' | 'dismissed';

/** A way the suggested copy beats every other, in the order they decide. */
export type KeepReason = 'lossless' | 'original' | 'bitrate' | 'inPlace' | 'plays' | 'earlier';

/** A find's totals. Groups, songs and pairs leave out what was dismissed. */
export interface DuplicateCounts {
	groups: number;
	songs: number;
	pairs: number;
	audio: number;
	recording: number;
	probably: number;
	dismissed: number;
	/** The live songs, and those the fingerprint pass has read. */
	library: number;
	checked: number;
}

export interface DuplicateAlbum {
	id: string;
	title: string;
	artist: string;
	year?: number;
	artwork?: Artwork;
}

/** A group as the list shows it. */
export interface DuplicateGroupSummary {
	key: string;
	title: string;
	artist: string;
	confidence: Confidence;
	copies: number;
	formats: string[];
	albums: DuplicateAlbum[];
	durationMs: number[];
	/** Everyone's, across the copies. */
	plays: number;
	pair?: string;
	dismissed?: boolean;
}

/** An album pair as the list shows it: songs in both, and each album's songs the other lacks. */
export interface DuplicatePairSummary {
	key: string;
	albums: [DuplicateAlbum, DuplicateAlbum];
	both: number;
	only: [number, number];
	dismissed?: boolean;
}

/** `GET /admin/duplicates`: the counts and one page. Pairs come on the first page only. */
export interface DuplicateList {
	counts: DuplicateCounts;
	filter: DuplicateFilter;
	matching: number;
	pairs: DuplicatePairSummary[];
	groups: DuplicateGroupSummary[];
	nextCursor?: string;
}

/** One copy of a recording. Its path is relative to library folder `root`. */
export interface DuplicateCopy {
	track: string;
	title: string;
	album: DuplicateAlbum;
	disc?: number;
	trackNo?: number;
	path: string;
	root: number;
	format: string;
	bitrate?: number;
	sampleRate?: number;
	bitDepth?: number;
	lossless: boolean;
	durationMs: number;
	/** Everyone's. */
	plays: number;
	/** The caller's own; 0 for none. */
	stars: number;
	/** How many people rated it. */
	raters: number;
	playlists: number;
	addedAt: string;
	inPlace: boolean;
	original: boolean;
}

/** What a merge moves, as its confirm words it, sent back verbatim with the merge. */
export interface MergePlan {
	plays: number;
	listens: number;
	stars: {
		/** The caller's own in a merge of one group in which they rated a trashed copy; 0 is none. */
		yours?: { keeper: number; other: number; after: number };
		come: number;
		stay: number;
	};
	loves: number;
	hearts: number;
	entries: number;
	playlists: { id: string; name: string; file?: boolean }[];
	otherPlaylists: number;
	excluded: number;
	runs: number;
	queues: number;
}

/** What keeping one copy, or one album of a pair, would move. */
export interface KeepPlan {
	keep: string;
	plan: MergePlan;
}

export interface DuplicateGroup {
	key: string;
	title: string;
	artist: string;
	confidence: Confidence;
	dismissed?: boolean;
	pair?: string;
	keeper: string;
	reasons: KeepReason[];
	copies: DuplicateCopy[];
	plans: KeepPlan[];
}

/** A row of a pair's track lists: the copy on each album, or null where only the other has it. */
export interface DuplicatePairRow {
	group?: string;
	copies: [DuplicateCopy | null, DuplicateCopy | null];
}

export interface DuplicatePair {
	key: string;
	albums: [DuplicateAlbum, DuplicateAlbum];
	dismissed?: boolean;
	keeper: string;
	rows: DuplicatePairRow[];
	plans: KeepPlan[];
}

/** `GET /admin/duplicates/{group}`. */
export type DuplicateDetail =
	{ kind: 'group'; group: DuplicateGroup } | { kind: 'pair'; pair: DuplicatePair };

/** What a merge keeps and trashes: copies of a group, or albums of a pair. */
export interface MergeRequest {
	keep: string;
	trash: string[];
	plan: MergePlan;
}

export interface Merged {
	kept: string[];
	trashed: TrashChange;
}

/** A refusal: the server's words, and when the library changed, what the merge would move now. */
export class DuplicatesError extends Error {
	override name = 'DuplicatesError';
	/** 0 when no answer came. */
	readonly status: number;
	readonly detail: string;
	readonly plan: MergePlan | undefined;

	constructor(status: number, detail: string, plan?: MergePlan) {
		super(detail === '' ? `duplicates: the server answered ${String(status)}` : detail);
		this.status = status;
		this.detail = detail;
		this.plan = plan;
	}
}

/** The Duplicates routes. Every one is for administrators. Each rejects with {@link DuplicatesError}. */
export interface DuplicatesSource {
	list(filter: DuplicateFilter, cursor?: string): Promise<DuplicateList>;
	/** A group or a pair: a 404 when no group has the key now. */
	detail(key: string): Promise<DuplicateDetail>;
	merge(key: string, request: MergeRequest): Promise<Merged>;
	dismiss(key: string): Promise<void>;
	bringBack(key: string): Promise<void>;
}
