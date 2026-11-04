import { afterEach, describe, expect, it, vi } from 'vitest';
import { createFixtureSource } from './fixtures';
import { AlbumNotFoundError, type LibrarySource } from './source';

const source: LibrarySource = createFixtureSource();

afterEach(() => {
	vi.useRealTimers();
});

describe('fixtures', () => {
	it('serves an album for every list entry, with the same first-frame fields', async () => {
		const home = await source.home();
		for (const entry of [...home.onRepeat, ...home.recentlyAdded, home.continueListening.album]) {
			const album = await source.album(entry.id);
			expect(album).toMatchObject(entry);
			expect(album.tracks.length).toBeGreaterThan(0);
		}
	});

	it('plays Glasshouse track 2 by default', async () => {
		const playing = await source.nowPlaying();
		expect(playing.album.title).toBe('Glasshouse');
		expect(playing.track.title).toBe('Salt in the Lungs');
		expect(playing.trackCount).toBe(10);
	});

	it('plays the album it is told to', async () => {
		const other = createFixtureSource({ playingAlbumId: 'tin-lantern-78' });
		expect((await other.nowPlaying()).album.title).toBe('Tin Lantern');
	});

	it('keeps one album off Home for the no-card path', async () => {
		const home = await source.home();
		const listed = [...home.onRepeat, ...home.recentlyAdded].map((entry) => entry.id);
		expect(listed).not.toContain('tin-lantern-78');
		await expect(source.album('tin-lantern-78')).resolves.toBeDefined();
	});

	it('counts the same issues it lists', async () => {
		const { health } = await source.home();
		const listed = (health.detail.match(/\d+/g) ?? []).map(Number);
		expect(listed.reduce((sum, count) => sum + count, 0)).toBe(health.issues);
	});

	it('rejects an unknown album', async () => {
		await expect(source.album('nope')).rejects.toBeInstanceOf(AlbumNotFoundError);
	});

	it('honors latency', async () => {
		vi.useFakeTimers();
		const slow = createFixtureSource({ latencyMs: 300 });
		let settled = false;
		const pending = slow.home().then(() => {
			settled = true;
		});
		await vi.advanceTimersByTimeAsync(299);
		expect(settled).toBe(false);
		await vi.advanceTimersByTimeAsync(1);
		await pending;
		expect(settled).toBe(true);
	});
});
