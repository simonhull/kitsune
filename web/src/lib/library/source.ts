/** The contract's types, by name: see `api/openapi.yaml`. Real tags are messy, so much is optional. */

export interface User {
	id: number;
	name: string;
	role: 'admin' | 'user';
	createdAt: string;
}

export interface Artwork {
	/** Names the cover for `GET /artwork/{hash}`. */
	hash: string;
	/** `#f28c28`, the fox, means the cover has no color of its own. */
	accent: string;
	ground: string;
	/** Of the source image. */
	width: number;
	height: number;
}

/** An album's list entry: everything a page needs for its first frame. */
export interface AlbumEntry {
	id: string;
	title: string;
	artist: string;
	year?: number;
	trackCount: number;
	compilation: boolean;
	artwork?: Artwork;
	/** The title as it sorts. Everything below serves the Albums page. */
	sortName?: string;
	/** Who the album sorts under. Only an album list says. */
	sortArtist?: string;
	addedAt?: string;
	/** Split, cleaned and spelled one way across the library. The most used on it come first. */
	displayGenres?: string[];
}

export interface AlbumPage {
	albums: AlbumEntry[];
	/** Absent on the last page. */
	nextCursor?: string;
}

export interface ReplayGain {
	trackGain?: number;
	albumGain?: number;
	trackPeak?: number;
	albumPeak?: number;
}

export interface Track {
	id: string;
	title: string;
	artists: string[];
	discNo?: number;
	trackNo?: number;
	durationMs: number;
	format: string;
	sampleRate?: number;
	bitDepth?: number;
	replayGain?: ReplayGain;
}

export interface Album extends AlbumEntry {
	/** Who `artist` links to. Empty for a compilation or Various Artists: then it's plain text. */
	artists: ArtistLink[];
	tracks: Track[];
	durationMs: number;
	/** Distinct and sorted. */
	formats: string[];
	addedAt: string;
}

/** An artist, as much as it takes to link to one. */
export interface ArtistLink {
	id: string;
	name: string;
}

/** An artist who owns an album, as the list of them shows it. */
export interface ArtistEntry extends ArtistLink {
	/** The albums it owns. */
	albumCount: number;
}

export interface ArtistDetail extends ArtistEntry {
	/** The albums it owns, newest first. */
	albums: AlbumEntry[];
	/** The other albums it has a track on, newest first. */
	appearsOn: AlbumEntry[];
}

/** A track a search found. Everything about it leads to its album. */
export interface TrackHit {
	id: string;
	title: string;
	artists: string[];
	albumId: string;
	album: string;
	durationMs: number;
	artwork?: Artwork;
}

/** What a search found, best first in each group. */
export interface SearchResults {
	artists: ArtistLink[];
	albums: AlbumEntry[];
	tracks: TrackHit[];
}

export interface LibraryFacts {
	albums: number;
	tracks: number;
	durationMs: number;
	roots: number;
	/** Files the scanner could not read. */
	issues: number;
	issueCounts: IssueCounts;
	scan: ScanStatus;
}

export interface IssueCounts {
	/** An I/O or permission error stopped them. They are tried again every scan. */
	unreadable: number;
	/** Structurally broken. */
	corrupted: number;
	/** Of a format Kitsune can't read. */
	unsupported: number;
	/** Live tracks read with warnings. They play, and `issues` leaves them out. */
	warnings: number;
}

export type IssueKind = 'unreadable' | 'corrupted' | 'unsupported';

/** A file a scan had trouble with. */
export interface ScanIssue {
	/** Relative to its library root. */
	path: string;
	kind: IssueKind;
	message: string;
}

/** A track that plays, and that a scan had something to say about. */
export interface TrackWarned {
	trackId: string;
	/** Relative to its library root. */
	path: string;
	messages: string[];
}

export interface ScanIssues {
	issues: ScanIssue[];
	warnings: TrackWarned[];
}

export type ScanMode = 'quick' | 'full';

/** Why a scan didn't start. A scan already running is no refusal: there's a scan to watch. */
export type ScanRefusal = 'forbidden' | 'stopping' | 'unavailable';

/** Where scanning stands. Idle, only `last` says anything. */
export interface ScanStatus {
	state: 'idle' | 'running';
	mode?: ScanMode;
	read: number;
	/** Files the scan has to read. 0 until it has walked the roots. */
	total: number;
	startedAt?: string;
	/** Held in the server's memory: a restart starts over. */
	last?: ScanResult;
}

/** How the last scan to end went. */
export interface ScanResult {
	mode: ScanMode;
	outcome: 'finished' | 'failed';
	startedAt: string;
	endedAt: string;
	/** Audio files found under the roots. */
	seen: number;
	read: number;
	added: number;
	updated: number;
	moved: number;
	missing: number;
	deleted: number;
	/** Files this scan read and could not use. */
	issues: number;
	/** Roots left alone because they looked like an outage. */
	skippedRoots: number;
	/** Why it failed. Administrators only. */
	error?: string;
}

export type AlbumSort = 'added' | 'name';

/** One listen that counted, as `POST /me/plays` takes it. */
export interface PlayReport {
	trackId: string;
	/** When the listen started. */
	playedAt: string;
	source: string;
	/** Ours for this listen, so a retry counts once. */
	clientId: string;
}

/** Where every page gets its data: the real API, or the fixtures for tests and clips. */
export interface LibrarySource {
	/** Rejects with {@link UnauthorizedError} without a session. */
	me(): Promise<User>;
	/** {@link LibrarySource.me} for work in the background: losing the session here goes nowhere. */
	sessionUser(): Promise<User>;
	/** Rejects with {@link ServerError} or {@link UnauthorizedError}, and goes nowhere either. */
	reportPlay(play: PlayReport): Promise<void>;
	/** Rejects with {@link LoginError} when the session wasn't granted. */
	login(name: string, password: string): Promise<User>;
	facts(): Promise<LibraryFacts>;
	/** Reports whether the first scan has finished. */
	ready(): Promise<boolean>;
	albumsPage(sort: AlbumSort, cursor?: string, limit?: number): Promise<AlbumPage>;
	/** Rejects with {@link AlbumNotFoundError} for an unknown ID. */
	album(id: string): Promise<Album>;
	/** Every artist who owns an album, by name. Guests are found through search and links. */
	artists(): Promise<ArtistEntry[]>;
	/** Rejects with {@link ArtistNotFoundError} for an unknown ID. */
	artist(id: string): Promise<ArtistDetail>;
	/** Finds artists, albums and tracks. Rejects with an `AbortError` when `signal` aborts. */
	search(query: string, signal?: AbortSignal): Promise<SearchResults>;
	/** The URL a track's audio streams from. */
	streamUrl(trackId: string): string;
	/** Starts a scan. Rejects with a {@link ServerError}: 409 when one is running, 503 at shutdown. */
	startScan(mode: ScanMode): Promise<ScanStatus>;
	/** What scans had trouble with, for administrators. */
	issues(): Promise<ScanIssues>;
}

export class AlbumNotFoundError extends Error {
	override name = 'AlbumNotFoundError';

	constructor(id: string) {
		super(`no album with ID ${id}`);
	}
}

export class ArtistNotFoundError extends Error {
	override name = 'ArtistNotFoundError';

	constructor(id: string) {
		super(`no artist with ID ${id}`);
	}
}

/** The session is missing or over. The app has already been sent to `/login`. */
export class UnauthorizedError extends Error {
	override name = 'UnauthorizedError';

	constructor() {
		super('no session');
	}
}

/** The server answered with something other than what the contract promises for success. */
export class ServerError extends Error {
	override name = 'ServerError';
	readonly status: number;

	constructor(status: number) {
		super(`the server answered ${String(status)}`);
		this.status = status;
	}
}

export type LoginFailure =
	| { kind: 'credentials' }
	/** Rate limited: the server said when to come back. */
	| { kind: 'throttled'; retryAfterSeconds: number }
	/** A bad request, a server error, or no answer at all. */
	| { kind: 'unavailable' };

export class LoginError extends Error {
	override name = 'LoginError';
	readonly failure: LoginFailure;

	constructor(failure: LoginFailure) {
		super(`login failed: ${failure.kind}`);
		this.failure = failure;
	}
}
