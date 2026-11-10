import { expect } from '@playwright/test';
import { runs, test } from './fixture.ts';
import { albumTracks, atRest, nextLoudAlbum, openLibrary, playing } from './pages.ts';

/**
 * Plays albums from their covers on the Albums page, by artist, where the loud albums come first. The
 * pointer rests on each card first, as a person's does on the way to its play.
 */
test('Albums: play one from its cover', async ({ page, perf }) => {
	// The order is the person's, not the URL's: by name, the loud albums are spread through all of it.
	const sorted = await page.request.put('/api/v1/me/prefs/albums.sort', {
		headers: { 'X-Kitsune-Request': '1' },
		data: { by: 'artist', dir: 'asc' }
	});
	expect(sorted.status()).toBe(204);
	await page.goto('/albums');
	await page.locator('main .card').first().waitFor();
	const play = perf.samples('play-cover');
	const island = page.locator('aside[aria-label="Player"]');
	for (const { artist, title } of perf.manifest.loud.slice(0, runs)) {
		let id: string | undefined;
		for (let tries = 0; tries < 100 && id === undefined; tries++) {
			id = await page.evaluate(
				([wantArtist, wantTitle]) =>
					[...document.querySelectorAll('main .card')]
						.find(
							(card) =>
								card.querySelector('.title')?.textContent === wantTitle &&
								card.querySelector('.artist')?.textContent === wantArtist
						)
						?.querySelector('a.open')
						?.getAttribute('href')
						?.split('/')
						.pop(),
				[artist, title]
			);
			if (id === undefined) {
				await page.mouse.wheel(0, 600);
				await page.waitForTimeout(100);
			}
		}
		if (id === undefined) {
			throw new Error(`${title} by ${artist} isn't on the Albums page`);
		}
		const [first] = await albumTracks(page, id);
		if (first === undefined) {
			throw new Error(`${title} has no songs`);
		}
		const card = page.locator('main .card', { has: page.locator(`a.open[href="/album/${id}"]`) });
		await card.scrollIntoViewIfNeeded();
		await card.hover();
		await page.waitForTimeout(300);
		await perf.time(play, 'click', playing(first), () => card.locator('button.cover-play').click());
		await island.locator('button.pause').click();
	}
});

/**
 * Plays albums from their covers in the Library, a new loud album each time, so every press starts
 * one the page hasn't fetched. The pointer comes to rest on the play first, as a person's does: that
 * is what asks for the songs. The Library is at rest before the first press.
 */
test('Library: play an album from its cover', async ({ page, perf }) => {
	await openLibrary(page);
	await atRest(page);
	const play = perf.samples('play-library-cover');
	const island = page.locator('aside[aria-label="Player"]');
	const used = new Set<string>();
	for (let i = 0; i < runs; i++) {
		const album = await nextLoudAlbum(page, perf.manifest, used);
		const [first] = await albumTracks(page, album.id);
		if (first === undefined) {
			throw new Error(`${album.title} has no songs`);
		}
		const cover = page.locator('main section.album .cover-box', {
			has: page.locator(`a.art[href="/album/${album.id}"]`)
		});
		await cover.scrollIntoViewIfNeeded();
		const button = cover.locator('button.cover-play');
		await button.hover();
		await page.waitForTimeout(300);
		await perf.time(play, 'click', playing(first), () => button.click());
		await island.locator('button.pause').click();
	}
});
