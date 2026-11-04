import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createFixtureSource } from './fixtures';
import { Library, pileSize, type ReadyMemory } from './library.svelte';
import {
	AlbumNotFoundError,
	ServerError,
	UnauthorizedError,
	type AlbumEntry,
	type AlbumPage,
	type LibraryFacts,
	type LibrarySource
} from './source';

function source(overrides: Partial<LibrarySource> = {}): LibrarySource {
	return { ...createFixtureSource(), ...overrides };
}

function forgetful(seen = false): ReadyMemory {
	return { seen: () => seen, mark: () => undefined };
}

describe('Library', () => {
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

	it('lands a burst oldest first, and spreads it over the expected gap', async () => {
		const scan = new Scripted();
		const library = new Library(scan.source(), { readyMemory: forgetful() });
		await library.loadShell();
		scan.found = 10;
		await vi.advanceTimersByTimeAsync(3001);
		expect(ids(library)).toEqual(['album-0']);
		// 27 s over 10 cards: 2.7 s each, give or take a quarter.
		await vi.advanceTimersByTimeAsync(1800);
		expect(library.pile.length).toBeLessThanOrEqual(2);
		await vi.advanceTimersByTimeAsync(27_000);
		expect(ids(library).at(-1)).toBe('album-9');
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
