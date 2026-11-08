import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { preload } from '#lib/cover/rungs.ts';
import { decodesAhead, mostQueued, openingFor, throwGap } from '#lib/pages/deck.ts';
import { theme } from '#lib/theme/theme.svelte.ts';
import { createApiSource } from './api.ts';
import { cueOf, retagged, type NowPlaying } from './cue.ts';
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
import { Marks } from './marks.svelte.ts';
import { Outbox } from './outbox.ts';
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
	type ArtistEntry,
	type ArtistLink,
	type IndexField,
	type LastfmStatus,
	type LastfmSwitches,
	type LibraryFacts,
	type LibraryIndex,
	type LibrarySource,
	type Listening,
	type Me,
	type ScanIssues,
	type ScanMode,
	type ScanRefusal,
	type SetupStatus
} from './source.ts';

/** The grid's page: 60 divides evenly into every column count, so pages stack with no seam. */
export const albumPageSize = 60;
/** How much of the library one request brings, when all of it is wanted. */
const wholeListPageSize = 200;
/** The feature tile and the five beside it. */
const recentCount = 6;
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
	/** Holds the cue on this album, for the lab: a hero condensing into the island needs another. */
	pinnedAlbumId?: string;
	readyMemory?: ReadyMemory;
	/** Where plays wait for the server. */
	outbox?: () => Storage;
	/** This library has no audio behind it: the player runs on a clock instead. */
	simulated?: boolean;
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
export class Library {
	user = $state.raw<Me | null>(null);
	facts = $state.raw<LibraryFacts | null>(null);
	/** Newest first: the feature album, then what's beside it. */
	recent = $state.raw<AlbumEntry[]>([]);
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
	artists = $state.raw<Load<ArtistEntry[]>>({ kind: 'loading' });
	/** What scans had trouble with, for administrators. */
	issues = $state.raw<Load<ScanIssues>>({ kind: 'loading' });
	/** Every song, for the Library page. Nothing asks until it opens. */
	index = $state.raw<Load<LibraryIndex>>({ kind: 'loading' });
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
	/** The signed-in person's hearts and stars: what every page shows, whatever it was given. */
	readonly marks: Marks;
	/** The signed-in person's preferences, kept on the server. */
	readonly prefs: PrefsStore;
	/** What {@link Library.index} was asked for besides the songs, joined by commas. */
	indexFields = $state('');

	readonly #source: LibrarySource;
	readonly #pinnedAlbumId: string | undefined;
	readonly #entries = new SvelteMap<string, AlbumEntry>();
	readonly #albums = new SvelteMap<string, Album>();
	readonly #missing = new SvelteSet<string>();
	readonly #unavailable = new SvelteSet<string>();
	/** Every album fetch still out: asking again waits for the same one. */
	readonly #loading = new SvelteMap<string, Promise<void>>();
	readonly #artists = new SvelteMap<string, Load<ArtistDetail>>();
	readonly #artistNames = new SvelteMap<string, string>();
	/** The scan the index was last read after, by when it ended. */
	#indexedAfter: string | undefined;
	#indexLoad: Promise<void> | null = null;
	/** Set when something other than a scan has changed what the index holds. */
	#indexEdited = $state(false);
	#player: PlayerView | null = null;
	#session: Promise<Session> | null = null;
	#setup: Promise<SetupStatus | null> | null = null;
	#shell: Promise<void> | null = null;
	#cursor: string | undefined;
	#more: Promise<void> | null = null;
	#retry: ReturnType<typeof setTimeout> | undefined;
	#expiring = false;
	#probedAt = -probeMs;
	/** Moves on at every reset: anything older that resolves late has no business here. */
	#epoch = 0;
	readonly #readyMemory: ReadyMemory;
	readonly #outbox: Outbox;
	#waited = false;
	#scanPoll: ReturnType<typeof setTimeout> | undefined;
	/** Set while a running scan is being followed, so there's only ever one of these. */
	#progressPoll: ReturnType<typeof setTimeout> | undefined;
	/** Running while the spacing after the last card lasts, or a hidden tab waits. */
	#throwing: ReturnType<typeof setTimeout> | undefined;
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

	constructor(
		source: LibrarySource | ((expired: () => void) => LibrarySource),
		options: LibraryOptions = {}
	) {
		this.#source = typeof source === 'function' ? source(() => void this.expire()) : source;
		this.#pinnedAlbumId = options.pinnedAlbumId;
		this.simulated = options.simulated ?? false;
		this.#readyMemory =
			options.readyMemory ?? readyMemoryIn(() => localStorage, 'kitsune:ready-seen');
		this.#outbox = new Outbox(options.outbox ?? (() => localStorage), this.#source);
		this.marks = new Marks(this.#source);
		this.prefs = createPrefs(this.#source);
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

	album(id: string): Album | undefined {
		return this.#albums.get(id);
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

	/** Sends what's waiting for whoever is signed in, and only for them. */
	flushPlays(): void {
		if (this.user !== null && !this.simulated) {
			void this.#outbox.flush(this.user.id);
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

	/** Counts the signed-in person's plays, hearts and rated songs. */
	listening(): Promise<Listening> {
		return this.#source.listening();
	}

	/** Ends the session and goes to the door, the way a lost session does. */
	async signOut(): Promise<void> {
		await this.#source.logout();
		await this.expire();
	}

	/** Exchanges a name and password for a session. Rejects with `LoginError`. */
	async login(name: string, password: string): Promise<void> {
		await this.#source.login(name, password);
		// Logging in names the user; only `/me` says what they may do.
		this.user = await this.#source.me();
		this.#session = Promise.resolve('signed-in');
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
			this.flushPlays();
			void this.refreshLastfm();
			void this.#adoptTheme(epoch);
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
			const cue = await this.#openingCue(leavingDeck);
			if (epoch !== this.#epoch) {
				return;
			}
			const open = (): void => {
				if (cue !== null) {
					this.#cue(cue);
				}
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

	/** Polls only while a scan runs. The deck follows a first scan its own way. */
	#watchScan(delayMs = progressPollMs): void {
		if (this.#progressPoll === undefined && !this.deck && this.facts?.scan.state === 'running') {
			this.#progressPoll = setTimeout(() => void this.#pollProgress(), delayMs);
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

	/** Asks what scans had trouble with. A list already here stays through a refresh. */
	async loadIssues(): Promise<void> {
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
	 * Finds what to cue for the island's first frame, without holding up the page if it can't, or null
	 * to leave the cue as it is. It only reads: the cue is what makes the island appear, and that's
	 * for whoever opens Home. Leaving the deck it asks again for what's already cued, because that
	 * was read part way through the scan. A queue that has started is left alone: it keeps the track
	 * list it was built from, even one read part way through a scan, because swapping that under
	 * the sound would lose its place. That list stays short until the album is next played.
	 */
	async #openingCue(again: boolean): Promise<Album | undefined | null> {
		if (this.#player?.started === true) {
			return null;
		}
		const cued = again ? this.playing?.album.id : undefined;
		const id = this.#pinnedAlbumId ?? cued ?? this.recent.at(0)?.id;
		if ((this.playing !== null && !again) || id === undefined) {
			return null;
		}
		await this.loadAlbum(id);
		return this.#albums.get(id);
	}

	/** Cues an album's first track, unless a queue has started: then the player speaks for the cue. */
	#cue(album: Album | undefined): void {
		if (this.#player?.started !== true) {
			this.playing = album === undefined ? null : cueOf(album);
		}
	}

	/** Shows what the player is on. It outranks the lab's pin: that only holds a cue, not a queue. */
	showQueue(): void {
		this.playing = this.#player?.cue ?? this.playing;
	}

	/**
	 * Cues the album just opened. The caller waits until its details are here and the flight has
	 * settled, so the island can't change under anything in the air.
	 */
	cueAlbum(album: Album): void {
		if (this.#pinnedAlbumId === undefined && this.playing?.album.id !== album.id) {
			this.#cue(album);
		}
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

	/** Reports whether a scan or an edit has finished since the index was read. */
	get indexStale(): boolean {
		return this.#indexEdited || this.facts?.scan.last?.endedAt !== this.#indexedAfter;
	}

	/** Says an edit has changed what the index holds: the next look reads it again. */
	markIndexStale(): void {
		this.#indexEdited = true;
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
			this.#adopt(await this.#source.editAlbum(id, etag, edit));
		} finally {
			// A save that stops partway has still written files, so the list reads them again either way.
			this.markIndexStale();
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
		this.#recue(album);
	}

	/** A cue only shows what would play, so it follows the album's order; a queue keeps its own. */
	#recue(album: Album): void {
		const { playing } = this;
		if (playing?.album.id !== album.id) {
			return;
		}
		const index = album.tracks.findIndex(({ id }) => id === playing.track.id);
		this.playing =
			this.#player?.started === true || index < 0
				? retagged(playing, album)
				: cueOf(album, album.tracks, index);
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
		clearTimeout(this.#progressPoll);
		this.#progressPoll = undefined;
		this.#stopFollowing();
		this.#epoch += 1;
		this.#waited = false;
		this.user = null;
		this.facts = null;
		this.lastfm = null;
		this.recent = [];
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
		this.#missing.clear();
		this.#unavailable.clear();
		this.artists = { kind: 'loading' };
		this.issues = { kind: 'loading' };
		this.index = { kind: 'loading' };
		this.indexFields = '';
		this.prefs.reset();
		theme.forget();
		this.#indexedAfter = undefined;
		this.#indexEdited = false;
		this.#artists.clear();
		this.#artistNames.clear();
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
	const pinnedAlbumId = query.get('playing') ?? undefined;
	const scan = Number(query.get('scan') ?? 0);
	if (scan <= 0) {
		// A fake library has no business vouching for the real one on this origin.
		const readyMemory = { seen: () => false, mark: () => undefined };
		return new Library(createFixtureSource({ latencyMs, health, role, editFault, lastfm, setup }), {
			pinnedAlbumId,
			readyMemory,
			simulated: true
		});
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
		pinnedAlbumId,
		readyMemory: readyMemoryIn(() => sessionStorage, `${key}:ready`),
		simulated: true
	});
}

/** Chosen once, from the URL the app opened on, and held here for every later navigation. */
export const library = fromQuery();
