import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SilentOutput } from '#lib/player/output.ts';
import { Player } from '#lib/player/player.svelte.ts';
import { theme } from '#lib/theme/theme.svelte.ts';
import { createFixtureSource } from './fixtures';
import { issuesRetryMs, Library, pileSize, type ReadyMemory } from './library.svelte';
import {
	AlbumNotFoundError,
	ArtistNotFoundError,
	ServerError,
	EditError,
	UnauthorizedError,
	type AlbumEntry,
	type AlbumPage,
	type LibraryFacts,
	type AlbumEdit,
	type LibrarySource,
	type Track
} from './source';

function source(overrides: Partial<LibrarySource> = {}): LibrarySource {
	return { ...createFixtureSource(), ...overrides };
}

function track(id: string): Track {
	return {
		id,
		title: id,
		artists: [],
		durationMs: 100_000,
		format: 'FLAC',
		genres: [],
		etag: '',
		hearted: false
	};
}

/** Nothing to report: what a library says with no scan running. */
const noTrouble: Pick<LibraryFacts, 'issues' | 'issueCounts' | 'scan'> = {
	issues: 0,
	issueCounts: { unreadable: 0, corrupted: 0, unsupported: 0, warnings: 0 },
	scan: { state: 'idle', read: 0, total: 0 }
};

function forgetful(seen = false): ReadyMemory {
	return { seen: () => seen, mark: () => undefined };
}

describe('following a scan', () => {
	const running = {
		...noTrouble,
		scan: { state: 'running', mode: 'full', read: 10, total: 100 }
	} as const;

	function facts(extra: Partial<LibraryFacts>): LibraryFacts {
		return { albums: 1, tracks: 8, durationMs: 0, roots: 1, ...noTrouble, ...extra };
	}

	/** A library whose counts the test sets, opened and ready. */
	async function opened(first: LibraryFacts, overrides: Partial<LibrarySource> = {}) {
		const read = vi.fn<LibrarySource['facts']>(() => Promise.resolve(first));
		const library = new Library(source({ facts: read, ...overrides }), {
			readyMemory: forgetful(true)
		});
		await library.loadShell();
		read.mockClear();
		return { library, read };
	}

	afterEach(() => {
		vi.useRealTimers();
	});

	it('asks nothing while no scan runs', async () => {
		vi.useFakeTimers();
		const { library, read } = await opened(facts({}));
		await vi.advanceTimersByTimeAsync(10_000);
		expect(read).not.toHaveBeenCalled();
		library.reset();
	});

	it('polls a scan it finds running, on any page, and stops when it ends', async () => {
		vi.useFakeTimers();
		const { library, read } = await opened(facts(running));
		await vi.advanceTimersByTimeAsync(3000);
		expect(read).toHaveBeenCalledTimes(3);

		read.mockResolvedValue(facts({ tracks: 9 }));
		read.mockClear();
		await vi.advanceTimersByTimeAsync(1000);
		expect(library.facts?.scan.state).toBe('idle');
		expect(library.facts?.tracks).toBe(9);
		// One to see the end, one for counts read after it. Then nothing.
		expect(read).toHaveBeenCalledTimes(2);
		await vi.advanceTimersByTimeAsync(5000);
		expect(read).toHaveBeenCalledTimes(2);
		library.reset();
	});

	it('takes its counts from a read after the end, not the one that saw it', async () => {
		vi.useFakeTimers();
		const { library, read } = await opened(facts(running));
		read.mockResolvedValueOnce(facts({ tracks: 8 }));
		read.mockResolvedValue(facts({ tracks: 12 }));
		await vi.advanceTimersByTimeAsync(1000);
		expect(library.facts?.tracks).toBe(12);
		library.reset();
	});

	it('finds a scan that began unseen when asked, and follows it', async () => {
		vi.useFakeTimers();
		const { library, read } = await opened(facts({}));
		read.mockResolvedValue(facts(running));
		await library.refreshFacts();
		read.mockClear();
		await vi.advanceTimersByTimeAsync(1000);
		expect(read).toHaveBeenCalledOnce();
		library.reset();
	});

	it('leaves a first scan to the deck, and picks it up if the deck ends with one running', async () => {
		vi.useFakeTimers();
		const scanning = facts({ albums: 0, tracks: 0, ...running });
		const read = vi.fn<LibrarySource['facts']>(() => Promise.resolve(scanning));
		const library = new Library(source({ facts: read, ready: () => Promise.resolve(false) }), {
			readyMemory: forgetful()
		});
		await library.loadShell();
		expect(library.deck).toBe(true);
		read.mockClear();
		await vi.advanceTimersByTimeAsync(3000);
		// The deck's own poll, and no second one beside it.
		expect(read).toHaveBeenCalledTimes(3);
		library.reset();
	});

	it('says the server has gone quiet mid-scan, and slows down until it answers', async () => {
		vi.useFakeTimers();
		const { library, read } = await opened(facts(running));
		read.mockRejectedValue(new TypeError('offline'));
		await vi.advanceTimersByTimeAsync(1000);
		expect(library.down).toBe(true);
		read.mockClear();
		await vi.advanceTimersByTimeAsync(4999);
		expect(read).not.toHaveBeenCalled();
		read.mockResolvedValue(facts({}));
		await vi.advanceTimersByTimeAsync(1);
		expect(library.down).toBe(false);
		expect(library.facts?.scan.state).toBe('idle');
		library.reset();
	});

	it('stops polling on sign-out', async () => {
		vi.useFakeTimers();
		const { library, read } = await opened(facts(running));
		library.reset();
		await vi.advanceTimersByTimeAsync(5000);
		expect(read).not.toHaveBeenCalled();
	});

	it('shows a scan it started at once, then follows it', async () => {
		vi.useFakeTimers();
		const startScan = vi.fn<LibrarySource['startScan']>(() => Promise.resolve(running.scan));
		const { library, read } = await opened(facts({}), { startScan });
		read.mockResolvedValue(facts(running));
		await expect(library.startScan('full')).resolves.toBeNull();
		expect(startScan).toHaveBeenCalledWith('full');
		expect(library.facts?.scan.state).toBe('running');
		expect(read).not.toHaveBeenCalled();
		await vi.advanceTimersByTimeAsync(1000);
		expect(read).toHaveBeenCalledOnce();
		library.reset();
	});

	it('reads the counts of a scan that was over before the answer left', async () => {
		const startScan = vi.fn<LibrarySource['startScan']>(() => Promise.resolve(noTrouble.scan));
		const { library, read } = await opened(facts({}), { startScan });
		read.mockResolvedValue(facts({ tracks: 9 }));
		await library.startScan('quick');
		expect(library.facts?.tracks).toBe(9);
	});

	it('treats a 409 as a scan to watch, not as a failure', async () => {
		vi.useFakeTimers();
		const startScan = () => Promise.reject(new ServerError(409));
		const { library, read } = await opened(facts({}), { startScan });
		read.mockResolvedValue(facts(running));
		await expect(library.startScan('full')).resolves.toBeNull();
		expect(library.facts?.scan.state).toBe('running');
		library.reset();
	});

	it('says why a scan did not start', async () => {
		const refused = async (error: Error) => {
			const { library } = await opened(facts({}), { startScan: () => Promise.reject(error) });
			return library.startScan('quick');
		};
		await expect(refused(new ServerError(403))).resolves.toBe('forbidden');
		await expect(refused(new ServerError(503))).resolves.toBe('stopping');
		await expect(refused(new ServerError(500))).resolves.toBe('unavailable');
		await expect(refused(new TypeError('off'))).resolves.toBe('unavailable');
		await expect(refused(new UnauthorizedError())).resolves.toBeNull();
	});

	it('knows an admin from anyone else', async () => {
		const admin = new Library(source());
		await admin.checkSession();
		expect(admin.admin).toBe(true);
		const user = new Library(createFixtureSource({ role: 'user' }));
		await user.checkSession();
		expect(user.admin).toBe(false);
	});

	it('loads the issues, and keeps a list through a failed refresh', async () => {
		const issues = vi.fn(createFixtureSource().issues);
		const library = new Library(source({ issues }));
		await library.loadIssues();
		expect(library.issues.kind).toBe('ready');
		issues.mockRejectedValueOnce(new ServerError(500));
		await library.loadIssues();
		expect(library.issues.kind).toBe('ready');
	});

	it('says when the issues could not be loaded, and forgets them on sign-out', async () => {
		const library = new Library(source({ issues: () => Promise.reject(new ServerError(500)) }));
		await library.loadIssues();
		expect(library.issues.kind).toBe('unavailable');
		library.reset();
		expect(library.issues.kind).toBe('loading');
	});

	it('asks once more for issues that failed to load, a moment later', async () => {
		vi.useFakeTimers();
		try {
			const issues = vi.fn(createFixtureSource().issues);
			issues.mockRejectedValueOnce(new ServerError(500));
			const library = new Library(source({ issues }));
			await library.loadIssues();
			expect(library.issues.kind).toBe('unavailable');
			await vi.advanceTimersByTimeAsync(issuesRetryMs);
			expect(library.issues.kind).toBe('ready');
			expect(issues).toHaveBeenCalledTimes(2);
		} finally {
			vi.useRealTimers();
		}
	});

	it('asks only once more, and not after sign-out', async () => {
		vi.useFakeTimers();
		try {
			const issues = vi.fn(() => Promise.reject(new ServerError(500)));
			const library = new Library(source({ issues }));
			await library.loadIssues();
			await vi.advanceTimersByTimeAsync(issuesRetryMs * 3);
			expect(issues).toHaveBeenCalledTimes(2);
			expect(library.issues.kind).toBe('unavailable');

			await library.loadIssues();
			library.reset();
			await vi.advanceTimersByTimeAsync(issuesRetryMs * 3);
			expect(issues).toHaveBeenCalledTimes(3);
		} finally {
			vi.useRealTimers();
		}
	});
});

describe('Library', () => {
	it('remembers the albums a search found, for the next first frame', async () => {
		const library = new Library(source());
		library.search.run('glasshouse');
		await vi.waitFor(() => {
			expect(library.search.state.kind).toBe('found');
		});
		expect(library.entry('glasshouse-66')?.title).toBe('Glasshouse');
	});

	it('loads the artists, and keeps a list through a failed refresh', async () => {
		const artists = vi.fn(createFixtureSource().artists);
		const library = new Library(source({ artists }));
		await library.loadArtists();
		expect(library.artists.kind).toBe('ready');
		artists.mockRejectedValueOnce(new ServerError(500));
		await library.loadArtists();
		expect(library.artists.kind).toBe('ready');
	});

	it('says when the artists could not be loaded', async () => {
		const library = new Library(source({ artists: () => Promise.reject(new TypeError('off')) }));
		await library.loadArtists();
		expect(library.artists.kind).toBe('unavailable');
	});

	it('loads an artist, seeding its albums for the next first frame', async () => {
		const library = new Library(source());
		expect(library.artist('artist-low-tide-choir').kind).toBe('loading');
		await library.loadArtist('artist-low-tide-choir');
		expect(library.artist('artist-low-tide-choir').kind).toBe('ready');
		expect(library.entry('glasshouse-66')?.title).toBe('Glasshouse');
	});

	it('tells a missing artist from one that could not be fetched', async () => {
		const missing = new Library(
			source({ artist: (id) => Promise.reject(new ArtistNotFoundError(id)) })
		);
		await missing.loadArtist('a');
		expect(missing.artist('a').kind).toBe('missing');
		const down = new Library(source({ artist: () => Promise.reject(new ServerError(500)) }));
		await down.loadArtist('a');
		expect(down.artist('a').kind).toBe('unavailable');
	});

	it('knows an artist by name from whatever linked to it', async () => {
		const library = new Library(source());
		await library.loadAlbum('glasshouse-66');
		expect(library.artistName('artist-low-tide-choir')).toBe('Low Tide Choir');
	});

	it('forgets artists with the session', async () => {
		const library = new Library(source());
		await library.loadArtists();
		await library.loadArtist('artist-low-tide-choir');
		library.reset();
		expect(library.artists.kind).toBe('loading');
		expect(library.artist('artist-low-tide-choir').kind).toBe('loading');
		expect(library.artistName('artist-low-tide-choir')).toBeUndefined();
	});

	it('forgets a search with the session', async () => {
		const library = new Library(source());
		library.search.run('glasshouse');
		await vi.waitFor(() => {
			expect(library.search.state.kind).toBe('found');
		});
		library.reset();
		expect(library.search.state.kind).toBe('idle');
	});

	it('asks who is signed in only once', async () => {
		const me = vi.fn(createFixtureSource().me);
		const library = new Library(source({ me }));
		await Promise.all([library.checkSession(), library.checkSession()]);
		await library.checkSession();
		expect(me).toHaveBeenCalledOnce();
	});

	it('tells a missing session from a server that is down', async () => {
		const signedOut = new Library(source({ me: () => Promise.reject(new UnauthorizedError()) }));
		const down = new Library(source({ me: () => Promise.reject(new TypeError('offline')) }));
		await expect(signedOut.checkSession()).resolves.toBe('signed-out');
		await expect(down.checkSession()).resolves.toBe('down');
	});

	it('takes the theme the person chose elsewhere, and forgets it with the session', async () => {
		const library = new Library(createFixtureSource({ prefs: { 'ui.theme': 'light' } }));
		await library.loadShell();
		await vi.waitFor(() => {
			expect(theme.choice).toBe('light');
		});
		library.reset();
		expect(theme.choice).toBeNull();
	});

	it('keeps the theme as it is when the person never chose one', async () => {
		const library = new Library(source());
		await library.loadShell();
		await Promise.resolve();
		expect(theme.choice).toBeNull();
	});

	it('cues the feature album once the shell has loaded', async () => {
		const library = new Library(source());
		await library.loadShell();
		expect(library.facts).not.toBeNull();
		expect(library.playing?.album.title).toBe('Glasshouse');
		expect(library.playing?.track.title).toBe('Breakwater');
	});

	it('keeps a 404 for good, but lets anything else clear on retry', async () => {
		const album = vi.fn(createFixtureSource().album);
		const library = new Library(source({ album }));

		album.mockRejectedValueOnce(new AlbumNotFoundError('a'));
		await library.loadAlbum('a');
		expect(library.failed('a')).toBe(true);

		for (const failure of [new ServerError(503), new TypeError('offline')]) {
			album.mockRejectedValueOnce(failure);
			await library.loadAlbum('glasshouse-66');
			expect(library.failed('glasshouse-66')).toBe(false);
			expect(library.unavailable('glasshouse-66')).toBe(true);
		}
		await library.loadAlbum('glasshouse-66');
		expect(library.unavailable('glasshouse-66')).toBe(false);
		expect(library.album('glasshouse-66')?.title).toBe('Glasshouse');
	});

	it('marks neither state for a lost session', async () => {
		const library = new Library(source({ album: () => Promise.reject(new UnauthorizedError()) }));
		await library.loadAlbum('a');
		expect(library.failed('a')).toBe(false);
		expect(library.unavailable('a')).toBe(false);
	});

	it('leaves once however many calls lose the session, and forgets what it saw', async () => {
		const library = new Library(source());
		await library.loadShell();
		const leave = vi.fn(() => Promise.resolve());
		library.onExpired = leave;
		await Promise.all([library.expire(), library.expire(), library.expire()]);
		expect(leave).toHaveBeenCalledOnce();
		expect(library.user).toBeNull();
		expect(library.playing).toBeNull();
		expect(library.entry('glasshouse-66')).toBeUndefined();
		await expect(library.checkSession()).resolves.toBe('signed-out');
	});

	it('loads one page at a time, and stops at the end', async () => {
		const first: AlbumPage = { albums: [], nextCursor: 'two' };
		const albumsPage = vi.fn<LibrarySource['albumsPage']>((_sort, cursor) =>
			Promise.resolve(cursor === undefined ? first : { albums: [] })
		);
		const library = new Library(source({ albumsPage }));
		await library.loadShell();
		expect(library.hasMoreAlbums).toBe(true);
		albumsPage.mockClear();
		await Promise.all([library.loadMoreAlbums(), library.loadMoreAlbums()]);
		expect(albumsPage).toHaveBeenCalledOnce();
		expect(library.hasMoreAlbums).toBe(false);
		await library.loadMoreAlbums();
		expect(albumsPage).toHaveBeenCalledOnce();
	});

	it('loads the whole list in big pages, from wherever Home got to', async () => {
		const albumsPage = vi.fn<LibrarySource['albumsPage']>((sort, cursor) => {
			const page = Number(cursor ?? 0);
			const last = page === 2;
			return Promise.resolve({
				albums: sort === 'name' ? [entry(page)] : [],
				...(last ? {} : { nextCursor: String(page + 1) })
			});
		});
		const library = new Library(source({ albumsPage }), { readyMemory: forgetful() });
		await library.loadShell();
		albumsPage.mockClear();

		await library.loadAllAlbums();
		expect(albumsPage.mock.calls).toEqual([
			['name', '1', 200],
			['name', '2', 200]
		]);
		expect(library.albums).toHaveLength(3);
		expect(library.hasMoreAlbums).toBe(false);
	});

	it('stops loading the whole list at a failure, and carries on when asked again', async () => {
		let broken = false;
		const albumsPage = vi.fn<LibrarySource['albumsPage']>((sort, cursor) => {
			if (cursor === undefined) {
				return Promise.resolve({ albums: [entry(0)], nextCursor: 'rest' });
			}
			return broken
				? Promise.reject(new Error('no'))
				: Promise.resolve({ albums: sort === 'name' ? [entry(1)] : [] });
		});
		const library = new Library(source({ albumsPage }), { readyMemory: forgetful() });
		await library.loadShell();

		broken = true;
		await library.loadAllAlbums();
		expect(library.hasMoreAlbums).toBe(true);

		broken = false;
		await library.loadAllAlbums();
		expect(library.albums).toHaveLength(2);
		expect(library.hasMoreAlbums).toBe(false);
	});

	it('asks only for counts on a ready library with nothing in it, until it has something', async () => {
		vi.useFakeTimers();
		const empty = { albums: 0, tracks: 0, durationMs: 0, roots: 1, ...noTrouble };
		const facts = vi.fn<LibrarySource['facts']>(() => Promise.resolve(empty));
		const albumsPage = vi.fn<LibrarySource['albumsPage']>(() => Promise.resolve({ albums: [] }));
		const library = new Library(source({ facts, albumsPage }), { readyMemory: forgetful() });
		await library.loadShell();
		expect(library.deck).toBe(false);
		expect(library.playing).toBeNull();

		albumsPage.mockClear();
		await vi.advanceTimersByTimeAsync(5000);
		expect(albumsPage).not.toHaveBeenCalled();

		facts.mockResolvedValue({ ...empty, albums: 1 });
		await vi.advanceTimersByTimeAsync(5000);
		expect(albumsPage).toHaveBeenCalled();
		library.reset();
		vi.useRealTimers();
	});

	it('does not open the deck with no roots to scan', async () => {
		vi.useFakeTimers();
		const none = { albums: 0, tracks: 0, durationMs: 0, roots: 0, ...noTrouble };
		const library = new Library(
			source({ facts: () => Promise.resolve(none), ready: () => Promise.resolve(false) }),
			{ readyMemory: forgetful() }
		);
		await library.loadShell();
		expect(library.deck).toBe(false);
		library.reset();
		vi.useRealTimers();
	});

	it('marks the shell down, and comes back by itself', async () => {
		vi.useFakeTimers();
		const facts = vi.fn(createFixtureSource().facts);
		facts.mockRejectedValueOnce(new ServerError(502));
		const library = new Library(source({ facts }));
		await library.loadShell();
		expect(library.down).toBe(true);
		await vi.advanceTimersByTimeAsync(5000);
		expect(library.down).toBe(false);
		expect(library.facts).not.toBeNull();
		vi.useRealTimers();
	});
});

function entry(index: number): AlbumEntry {
	return {
		id: `album-${String(index)}`,
		title: `Album ${String(index)}`,
		artist: 'Someone',
		trackCount: 8,
		compilation: false
	};
}

/** A scan the test drives by hand: what's been found so far, and whether it's done. */
class Scripted {
	found = 0;
	done = false;
	down = false;

	get facts(): LibraryFacts {
		const { found: albums } = this;
		return { albums, tracks: albums * 8, durationMs: 0, roots: 1, ...noTrouble };
	}

	source(): LibrarySource {
		const answer = <T>(value: () => T): Promise<T> =>
			this.down ? Promise.reject(new TypeError('offline')) : Promise.resolve(value());
		return source({
			facts: () => answer(() => this.facts),
			ready: () => answer(() => this.done),
			albumsPage: (sort, _cursor, limit = 60) =>
				answer(() => {
					const newestFirst = Array.from({ length: this.found }, (_, index) =>
						entry(this.found - 1 - index)
					);
					return { albums: (sort === 'added' ? newestFirst : []).slice(0, limit) };
				}),
			album: (id) =>
				answer(() => ({
					...entry(Number(id.slice('album-'.length))),
					artists: [],
					tracks: [
						{
							id: `${id}-1`,
							title: 'One',
							artists: [],
							durationMs: 1000,
							format: 'FLAC',
							genres: [],
							etag: '',
							hearted: false
						}
					],
					durationMs: 0,
					formats: [],
					addedAt: '',
					genres: [],
					etag: '',
					hearted: false
				}))
		});
	}
}

describe('the deck', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	function ids(library: Library): string[] {
		return library.pile.map(({ id }) => id);
	}

	it('opens on a scan with nothing found yet, without a cue', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		expect(library.deck).toBe(true);
		expect(library.facts?.albums).toBe(0);
		expect(library.playing).toBeNull();
		library.reset();
	});

	it('treats a readiness check that fails as not ready', async () => {
		const scan = new Scripted();
		const library = new Library(
			{ ...scan.source(), ready: () => Promise.reject(new TypeError('offline')) },
			{ readyMemory: forgetful() }
		);
		await library.loadShell();
		expect(library.deck).toBe(true);
		library.reset();
	});

	it('throws a burst down oldest first, as fast as the spacing allows', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		scan.found = 10;
		await vi.advanceTimersByTimeAsync(1001);
		expect(ids(library)).toEqual(['album-0']);
		await vi.advanceTimersByTimeAsync(40);
		expect(library.pile.length).toBe(1);
		// Ten cards at no more than 84 ms apart.
		await vi.advanceTimersByTimeAsync(9 * 84);
		expect(ids(library).at(-1)).toBe('album-9');
		library.reset();
	});

	it('asks every second', async () => {
		const scan = new Scripted();
		const facts = vi.fn<LibrarySource['facts']>(() => Promise.resolve(scan.facts));
		const library = new Library({ ...scan.source(), facts }, { readyMemory: forgetful() });
		await library.loadShell();
		facts.mockClear();
		await vi.advanceTimersByTimeAsync(5000);
		expect(facts).toHaveBeenCalledTimes(5);
		library.reset();
	});

	it('hurries under a backlog', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		scan.found = 100;
		await vi.advanceTimersByTimeAsync(1001);
		await vi.advanceTimersByTimeAsync(3000);
		// About 45 ms apart: at 70 it would be nearer 43.
		expect(Number(ids(library).at(-1)?.slice('album-'.length))).toBeGreaterThan(55);
		library.reset();
	});

	it('drops the oldest only past 200 waiting', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		const landed = new Set<string>();
		const watch = setInterval(() => {
			for (const id of ids(library)) {
				landed.add(id);
			}
		}, 30);
		for (const found of [100, 200, 300, 400]) {
			scan.found = found;
			await vi.advanceTimersByTimeAsync(1000);
		}
		await vi.advanceTimersByTimeAsync(30_000);
		clearInterval(watch);
		// The feed sees the newest 100 at a time, and about 70 cards have gone in four seconds.
		expect(landed.has('album-399')).toBe(true);
		expect(landed.size).toBeGreaterThan(250);
		library.reset();
	});

	it('remembers every entry it polls, for the first frame of the album page', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		scan.found = 3;
		await vi.advanceTimersByTimeAsync(3000);
		expect(library.entry('album-2')?.title).toBe('Album 2');
		library.reset();
	});

	it('takes only what the last burst added, and keeps at most a full pile', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		scan.found = 5;
		await vi.advanceTimersByTimeAsync(3000);
		await vi.advanceTimersByTimeAsync(30_000);
		expect(library.pile.length).toBe(5);
		scan.found = 30;
		await vi.advanceTimersByTimeAsync(60_000);
		expect(library.pile.length).toBe(pileSize);
		expect(new Set(ids(library)).size).toBe(pileSize);
		expect(ids(library).at(-1)).toBe('album-29');
		library.reset();
	});

	it('stays on the deck through a server that has gone, and says so', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		scan.found = 2;
		await vi.advanceTimersByTimeAsync(9000);
		const held = ids(library);
		scan.down = true;
		await vi.advanceTimersByTimeAsync(3000);
		expect(library.down).toBe(true);
		expect(library.deck).toBe(true);
		expect(ids(library)).toEqual(held);
		scan.down = false;
		await vi.advanceTimersByTimeAsync(3000);
		expect(library.down).toBe(false);
		library.reset();
	});

	it('hands over to Home when the scan ends, on a fresh read', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		scan.found = 4;
		await vi.advanceTimersByTimeAsync(3000);
		scan.found = 6;
		scan.done = true;
		await vi.advanceTimersByTimeAsync(3000);
		expect(library.deck).toBe(false);
		expect(library.pile).toEqual([]);
		expect(library.facts?.albums).toBe(6);
		expect(library.recent.at(0)?.id).toBe('album-5');
		expect(library.playing?.album.id).toBe('album-5');
		library.reset();
	});

	it('stays on the deck if that fresh read fails, and tries again', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		scan.found = 4;
		scan.done = true;
		const facts = vi.spyOn(scan, 'facts', 'get');
		facts.mockReturnValueOnce(scan.facts).mockImplementationOnce(() => {
			throw new ServerError(502);
		});
		await vi.advanceTimersByTimeAsync(3000);
		expect(library.down).toBe(true);
		expect(library.deck).toBe(true);
		facts.mockRestore();
		await vi.advanceTimersByTimeAsync(5000);
		expect(library.deck).toBe(false);
		expect(library.down).toBe(false);
		library.reset();
	});

	it('ends with the empty line when the scan found nothing', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		scan.done = true;
		await vi.advanceTimersByTimeAsync(3000);
		expect(library.deck).toBe(false);
		expect(library.facts?.albums).toBe(0);
		library.reset();
	});

	it('forgets a cue read part way through the scan', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		scan.found = 4;
		await vi.advanceTimersByTimeAsync(3000);
		await library.loadAlbum('album-1');
		const partial = library.album('album-1');
		scan.done = true;
		await vi.advanceTimersByTimeAsync(3000);
		expect(library.album('album-1')).not.toBe(partial);
		library.reset();
	});

	describe('with real covers', () => {
		interface Decode {
			hash: string;
			finish: () => void;
			fail: () => void;
		}

		let decodes: Decode[] = [];

		class FakeImage {
			fetchPriority = '';
			src = '';

			decode(): Promise<undefined> {
				const { promise, resolve, reject } = Promise.withResolvers<undefined>();
				const hash = /artwork\/([^?]+)/.exec(this.src)?.[1] ?? '';
				decodes.push({
					hash,
					finish: () => {
						resolve(undefined);
					},
					fail: () => {
						reject(new Error('no'));
					}
				});
				return promise;
			}
		}

		let run = 0;

		/** A deck with this many albums found, each with a cover no other test has decoded. */
		async function withCovers(count: number): Promise<Library> {
			run += 1;
			decodes = [];
			vi.stubGlobal('Image', FakeImage);
			const scan = new Scripted();
			const base = scan.source();
			const library = new Library(
				{
					...base,
					albumsPage: async (...args) => {
						const page = await base.albumsPage(...args);
						return {
							...page,
							albums: page.albums.map((album) => ({
								...album,
								artwork: {
									hash: `${String(run)}-${album.id}`,
									accent: '#123456',
									ground: '#000000',
									width: 512,
									height: 512
								}
							}))
						};
					}
				},
				{ readyMemory: forgetful() }
			);
			await library.loadShell();
			scan.found = count;
			await vi.advanceTimersByTimeAsync(1001);
			return library;
		}

		afterEach(() => {
			vi.unstubAllGlobals();
		});

		it('decodes six at once, ahead of the throw, and lands nothing before its decode', async () => {
			const library = await withCovers(20);
			expect(decodes.length).toBe(6);
			expect(library.pile).toEqual([]);
			await vi.advanceTimersByTimeAsync(500);
			expect(library.pile).toEqual([]);

			decodes.at(0)?.finish();
			await vi.advanceTimersByTimeAsync(0);
			expect(library.pile.length).toBe(1);
			expect(library.pile.at(0)?.artwork).toBeDefined();
			expect(decodes.length).toBe(7);
			library.reset();
		});

		it('does not hold a decoded card behind a slow one', async () => {
			const library = await withCovers(6);
			decodes.at(3)?.finish();
			await vi.advanceTimersByTimeAsync(0);
			expect(ids(library)).toEqual(['album-3']);
			library.reset();
		});

		it('keeps the spacing, however many decode together', async () => {
			const library = await withCovers(6);
			for (const decode of decodes) {
				decode.finish();
			}
			await vi.advanceTimersByTimeAsync(0);
			expect(library.pile.length).toBe(1);
			await vi.advanceTimersByTimeAsync(90);
			expect(library.pile.length).toBe(2);
			library.reset();
		});

		it('lands a generated cover at the bound, not a blank', async () => {
			const library = await withCovers(1);
			await vi.advanceTimersByTimeAsync(790);
			expect(library.pile).toEqual([]);
			await vi.advanceTimersByTimeAsync(10);
			expect(library.pile.length).toBe(1);
			expect(library.pile.at(0)?.artwork).toBeUndefined();
			library.reset();
		});

		it('lands a generated cover when the decode fails', async () => {
			const library = await withCovers(1);
			decodes.at(0)?.fail();
			await vi.advanceTimersByTimeAsync(0);
			expect(library.pile.at(0)?.artwork).toBeUndefined();
			library.reset();
		});

		it('drops a decode that finishes after a reset', async () => {
			const library = await withCovers(3);
			library.reset();
			decodes.at(0)?.finish();
			await vi.advanceTimersByTimeAsync(1000);
			expect(library.pile).toEqual([]);
		});
	});

	describe('at the end of the scan', () => {
		async function finished(
			dealer: (open: () => void) => Promise<void>
		): Promise<{ scan: Scripted; library: Library }> {
			const scan = new Scripted();
			const library = new Library(scan.source(), { readyMemory: forgetful() });
			await library.loadShell();
			library.dealer = dealer;
			scan.found = 4;
			await vi.advanceTimersByTimeAsync(3001);
			scan.done = true;
			await vi.advanceTimersByTimeAsync(3001);
			return { scan, library };
		}

		it('leaves a queue at a later track where it is', async () => {
			const scan = new Scripted();
			const library = new Library(scan.source(), { readyMemory: forgetful() });
			const player = new Player(library, () => new SilentOutput({ ticking: false }));
			await library.loadShell();
			scan.found = 4;
			await vi.advanceTimersByTimeAsync(3001);
			await library.loadAlbum('album-3');
			const partial = library.album('album-3');
			if (partial === undefined) {
				throw new Error('the album should have loaded');
			}
			const tracks = [track('first'), track('second'), track('third')];
			player.play({ ...partial, tracks }, 1);
			scan.done = true;
			await vi.advanceTimersByTimeAsync(3001);
			expect(library.deck).toBe(false);
			expect(library.playing?.track.id).toBe('second');
			expect(library.playing?.index).toBe(1);
			expect(library.playing?.trackCount).toBe(3);
			expect(player.current?.id).toBe('second');
			library.reset();
		});

		it('leaves mounting Home to the deck, which stays until the deal is over', async () => {
			let finish = (): void => undefined;
			let opened = false;
			const { library } = await finished(
				(open) =>
					new Promise((resolve) => {
						open();
						opened = true;
						finish = resolve;
					})
			);
			expect(opened).toBe(true);
			expect(library.deck).toBe(true);
			expect(library.dealing).toBe('held');
			expect(library.recent.length).toBeGreaterThan(0);
			expect(library.playing?.album.id).toBe('album-3');
			finish();
			await vi.advanceTimersByTimeAsync(0);
			expect(library.deck).toBe(false);
			expect(library.dealing).toBe('no');
			expect(library.pile).toEqual([]);
			library.reset();
		});

		it('does not cue anything before Home opens, so the island can form with it', async () => {
			const scan = new Scripted();
			const library = new Library(scan.source(), { readyMemory: forgetful() });
			await library.loadShell();
			const dealer = vi.fn((open: () => void) => {
				expect(library.playing).toBeNull();
				open();
				expect(library.playing).not.toBeNull();
				return Promise.resolve();
			});
			library.dealer = dealer;
			scan.found = 4;
			scan.done = true;
			await vi.advanceTimersByTimeAsync(3001);
			expect(dealer).toHaveBeenCalledOnce();
			expect(library.down).toBe(false);
			library.reset();
		});

		it('stops landing cards while the deal is on', async () => {
			let pile: AlbumEntry[] = [];
			const scan = new Scripted();
			const library = new Library(scan.source(), { readyMemory: forgetful() });
			await library.loadShell();
			library.dealer = async (open) => {
				open();
				pile = library.pile;
				await new Promise((resolve) => setTimeout(resolve, 20_000));
				expect(library.pile).toBe(pile);
			};
			scan.found = 30;
			await vi.advanceTimersByTimeAsync(3001);
			scan.done = true;
			await vi.advanceTimersByTimeAsync(24_000);
			expect(library.deck).toBe(false);
			library.reset();
		});

		it('never deals an empty library', async () => {
			const dealer = vi.fn(() => Promise.resolve());
			const scan = new Scripted();
			const library = new Library(scan.source(), { readyMemory: forgetful() });
			await library.loadShell();
			library.dealer = dealer;
			scan.done = true;
			await vi.advanceTimersByTimeAsync(3001);
			expect(dealer).not.toHaveBeenCalled();
			expect(library.deck).toBe(false);
			library.reset();
		});
	});

	describe('a riffle', () => {
		const card = (index: number): AlbumEntry => entry(100 + index);

		it('lands a card on top, and makes room with the oldest it may lose', async () => {
			const scan = new Scripted();
			scan.found = 12;
			const library = new Library(scan.source(), { readyMemory: forgetful() });
			await library.loadShell();
			await vi.advanceTimersByTimeAsync(1500);
			expect(library.pile.length).toBe(pileSize);

			library.riffle(card(0), new Set(['album-11', 'album-0']));
			expect(ids(library).at(-1)).toBe('album-100');
			expect(library.pile.length).toBe(pileSize);
			expect(ids(library)).toContain('album-0');
			expect(ids(library)).not.toContain('album-1');
			library.reset();
		});

		it('never loses what it has been told to keep, even past a full pile', async () => {
			const scan = new Scripted();
			scan.found = 12;
			const library = new Library(scan.source(), { readyMemory: forgetful() });
			await library.loadShell();
			await vi.advanceTimersByTimeAsync(1500);
			library.riffle(card(0), new Set(ids(library)));
			expect(library.pile.length).toBe(pileSize + 1);
			library.reset();
		});
	});

	describe('after a reset', () => {
		it('drops a poll still in flight: no writes, and no next poll', async () => {
			const scan = new Scripted();
			const facts = vi.fn<LibrarySource['facts']>(() => Promise.resolve(scan.facts));
			const library = new Library({ ...scan.source(), facts }, { readyMemory: forgetful() });
			await library.loadShell();
			await vi.advanceTimersByTimeAsync(0);

			let answer: (facts: LibraryFacts) => void = () => undefined;
			facts.mockImplementationOnce(
				() =>
					new Promise((resolve) => {
						answer = resolve;
					})
			);
			await vi.advanceTimersByTimeAsync(3000);
			library.reset();
			facts.mockClear();
			scan.found = 3;
			answer(scan.facts);
			await vi.advanceTimersByTimeAsync(60_000);
			expect(library.facts).toBeNull();
			expect(library.pile).toEqual([]);
			expect(facts).not.toHaveBeenCalled();
		});

		it('drops a shell load still in flight too', async () => {
			const scan = new Scripted();
			let answer: (facts: LibraryFacts) => void = () => undefined;
			const library = new Library(
				{
					...scan.source(),
					facts: () =>
						new Promise((resolve) => {
							answer = resolve;
						})
				},
				{ readyMemory: forgetful() }
			);
			const loading = library.loadShell();
			await vi.advanceTimersByTimeAsync(0);
			library.reset();
			answer(scan.facts);
			await loading;
			expect(library.facts).toBeNull();
			expect(library.deck).toBe(false);
		});
	});

	describe('joining a scan under way', () => {
		it('waits a moment with nothing assigned, then resumes the pile with nothing replayed', async () => {
			const scan = new Scripted();
			scan.found = 40;
			const library = new Library(scan.source(), { readyMemory: forgetful() });
			await library.loadShell();
			expect(library.facts).toBeNull();
			expect(library.deck).toBe(false);
			expect(library.recent).toEqual([]);

			await vi.advanceTimersByTimeAsync(1500);
			expect(library.deck).toBe(true);
			expect(library.pile.length).toBe(pileSize);
			expect(ids(library).at(-1)).toBe('album-39');

			const seeded = ids(library);
			await vi.advanceTimersByTimeAsync(30_000);
			expect(ids(library)).toEqual(seeded);

			scan.found = 41;
			await vi.advanceTimersByTimeAsync(3001);
			expect(ids(library).at(-1)).toBe('album-40');
			library.reset();
		});

		it('reads a scan that ends during the wait as a restart: plain Home', async () => {
			const scan = new Scripted();
			scan.found = 40;
			const library = new Library(scan.source(), { readyMemory: forgetful() });
			await library.loadShell();
			scan.done = true;
			await vi.advanceTimersByTimeAsync(1500);
			expect(library.deck).toBe(false);
			expect(library.facts?.albums).toBe(40);
			library.reset();
		});

		it('never shows a browser that has seen this library ready the deck on a restart', async () => {
			const scan = new Scripted();
			scan.found = 40;
			const library = new Library(scan.source(), { readyMemory: forgetful(true) });
			await library.loadShell();
			expect(library.deck).toBe(false);
			expect(library.facts?.albums).toBe(40);
			library.reset();
		});

		it('remembers a ready library for the next restart', async () => {
			const scan = new Scripted();
			scan.done = true;
			const mark = vi.fn();
			const library = new Library(scan.source(), {
				readyMemory: { seen: () => false, mark }
			});
			await library.loadShell();
			expect(mark).toHaveBeenCalled();
			library.reset();
		});
	});
});

describe('tag edits', () => {
	const id = 'glasshouse-66';

	async function opened(overrides: Partial<LibrarySource> = {}) {
		const library = new Library(source(overrides));
		const output = new SilentOutput({ ticking: false });
		const player = new Player(library, () => output);
		await library.loadShell();
		await library.loadAlbum(id);
		const album = library.album(id);
		if (album === undefined) {
			throw new Error('no album');
		}
		return { library, player, output, album };
	}

	function trackAt(library: Library, index: number): Track {
		const track = library.album(id)?.tracks.at(index);
		if (track === undefined) {
			throw new Error('no such track');
		}
		return track;
	}

	/** Edits the album on the ETag held now, as a save would. */
	function edit(library: Library, change: AlbumEdit): Promise<void> {
		return library.editAlbum(id, library.album(id)?.etag ?? '', change);
	}

	it('asks who it is after logging in: only /me says what they may do', async () => {
		const plain = { id: 1, name: 'S', role: 'admin', createdAt: '2026-01-01T00:00:00Z' } as const;
		const library = new Library(source({ login: () => Promise.resolve(plain) }));
		await library.login('simon', 'pw');
		expect(library.user?.canEditTags).toBe(true);
	});

	it('shows an album edit everywhere at once', async () => {
		const { library, album } = await opened();
		await edit(library, { all: { set: { date: ['1999'], genres: ['Noise'] } } });
		expect(library.album(id)?.year).toBe(1999);
		expect(library.album(id)?.etag).not.toBe(album.etag);
		expect(library.entry(id)?.year).toBe(1999);
		expect(library.recent.find((entry) => entry.id === id)?.year).toBe(1999);
		expect(library.albums.find((entry) => entry.id === id)?.year).toBe(1999);
		expect(library.playing?.album.year).toBe(1999);
	});

	it('has the library list read again after an edit, even one that fails', async () => {
		const { library } = await opened({ editAlbum: () => Promise.reject(new Error('stopped')) });
		await library.loadIndex();
		expect(library.indexStale).toBe(false);
		await expect(edit(library, { all: { set: { date: ['1999'] } } })).rejects.toThrow('stopped');
		expect(library.indexStale).toBe(true);
	});

	it('takes the album the server sends back: a track renumbered moves', async () => {
		const { library } = await opened();
		const first = trackAt(library, 0);
		await edit(library, { tracks: [{ id: first.id, set: { track: ['99'] } }] });
		expect(library.album(id)?.tracks.at(-1)?.id).toBe(first.id);
	});

	it('renames the cued track, and follows it if it moves', async () => {
		const { library } = await opened();
		const first = trackAt(library, 0);
		await edit(library, { tracks: [{ id: first.id, set: { title: ['New'], track: ['99'] } }] });
		expect(library.playing?.track.title).toBe('New');
		expect(library.playing?.index).toBe(9);
	});

	it('renames what is playing without touching the sound or the order', async () => {
		const { library, player, output, album } = await opened();
		player.play(album, 1);
		await vi.waitFor(() => {
			expect(player.status).toBe('playing');
		});
		output.elapse(30);
		const playing = trackAt(library, 1);
		const following = trackAt(library, 2);
		await edit(library, {
			tracks: [
				{ id: playing.id, set: { title: ['New'], track: ['99'] } },
				{ id: following.id, set: { title: ['Next'] } }
			]
		});
		expect(player.current?.title).toBe('New');
		expect(library.playing?.track.title).toBe('New');
		expect(library.playing?.index).toBe(1);
		expect(library.playing?.upNext.at(0)?.title).toBe('Next');
		expect(player.queue?.tracks.at(1)?.id).toBe(playing.id);
		expect(player.position).toBe(30);
		expect(output.playing).toBe(true);
	});

	it('leaves another album in the player alone', async () => {
		const { library, player } = await opened();
		const other = await createFixtureSource().album('night-ferry-94');
		player.play(other);
		await edit(library, { all: { set: { date: ['1999'] } } });
		expect(library.playing?.album.id).toBe('night-ferry-94');
		expect(player.queue?.album.year).toBe(other.year);
	});

	it('changes nothing when an edit is refused', async () => {
		const { library, album } = await opened({
			editAlbum: () => Promise.reject(new EditError({ kind: 'stale' }))
		});
		await expect(edit(library, { all: { set: { date: ['1999'] } } })).rejects.toBeInstanceOf(
			EditError
		);
		expect(library.album(id)).toEqual(album);
	});

	it('reads an album again over what it holds', async () => {
		const fixtures = createFixtureSource();
		const { library, album } = await opened({ album: fixtures.album });
		await fixtures.editAlbum(id, album.etag, { all: { set: { date: ['1999'] } } });
		await library.reread(id);
		expect(library.album(id)?.year).toBe(1999);
		expect(library.playing?.album.year).toBe(1999);
	});
});

describe('the library index', () => {
	const scanned = (endedAt: string): LibraryFacts => ({
		albums: 1,
		tracks: 8,
		durationMs: 0,
		roots: 1,
		...noTrouble,
		scan: {
			state: 'idle',
			read: 0,
			total: 0,
			last: {
				mode: 'quick',
				outcome: 'finished',
				startedAt: endedAt,
				endedAt,
				seen: 0,
				read: 0,
				added: 0,
				updated: 0,
				moved: 0,
				missing: 0,
				deleted: 0,
				issues: 0,
				skippedRoots: 0
			}
		}
	});

	it('reads it, and seeds the first frame of the album and artist a row leads to', async () => {
		const library = new Library(source(), { readyMemory: forgetful() });
		await library.loadIndex();
		if (library.index.kind !== 'ready') {
			throw new Error(library.index.kind);
		}
		const [group] = library.index.value.artists.filter(({ id }) => id !== undefined);
		const album = group?.albums.at(0);
		expect(library.entry(album?.id ?? '')).toMatchObject({
			title: album?.title,
			trackCount: album?.tracks.length
		});
		expect(library.artistName(group?.id ?? '')).toBe(group?.name);
	});

	it('keeps an entry that knows more than the index does', async () => {
		const library = new Library(source(), { readyMemory: forgetful() });
		await library.loadShell();
		const before = library.entry(library.recent.at(0)?.id ?? '');
		await library.loadIndex();
		expect(library.entry(before?.id ?? '')).toBe(before);
	});

	it('is stale once a scan ends, and fresh again once read', async () => {
		const library = new Library(source(), { readyMemory: forgetful() });
		library.facts = scanned('2026-11-06T10:00:00Z');
		await library.loadIndex();
		expect(library.indexStale).toBe(false);
		library.facts = scanned('2026-11-06T11:00:00Z');
		expect(library.indexStale).toBe(true);
		await library.loadIndex();
		expect(library.indexStale).toBe(false);
	});

	it('is stale after an edit', async () => {
		const library = new Library(source(), { readyMemory: forgetful() });
		await library.loadIndex();
		library.markIndexStale();
		expect(library.indexStale).toBe(true);
	});

	it('asks once however often it is asked', async () => {
		const libraryIndex = vi.fn<LibrarySource['libraryIndex']>(() =>
			Promise.resolve({ artists: [] })
		);
		const library = new Library(source({ libraryIndex }), { readyMemory: forgetful() });
		await Promise.all([library.loadIndex(), library.loadIndex()]);
		expect(libraryIndex).toHaveBeenCalledTimes(1);
	});

	it('keeps the list through a failed refresh, and says so with nothing to show', async () => {
		const libraryIndex = vi
			.fn<LibrarySource['libraryIndex']>()
			.mockRejectedValueOnce(new ServerError(500))
			.mockResolvedValueOnce({ artists: [] })
			.mockRejectedValueOnce(new ServerError(500));
		const library = new Library(source({ libraryIndex }), { readyMemory: forgetful() });
		await library.loadIndex();
		expect(library.index.kind).toBe('unavailable');
		await library.loadIndex();
		await library.loadIndex();
		expect(library.index.kind).toBe('ready');
	});

	it('forgets it with the session, and drops an answer from before', async () => {
		let answer: (index: { artists: never[] }) => void = () => undefined;
		const libraryIndex = vi.fn<LibrarySource['libraryIndex']>(
			() =>
				new Promise((resolve) => {
					answer = resolve;
				})
		);
		const library = new Library(source({ libraryIndex }), { readyMemory: forgetful() });
		const loading = library.loadIndex();
		library.reset();
		answer({ artists: [] });
		await loading;
		expect(library.index.kind).toBe('loading');
	});
});

describe('loading an album twice', () => {
	it('waits for the fetch already out, so the second caller finds the album there', async () => {
		let answer: (album: Awaited<ReturnType<LibrarySource['album']>>) => void = () => undefined;
		const fixture = createFixtureSource();
		const known = await fixture.album('glasshouse-66');
		const album = vi.fn<LibrarySource['album']>(
			() =>
				new Promise((resolve) => {
					answer = resolve;
				})
		);
		const library = new Library(source({ album }), { readyMemory: forgetful() });
		void library.loadAlbum(known.id);
		const second = library.loadAlbum(known.id);
		answer(known);
		await second;
		expect(library.album(known.id)).toBe(known);
		expect(album).toHaveBeenCalledTimes(1);
	});
});

describe('marks', () => {
	it('holds what the index and album pages say, and one press shows on both', async () => {
		const library = new Library(source(), { readyMemory: forgetful() });
		await library.loadIndex();
		expect(library.marks.hearted('artists', 'artist-low-tide-choir')).toBe(true);
		expect(library.marks.hearted('albums', 'glasshouse-66')).toBe(true);
		expect(library.marks.stars('glasshouse-66-1')).toBe(5);
		expect(library.marks.hearted('tracks', 'glasshouse-66-2')).toBe(false);

		library.marks.toggle('tracks', 'glasshouse-66-2');
		// The album page asks after the press: what it's given mustn't undo it.
		await library.loadAlbum('glasshouse-66');
		expect(library.marks.hearted('tracks', 'glasshouse-66-2')).toBe(true);
		await vi.waitFor(() => {
			expect(library.marks.failure).toBeNull();
		});

		// A reread after it landed agrees: the server has it.
		await library.reread('glasshouse-66');
		expect(library.marks.hearted('tracks', 'glasshouse-66-2')).toBe(true);
	});

	it('forgets the marks of the last session', async () => {
		const library = new Library(source(), { readyMemory: forgetful() });
		await library.loadIndex();
		library.reset();
		expect(library.marks.hearted('albums', 'glasshouse-66')).toBe(false);
		expect(library.marks.stars('glasshouse-66-1')).toBeNull();
	});
});

describe('Last.fm', () => {
	it('reads the connection with the shell, and forgets it with the session', async () => {
		const library = new Library(createFixtureSource({ lastfm: 'reconnect' }), {
			readyMemory: forgetful()
		});
		await library.loadShell();
		await vi.waitFor(() => {
			expect(library.lastfm?.state).toBe('reconnect');
		});
		library.reset();
		expect(library.lastfm).toBeNull();
	});

	it('keeps what it knew when the server does not answer', async () => {
		const failing = vi.fn(() => Promise.reject(new ServerError(500)));
		const library = new Library(source({ lastfm: failing }), { readyMemory: forgetful() });
		library.lastfm = { ...(await createFixtureSource().lastfm()), connected: true };
		await library.refreshLastfm();
		expect(library.lastfm.connected).toBe(true);
	});

	it('takes the switches the server sends back', async () => {
		const library = new Library(createFixtureSource({ lastfm: 'connected' }));
		await library.setLastfm({ loves: false });
		expect(library.lastfm).toMatchObject({ scrobble: true, loves: false });
		await library.disconnectLastfm();
		expect(library.lastfm).toMatchObject({ configured: true, connected: false });
	});

	it('says what started only for a real session', async () => {
		const nowPlaying = vi.fn(() => Promise.resolve());
		const real = new Library(source({ nowPlaying }));
		real.nowPlaying('t1');
		expect(nowPlaying).not.toHaveBeenCalled();
		await real.checkSession();
		real.nowPlaying('t1');
		expect(nowPlaying).toHaveBeenCalledExactlyOnceWith('t1');
		const simulated = new Library(source({ nowPlaying }), { simulated: true });
		await simulated.checkSession();
		simulated.nowPlaying('t2');
		expect(nowPlaying).toHaveBeenCalledOnce();
	});
});
