import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SilentOutput } from '#lib/player/output.ts';
import { Player } from '#lib/player/player.svelte.ts';
import { createFixtureSource } from './fixtures';
import { Library, pileSize, type ReadyMemory } from './library.svelte';
import {
	AlbumNotFoundError,
	ArtistNotFoundError,
	ServerError,
	UnauthorizedError,
	type AlbumEntry,
	type AlbumPage,
	type LibraryFacts,
	type LibrarySource,
	type Track
} from './source';

function source(overrides: Partial<LibrarySource> = {}): LibrarySource {
	return { ...createFixtureSource(), ...overrides };
}

function track(id: string): Track {
	return { id, title: id, artists: [], durationMs: 100_000, format: 'FLAC' };
}

function forgetful(seen = false): ReadyMemory {
	return { seen: () => seen, mark: () => undefined };
}

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
		const empty = { albums: 0, tracks: 0, durationMs: 0, roots: 1, issues: 0 };
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
		const none = { albums: 0, tracks: 0, durationMs: 0, roots: 0, issues: 0 };
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
		return { albums, tracks: albums * 8, durationMs: 0, roots: 1, issues: 0 };
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
					tracks: [{ id: `${id}-1`, title: 'One', artists: [], durationMs: 1000, format: 'FLAC' }],
					durationMs: 0,
					formats: [],
					addedAt: ''
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
