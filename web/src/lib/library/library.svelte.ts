import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { preload } from '#lib/cover/rungs.ts';
import { decodesAhead, mostQueued, openingFor, throwGap } from '#lib/pages/deck.ts';
import { theme } from '#lib/theme/theme.svelte.ts';
import { createApiSource } from './api.ts';
import { once } from './edit.ts';
import type { RuleChange } from './shuffleRules.ts';
import { retagged, type NowPlaying } from './cue.ts';
import {
	createFixtureSource,
	createScanFixtureSource,
	defaultScanGap,
	lastfmFixture,
	scanStart,
	setupFixture,
	type FixtureEditFault
} from './fixtures.ts';
import { healthFixture } from './healthFixtures.ts';
import { patchIndex } from './indexPatch.ts';
import { Trashed } from './trashed.svelte.ts';
import type { PlaybackFixture } from './playbackFixture.ts';
import { eventsUrl, Live, type EventStream } from './live.ts';
import { Marks } from './marks.svelte.ts';
import { ListenOutbox, Outbox } from './outbox.ts';
import { createPrefs, type PrefsStore } from './prefs.ts';
import { Search } from './search.svelte.ts';
import {
	AlbumNotFoundError,
	ArtistNotFoundError,
	ServerError,
	SetupError,
	UnauthorizedError,
	type Album,
	type AlbumEdit,
	type AlbumEntry,
	type AlbumTags,
	type ArtistDetail,
	type ListedArtist,
	type ArtistLink,
	type EditedTracks,
	type IndexField,
	type PlaybackRecord,
	type PlaybackSave,
	type LastfmStatus,
	type HeartsConverted,
	type HeartsSummary,
	type LastfmSwitches,
	type History,
	type LibraryFacts,
	type LibraryAlbum,
	type LibraryIndex,
	type LibrarySource,
	type Listening,
	type Me,
	type ScanIssues,
	type ScanMode,
	type ScanRefusal,
	type FolderListing,
	type FingerprintStatus,
	type LibraryEvent,
	type PrefKey,
	type Prefs,
	type ScanStatus,
	type ServerSettings,
	type SettingsChange,
	type SetupStatus,
	type Account,
	type ListenReport,
	type ShufflePage,
	type ShuffleRules,
	type ShuffleStart,
	type TracksEdit,
	type Trash,
	type TrashChange,
	type TrashEmpty,
	type TrashMove
} from './source.ts';

/** The grid's page: 60 divides evenly into every column count, so pages stack with no seam. */
export const albumPageSize = 60;
/** How much of the library one request brings, when all of it is wanted. */
const wholeListPageSize = 200;
/** The feature tile and the covers beside it: as many as a 4K screen's row holds, and some. */
const recentCount = 25;
/** How long Home's copy of the history is good for: a visit after this asks again. */
export const historyFreshMs = 60_000;
/** How long to leave a server that isn't answering, or a library still empty. */
const retryMs = 5000;
/** How often a broken cover may ask whether the session is still good. */
const probeMs = 30_000;
/** How long a library with albums gets to turn out to be a restart, not a first scan. */
const restartGraceMs = 1500;
/** How often the deck asks what the scan has found. */
const scanPollMs = 1000;
/** How often anyone else asks how a running scan is doing. */
const progressPollMs = 1000;
/** How often anyone asks how the fingerprint pass is doing: it runs for many minutes. */
const passPollMs = 5000;
/** When to read again what a settings change changes, which the server applies a moment later. */
const settledReadsMs = [300, 1500] as const;
/** How much of the newest end of the library each of those asks for. */
const feedSize = 100;
/** The most cards on the pile at once: older ones have left the DOM, under it. */
export const pileSize = 12;
/** How long a card may hold up its landing for its cover. */
const decodeBoundMs = 800;
/** How many decoded covers may wait for their turn: a decode kept too long has been wasted. */
const readyAhead = 12;
/** How often a hidden tab looks up to see if it can deal again. */
const hiddenRetryMs = 1000;
/** How long a list of issues that failed to load waits before its one try again. */
export const issuesRetryMs = 3000;
/** How long a listen's report waits to go, so the sound starting has the connection to itself. */
export const listenFlushMs = 1500;
/** The longest sign-out waits for where the player is to be saved. */
const signOutSaveMs = 1000;

export type Session = 'signed-in' | 'signed-out' | 'down';

/** Where something asked for has got to. */
export type Load<T> =
	| { kind: 'loading' }
	| { kind: 'ready'; value: T }
	/** The server says there's no such thing. */
	| { kind: 'missing' }
	/** It couldn't be fetched this time. */
	| { kind: 'unavailable' };

/** Whether this browser has seen the library ready, which outlives any session. */
export interface ReadyMemory {
	seen(): boolean;
	mark(): void;
}

/** Remembers in a browser store, if there is one. Without it a restart can look like a first scan. */
export function readyMemoryIn(store: () => Storage, key: string): ReadyMemory {
	return {
		seen() {
			try {
				return store().getItem(key) !== null;
			} catch {
				return false;
			}
		},
		mark() {
			try {
				store().setItem(key, '1');
			} catch {
				// Nothing to do: the next restart waits a moment longer, and may show the deck.
			}
		}
	};
}

export interface LibraryOptions {
	readyMemory?: ReadyMemory;
	/** Where plays wait for the server. */
	outbox?: () => Storage;
	/** This library has no audio behind it: the player runs on a clock instead. */
	simulated?: boolean;
	/**
	 * Opens the server's stream of events. Null means none, as for the fixtures, which have no server
	 * to hear; left out, it's the browser's `EventSource`, where there is one.
	 */
	events?: (() => EventStream) | null;
}

/** The browser's event stream, or none outside a browser. */
function browserEvents(): (() => EventStream) | null {
	return typeof EventSource === 'undefined' ? null : () => new EventSource(eventsUrl);
}

/** What the library sees of the player, which is built on it and so can't be imported here. */
export interface PlayerView {
	/** From the first play until a reset, whatever has happened since. */
	readonly started: boolean;
	readonly cue: NowPlaying | null;
	/** Takes in an album just edited. */
	retag(album: Album): void;
	reset(): void;
}

/**
 * Gives a card what it needs to land: its decoded cover, so the image is what lands. One that fails,
 * or that's still not here at the bound, lands as its generated cover instead of as a blank.
 */
async function cardFor(entry: AlbumEntry): Promise<AlbumEntry> {
	if (entry.artwork === undefined) {
		return entry;
	}
	const timeout = new Promise<boolean>((resolve) => {
		setTimeout(() => {
			resolve(false);
		}, decodeBoundMs);
	});
	const decoded = await Promise.race([preload(entry.artwork, 'card'), timeout]);
	return decoded ? entry : { ...entry, artwork: undefined };
}

/** The client's view of the library. List entries seed the first frame of the next page. */
/** An artist's songs as read for its page. */
interface ArtistSongsRead {
	/** The fields asked for, joined by commas. */
	fields: string;
	/** A change has come since: the next look reads them again. */
	stale: boolean;
	/** Which read this is: only the latest may settle. */
	run: number;
	load: Load<LibraryIndex>;
}

export class Library {
	user = $state.raw<Me | null>(null);
	facts = $state.raw<LibraryFacts | null>(null);
	/** Newest first: the feature album, then what's beside it. */
	recent = $state.raw<AlbumEntry[]>([]);
	/**
	 * What the person has played, for Home's rows: null until the first answer. A copy is kept
	 * through every later read, so Home shows it at once and swaps it in place.
	 */
	history = $state.raw<History | null>(null);
	/** Every album loaded so far, by name. */
	albums = $state.raw<AlbumEntry[]>([]);
	/** Whether the server has more albums than {@link Library.albums} holds. */
	hasMoreAlbums = $state(false);
	/** Set once a first load has failed for any reason but the session. */
	down = $state(false);
	/** Whether Home is the deck, following a first scan, in place of the library it's still finding. */
	deck = $state(false);
	/** The covers on the deck, the top one last. */
	pile = $state.raw<AlbumEntry[]>([]);
	/** How far the deck has got into Home: mounted under it and held back, then dealt to. */
	dealing = $state<'no' | 'held' | 'dealt'>('no');
	/**
	 * Set by a deck on screen: plays the end of the scan, calling `open` to mount Home. With nobody
	 * there to play it, the moment has been missed, and Home is simply there.
	 */
	dealer: ((open: () => void) => Promise<void>) | null = null;
	playing = $state.raw<NowPlaying | null>(null);
	/** The artists who own an album, by name. */
	artists = $state.raw<Load<ListedArtist[]>>({ kind: 'loading' });
	/** What scans had trouble with, for administrators. */
	issues = $state.raw<Load<ScanIssues>>({ kind: 'loading' });
	/** Every song as the server last said, songs on their way to Trash included. */
	#index = $state.raw<Load<LibraryIndex>>({ kind: 'loading' });
	/** Songs moved to Trash here, which every list leaves out before the server has answered. */
	readonly trashed = new Trashed();
	readonly #listed = $derived(this.trashed.index(this.#index));

	/** Every song, for the Library page, less what's on its way to Trash. Nothing asks until it opens. */
	get index(): Load<LibraryIndex> {
		return this.#listed;
	}

	set index(load: Load<LibraryIndex>) {
		this.#index = load;
	}
	/**
	 * The signed-in person's Last.fm connection, read with the shell and after every change. Null
	 * until it's known, and when it can't be: nothing waits on it.
	 */
	lastfm = $state.raw<LastfmStatus | null>(null);
	/** Sends the app to the door. Set by the shell once it can navigate. */
	onExpired: (() => Promise<void>) | null = null;
	/** The search page's question and answer, kept while the page is away. */
	readonly search = new Search(async (query, signal) => {
		const found = await this.#source.search(query, signal);
		this.#remember(found.albums);
		this.#rememberNames(found.artists);
		return found;
	});
	readonly simulated: boolean;
	/** The signed-in person's stars: what every page shows, whatever it was given. */
	readonly marks: Marks;
	/** The signed-in person's preferences, kept on the server. */
	readonly prefs: PrefsStore;
	/** What {@link Library.index} was asked for besides the songs, joined by commas. */
	indexFields = $state('');
	/** Moves on whenever the server says its settings changed, so an open Server page reads them. */
	settingsHeard = $state(0);
	/** Moves on whenever the server says albums changed, so an open Trash count reads again. */
	libraryHeard = $state(0);

	readonly #source: LibrarySource;
	readonly #entries = new SvelteMap<string, AlbumEntry>();
	readonly #albums = new SvelteMap<string, Album>();
	/** Albums' songs as song rows show them, read one album at a time, with the fields asked. */
	readonly #rows = new SvelteMap<string, { fields: string; album: LibraryAlbum }>();
	readonly #missing = new SvelteSet<string>();
	/** Albums an edit merged away, and where their tracks went. */
	readonly #moved = new SvelteMap<string, string>();
	readonly #unavailable = new SvelteSet<string>();
	/** Every album fetch still out: asking again waits for the same one. */
	readonly #loading = new SvelteMap<string, Promise<void>>();
	readonly #artists = new SvelteMap<string, Load<ArtistDetail>>();
	readonly #artistNames = new SvelteMap<string, string>();
	/** Each artist's songs read for its page, with the fields they carry and whether they're behind. */
	readonly #artistSongs = new SvelteMap<string, ArtistSongsRead>();
	/** The latest read of each artist's songs: an older answer that comes late is let go. */
	#artistSongsRuns = 0;
	/** The scan the index was last read after, by when it ended. */
	#indexedAfter: string | undefined;
	#indexLoad: Promise<void> | null = null;
	/** Set when something other than a scan has changed what the index holds. */
	#indexEdited = $state(false);
	#player: PlayerView | null = null;
	#session: Promise<Session> | null = null;
	#setup: Promise<SetupStatus | null> | null = null;
	#shell: Promise<void> | null = null;
	/** When {@link Library.history} was asked for: 0 makes the next look ask again. */
	#historyAt = 0;
	#historyLoad: Promise<void> | null = null;
	#cursor: string | undefined;
	#more: Promise<void> | null = null;
	#retry: ReturnType<typeof setTimeout> | undefined;
	#expiring = false;
	#probedAt = -probeMs;
	/** Moves on at every reset: anything older that resolves late has no business here. */
	#epoch = 0;
	readonly #readyMemory: ReadyMemory;
	readonly #outbox: Outbox;
	readonly #listens: ListenOutbox;
	#listensFlush: ReturnType<typeof setTimeout> | undefined;
	#waited = false;
	#scanPoll: ReturnType<typeof setTimeout> | undefined;
	/** Set while a running scan is being followed, so there's only ever one of these. */
	#progressPoll: ReturnType<typeof setTimeout> | undefined;
	/** Running while the spacing after the last card lasts, or a hidden tab waits. */
	#throwing: ReturnType<typeof setTimeout> | undefined;
	/** Waiting to ask again for a list of issues that failed to load. */
	#issuesRetry: ReturnType<typeof setTimeout> | undefined;
	/** Everything the feed has already handed over, so no album lands twice. */
	readonly #seen = new SvelteSet<string>();
	/** Found, and waiting for a decode. */
	#queue: AlbumEntry[] = [];
	#decoding = 0;
	/** Decoded, and waiting for their turn. */
	#ready: AlbumEntry[] = [];
	/** Moves on whenever the following stops: a cover decoded for an older run has missed its turn. */
	#followRun = 0;
	/** Set on a deck joined part way: its first poll fills the pile without replaying it. */
	#seeding = false;
	/** What the server says as it happens, while someone is signed in. Null without a server. */
	readonly #live: Live | null;
	/** Who wants to hear of a preference changed elsewhere. */
	readonly #prefWatchers = new SvelteMap<PrefKey, SvelteSet<(value: never) => void>>();
	/** The index patches, one after another, so none lands on a list it wasn't read against. */
	#patching: Promise<void> = Promise.resolve();

	constructor(
		source: LibrarySource | ((expired: () => void) => LibrarySource),
		options: LibraryOptions = {}
	) {
		this.#source = typeof source === 'function' ? source(() => void this.expire()) : source;
		this.simulated = options.simulated ?? false;
		this.#readyMemory =
			options.readyMemory ?? readyMemoryIn(() => localStorage, 'kitsune:ready-seen');
		this.#outbox = new Outbox(options.outbox ?? (() => localStorage), this.#source);
		this.#listens = new ListenOutbox(options.outbox ?? (() => localStorage), this.#source);
		this.marks = new Marks(this.#source);
		this.prefs = createPrefs(this.#source);
		const events = options.events === undefined ? browserEvents() : options.events;
		this.#live =
			events === null
				? null
				: new Live(events, {
						library: (change) => void this.#heardLibrary(change),
						scan: (scan) => {
							this.#heardScan(scan);
						},
						fingerprints: (pass) => {
							this.#heardPass(pass);
						},
						loudness: (loudness) => {
							if (this.facts !== null && !this.deck) {
								this.facts = { ...this.facts, loudness };
							}
						},
						marks: (change) => {
							this.marks.heard(change);
						},
						prefs: ({ key }) => void this.#heardPref(key as PrefKey),
						lastfm: () => void this.refreshLastfm(),
						'shuffle-rules': () => {
							this.#heardShuffleRules();
						},
						settings: () => {
							this.settingsHeard += 1;
							void this.#readSettled();
						},
						resync: () => void this.#resync(),
						broken: () => {
							this.probe();
						}
					});
	}

	/** Whether the server's events are arriving: then nothing polls for what they say. */
	get live(): boolean {
		return this.#live?.connected === true;
	}

	/**
	 * Calls `heard` with a preference's value whenever it's changed on another device, or after a gap
	 * in the events. A change of this browser's own still waiting to be saved is newer, and wins.
	 */
	watchPref<K extends PrefKey>(key: K, heard: (value: Prefs[K]) => void): void {
		const watchers = this.#prefWatchers.get(key) ?? new SvelteSet();
		watchers.add(heard as (value: never) => void);
		this.#prefWatchers.set(key, watchers);
	}

	/** Whether the signed-in user may run scans and see paths. */
	get admin(): boolean {
		return this.user?.role === 'admin';
	}

	/** Takes the player on: everything the library forgets, it forgets too. */
	follow(player: PlayerView): void {
		this.#player = player;
	}

	/** Returns what's known about an album right now, without waiting. */
	entry(id: string): AlbumEntry | undefined {
		return this.#albums.get(id) ?? this.#entries.get(id);
	}

	/** An album as read, less its songs on their way to Trash. */
	album(id: string): Album | undefined {
		const album = this.#albums.get(id);
		return album === undefined ? undefined : this.trashed.album(album);
	}

	/** Reports whether the server says this album doesn't exist. */
	failed(id: string): boolean {
		return this.#missing.has(id);
	}

	/** Reports whether the album couldn't be loaded this time; {@link Library.loadAlbum} retries. */
	unavailable(id: string): boolean {
		return this.#unavailable.has(id);
	}

	/** Asks who's signed in, once: every navigation must not wait for this. */
	checkSession(): Promise<Session> {
		this.#session ??= this.#askSession();
		return this.#session;
	}

	async #askSession(): Promise<Session> {
		try {
			this.user = await this.#source.me();
			return 'signed-in';
		} catch (error) {
			return error instanceof UnauthorizedError ? 'signed-out' : 'down';
		}
	}

	/** Asks again, now. A 401 goes to the door like any other, and anything else counts as down. */
	askSession(): Promise<Session> {
		return this.#askSession();
	}

	/** The URL a track's audio streams from. */
	streamUrl(trackId: string): string {
		return this.#source.streamUrl(trackId);
	}

	/** Asks whether the session survives: a 401 here goes to the door like any other. */
	probe(): void {
		const now = performance.now();
		if (now - this.#probedAt >= probeMs) {
			this.#probedAt = now;
			this.#source.me().catch(() => undefined);
		}
	}

	/** Reports a listen that counted, in Unix ms from when it started. Fixtures have nobody to tell. */
	reportPlay(trackId: string, startedAt: number): void {
		if (this.user !== null && !this.simulated) {
			this.#outbox.add(this.user.id, trackId, startedAt);
			this.flushPlays();
		}
	}

	/**
	 * Reports a listen's start or end. It goes a moment later, with whatever else came by then: a
	 * track change mustn't spend the connection the next song's sound needs.
	 */
	reportListen(report: ListenReport): void {
		if (this.user === null || this.simulated) {
			return;
		}
		this.#listens.add(this.user.id, report);
		clearTimeout(this.#listensFlush);
		this.#listensFlush = setTimeout(() => {
			this.flushPlays();
		}, listenFlushMs);
	}

	/** Sends what's waiting for whoever is signed in, and only for them. */
	flushPlays(): void {
		if (this.user !== null && !this.simulated) {
			void this.#outbox.flush(this.user.id);
			void this.#listens.flush(this.user.id);
		}
	}

	/** Tells the server a track started, for Last.fm. Fixtures have nobody to tell. */
	nowPlaying(trackId: string): void {
		if (this.user !== null && !this.simulated) {
			this.#source.nowPlaying(trackId).catch(() => undefined);
		}
	}

	/** Reads the Last.fm connection again. A failure leaves what was known. */
	async refreshLastfm(): Promise<void> {
		const epoch = this.#epoch;
		try {
			const status = await this.#source.lastfm();
			if (epoch === this.#epoch) {
				this.lastfm = status;
			}
		} catch {
			// The You page says nothing it doesn't know; the dot stays as it was.
		}
	}

	/** Starts connecting Last.fm, and returns the last.fm page to go to. Rejects with `ServerError`. */
	connectLastfm(): Promise<string> {
		return this.#source.connectLastfm();
	}

	/** Changes the Last.fm switches. Rejects with `ServerError`, and then nothing has changed. */
	async setLastfm(patch: LastfmSwitches): Promise<void> {
		this.lastfm = await this.#source.setLastfm(patch);
	}

	/** Disconnects Last.fm: everything waiting for it goes too. */
	async disconnectLastfm(): Promise<void> {
		await this.#source.disconnectLastfm();
		await this.refreshLastfm();
	}

	/** Counts the signed-in person's plays and rated songs. */
	listening(): Promise<Listening> {
		return this.#source.listening();
	}

	/** The hearts the signed-in person kept from before stars were the only like. */
	heartsSummary(): Promise<HeartsSummary> {
		return this.#source.heartsSummary();
	}

	/**
	 * Turns the person's hearts into stars, once: their unrated hearted songs get `stars`, or stay
	 * unrated for null, and every heart goes. Each song rated arrives as a `marks` event, here and on
	 * the person's other devices, as any rating does; without the stream, the songs are read again.
	 */
	async convertHearts(stars: number | null): Promise<HeartsConverted> {
		const done = await this.#source.convertHearts(stars);
		if (done.rated > 0 && !this.live && this.index.kind === 'ready') {
			void this.loadIndex(this.#fields());
		}
		return done;
	}

	/**
	 * Set by the shell: saves where the player is, so the next sign-in, anywhere, comes back to it.
	 * The library can't import the player, which is built on it.
	 */
	beforeSignOut: (() => Promise<void>) | null = null;

	/**
	 * Ends the session and goes to the door, the way a lost session does. Where the player is gets
	 * saved first, for a second at the most.
	 */
	async signOut(): Promise<void> {
		const saving = this.beforeSignOut?.();
		if (saving !== undefined) {
			await Promise.race([
				saving.catch(() => undefined),
				new Promise((done) => setTimeout(done, signOutSaveMs))
			]);
		}
		await this.#source.logout();
		await this.expire();
	}

	/** Where the signed-in person left off, or null: nothing remembered, or nobody signed in. */
	async playback(): Promise<PlaybackRecord | null> {
		if (this.user === null) {
			return null;
		}
		return this.#source.playback();
	}

	/** Remembers where the signed-in person is. Nobody signed in has nowhere to keep it. */
	async savePlayback(save: PlaybackSave, options: { keepalive: boolean }): Promise<void> {
		if (this.user !== null) {
			await this.#source.savePlayback(save, options);
		}
	}

	/** Exchanges a name and password for a session. Rejects with `LoginError`. */
	async login(name: string, password: string): Promise<void> {
		await this.#source.login(name, password);
		// Logging in names the user; only `/me` says what they may do.
		this.user = await this.#source.me();
		this.#session = Promise.resolve('signed-in');
	}

	/** Asks for a reset code, which lands in the server's log. Rejects with `ResetError`. */
	requestReset(name: string): Promise<void> {
		return this.#source.requestReset(name);
	}

	/** Sets a new password with a reset code and signs in, as {@link Library.login} does. Rejects with `ResetError`. */
	async confirmReset(name: string, code: string, password: string): Promise<void> {
		await this.#source.confirmReset(name, code, password);
		this.user = await this.#source.me();
		this.#session = Promise.resolve('signed-in');
	}

	/** Changes the caller's password: this browser stays signed in. Rejects with `PasswordError`. */
	changePassword(current: string, password: string): Promise<void> {
		return this.#source.changePassword(current, password);
	}

	/** Every account, for an administrator's People card. */
	accounts(): Promise<Account[]> {
		return this.#source.accounts();
	}

	/** Sets someone else's password and signs them out everywhere. Rejects with `PasswordError`. */
	setPassword(userId: number, password: string): Promise<void> {
		return this.#source.setPassword(userId, password);
	}

	/**
	 * Asks whether the first account is still to be made, once: it can only stop being needed, and
	 * this browser says when. Null when the server didn't answer.
	 */
	setupStatus(): Promise<SetupStatus | null> {
		this.#setup ??= this.#source.setupStatus().catch(() => {
			this.#setup = null; // Unknown isn't an answer to keep: ask again next time.
			return null;
		});
		return this.#setup;
	}

	/** Makes the first account and signs in as it, as {@link Library.login} does. Rejects with `SetupError`. */
	async setup(name: string, password: string, code?: string): Promise<void> {
		try {
			await this.#source.setup(name, password, code);
		} catch (error) {
			if (error instanceof SetupError && error.failure.kind === 'done') {
				this.#setup = Promise.resolve({ needed: false, codeRequired: false });
			}
			throw error;
		}
		this.#setup = Promise.resolve({ needed: false, codeRequired: false });
		this.user = await this.#source.me();
		this.#session = Promise.resolve('signed-in');
	}

	/**
	 * Takes the theme the person chose, wherever they chose it: the server's value wins over this
	 * browser's. With none kept, or none to be had, the page stays as it is.
	 */
	async #adoptTheme(epoch: number): Promise<void> {
		const chosen = await this.prefs.get('ui.theme');
		if (chosen !== null && epoch === this.#epoch) {
			theme.choose(chosen);
		}
	}

	/** Loads everything the shell shows. Failures surface through {@link Library.down}. */
	loadShell(): Promise<void> {
		this.#shell ??= this.#loadShell().finally(() => {
			this.#shell = null;
		});
		return this.#shell;
	}

	async #loadShell(): Promise<void> {
		clearTimeout(this.#retry);
		const epoch = this.#epoch;
		try {
			this.user ??= await this.#source.me();
			this.#live?.start();
			this.flushPlays();
			void this.refreshLastfm();
			void this.#adoptTheme(epoch);
			void this.loadHistory();
			const [facts, recent, first, ready] = await Promise.all([
				this.#source.facts(),
				this.#source.albumsPage('added', undefined, recentCount),
				this.#source.albumsPage('name', undefined, albumPageSize),
				// Not answering means unknown, and unknown isn't ready.
				this.#source.ready().catch(() => false)
			]);
			if (epoch !== this.#epoch) {
				return;
			}
			if (ready) {
				this.#readyMemory.mark();
			}
			const opening = openingFor(facts, ready, {
				remembered: this.#readyMemory.seen(),
				waited: this.#waited
			});
			if (opening === 'wait') {
				// Nothing is assigned, so Home can't flash, and nobody waits for this: not even login.
				this.#waited = true;
				this.#retry = setTimeout(() => void this.loadShell(), restartGraceMs);
				return;
			}
			if (opening === 'deck') {
				this.#followScan(facts);
				return;
			}
			const leavingDeck = this.deck;
			this.#stopFollowing();
			if (leavingDeck) {
				// An album opened mid-scan holds the tracks found so far.
				this.#albums.clear();
			}
			this.#remember([...recent.albums, ...first.albums]);
			this.recent = recent.albums;
			this.albums = first.albums;
			this.#cursor = first.nextCursor;
			this.hasMoreAlbums = first.nextCursor !== undefined;
			const open = (): void => {
				this.facts = facts;
				this.down = false;
				this.dealing = leavingDeck ? 'held' : 'no';
			};
			if (leavingDeck && facts.albums > 0 && this.dealer !== null) {
				await this.dealer(open);
			} else {
				open();
			}
			this.leaveDeck();
			this.#watchScan();
			if (facts.albums === 0) {
				this.#retryLater(() => this.#pollForAlbums());
			}
		} catch (error) {
			if (epoch === this.#epoch && !(error instanceof UnauthorizedError)) {
				this.down = true;
				this.#retryLater(() => this.loadShell());
			}
		}
	}

	/**
	 * Reads the person's history again when the copy here is older than {@link historyFreshMs}, or
	 * there is none. Nothing waits on it: Home shows the copy it has. A failure keeps that copy, and
	 * one with nothing to keep shows no history, so the next look asks again.
	 */
	freshenHistory(now = Date.now()): Promise<void> {
		return now - this.#historyAt < historyFreshMs ? Promise.resolve() : this.loadHistory(now);
	}

	/** Reads the person's history, once at a time. */
	loadHistory(now = Date.now()): Promise<void> {
		this.#historyLoad ??= this.#loadHistory(now).finally(() => {
			this.#historyLoad = null;
		});
		return this.#historyLoad;
	}

	async #loadHistory(now: number): Promise<void> {
		const epoch = this.#epoch;
		this.#historyAt = now;
		try {
			const history = await this.#source.history();
			if (epoch === this.#epoch) {
				this.#remember([...history.recent, ...history.onRepeat.map(({ album }) => album)]);
				this.history = history;
			}
		} catch {
			if (epoch === this.#epoch) {
				this.#historyAt = 0;
				this.history ??= { recent: [], onRepeat: [] };
			}
		}
	}

	/** Asks only for the counts while there's nothing to show: a later scan may yet find something. */
	async #pollForAlbums(): Promise<void> {
		const epoch = this.#epoch;
		try {
			const facts = await this.#source.facts();
			if (epoch !== this.#epoch) {
				return;
			}
			if (facts.albums > 0) {
				await this.loadShell();
				return;
			}
			this.facts = facts;
		} catch {
			// Still nothing to show either way, and a lost session has gone to the door.
		}
		if (epoch === this.#epoch) {
			this.#retryLater(() => this.#pollForAlbums());
		}
	}

	/** Reads the counts now. A scan this finds running is followed to its end. */
	async refreshFacts(): Promise<void> {
		const epoch = this.#epoch;
		try {
			const facts = await this.#source.facts();
			if (epoch === this.#epoch && !this.deck) {
				this.facts = facts;
				this.#watchScan();
			}
		} catch {
			// The counts on screen stand, and a lost session has gone to the door.
		}
	}

	/**
	 * Polls only while a scan runs, or, slower, while the fingerprint pass does. The deck follows a
	 * first scan its own way.
	 */
	#watchScan(delayMs = progressPollMs): void {
		if (this.#progressPoll !== undefined || this.deck || this.facts === null || this.live) {
			return;
		}
		const { scan, fingerprints } = this.facts;
		if (scan.state === 'running') {
			this.#progressPoll = setTimeout(() => void this.#pollProgress(), delayMs);
		} else if (fingerprints !== undefined && fingerprints.done < fingerprints.total) {
			this.#progressPoll = setTimeout(
				() => void this.#pollProgress(),
				Math.max(delayMs, passPollMs)
			);
		}
	}

	/** The folders the server sees in a path, for the folder picker. */
	folders(path: string): Promise<FolderListing> {
		return this.#source.folders(path);
	}

	/** The server's settings, for an administrator's Server page. */
	serverSettings(): Promise<ServerSettings> {
		return this.#source.settings();
	}

	/**
	 * Changes settings, all or none. The server applies most of them a moment later, so what they
	 * change is read again then: a scan to follow, Last.fm on or off, tag editing.
	 */
	async changeSettings(change: SettingsChange): Promise<ServerSettings> {
		const saved = await this.#source.changeSettings(change);
		for (const delayMs of settledReadsMs) {
			setTimeout(() => void this.#readSettled(), delayMs);
		}
		return saved;
	}

	async #readSettled(): Promise<void> {
		const epoch = this.#epoch;
		await Promise.all([this.refreshFacts(), this.refreshLastfm()]);
		try {
			const me = await this.#source.me();
			if (epoch === this.#epoch && this.user !== null) {
				this.user = me;
			}
		} catch {
			// The page stands as it is; a lost session has gone to the door.
		}
	}

	async #pollProgress(): Promise<void> {
		const epoch = this.#epoch;
		let retryIn = progressPollMs;
		try {
			let facts = await this.#source.facts();
			if (facts.scan.state === 'idle') {
				// The server reads the counts before the status, so these can be from just before the end.
				facts = await this.#source.facts();
			}
			if (epoch !== this.#epoch) {
				return;
			}
			this.facts = facts;
			this.down = false;
		} catch (error) {
			if (epoch !== this.#epoch || error instanceof UnauthorizedError) {
				return;
			}
			this.down = true;
			retryIn = retryMs;
		}
		this.#progressPoll = undefined;
		this.#watchScan(retryIn);
	}

	/** Starts a scan and follows it. Reports why it didn't start, or null if there's one to watch. */
	async startScan(mode: ScanMode): Promise<ScanRefusal | null> {
		const epoch = this.#epoch;
		try {
			const scan = await this.#source.startScan(mode);
			if (epoch === this.#epoch && this.facts !== null) {
				this.facts = { ...this.facts, scan };
				this.#watchScan();
			}
			if (scan.state === 'idle') {
				// It was over before the answer left: there's nothing to follow, only counts to read.
				await this.refreshFacts();
			}
			return null;
		} catch (error) {
			if (error instanceof UnauthorizedError) {
				return null;
			}
			if (!(error instanceof ServerError)) {
				return 'unavailable';
			}
			if (error.status === 409) {
				await this.refreshFacts();
				return null;
			}
			if (error.status === 403) {
				return 'forbidden';
			}
			return error.status === 503 ? 'stopping' : 'unavailable';
		}
	}

	/**
	 * Asks what scans had trouble with. A list already here stays through a refresh. One that
	 * fails to load is asked for once more, a moment later.
	 */
	loadIssues(): Promise<void> {
		return this.#loadIssues(true);
	}

	async #loadIssues(retry: boolean): Promise<void> {
		clearTimeout(this.#issuesRetry);
		const epoch = this.#epoch;
		if (this.issues.kind !== 'ready') {
			this.issues = { kind: 'loading' };
		}
		try {
			const issues = await this.#source.issues();
			if (epoch === this.#epoch) {
				this.issues = { kind: 'ready', value: issues };
			}
		} catch (error) {
			const lost = this.issues.kind !== 'ready' && !(error instanceof UnauthorizedError);
			if (epoch === this.#epoch && lost) {
				this.issues = { kind: 'unavailable' };
				if (retry) {
					this.#issuesRetry = setTimeout(() => void this.#loadIssues(false), issuesRetryMs);
				}
			}
		}
	}

	/** Turns Home into the deck. Joining a scan part way, nothing already found lands again. */
	#followScan(facts: LibraryFacts): void {
		if (!this.deck) {
			this.#seeding = facts.albums > 0;
		}
		this.facts = facts;
		this.down = false;
		this.deck = true;
		clearTimeout(this.#scanPoll);
		void this.#pollScan();
	}

	#stopFollowing(): void {
		clearTimeout(this.#scanPoll);
		clearTimeout(this.#throwing);
		this.#throwing = undefined;
		this.#followRun += 1;
		this.#seen.clear();
		this.#queue = [];
		this.#ready = [];
		this.#decoding = 0;
		this.#seeding = false;
	}

	/** Asks what the scan has found, until it says it's done. */
	async #pollScan(): Promise<void> {
		const epoch = this.#epoch;
		try {
			const [facts, page, ready] = await Promise.all([
				this.#source.facts(),
				this.#source.albumsPage('added', undefined, feedSize),
				this.#source.ready().catch(() => false)
			]);
			if (epoch !== this.#epoch) {
				return;
			}
			this.down = false;
			this.facts = facts;
			this.#remember(page.albums);
			this.#feed(page.albums);
			if (ready) {
				// Always a fresh read: the counts can still say 0 right beside ready.
				await this.loadShell();
				return;
			}
		} catch (error) {
			if (epoch !== this.#epoch || error instanceof UnauthorizedError) {
				return;
			}
			this.down = true;
		}
		this.#scanPoll = setTimeout(() => void this.#pollScan(), scanPollMs);
	}

	/** Takes in one look at the newest end of the library. */
	#feed(newestFirst: AlbumEntry[]): void {
		if (this.#seeding) {
			this.#seeding = false;
			this.pile = newestFirst.slice(0, pileSize).toReversed();
			for (const { id } of newestFirst) {
				this.#seen.add(id);
			}
			void this.#checkCovers();
			return;
		}
		const fresh = newestFirst.filter(({ id }) => !this.#seen.has(id));
		for (const { id } of fresh) {
			this.#seen.add(id);
		}
		this.#queue = [...this.#queue, ...fresh.toReversed()].slice(-mostQueued);
		this.#decodeAhead();
	}

	/** Gives a pile that went straight on, with no landing, the generated covers it turns out to need. */
	async #checkCovers(): Promise<void> {
		const cards = await this.cardsFor(this.pile);
		this.pile = this.pile.map((entry) => cards.find(({ id }) => id === entry.id) ?? entry);
	}

	/** Takes the deck away, leaving Home. */
	leaveDeck(): void {
		this.deck = false;
		this.dealing = 'no';
		this.pile = [];
	}

	/** Gets these cards ready to land, together: what a riffle waits for is the slowest one. */
	cardsFor(entries: readonly AlbumEntry[]): Promise<AlbumEntry[]> {
		return Promise.all(entries.map(cardFor));
	}

	/** Lands a card now, in the riffle before a deal. Whatever's in `keep` stays on the pile. */
	riffle(card: AlbumEntry, keep: ReadonlySet<string>): void {
		const oldest = this.pile.find(({ id }) => !keep.has(id));
		const room = this.pile.length < pileSize || oldest === undefined;
		this.pile = [...this.pile.filter((entry) => room || entry !== oldest), card];
	}

	/** Keeps covers decoding ahead of the throw, a few at once, each going down as soon as it's done. */
	#decodeAhead(): void {
		const run = this.#followRun;
		while (this.#decoding < decodesAhead && this.#ready.length + this.#decoding < readyAhead) {
			const next = this.#queue.shift();
			if (next === undefined) {
				return;
			}
			this.#decoding += 1;
			void cardFor(next).then((card) => {
				if (run === this.#followRun) {
					this.#decoding -= 1;
					this.#ready.push(card);
					this.#throwNext();
					this.#decodeAhead();
				}
			});
		}
	}

	/** Throws the next decoded card down, unless the last one's spacing is still running. */
	#throwNext(): void {
		if (this.#throwing !== undefined) {
			return;
		}
		const carryOn = (): void => {
			this.#throwing = undefined;
			this.#throwNext();
		};
		if (typeof document !== 'undefined' && document.hidden) {
			this.#throwing = setTimeout(carryOn, hiddenRetryMs);
			return;
		}
		const card = this.#ready.shift();
		if (card !== undefined) {
			this.pile = [...this.pile, card].slice(-pileSize);
			const waiting = this.#ready.length + this.#decoding + this.#queue.length;
			this.#throwing = setTimeout(carryOn, throwGap(waiting, card.id));
			this.#decodeAhead();
		}
	}

	#retryLater(work: () => Promise<void>): void {
		clearTimeout(this.#retry);
		this.#retry = setTimeout(() => void work(), retryMs);
	}

	#remember(entries: AlbumEntry[]): void {
		for (const entry of entries) {
			this.#entries.set(entry.id, entry);
		}
	}

	/**
	 * Shows what the player is on, and nothing when it's on nothing: nothing is cued before something
	 * plays or a remembered record comes back.
	 */
	showQueue(): void {
		this.playing = this.#player?.cue ?? null;
	}

	/** Loads the next page of {@link Library.albums}. Only one page is ever in flight. */
	loadMoreAlbums(limit = albumPageSize): Promise<void> {
		this.#more ??= this.#loadMoreAlbums(limit).finally(() => {
			this.#more = null;
		});
		return this.#more;
	}

	/** Loads every album still to come. A page that fails stops it: asking again carries on. */
	async loadAllAlbums(): Promise<void> {
		let cursor: string | undefined;
		while (this.hasMoreAlbums && this.#cursor !== cursor) {
			cursor = this.#cursor;
			await this.loadMoreAlbums(wholeListPageSize);
		}
	}

	async #loadMoreAlbums(limit: number): Promise<void> {
		if (this.#cursor === undefined) {
			return;
		}
		try {
			const page = await this.#source.albumsPage('name', this.#cursor, limit);
			this.#remember(page.albums);
			this.albums = [...this.albums, ...page.albums];
			this.#cursor = page.nextCursor;
			this.hasMoreAlbums = page.nextCursor !== undefined;
		} catch {
			// The next scroll asks again; a lost session has already gone to the door.
		}
	}

	/** Returns how far an artist has got. Nothing asked for yet is on its way. */
	artist(id: string): Load<ArtistDetail> {
		return this.#artists.get(id) ?? { kind: 'loading' };
	}

	/** Returns the name of an artist anything has linked to, without waiting. */
	artistName(id: string): string | undefined {
		return this.#artistNames.get(id);
	}

	#rememberNames(artists: readonly ArtistLink[]): void {
		for (const { id, name } of artists) {
			this.#artistNames.set(id, name);
		}
	}

	/** Asks for the artists. A list already here stays through a refresh, even a failed one. */
	async loadArtists(): Promise<void> {
		const epoch = this.#epoch;
		if (this.artists.kind !== 'ready') {
			this.artists = { kind: 'loading' };
		}
		try {
			const artists = await this.#source.artists();
			if (epoch === this.#epoch) {
				this.#rememberNames(artists);
				this.artists = { kind: 'ready', value: artists };
			}
		} catch (error) {
			const lost = this.artists.kind !== 'ready' && !(error instanceof UnauthorizedError);
			if (epoch === this.#epoch && lost) {
				this.artists = { kind: 'unavailable' };
			}
		}
	}

	/**
	 * Reports whether a scan or an edit has finished since the index was read. While the server's
	 * events arrive, they keep it up to date instead, and only an edit they missed counts.
	 */
	get indexStale(): boolean {
		if (this.live && this.index.kind === 'ready') {
			return this.#indexEdited;
		}
		return this.#indexEdited || this.facts?.scan.last?.endedAt !== this.#indexedAfter;
	}

	/** Says an edit has changed what the index holds: the next look reads it again. */
	markIndexStale(): void {
		this.#indexEdited = true;
		this.#artistSongsChanged();
	}

	/**
	 * Reads the whole library, once at a time, with what `fields` asks for besides. A list already
	 * here stays through a refresh, and through a change of fields.
	 */
	loadIndex(fields: readonly IndexField[] = []): Promise<void> {
		this.#indexLoad ??= this.#loadIndex(fields).finally(() => {
			this.#indexLoad = null;
		});
		return this.#indexLoad;
	}

	async #loadIndex(fields: readonly IndexField[]): Promise<void> {
		const epoch = this.#epoch;
		if (this.index.kind !== 'ready') {
			this.index = { kind: 'loading' };
		}
		this.#indexedAfter = this.facts?.scan.last?.endedAt;
		this.#indexEdited = false;
		this.indexFields = fields.join(',');
		try {
			const index = await this.#source.libraryIndex(fields);
			if (epoch === this.#epoch) {
				this.#rememberIndex(index);
				this.marks.seedIndex(index);
				this.index = { kind: 'ready', value: index };
			}
		} catch (error) {
			const lost = this.index.kind !== 'ready' && !(error instanceof UnauthorizedError);
			if (epoch === this.#epoch && lost) {
				this.index = { kind: 'unavailable' };
			}
		}
	}

	/**
	 * Whether the index held, or on its way, carries every one of `fields`. One that carries more is
	 * as good: a page that needs less never reads the whole library again to drop some.
	 */
	holdsFields(fields: readonly IndexField[]): boolean {
		const held = this.#fields();
		return fields.every((field) => held.includes(field));
	}

	/** What {@link Library.index} was asked for, as fields. */
	#fields(): IndexField[] {
		return this.indexFields === '' ? [] : (this.indexFields.split(',') as IndexField[]);
	}

	/**
	 * Takes in albums that changed: Home's newest and counts are read again, an album already read
	 * is read again, and the index is patched in place, keeping its place on the screen.
	 */
	async #heardLibrary({ albums, all }: LibraryEvent): Promise<void> {
		if (this.deck) {
			return; // A first scan's deck follows the library its own way.
		}
		this.libraryHeard += 1;
		void this.#refreshHome();
		// An album in the history may have changed: the next look at Home reads it again.
		this.#historyAt = 0;
		// An artist's songs can gain an album nobody has read yet, so any change reads them again.
		this.#artistSongsChanged();
		const reads: Promise<void>[] = [];
		const held = all ? [...this.#albums.keys()] : albums.filter((id) => this.#albums.has(id));
		for (const id of held) {
			reads.push(this.#refreshAlbum(id));
		}
		for (const id of all ? [...this.#rows.keys()] : albums) {
			const read = this.#rows.get(id);
			if (read !== undefined) {
				reads.push(
					this.loadAlbumRows(
						[id],
						read.fields === '' ? [] : (read.fields.split(',') as IndexField[])
					)
				);
			}
		}
		if (this.index.kind === 'ready') {
			if (all) {
				reads.push(this.loadIndex(this.#fields()));
			} else {
				this.#patching = this.#patching.then(() => this.#patchIndex(albums));
				reads.push(this.#patching);
			}
		}
		// What every list holds now is the server's word on these albums: Trash's own can go.
		await Promise.allSettled(reads);
		this.trashed.settled(albums, all);
	}

	/** Reads just the albums that changed, and puts them in the index where they go. */
	async #patchIndex(ids: readonly string[]): Promise<void> {
		await this.#indexLoad; // A whole read under way lands first, and the patch on top of it.
		const epoch = this.#epoch;
		const fields = this.indexFields;
		if (this.index.kind !== 'ready') {
			return;
		}
		try {
			const part = await this.#source.libraryIndexOf(ids, this.#fields());
			if (epoch !== this.#epoch || this.index.kind !== 'ready' || fields !== this.indexFields) {
				return; // A new read is coming, with what's wanted now.
			}
			if (this.#index.kind !== 'ready') {
				return;
			}
			const patched = patchIndex(this.#index.value, ids, part);
			if (patched === null) {
				await this.loadIndex(this.#fields());
				return;
			}
			for (const id of ids) {
				this.#entries.delete(id);
			}
			this.#rememberIndex(part);
			this.marks.seedIndex(part);
			this.index = { kind: 'ready', value: patched };
			this.#indexEdited = false;
			this.#indexedAfter = this.facts?.scan.last?.endedAt;
		} catch {
			// The index stands as it was; the next change, or a resync, reads it again.
		}
	}

	/** Reads Home's newest albums and the counts again. */
	async #refreshHome(): Promise<void> {
		const epoch = this.#epoch;
		try {
			const [facts, recent] = await Promise.all([
				this.#source.facts(),
				this.#source.albumsPage('added', undefined, recentCount)
			]);
			if (epoch === this.#epoch && !this.deck && this.facts !== null) {
				this.#remember(recent.albums);
				this.recent = recent.albums;
				this.facts = facts;
			}
		} catch {
			// Home stands as it is; a lost session has gone to the door.
		}
	}

	/** Reads an album that changed, and shows it everywhere. One that's gone is gone. */
	async #refreshAlbum(id: string): Promise<void> {
		const epoch = this.#epoch;
		try {
			const album = await this.#source.album(id);
			if (epoch === this.#epoch) {
				this.#adopt(album);
			}
		} catch (error) {
			if (epoch === this.#epoch && error instanceof AlbumNotFoundError) {
				this.#albums.delete(id);
				this.#missing.add(id);
			}
		}
	}

	/** Takes a scan's progress in, as polling would have: its end reads the counts again. */
	#heardScan(scan: ScanStatus): void {
		if (this.facts === null || this.deck) {
			return;
		}
		const ended = this.facts.scan.state === 'running' && scan.state === 'idle';
		this.facts = { ...this.facts, scan };
		if (ended) {
			void this.refreshFacts();
		}
	}

	/**
	 * The person's shuffle rules changed, here or on another device. The event says only that, so an
	 * index that shows the shuffle marks (`fields=shuffle`) is read again when the Library page looks.
	 */
	#heardShuffleRules(): void {
		if (this.index.kind === 'ready' && this.#fields().includes('shuffle')) {
			this.markIndexStale();
		}
	}

	#heardPass(fingerprints: FingerprintStatus): void {
		if (this.facts !== null && !this.deck) {
			this.facts = { ...this.facts, fingerprints };
		}
	}

	/** Reads a preference changed elsewhere, unless this browser has a newer one waiting to save. */
	async #heardPref(key: PrefKey): Promise<void> {
		if (this.prefs.pending(key)) {
			return;
		}
		const epoch = this.#epoch;
		if (key === 'ui.theme') {
			await this.#adoptTheme(epoch);
			return;
		}
		const watchers = this.#prefWatchers.get(key);
		if (watchers === undefined) {
			return;
		}
		const value = await this.prefs.get(key);
		if (epoch === this.#epoch && value !== null && !this.prefs.pending(key)) {
			for (const heard of watchers) {
				heard(value as never);
			}
		}
	}

	/** The events had a gap nothing can fill: everything they'd have said is read again. */
	async #resync(): Promise<void> {
		if (this.user === null) {
			return;
		}
		void this.#refreshHome();
		void this.refreshLastfm();
		for (const key of ['ui.theme', ...this.#prefWatchers.keys()] as PrefKey[]) {
			void this.#heardPref(key);
		}
		for (const id of this.#albums.keys()) {
			void this.#refreshAlbum(id);
		}
		this.settingsHeard += 1;
		this.#artistSongsChanged();
		if (this.index.kind === 'ready') {
			await this.loadIndex(this.#fields());
		}
	}

	/**
	 * Marks every artist's songs behind: an artist page on screen reads its own again at once, as
	 * {@link Library.holdsArtistSongs} says, and any other the next time it's opened.
	 */
	#artistSongsChanged(): void {
		for (const [id, read] of this.#artistSongs) {
			this.#artistSongs.set(id, { ...read, stale: true });
		}
	}

	/** Returns how far an artist's songs have got. Nothing asked for yet is on its way. */
	artistSongs(id: string): Load<LibraryIndex> {
		return this.trashed.index(this.#artistSongs.get(id)?.load ?? { kind: 'loading' });
	}

	/**
	 * Whether an artist's songs are here, or on their way, with every one of `fields` and not behind
	 * a change: a page that needs less never reads them again to drop some.
	 */
	holdsArtistSongs(id: string, fields: readonly IndexField[]): boolean {
		const read = this.#artistSongs.get(id);
		if (read === undefined || read.stale || read.load.kind === 'unavailable') {
			return false;
		}
		const held = read.fields.split(',');
		return fields.every((field) => held.includes(field));
	}

	/**
	 * Reads the songs an artist is on, with what `fields` asks for besides. What's here stays through
	 * a refresh, a failed one included, unless the server says the artist has gone.
	 */
	async loadArtistSongs(id: string, fields: readonly IndexField[]): Promise<void> {
		const epoch = this.#epoch;
		this.#artistSongsRuns += 1;
		const run = this.#artistSongsRuns;
		const asked = fields.join(',');
		const held = this.#artistSongs.get(id);
		this.#artistSongs.set(id, {
			fields: asked,
			stale: false,
			run,
			load: held?.load.kind === 'ready' ? held.load : { kind: 'loading' }
		});
		/** Whether this is still the latest read, and if a change has come since it began. */
		const latest = (): { stale: boolean } | null => {
			const read = this.#artistSongs.get(id);
			return epoch === this.#epoch && read?.run === run ? { stale: read.stale } : null;
		};
		const settle = (load: Load<LibraryIndex>, still: { stale: boolean }): void => {
			this.#artistSongs.set(id, { fields: asked, stale: still.stale, run, load });
		};
		try {
			const index = await this.#source.artistSongs(id, fields);
			const still = latest();
			if (still !== null) {
				this.#rememberIndex(index);
				this.marks.seedIndex(index);
				settle({ kind: 'ready', value: index }, still);
			}
		} catch (error) {
			const still = latest();
			if (still === null || error instanceof UnauthorizedError) {
				return;
			}
			if (error instanceof ArtistNotFoundError) {
				settle({ kind: 'missing' }, still);
			} else if (this.artistSongs(id).kind !== 'ready') {
				settle({ kind: 'unavailable' }, still);
			}
		}
	}

	/** Seeds the next page's first frame, without replacing an entry that knows more. */
	#rememberIndex({ artists }: LibraryIndex): void {
		for (const { id, name, albums } of artists) {
			if (id !== undefined) {
				this.#artistNames.set(id, name);
			}
			for (const { tracks, ...album } of albums) {
				if (!this.#entries.has(album.id)) {
					this.#entries.set(album.id, { ...album, trackCount: tracks.length });
				}
			}
		}
	}

	/** Asks for an artist. One already here stays through a refresh, unless the server disowns it. */
	async loadArtist(id: string): Promise<void> {
		const epoch = this.#epoch;
		if (this.artist(id).kind !== 'ready') {
			this.#artists.set(id, { kind: 'loading' });
		}
		try {
			const artist = await this.#source.artist(id);
			if (epoch === this.#epoch) {
				this.#remember([...artist.albums, ...artist.appearsOn]);
				this.#rememberNames([artist]);
				this.#artists.set(id, { kind: 'ready', value: artist });
			}
		} catch (error) {
			if (epoch !== this.#epoch || error instanceof UnauthorizedError) {
				return;
			}
			if (error instanceof ArtistNotFoundError) {
				this.#artists.set(id, { kind: 'missing' });
			} else if (this.artist(id).kind !== 'ready') {
				this.#artists.set(id, { kind: 'unavailable' });
			}
		}
	}

	/**
	 * An album's songs as song rows show them, with what the columns ask for: read for it alone, or
	 * the index's own entry when that already carries the fields.
	 */
	albumRows(id: string, fields: readonly IndexField[]): LibraryAlbum | undefined {
		const read = this.#rows.get(id);
		if (read !== undefined && read.fields === fields.join(',')) {
			return this.trashed.rows(read.album);
		}
		if (this.#index.kind === 'ready' && fields.every((field) => this.#fields().includes(field))) {
			for (const artist of this.#index.value.artists) {
				const found = artist.albums.find((album) => album.id === id);
				if (found !== undefined) {
					return this.trashed.rows(found);
				}
			}
		}
		return read === undefined ? undefined : this.trashed.rows(read.album);
	}

	/** Whether the index holds an album's songs with `fields` already, so nothing need be read. */
	indexHolds(id: string, fields: readonly IndexField[]): boolean {
		const read = this.#rows.get(id);
		return (
			(read !== undefined && read.fields === fields.join(',')) ||
			(this.index.kind === 'ready' &&
				fields.every((field) => this.#fields().includes(field)) &&
				this.index.value.artists.some((artist) => artist.albums.some((album) => album.id === id)))
		);
	}

	/** Reads albums' songs with `fields`, for {@link Library.albumRows}. A failure leaves what was. */
	async loadAlbumRows(ids: readonly string[], fields: readonly IndexField[]): Promise<void> {
		const epoch = this.#epoch;
		if (ids.length === 0) {
			return;
		}
		try {
			const part = await this.#source.libraryIndexOf(ids, fields);
			if (epoch !== this.#epoch) {
				return;
			}
			this.marks.seedIndex(part);
			for (const album of part.artists.flatMap((artist) => artist.albums)) {
				this.#rows.set(album.id, { fields: fields.join(','), album });
			}
		} catch {
			// The rows stand as they were, or as the album itself says them.
		}
	}

	/** Streams an album's tracks in. Failures surface through {@link Library.failed}. */
	loadAlbum(id: string): Promise<void> {
		if (this.#albums.has(id)) {
			return Promise.resolve();
		}
		const loading =
			this.#loading.get(id) ??
			this.#fetchAlbum(id).finally(() => {
				this.#loading.delete(id);
			});
		this.#loading.set(id, loading);
		return loading;
	}

	async #fetchAlbum(id: string): Promise<void> {
		this.#unavailable.delete(id);
		try {
			const album = await this.#source.album(id);
			this.marks.seedAlbum(album);
			this.#albums.set(id, album);
			this.#rememberNames(album.artists);
			this.#missing.delete(id);
		} catch (error) {
			if (error instanceof AlbumNotFoundError) {
				this.#missing.add(id);
			} else if (!(error instanceof UnauthorizedError)) {
				this.#unavailable.add(id);
			}
		}
	}

	/** Reads every tag of an album from its files. Rejects with `EditError`. */
	albumTags(id: string): Promise<AlbumTags> {
		return this.#source.albumTags(id);
	}

	/** Saves an edit of an album's tracks, and shows it. Rejects with `EditError`. */
	async editAlbum(id: string, etag: string, edit: AlbumEdit): Promise<void> {
		try {
			const album = await this.#source.editAlbum(id, etag, edit);
			// An edit that moved every track answers with the album they went to: this one is gone.
			if (album.id !== id) {
				this.#albums.delete(id);
				this.#missing.add(id);
				this.#moved.set(id, album.id);
			}
			this.#adopt(album);
		} finally {
			// A save that stops partway has still written files, so the list reads them again either way,
			// unless the server's events are arriving to say what changed.
			if (!this.live) {
				this.markIndexStale();
			}
		}
	}

	/**
	 * Saves an edit of songs on any albums, and shows it: every album they were on or went to is read
	 * again. Rejects with `EditError`.
	 */
	async editTracks(edit: TracksEdit): Promise<EditedTracks> {
		try {
			const edited = await this.#source.editTracks(edit);
			const albums = once([
				...edit.albums.map(({ id }) => id),
				...edited.tracks.map(({ albumId }) => albumId)
			]);
			await Promise.all(albums.map((id) => this.#refetch(id)));
			return edited;
		} finally {
			// The library event the edit sends patches the list, while the server's events arrive.
			if (!this.live) {
				this.markIndexStale();
			}
		}
	}

	/** Moves songs to Trash on the server. Rejects with `TrashError`. */
	trash(move: TrashMove): Promise<TrashChange> {
		return this.#source.trash(move);
	}

	/** What's in Trash, for administrators. */
	trashList(): Promise<Trash> {
		return this.#source.trashList();
	}

	/** Puts songs back from Trash, all or none. Rejects with `TrashError`. */
	restoreTrash(trackIds: readonly string[]): Promise<TrashChange> {
		return this.#source.restoreTrash(trackIds);
	}

	/** Deletes songs in Trash for good. Rejects with `TrashError`. */
	emptyTrash(target: TrashEmpty): Promise<number> {
		return this.#source.emptyTrash(target);
	}

	/** Starts a shuffle on the server, with its first page. Rejects with `ServerError`. */
	startShuffle(start: ShuffleStart): Promise<ShufflePage> {
		return this.#source.startShuffle(start);
	}

	/** A shuffle's page after a position. Rejects with `ServerError`. */
	nextShuffle(id: string, after: number): Promise<ShufflePage> {
		return this.#source.nextShuffle(id, after);
	}

	/** The caller's shuffle rules touching these songs. Rejects with `ServerError`. */
	shuffleRules(trackIds: readonly string[]): Promise<ShuffleRules> {
		return this.#source.shuffleRules(trackIds);
	}

	/**
	 * Makes the caller's shuffle rules what `changes` say, in order, and has the list show them.
	 * Rejects with `ServerError` at the first that fails: those before it hold.
	 */
	async changeShuffleRules(changes: readonly RuleChange[]): Promise<void> {
		try {
			for (const change of changes) {
				switch (change.kind) {
					case 'run':
						await this.#source.setRun(change.tracks);
						break;
					case 'unrun':
						await this.#source.deleteRun(change.id);
						break;
					case 'exclude':
						await this.#source.setExcluded(change.tracks, change.on);
						break;
				}
			}
		} finally {
			// While the server's events arrive, its shuffle-rules event marks it, once, on every device.
			if (changes.length > 0 && !this.live) {
				this.markIndexStale();
			}
		}
	}

	/** Where an album went, when an edit moved every one of its tracks to another and it's gone. */
	movedTo(id: string): string | undefined {
		return this.#moved.get(id);
	}

	/** The album a song is on now. Rejects with `EditError`. */
	trackAlbum(trackId: string): Promise<string> {
		return this.#source.trackAlbum(trackId);
	}

	/** Reads an album that may be gone, for an edit that may have merged it away. */
	async #refetch(id: string): Promise<void> {
		try {
			this.#adopt(await this.#source.album(id));
		} catch (error) {
			if (error instanceof AlbumNotFoundError) {
				this.#albums.delete(id);
				this.#missing.add(id);
			}
		}
	}

	/** Reads an album again, for when what's held has turned out to be behind the server. */
	async reread(id: string): Promise<void> {
		this.#adopt(await this.#source.album(id));
	}

	/** Puts an album everywhere it's shown, so nothing waits for a reload to catch up. */
	#adopt(album: Album): void {
		this.marks.seedAlbum(album);
		this.#albums.set(album.id, album);
		const { year, displayGenres } = album;
		const retold = (entry: AlbumEntry): AlbumEntry =>
			entry.id === album.id ? { ...entry, year, displayGenres } : entry;
		const entry = this.#entries.get(album.id);
		if (entry !== undefined) {
			this.#entries.set(album.id, retold(entry));
		}
		this.recent = this.recent.map(retold);
		this.albums = this.albums.map(retold);
		this.#player?.retag(album);
		const { playing } = this;
		if (playing?.album.id === album.id) {
			// The queue keeps its own order: the names move, the place doesn't.
			this.playing = retagged(playing, album);
		}
	}

	/** Reacts to a lost session. Parallel calls are one event: the app leaves once. */
	async expire(): Promise<void> {
		if (this.#expiring || this.onExpired === null) {
			return;
		}
		this.#expiring = true;
		try {
			await this.onExpired();
			this.reset();
		} finally {
			this.#expiring = false;
		}
	}

	/** Forgets everything the last session saw. */
	reset(): void {
		clearTimeout(this.#retry);
		clearTimeout(this.#issuesRetry);
		clearTimeout(this.#progressPoll);
		this.#progressPoll = undefined;
		this.#stopFollowing();
		this.trashed.clear();
		this.#epoch += 1;
		this.#waited = false;
		this.user = null;
		this.facts = null;
		this.lastfm = null;
		this.recent = [];
		this.history = null;
		this.#historyAt = 0;
		this.albums = [];
		this.hasMoreAlbums = false;
		this.down = false;
		this.leaveDeck();
		this.#player?.reset();
		this.playing = null;
		this.#cursor = undefined;
		this.#session = Promise.resolve('signed-out');
		this.#entries.clear();
		this.#albums.clear();
		this.#rows.clear();
		this.#missing.clear();
		this.#unavailable.clear();
		this.artists = { kind: 'loading' };
		this.issues = { kind: 'loading' };
		this.index = { kind: 'loading' };
		this.indexFields = '';
		this.prefs.reset();
		this.#live?.stop();
		theme.forget();
		this.#indexedAfter = undefined;
		this.#indexEdited = false;
		this.#artists.clear();
		this.#artistNames.clear();
		this.#artistSongs.clear();
		this.marks.reset();
		this.search.reset();
	}
}

function fromQuery(): Library {
	// Tests and the build's route analysis import this with no window: they get the API source.
	const query = new URLSearchParams(typeof location === 'undefined' ? '' : location.search);
	if (query.get('source') !== 'fixtures') {
		return new Library((expired) => createApiSource({ onUnauthorized: expired }));
	}
	const latencyMs = Number(query.get('latency') ?? 0);
	const health = healthFixture(query.get('health'));
	const role = query.get('role') === 'user' ? 'user' : 'admin';
	const editFault = (query.get('editFault') ?? undefined) as FixtureEditFault | undefined;
	const lastfm = lastfmFixture(query.get('lastfm'));
	const setup = setupFixture(query.get('setup'));
	const playback = playbackFixture(query.get('playback'));
	const history = query.get('history') === 'none' ? 'none' : undefined;
	const tagWrites = query.get('tagWrites') === 'off' ? 'off' : undefined;
	const trash = query.get('trash') === 'board' ? 'board' : undefined;
	const scan = Number(query.get('scan') ?? 0);
	if (scan <= 0) {
		// A fake library has no business vouching for the real one on this origin.
		const readyMemory = { seen: () => false, mark: () => undefined };
		return new Library(
			createFixtureSource({
				latencyMs,
				health,
				role,
				editFault,
				lastfm,
				setup,
				playback,
				history,
				tagWrites,
				trash
			}),
			{
				readyMemory,
				simulated: true,
				events: null
			}
		);
	}
	const gap = Number(query.get('gap') ?? defaultScanGap);
	const key = `kitsune:scan:${String(scan)}:${String(gap)}`;
	const source = createScanFixtureSource({
		scanMs: scan * 1000,
		gapMs: gap * 1000,
		startedAt: scanStart(key, Date.now()),
		latencyMs
	});
	return new Library(source, {
		readyMemory: readyMemoryIn(() => sessionStorage, `${key}:ready`),
		simulated: true,
		events: null
	});
}

/** The remembered record a fixtures page starts with: `?playback=phone` or `?playback=gone`. */
function playbackFixture(value: string | null): PlaybackFixture | undefined {
	return value === 'phone' || value === 'gone' ? value : undefined;
}

/** Chosen once, from the URL the app opened on, and held here for every later navigation. */
export const library = fromQuery();
