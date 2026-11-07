/** The contract's types, by name: see `api/openapi.yaml`. Real tags are messy, so much is optional. */

export interface User {
	id: number;
	name: string;
	role: 'admin' | 'user';
	createdAt: string;
}

/** A user, as the session sees them: what `/me` says, and logging in doesn't. */
export interface Me extends User {
	/** An administrator, on a server with tag writes on. */
	canEditTags: boolean;
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
	trackTotal?: number;
	discTotal?: number;
	year?: number;
	genres: string[];
	/** Changes when the file does. It isn't the stream's ETag. */
	etag: string;
	/** Whether the caller has hearted it. {@link Library.marks} holds what's current. */
	hearted: boolean;
	/** The caller's rating, 1 to 5. Absent when unrated. */
	stars?: number;
}

export interface Album extends AlbumEntry {
	/** Who `artist` links to. Empty for a compilation or Various Artists: then it's plain text. */
	artists: ArtistLink[];
	tracks: Track[];
	durationMs: number;
	/** Distinct and sorted. */
	formats: string[];
	addedAt: string;
	/** Every genre of the tracks as tagged, each once, in track order. */
	genres: string[];
	/** Sent back as `If-Match` to edit this album. */
	etag: string;
	/** Whether the caller has hearted it. {@link Library.marks} holds what's current. */
	hearted: boolean;
}

/** Everything `GET /albums/{id}/tags` names. Four of them can be read but not edited yet. */
export type TagField =
	| 'title'
	| 'album'
	| 'artists'
	| 'albumArtists'
	| 'compilation'
	| 'track'
	| 'trackTotal'
	| 'disc'
	| 'discTotal'
	| 'date'
	| 'genres'
	| 'composers'
	| 'conductor'
	| 'lyricist'
	| 'performers'
	| 'remixer'
	| 'grouping'
	| 'label'
	| 'catalogNumber'
	| 'barcode'
	| 'isrc'
	| 'originalDate'
	| 'language'
	| 'bpm'
	| 'key'
	| 'copyright'
	| 'comment'
	| 'lyrics'
	| 'sortTitle'
	| 'sortArtist'
	| 'sortAlbum'
	| 'sortAlbumArtist';

/** Values by field, the same in every format. Numbers are decimal text. */
export type FieldValues = Partial<Record<TagField, string[]>>;

/** A tag no field covers, under its format's own key. */
export interface RawTag {
	key: string;
	values: string[];
	/** Gain, a fingerprint, or a value too long to show whole. */
	readOnly?: boolean;
}

/** One file's tags, as a tag editor shows them. */
export interface TrackTags {
	id: string;
	/** The file's base name. */
	file: string;
	fields: FieldValues;
	/** Fields to show and not edit, such as timestamped lyrics. */
	readOnly?: TagField[];
	tags: RawTag[];
}

/** What the files hold, in album order. */
export interface AlbumTags {
	/** The album's `If-Match` value. */
	etag: string;
	tracks: TrackTags[];
}

/** A change to a file's tags. */
export interface TagEdit {
	set?: FieldValues;
	/** Any field but `title`. A field also in `set` is a 400. */
	clear?: TagField[];
	/** By key. Empty values remove the tag. */
	tags?: { key: string; values: string[] }[];
}

/** `all` goes to every track, and `tracks` on top of it: a field in both takes the track's. */
export interface AlbumEdit {
	all?: TagEdit;
	tracks?: ({ id: string } & TagEdit)[];
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

/** A song in the library list: as much as a row shows, and what it takes to find the rest. */
export interface LibraryTrack {
	id: string;
	title: string;
	durationMs: number;
	artists: string[];
	discNo?: number;
	trackNo?: number;
	/** Whether the caller has hearted it. {@link Library.marks} holds what's current. */
	hearted: boolean;
	/** The caller's rating, 1 to 5. Absent when unrated. */
	stars?: number;
}

export interface LibraryAlbum {
	id: string;
	title: string;
	artist: string;
	compilation: boolean;
	year?: number;
	artwork?: Artwork;
	/** Whether the caller has hearted it. */
	hearted: boolean;
	tracks: LibraryTrack[];
}

export interface LibraryArtist {
	/** Absent for a group with nobody to link to, such as Various Artists. */
	id?: string;
	name: string;
	/** Who it files under, which decides its letter. */
	sortName: string;
	/** Whether the caller has hearted it. Always false without an `id`. */
	hearted: boolean;
	albums: LibraryAlbum[];
}

/** Every song, by artist and album, in the order they're shown. */
export interface LibraryIndex {
	artists: LibraryArtist[];
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

/** What a heart can be on, as the API names it. */
export type MarkKind = 'tracks' | 'albums' | 'artists';

/** One listen that counted, as `POST /me/plays` takes it. */
export interface PlayReport {
	trackId: string;
	/** When the listen started. */
	playedAt: string;
	source: string;
	/** Ours for this listen, so a retry counts once. */
	clientId: string;
}

/** What someone has done with the library, counted. */
export interface Listening {
	/** Plays that counted. */
	plays: number;
	/** Hearts on tracks, albums and artists. */
	hearts: number;
	/** Tracks with stars. */
	rated: number;
}

/**
 * How a Last.fm connection stands: `waiting` while Last.fm isn't answering, `reconnect` once it has
 * stopped taking the session.
 */
export type LastfmState = 'ok' | 'waiting' | 'reconnect';

/** The caller's Last.fm connection. The session key never leaves the server. */
export interface LastfmStatus {
	/** False when the server has no Last.fm key. */
	configured: boolean;
	connected: boolean;
	/** The Last.fm user name, when connected. */
	name?: string;
	scrobble: boolean;
	nowPlaying: boolean;
	loves: boolean;
	state: LastfmState;
	/** Plays the server holds for Last.fm. */
	waiting: number;
	lastSentAt?: string;
	/** When a waiting outbox is next tried. */
	retryAt?: string;
}

/** The three switches. One left out stays as it is. */
export type LastfmSwitches = Partial<Pick<LastfmStatus, 'scrobble' | 'nowPlaying' | 'loves'>>;

/** Where every page gets its data: the real API, or the fixtures for tests and clips. */
export interface LibrarySource {
	/** Rejects with {@link UnauthorizedError} without a session. */
	me(): Promise<Me>;
	/** {@link LibrarySource.me} for work in the background: losing the session here goes nowhere. */
	sessionUser(): Promise<User>;
	/** Rejects with {@link ServerError} or {@link UnauthorizedError}, and goes nowhere either. */
	reportPlay(play: PlayReport): Promise<void>;
	/** Hearts an item, or takes the heart off. Rejects with {@link ServerError}: 404 for an unknown ID. */
	setHeart(kind: MarkKind, id: string, on: boolean): Promise<void>;
	/** Rates a track 1 to 5, or clears it with null. Rejects as {@link LibrarySource.setHeart} does. */
	setStars(trackId: string, stars: number | null): Promise<void>;
	/** Rejects with {@link LoginError} when the session wasn't granted. */
	login(name: string, password: string): Promise<User>;
	/** Ends the session. */
	logout(): Promise<void>;
	/** Counts the caller's plays, hearts and rated tracks. */
	listening(): Promise<Listening>;
	lastfm(): Promise<LastfmStatus>;
	/** The last.fm page to send the browser to. Rejects with a {@link ServerError}: 409 when not set up. */
	connectLastfm(): Promise<string>;
	/** Rejects with a {@link ServerError}: 404 when not connected. */
	setLastfm(patch: LastfmSwitches): Promise<LastfmStatus>;
	disconnectLastfm(): Promise<void>;
	/** Says a track started. Best effort, so it goes nowhere on a 401 either. */
	nowPlaying(trackId: string): Promise<void>;
	facts(): Promise<LibraryFacts>;
	libraryIndex(): Promise<LibraryIndex>;
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
	/** Reads every tag of an album from its files. Rejects with {@link EditError}. */
	albumTags(id: string): Promise<AlbumTags>;
	/** Rejects with {@link EditError} unless the edit reached every file it named. */
	editAlbum(id: string, etag: string, edit: AlbumEdit): Promise<Album>;
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

/** An edit the server stopped. Each track is atomic; an album isn't. */
export interface EditStopped {
	/** The server's reason. */
	detail: string;
	/** The tracks that hold the edit. Empty when it never started. */
	changed: string[];
	/** The tracks that don't. */
	remaining: string[];
}

export type EditFailure =
	/** A scan or another edit holds the library: the server said when to come back. */
	| { kind: 'busy'; retryAfterSeconds: number }
	/** The ETag isn't current, or a file no longer matches what the server knows. */
	| { kind: 'stale' }
	/** A file changed under a write already running. */
	| ({ kind: 'stale' } & EditStopped)
	/** Not an administrator, or tag writes are off. */
	| { kind: 'forbidden' }
	/** No such album any more. */
	| { kind: 'gone' }
	/** A file of the album is gone, and a scan has yet to notice. */
	| { kind: 'missing' }
	/** The files can't take this edit. */
	| ({ kind: 'refused' } & EditStopped)
	/** The server may not rewrite a file. */
	| ({ kind: 'unwritable' } & EditStopped)
	/** Something unexpected, on the server's side. */
	| ({ kind: 'failed' } & EditStopped)
	/** No answer, or one the contract doesn't promise. The edit may or may not have landed. */
	| { kind: 'unavailable' };

export class EditError extends Error {
	override name = 'EditError';
	readonly failure: EditFailure;

	constructor(failure: EditFailure) {
		super(`edit failed: ${failure.kind}`);
		this.failure = failure;
	}
}
