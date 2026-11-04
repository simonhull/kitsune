import { afterEach, describe, expect, it, vi } from 'vitest';
import { createFixtureSource } from './fixtures';
import { AlbumNotFoundError, LoginError, type LibrarySource } from './source';

const source: LibrarySource = createFixtureSource();

afterEach(() => {
	vi.useRealTimers();
});

describe('fixtures', () => {
	it('serves an album for every list entry, with the same first-frame fields', async () => {
		const pages = await Promise.all([source.albumsPage('added'), source.albumsPage('name')]);
		for (const entry of pages.flatMap((page) => page.albums)) {
			const album = await source.album(entry.id);
			expect(album).toMatchObject(entry);
			expect(album.tracks.length).toBe(entry.trackCount);
		}
	});

	it('lists the newest album first and honors a limit', async () => {
		const { albums, nextCursor } = await source.albumsPage('added', undefined, 3);
		expect(albums.map((album) => album.title)).toEqual(['Glasshouse', 'Paper Suns', 'Red Signal']);
		expect(nextCursor).toBeUndefined();
	});

	it('sorts by name', async () => {
		const titles = (await source.albumsPage('name')).albums.map((album) => album.title);
		expect(titles).toEqual([...titles].sort((a, b) => a.localeCompare(b)));
	});

	it('keeps one album off Home for the no-card path', async () => {
		const listed = (await source.albumsPage('name')).albums.map((entry) => entry.id);
		expect(listed).not.toContain('tin-lantern-78');
		await expect(source.album('tin-lantern-78')).resolves.toBeDefined();
	});

	it('has a user, and turns away a login with no name', async () => {
		expect((await source.me()).name).toBe('Simon');
		await expect(source.login('', 'x')).rejects.toBeInstanceOf(LoginError);
	});

	it('rejects an unknown album', async () => {
		await expect(source.album('nope')).rejects.toBeInstanceOf(AlbumNotFoundError);
	});

	it('honors latency', async () => {
		vi.useFakeTimers();
		const slow = createFixtureSource({ latencyMs: 300 });
		let settled = false;
		const pending = slow.facts().then(() => {
			settled = true;
		});
		await vi.advanceTimersByTimeAsync(299);
		expect(settled).toBe(false);
		await vi.advanceTimersByTimeAsync(1);
		await pending;
		expect(settled).toBe(true);
	});
});
