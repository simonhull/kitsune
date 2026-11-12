import type { Page } from '@playwright/test';
import { runs, test } from './fixture.ts';
import { atRest, playing } from './pages.ts';
import type { Wait } from './probe.ts';

/** The prolific artist's ID, and its newest album's: the first songs its page lists, unplayed. */
async function prolific(page: Page, name: string): Promise<{ id: string; album: string }> {
	const list = await page.request.get('/api/v1/artists');
	const { artists } = (await list.json()) as { artists: { id: string; name: string }[] };
	const id = artists.find((artist) => artist.name === name)?.id;
	if (id === undefined) {
		throw new Error(`no artist ${name}`);
	}
	const detail = await page.request.get(`/api/v1/artists/${id}`);
	const { albums } = (await detail.json()) as { albums: { id: string }[] };
	const album = albums.at(0)?.id;
	if (album === undefined) {
		throw new Error(`${name} has no album`);
	}
	return { id, album };
}

/** Top songs' rows, on the artist's page. */
function topRows(id: string): Wait {
	return {
		kind: 'selector',
		selector: 'main .songs .list > .row[data-track]',
		path: `/artists/${id}`
	};
}

/** The full list's rows past Top's ten. */
const allRows: Wait = {
	kind: 'selector',
	selector: 'main .songs.expanded .list > .row:nth-child(12)'
};

/** By album's first block, with its rows. */
const albumRows: Wait = {
	kind: 'selector',
	selector: 'main .songs.grouped .list > .album .row[data-track]'
};

async function openArtist(page: Page, id: string): Promise<void> {
	await page.goto(`/artists/${id}`);
	await page.locator('main .songs .list > .row[data-track]').first().waitFor();
	await atRest(page);
}

/**
 * Opens the full list, flat, from Top songs. The grouping is a saved pref, so a test before this one
 * may have left it by album, if its page closed before the save went: choose flat here, never assume it.
 */
async function expand(page: Page): Promise<void> {
	await page.locator('main .more-all').click();
	await page.locator('main .songs.expanded').waitFor();
	const flat = page.getByRole('button', { name: 'FLAT' });
	if ((await flat.getAttribute('aria-pressed')) !== 'true') {
		await flat.click();
	}
	await page.locator('main .songs.expanded:not(.grouped) .list > .row').nth(11).waitFor();
	await atRest(page);
}

test('Artist: open its songs from its album, and all of them', async ({ page, perf }) => {
	const { id, album } = await prolific(page, perf.manifest.prolific.name);
	const paint = perf.samples('artist-songs-paint');
	const grow = perf.samples('artist-expand');
	for (let i = 0; i < runs; i++) {
		// A fresh app each time: the artist's songs are read for the page, never held from before.
		await page.goto(`/album/${album}`);
		const link = page.locator(`main a[href="/artists/${id}"]`).first();
		await link.waitFor();
		await atRest(page);
		await perf.time(paint, 'click', topRows(id), () => link.click());
		await atRest(page);
		await perf.time(grow, 'click', allRows, () => page.locator('main .more-all').click());
	}
});

test('Artist: flat to by album', async ({ page, perf }) => {
	const { id } = await prolific(page, perf.manifest.prolific.name);
	await openArtist(page, id);
	await expand(page);
	const regroup = perf.samples('artist-regroup');
	for (let i = 0; i < runs; i++) {
		await perf.time(regroup, 'click', albumRows, () =>
			page.getByRole('button', { name: 'BY ALBUM' }).click()
		);
		await atRest(page);
		await page.getByRole('button', { name: 'FLAT' }).click();
		await page.locator('main .songs:not(.grouped) .list > .row').first().waitFor();
		await atRest(page);
	}
});

test('Artist: play a song from the list, and fling it', async ({ page, perf }) => {
	const { id } = await prolific(page, perf.manifest.prolific.name);
	await openArtist(page, id);
	await expand(page);
	const play = perf.samples('play-artist');
	const scroll = perf.samples('artist-scroll');
	// With nothing played, the list is newest first: its first songs are the ones with real audio.
	const ids = await page.evaluate(() =>
		[...document.querySelectorAll<HTMLElement>('main .songs .list > .row[data-track]')].map(
			(row) => row.dataset['track'] ?? ''
		)
	);
	for (let i = 0; i < runs; i++) {
		const track = ids.at(i) ?? '';
		const row = page.locator(`main .songs .row[data-track="${track}"]`);
		await row.scrollIntoViewIfNeeded();
		await perf.time(play, 'click', playing(track), () => row.locator('.play').click());
		await page.locator('aside[aria-label="Player"] button.pause').click();
	}
	await page.mouse.move(900, 600);
	for (let i = 0; i < runs; i++) {
		const frames = page.evaluate(() => window.perf.frames(2000));
		const until = Date.now() + 2000;
		const step = i % 2 === 0 ? 240 : -240;
		while (Date.now() < until) {
			await page.mouse.wheel(0, step);
		}
		scroll.add(Math.max(...(await frames)));
		await page.waitForTimeout(300);
	}
});
