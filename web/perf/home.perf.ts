import { expect, type Page } from '@playwright/test';
import { runs, test } from './fixture.ts';
import type { Manifest } from './library.ts';
import { albumTracks, atRest, playing } from './pages.ts';
import type { Wait } from './probe.ts';

/** Home's first history row is on screen. */
const homeRows: Wait = {
	kind: 'selector',
	selector: 'main section[aria-label="Recently played"] .cover',
	path: '/'
};

/** The Continue card holding a paused record: its button offers Play. */
const continuePlay = 'main section[aria-label="Start"] button.play[aria-label^="Play "]';

/** How many loud albums get a play, so Recently played always has a new one to press. */
const playedAlbums = 25;

/** The loud albums' IDs, from the album list. */
async function loudAlbumIds(page: Page, manifest: Manifest): Promise<string[]> {
	const loud = new Set(manifest.loud.map(({ artist, title }) => `${artist}\u0000${title}`));
	const ids: string[] = [];
	let cursor: string | undefined;
	do {
		const query = new URLSearchParams({ sort: 'name', limit: '200' });
		if (cursor !== undefined) {
			query.set('cursor', cursor);
		}
		const response = await page.request.get(`/api/v1/albums?${query.toString()}`);
		const body = (await response.json()) as {
			albums: { id: string; title: string; artist: string }[];
			nextCursor?: string;
		};
		ids.push(
			...body.albums
				.filter(({ artist, title }) => loud.has(`${artist}\u0000${title}`))
				.map(({ id }) => id)
		);
		cursor = body.nextCursor;
	} while (cursor !== undefined && ids.length < playedAlbums);
	return ids.slice(0, playedAlbums);
}

/**
 * Gives loud albums a play each, a minute apart, so Home has a history to show. Their client IDs
 * are fixed, so a second run on the same server adds nothing.
 */
async function seedPlays(page: Page, manifest: Manifest): Promise<void> {
	const ids = await loudAlbumIds(page, manifest);
	const now = Date.now();
	for (const [i, id] of ids.entries()) {
		const [first] = await albumTracks(page, id);
		if (first === undefined) {
			throw new Error(`album ${id} has no songs`);
		}
		const response = await page.request.post('/api/v1/me/plays', {
			headers: { 'X-Kitsune-Request': '1' },
			data: {
				trackId: first,
				playedAt: new Date(now - i * 60_000).toISOString(),
				source: 'perf',
				clientId: `perf-home-${id}`
			}
		});
		expect(response.status()).toBe(204);
	}
}

/**
 * Plays albums from their covers in Home's Recently played row, a new one while the row shows one
 * (the album playing moves to the Continue card, and the one before comes back), then round the row
 * again. The pointer rests on the cover's play first, as a person's does on the way to it, which
 * asks for the songs: a press finds them here either way, as on the Albums page.
 */
test('Home: play an album from its cover', async ({ page, perf }) => {
	await seedPlays(page, perf.manifest);
	await page.goto('/');
	await page.locator(homeRows.selector).first().waitFor();
	await atRest(page);
	const play = perf.samples('play-home-cover');
	const island = page.locator('aside[aria-label="Player"]');
	const used = new Set<string>();
	for (let i = 0; i < runs; i++) {
		const covers = page.locator('main section[aria-label="Recently played"] .cover');
		const hrefs = await covers
			.locator('a.open')
			.evaluateAll((links) => links.map((link) => link.getAttribute('href') ?? ''));
		// A new album while the row has one; then round the row again, never the one playing.
		const href = hrefs.find((each) => !used.has(each)) ?? hrefs[i % hrefs.length];
		if (href === undefined) {
			throw new Error('Recently played is empty');
		}
		used.add(href);
		const [first] = await albumTracks(page, href.split('/').pop() ?? '');
		if (first === undefined) {
			throw new Error(`${href} has no songs`);
		}
		const button = covers
			.filter({ has: page.locator(`a.open[href="${href}"]`) })
			.locator('button.cover-play');
		await button.hover();
		await page.waitForTimeout(300);
		await perf.time(play, 'click', playing(first), () => button.click());
		await island.locator('button.pause').click();
	}
});

/**
 * Loads Home with a record saved, and times the Continue card's Play to sound: the song is loaded
 * by the press, as after a reload. The record is the one the last press and pause left.
 */
test('Home: pick up where you left off', async ({ page, perf }) => {
	await page.goto('/');
	const island = page.locator('aside[aria-label="Player"]');
	const press = perf.samples('play-continue');
	for (let i = 0; i < runs; i++) {
		await page.goto('/');
		const button = page.locator(continuePlay);
		await button.waitFor();
		await atRest(page);
		const response = await page.request.get('/api/v1/me/playback');
		const record = (await response.json()) as { tracks: { id: string }[]; index: number };
		const song = record.tracks[record.index]?.id;
		if (song === undefined) {
			throw new Error('no record to pick up');
		}
		await button.hover();
		await page.waitForTimeout(300);
		await perf.time(press, 'click', playing(song), () => button.click());
		const saved = page.waitForResponse(
			(each) => each.url().endsWith('/api/v1/me/playback') && each.request().method() === 'PUT'
		);
		await island.locator('button.pause').click();
		await saved;
	}
});

/**
 * Comes back to Home from the Albums page, and times the click to Home's rows on screen: they
 * come from the copy the shell already holds, never from a request on the way.
 */
test('Home: rows painted on the way back', async ({ page, perf }) => {
	const paint = perf.samples('home-paint');
	for (let i = 0; i < runs; i++) {
		await page.goto('/albums');
		await page.locator('main .card').first().waitFor();
		await atRest(page);
		await perf.time(paint, 'click', homeRows, () =>
			page.locator('nav[aria-label="Main"] a[href="/"]').click()
		);
	}
});
