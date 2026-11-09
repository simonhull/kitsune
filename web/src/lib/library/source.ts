/** The contract's types, by name: see `api/openapi.yaml`. Real tags are messy, so much is optional. */

import type { ThemeChoice } from '#lib/theme/theme.svelte.ts';
import type { AlbumOrder, ArtistOrder, LibrarySort, Sort } from './sorts.ts';

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
	/** The file's tags, or Kitsune's own measure of a file with no gain tagged. */
	source: 'tags' | 'measured';
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
	/** The disc's name, as this track is tagged. */
	discSubtitle?: string;
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
	/** In disc order: one for an album on one disc. */
	discs: Disc[];
}

/** One disc of an album, as its songs say. A song with no disc number is on disc 1. */
export interface Disc {
	no: number;
	/** Its subtitle, from its first song that has one. */
	name?: string;
	songs: number;
	durationMs: number;
}

/** Everything `GET /albums/{id}/tags` names. Album, artists, album artists and compilation group a track. */
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
	| 'discSubtitle'
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
	| 'sortAlbumArtist'
	| 'musicbrainzAlbumId'
	| 'musicbrainzReleaseGroupId'
	| 'musicbrainzAlbumArtistId'
	| 'musicbrainzArtistId'
	| 'musicbrainzRecordingId'
	| 'musicbrainzTrackId';

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

/**
 * An edit of songs that may lie on several albums, as `POST /edits` takes it: `all` goes to every
 * track named, each one's own on top, and `albums` gives the etag of every album they're on.
 */
export interface TracksEdit {
	albums: { id: string; etag: string }[];
	all?: TagEdit;
	tracks: ({ id: string } & TagEdit)[];
}

/** Where each song of an edit is now: a song given another album has moved to it. */
export interface EditedTracks {
	tracks: { id: string; albumId: string }[];
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

/** An artist in the list of them all, with what the list sorts by. */
export interface ListedArtist extends ArtistEntry {
	/** When the newest album it owns arrived. */
	addedAt: string;
	/** How often the caller played the songs of the albums it owns. */
	plays: number;
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
	/** Everything below comes only when {@link IndexField}s ask for it, and is absent with nothing to say. */
	albumArtists?: string[];
	/** Split and spelled as an album's `displayGenres` are. */
	displayGenres?: string[];
	year?: number;
	composers?: string[];
	/** Such as `FLAC`, with `sampleRate`, `bitDepth` and `bitrate`. */
	format?: string;
	sampleRate?: number;
	bitDepth?: number;
	/** kbps. */
	bitrate?: number;
	addedAt?: string;
	/** The caller's. */
	plays?: number;
	lastPlayedAt?: string;
	/** Its ReplayGain, in dB. Absent when the file has none. */
	trackGain?: number;
	/** The caller's shuffle rules on it. */
	shuffle?: ShuffleMark;
}

/** What a person's shuffle rules say of one song, on the Library list. */
export interface ShuffleMark {
	/** The run it's combined in, its place from 1, and how many songs the run has. */
	run?: string;
	position?: number;
	length?: number;
	/** Shuffle never picks it. */
	excluded?: boolean;
}

/** Songs that always play together in shuffle, in order. */
export interface ShuffleRun {
	id: string;
	tracks: string[];
}

/** A person's rules touching some songs: every run holding one, whole, and which they left out. */
export interface ShuffleRules {
	runs: ShuffleRun[];
	excluded: string[];
}

/** What a shuffle draws from: the library, a Library filter, an album, an artist, or songs named. */
export type ShuffleScope =
	| { kind: 'library' | 'hearted' | 'rated' }
	| { kind: 'album' | 'artist'; id: string }
	| { kind: 'tracks'; tracks: string[] };

/** Finite plays every unit once; endless goes on, page by page. */
export type ShuffleMode = 'finite' | 'endless';

/** What `POST /me/shuffles` takes. */
export interface ShuffleStart {
	scope: ShuffleScope;
	mode: ShuffleMode;
	/** The song to lead with: the one already playing. */
	first?: string;
	seed?: number;
}

/** Why a pick came up: its weight's factors, each a multiplier. The client words them. */
export interface ShuffleWhy {
	/** The mean of its rated songs; 0 unrated. */
	stars: number;
	rating: number;
	hearted: boolean;
	heart: number;
	/** Whether it has come up before. */
	played: boolean;
	sinceDays: number;
	recency: number;
	/** Whether skips and listen time count. */
	memory: boolean;
	earlySkips: number;
	skipMemory: number;
	starts: number;
	skips: number;
	skipRate: number;
	listens: number;
	/** How much of it those heard, on average, from 0 to 1. */
	heard: number;
	listen: number;
	/** Flow's factor, 1 for none, leaning from `after`. */
	flow: number;
	after?: ArtistLink;
	similarity?: number;
}

/** One pick: a song, or songs combined, in the order they play, and why it came up. */
export interface ShufflePick {
	songs: TrackHit[];
	why: ShuffleWhy;
}

/** A page of a shuffle. The next is asked for after `to`. */
export interface ShufflePage {
	id: string;
	mode: ShuffleMode;
	from: number;
	to: number;
	/** How many songs it picks from. */
	songs: number;
	/** A finite shuffle has nothing after this page. */
	done: boolean;
	picks: ShufflePick[];
}

/** What `GET /library/index?fields=` can add to each track. */
export type IndexField =
	| 'albumArtist'
	| 'genre'
	| 'year'
	| 'composer'
	| 'format'
	| 'bitrate'
	| 'added'
	| 'plays'
	| 'lastPlayed'
	| 'gain'
	| 'shuffle';

/** The preferences a person keeps on the server, by key, with what each holds. */
export interface Prefs {
	'library.columns': ColumnsPref;
	'ui.theme': ThemeChoice;
	/** How much ratings and hearts matter to shuffle, from 0 to 1. */
	'shuffle.ratings': number;
	/** How much the next song leans toward one that suits the last, from 0 to 1. */
	'shuffle.flow': number;
	/** Whether shuffle learns from skips and how much of a song is heard. */
	'shuffle.memory': boolean;
	/** How the Library orders its artists and their albums; artist pages take the albums' order. */
	'library.sort': LibrarySort;
	'albums.sort': Sort<AlbumOrder>;
	'artists.sort': Sort<ArtistOrder>;
	/** The gain in dB, from -20 to 0, a song plays at while it has no ReplayGain. */
	'playback.untagged_gain': number;
}

export type PrefKey = keyof Prefs;

/**
 * A song column a person may choose. Checkbox, time and ⋯ are always there, in their places; the
 * number and the title are always there too, but may be moved ({@link MovableId}).
 */
export type ColumnId =
	| 'artist'
	| 'albumArtist'
	| 'album'
	| 'genre'
	| 'year'
	| 'rating'
	| 'heart'
	| 'plays'
	| 'lastPlayed'
	| 'added'
	| 'composer'
	| 'format'
	| 'bitrate'
	| 'gain';

/** The number and the title: always shown, never chosen or sized, but placed like any column. */
export type FrameId = 'number' | 'title';

/** What an order places: the columns chosen, and the number and the title among them. */
export type MovableId = ColumnId | FrameId;

/** `library.columns`: the columns shown, in their order, and the Artist switch. */
export interface ColumnsPref {
	/** The server puts back a number or a title an old value lacks, so both are always here. */
	order: MovableId[];
	/** Leave Artist empty where the track's artists are the album's. */
	artistWhenDifferent: boolean;
	/** The widths columns were dragged to, at the base size step. One not named has its default. */
	widths?: Partial<Record<ColumnId, number>>;
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
	/** Only when they say what the songs don't: more than one disc, or one with a name. */
	discs?: Disc[];
	/** Only in an index of some albums: the album before this one in the whole index, `''` when first. */
	after?: string;
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
	/** The background pass that fingerprints files after a scan. Both 0 while none runs; the fixtures leave it out. */
	fingerprints?: FingerprintStatus;
	/** The background pass that measures songs with no ReplayGain, and what it has measured. */
	loudness?: LoudnessStatus;
}

export interface LoudnessStatus {
	/** Files read, of `total` waiting when the pass began; both 0 while none runs. */
	done: number;
	total: number;
	/** Songs with no ReplayGain tags. */
	ungained: number;
	/** Of those, the ones Kitsune has measured. */
	measured: number;
	/** Of those, the ones it can't measure yet. */
	unmeasurable: number;
}

export interface FingerprintStatus {
	done: number;
	total: number;
}

/** A server setting's key, as server.toml spells it. */
export type SettingKey =
	| 'lastfm.api_key'
	| 'lastfm.shared_secret'
	| 'library.roots'
	| 'library.tag_writes'
	| 'library.quick_scan'
	| 'library.missing_grace'
	| 'log.level'
	| 'server.base_url'
	| 'server.trusted_proxies';

/** Where a setting's value came from. `default`: nothing has set it. */
export type SettingSource = 'default' | 'web' | 'server.toml' | 'env' | 'flag';

export interface Setting<T> {
	value: T;
	source: SettingSource;
	/** The source for a person, such as `KITSUNE_LASTFM_API_KEY`. Absent for `default` and `web`. */
	from?: string;
	/** A change applies only after a restart. */
	restartRequired?: boolean;
	/** Changed since the server started, so waiting for one. */
	pendingRestart?: boolean;
}

/** A write-only setting: nothing returns its value. */
export interface SecretSetting {
	set: boolean;
	source: SettingSource;
	from?: string;
}

/** Only `folder` can be added; the rest are the library's, or Kitsune's own. */
export type FolderKind = 'folder' | 'library' | 'inside' | 'holds' | 'data' | 'config';

/** `GET /admin/folders`: what the server sees in a folder. Directories only. */
export interface FolderListing {
	path: string;
	/** Absent at `/`. */
	parent?: string;
	kind: FolderKind;
	folders: { name: string; path: string; kind: FolderKind }[];
	/** Counted for a second or two at most: with `complete` false, at least this. */
	audio: { files: number; folders: number; complete: boolean };
}

/** A library folder and what it holds now. */
export interface LibraryFolder {
	path: string;
	tracks: number;
	bytes: number;
	/** How the server follows it for new music. Absent until it has looked. */
	watch?: FolderWatch;
}

/** How the server follows a library folder: told of every change, looking every 5 minutes, or not. */
export interface FolderWatch {
	mode: 'watching' | 'polling' | 'off';
	/** Why it isn't watched: a network or FUSE filesystem, the OS's watch limit, or a missing folder. */
	reason?: 'network' | 'watch-limit' | 'missing';
	/** The filesystem's type, such as `cifs`, for `network`. */
	filesystem?: string;
}

/** `library` on `GET /events`: albums added, changed or gone, or `all` when too many did to list. */
export interface LibraryEvent {
	albums: string[];
	all: boolean;
}

/** `marks` on `GET /events`: a heart, with `hearted`, or a rating, with `stars` (0 for none). */
export interface MarksEvent {
	kind: MarkKind;
	id: string;
	hearted?: boolean;
	stars?: number;
}

/** What `GET /events` carries, by event name. Each says what changed, never the thing itself. */
export interface ServerEvents {
	ready: Record<string, never>;
	resync: Record<string, never>;
	library: LibraryEvent;
	scan: ScanStatus;
	fingerprints: FingerprintStatus;
	loudness: LoudnessStatus;
	marks: MarksEvent;
	prefs: { key: string };
	'shuffle-rules': Record<string, never>;
	shuffle: { id: string; to: number };
	listening: { trackId: string; clientId?: string };
	lastfm: Record<string, never>;
	settings: { keys: string[] };
}

export type ServerEventKind = keyof ServerEvents;

/** `GET /admin/settings`. */
export interface ServerSettings {
	settings: {
		'lastfm.api_key': Setting<string>;
		'lastfm.shared_secret': SecretSetting;
		'library.roots': Setting<string[]>;
		'library.tag_writes': Setting<boolean>;
		/** HH:MM, local to the server. */
		'library.quick_scan': Setting<string>;
		/** A Go duration, such as `720h0m0s`. */
		'library.missing_grace': Setting<string>;
		'log.level': Setting<LogLevel>;
		'server.base_url': Setting<string>;
		'server.trusted_proxies': Setting<string[]>;
	};
	listen: string;
	listenSource: 'server.toml' | 'default';
	version: string;
	folders: LibraryFolder[];
}

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

/** The settings to change. Null clears one: it takes its default, and the next start seeds it. */
export type SettingsChange = Partial<{
	'lastfm.api_key': string | null;
	'lastfm.shared_secret': string | null;
	'library.roots': string[] | null;
	'library.tag_writes': boolean | null;
	'library.quick_scan': string | null;
	'library.missing_grace': string | null;
	'log.level': LogLevel | null;
	'server.base_url': string | null;
	'server.trusted_proxies': string[] | null;
}>;

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

/** How a listen ended. Only `skipped`, moving on before the end, counts against a song. */
export type ListenEnding = 'completed' | 'skipped' | 'stopped' | 'interrupted';

/** A listen's start, or with `ended`, its end, as `POST /me/listens` takes it. */
export interface ListenReport {
	/** Ours for this listen, shared by its start and end. */
	clientId: string;
	trackId: string;
	startedAt: string;
	/** The shuffle session it plays in. */
	shuffle?: string;
	ended?: ListenEnding;
	listenedMs?: number;
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
	/** A listen's start or end, for shuffle to learn from. Rejects as {@link LibrarySource.reportPlay} does. */
	reportListen(listen: ListenReport): Promise<void>;
	/** Hearts an item, or takes the heart off. Rejects with {@link ServerError}: 404 for an unknown ID. */
	setHeart(kind: MarkKind, id: string, on: boolean): Promise<void>;
	/** Rates a track 1 to 5, or clears it with null. Rejects as {@link LibrarySource.setHeart} does. */
	setStars(trackId: string, stars: number | null): Promise<void>;
	/** Rejects with {@link LoginError} when the session wasn't granted. */
	login(name: string, password: string): Promise<User>;
	/** Whether the first account is still to be made. Asks nobody to sign in. */
	setupStatus(): Promise<SetupStatus>;
	/** Makes the first account, an administrator, and signs in as it. Rejects with {@link SetupError}. */
	setup(name: string, password: string, code?: string): Promise<User>;
	/**
	 * Asks for a reset code for whoever is called `name`, which the server puts in its log. The answer
	 * is the same whether or not they exist. Rejects with {@link ResetError}: throttled or unavailable.
	 */
	requestReset(name: string): Promise<void>;
	/**
	 * Sets a new password with a reset code, ends every session of that person and signs in as them.
	 * Rejects with {@link ResetError}.
	 */
	confirmReset(name: string, code: string, password: string): Promise<User>;
	/**
	 * Changes the caller's password. This session stays; their others end. Rejects with
	 * {@link PasswordError}, or {@link UnauthorizedError} without a session.
	 */
	changePassword(current: string, password: string): Promise<void>;
	/** Every account, for administrators, with how each signs in. */
	accounts(): Promise<Account[]>;
	/**
	 * Sets someone else's password, for administrators, and ends their sessions. Rejects with
	 * {@link PasswordError}: `password` for one the server won't take.
	 */
	setPassword(userId: number, password: string): Promise<void>;
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
	/** Every song, with what `fields` asks for besides. */
	libraryIndex(fields?: readonly IndexField[]): Promise<LibraryIndex>;
	/**
	 * Just these albums, in the index's shape, each with its `after`: what a `library` event asks for.
	 * One that has no song any more isn't there.
	 */
	libraryIndexOf(albums: readonly string[], fields?: readonly IndexField[]): Promise<LibraryIndex>;
	/** One of the caller's preferences, or null when it was never set. */
	pref<K extends PrefKey>(key: K): Promise<Prefs[K] | null>;
	/** Rejects with a {@link ServerError}: 400 for a value the server won't keep. */
	setPref<K extends PrefKey>(key: K, value: Prefs[K]): Promise<void>;
	/** Reports whether the first scan has finished. */
	ready(): Promise<boolean>;
	albumsPage(sort: AlbumSort, cursor?: string, limit?: number): Promise<AlbumPage>;
	/** Rejects with {@link AlbumNotFoundError} for an unknown ID. */
	album(id: string): Promise<Album>;
	/** Every artist who owns an album, by name. Guests are found through search and links. */
	artists(): Promise<ListedArtist[]>;
	/** Rejects with {@link ArtistNotFoundError} for an unknown ID. */
	artist(id: string): Promise<ArtistDetail>;
	/** Finds artists, albums and tracks. Rejects with an `AbortError` when `signal` aborts. */
	search(query: string, signal?: AbortSignal): Promise<SearchResults>;
	/** Reads every tag of an album from its files. Rejects with {@link EditError}. */
	albumTags(id: string): Promise<AlbumTags>;
	/** Rejects with {@link EditError} unless the edit reached every file it named. */
	editAlbum(id: string, etag: string, edit: AlbumEdit): Promise<Album>;
	/** {@link LibrarySource.editAlbum} for songs on any albums. A stale album is named. */
	editTracks(edit: TracksEdit): Promise<EditedTracks>;
	/** The album a song is on now. Rejects with {@link EditError}: `gone` for an unknown one. */
	trackAlbum(trackId: string): Promise<string>;
	/** The caller's shuffle rules touching these songs. Rejects with {@link ServerError}. */
	shuffleRules(trackIds: readonly string[]): Promise<ShuffleRules>;
	/** Starts a shuffle, with its first page. Rejects with {@link ServerError}: 404 for an unknown album. */
	startShuffle(start: ShuffleStart): Promise<ShufflePage>;
	/** A shuffle's page after a position. Rejects with {@link ServerError}: 404 for one gone. */
	nextShuffle(id: string, after: number): Promise<ShufflePage>;
	/** Combines songs into one run, in order. Rejects with {@link ServerError}: 404 for an unknown song. */
	setRun(trackIds: readonly string[]): Promise<ShuffleRun>;
	/** Undoes a run. Rejects with {@link ServerError}. */
	deleteRun(id: string): Promise<void>;
	/** Leaves songs out of shuffle, or lets them back. Rejects with {@link ServerError}. */
	setExcluded(trackIds: readonly string[], on: boolean): Promise<void>;
	/** The URL a track's audio streams from. */
	streamUrl(trackId: string): string;
	/** Starts a scan. Rejects with a {@link ServerError}: 409 when one is running, 503 at shutdown. */
	startScan(mode: ScanMode): Promise<ScanStatus>;
	/** What scans had trouble with, for administrators. */
	issues(): Promise<ScanIssues>;
	/** Administrators only. */
	settings(): Promise<ServerSettings>;
	/** All of it or none: a refusal is a {@link SettingsError} naming the key. */
	changeSettings(change: SettingsChange): Promise<ServerSettings>;
	/** Administrators only. Rejects with a 404 `ServerError` for a folder the server can't read. */
	folders(path: string): Promise<FolderListing>;
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

/** A change the server refused. `detail` names the key and why, never the value. */
export class SettingsError extends Error {
	override name = 'SettingsError';
	/** The key refused, when the server named one it knows. */
	readonly key: SettingKey | undefined;
	readonly detail: string;

	constructor(detail: string, key: SettingKey | undefined) {
		super(detail);
		this.detail = detail;
		this.key = key;
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

export type ResetFailure =
	/** The code is wrong, used, replaced or stale, or nobody has the name: the server can't say which. */
	| { kind: 'code' }
	/** Too short or too long, as the server measures it. */
	| { kind: 'password' }
	/** Too many tries, or too many codes asked for: the server said when to come back. */
	| { kind: 'throttled'; retryAfterSeconds: number }
	/** A server error, or no answer at all. */
	| { kind: 'unavailable' };

export class ResetError extends Error {
	override name = 'ResetError';
	readonly failure: ResetFailure;

	constructor(failure: ResetFailure) {
		super(`reset failed: ${failure.kind}`);
		this.failure = failure;
	}
}

export type PasswordFailure =
	/** The current password isn't the caller's. */
	| { kind: 'current' }
	/** Too short or too long, as the server measures it. */
	| { kind: 'password' }
	/** Too many wrong current passwords: the server said when to come back. */
	| { kind: 'throttled'; retryAfterSeconds: number }
	/** A server error, or no answer at all. */
	| { kind: 'unavailable' };

export class PasswordError extends Error {
	override name = 'PasswordError';
	readonly failure: PasswordFailure;

	constructor(failure: PasswordFailure) {
		super(`password not set: ${failure.kind}`);
		this.failure = failure;
	}
}

/** An account as an administrator sees it. */
export interface Account {
	user: User;
	/** When the newest of their sessions was last used. Absent when there's none. */
	lastSeen?: string;
	/** How many sessions they have live: a device each, near enough. */
	sessions: number;
}

/** What `GET /setup` says. */
export interface SetupStatus {
	/** The server has no accounts yet. False for good once it has one. */
	needed: boolean;
	/** Making it needs the setup code from the server's log: this browser is outside its network. */
	codeRequired: boolean;
}

export type SetupFailure =
	/** An account exists: someone finished first, or setup was never open. */
	| { kind: 'done' }
	/** The setup code was missing or wrong. */
	| { kind: 'code' }
	/** Too short or too long, as the server measures it. */
	| { kind: 'password' }
	/** A name the server won't take. */
	| { kind: 'name' }
	/** Too many wrong codes: the server said when to come back. */
	| { kind: 'throttled'; retryAfterSeconds: number }
	/** A server error, or no answer at all. */
	| { kind: 'unavailable' };

export class SetupError extends Error {
	override name = 'SetupError';
	readonly failure: SetupFailure;

	constructor(failure: SetupFailure) {
		super(`setup failed: ${failure.kind}`);
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
	| { kind: 'stale'; album?: string }
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
