import { describe, expect, it, vi } from 'vitest';
import { createFixtureSource } from './fixtures';
import { Library } from './library.svelte';
import {
	AlbumNotFoundError,
	ServerError,
	UnauthorizedError,
	type AlbumPage,
	type LibrarySource
} from './source';

function source(overrides: Partial<LibrarySource> = {}): LibrarySource {
	return { ...createFixtureSource(), ...overrides };
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

	it('tells why a library is empty, and asks only for counts until it is not', async () => {
		vi.useFakeTimers();
		const empty = { albums: 0, tracks: 0, durationMs: 0, roots: 1, issues: 0 };
		const facts = vi.fn<LibrarySource['facts']>(() => Promise.resolve(empty));
		const ready = vi.fn<LibrarySource['ready']>(() => Promise.resolve(false));
		const albumsPage = vi.fn<LibrarySource['albumsPage']>(() => Promise.resolve({ albums: [] }));
		const library = new Library(source({ facts, ready, albumsPage }));
		await library.loadShell();
		expect(library.scanning).toBe(true);
		expect(library.playing).toBeNull();

		albumsPage.mockClear();
		await vi.advanceTimersByTimeAsync(5000);
		expect(albumsPage).not.toHaveBeenCalled();

		ready.mockResolvedValue(true);
		await vi.advanceTimersByTimeAsync(5000);
		expect(library.scanning).toBe(false);

		facts.mockResolvedValue({ ...empty, albums: 1 });
		await vi.advanceTimersByTimeAsync(5000);
		expect(library.arrivedLate).toBe(true);
		expect(albumsPage).toHaveBeenCalled();
		library.reset();
		vi.useRealTimers();
	});

	it('does not ask about a scan with no roots to scan', async () => {
		vi.useFakeTimers();
		const none = { albums: 0, tracks: 0, durationMs: 0, roots: 0, issues: 0 };
		const ready = vi.fn<LibrarySource['ready']>(() => Promise.resolve(false));
		const library = new Library(source({ facts: () => Promise.resolve(none), ready }));
		await library.loadShell();
		expect(library.scanning).toBe(false);
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
