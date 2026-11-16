import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** A row of the playlist on screen, by its place, from 0. */
const rowAt = (page: Page, at: number) => page.locator('section.songs .slot [data-track]').nth(at);

/** The card of lines that found no song. */
const missing = (page: Page) => page.getByRole('region', { name: 'Lines that found no song' });

async function openEssentials(page: Page): Promise<void> {
	await page.goto(fixtures('/playlists'));
	await page.getByRole('link', { name: /Falling Up essentials/ }).click();
	await expect(page).toHaveURL(/\/playlists\/falling-up-essentials/);
	await expect(rowAt(page, 0)).toBeVisible();
}

async function openExport(page: Page): Promise<void> {
	await page.goto(fixtures('/playlists'));
	await page.getByRole('link', { name: /^Falling Up 15 SONGS/ }).click();
	await expect(rowAt(page, 0)).toBeVisible();
	await page.getByRole('button', { name: 'More for this playlist' }).click();
	await page.getByRole('menuitem', { name: 'Export .m3u' }).click();
	await expect(page.getByText('EXPORT · 15 SONGS AS .M3U8')).toBeVisible();
}

test('From files: the files everyone sees, below your own, each with its paper tag', async ({
	page
}) => {
	await page.goto(fixtures('/playlists'));
	await expect(page.getByText('5 PLAYLISTS · 1 SMART · 2 FROM FILES')).toBeVisible();
	const files = page.getByRole('region', { name: 'From files' });
	await expect(files).toContainText('EVERYONE ON THIS SERVER SEES THESE');
	const essentials = files.getByRole('link', { name: /Falling Up essentials/ });
	await expect(essentials).toContainText('M3U8');
	await expect(essentials).toContainText(/19 SONGS · .* · 3 NOT FOUND/);
	const drive = files.getByRole('link', { name: /Night drive/ });
	await expect(drive).toContainText('M3U');
	await expect(drive).not.toContainText('NOT FOUND');
	// Your own aren't in it.
	await expect(files.getByRole('link', { name: /Slow mornings/ })).toHaveCount(0);
});

test('a file page: a plain name, no grip, its FROM line, and a menu with no Rename or Delete', async ({
	page
}) => {
	await openEssentials(page);
	await expect(
		page.getByText(/^FILE PLAYLIST · 19 SONGS · 7 ALBUMS · .* · EVERYONE HERE$/)
	).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Falling Up essentials' })).toBeVisible();
	await expect(page.getByRole('textbox', { name: 'Playlist name' })).toHaveCount(0);
	await expect(page.getByText('Playlists/Falling Up essentials.m3u8')).toBeVisible();
	await expect(page.getByText(/READ 2 DAYS AGO/)).toBeVisible();
	await expect(rowAt(page, 0).locator('.handle')).toHaveCount(0);
	await expect(page.getByText(/FROM A FILE, SO EVERYONE HERE SEES IT/)).toBeVisible();

	await page.getByRole('button', { name: 'More for this playlist' }).click();
	await expect(page.getByRole('menuitem', { name: /Copy to a playlist/ })).toBeVisible();
	await expect(page.getByRole('menuitem', { name: 'Export .m3u' })).toBeVisible();
	await expect(page.getByRole('menuitem', { name: /Rename|Delete/ })).toHaveCount(0);
	await expect(page.getByText(/NO RENAME, DRAG OR TAKE OUT/)).toBeVisible();
});

test('the not-found pill opens the card of lines and closes it', async ({ page }) => {
	await openEssentials(page);
	const pill = page.getByRole('button', { name: /3 SONGS NOT FOUND/ });
	await expect(missing(page)).toHaveCount(0);
	await pill.click();
	await expect(pill).toHaveAttribute('aria-expanded', 'true');
	await expect(missing(page)).toContainText('3 lines found no song');
	await expect(missing(page)).toContainText('LINE 9');
	await expect(missing(page)).toContainText('NO SONG AT THIS PATH');
	await expect(missing(page)).toContainText(
		'Fangs! has 04 Golden Arrows.flac: the file names the .mp3.'
	);
	await expect(missing(page)).toContainText('https://… (a web address)');
	await expect(missing(page)).toContainText('OUTSIDE THE LIBRARY');
	await pill.click();
	await expect(missing(page)).toHaveCount(0);

	await page.getByRole('button', { name: 'More for this playlist' }).click();
	await page.getByRole('menuitem', { name: /Show what’s not found/ }).click();
	await expect(missing(page)).toBeVisible();
	await missing(page).getByRole('button', { name: 'Hide' }).click();
	await expect(missing(page)).toHaveCount(0);
});

test('Copy to a playlist lands on a copy of your own, which moves', async ({ page }) => {
	await openEssentials(page);
	await page.getByRole('button', { name: 'Copy to a playlist' }).click();
	await expect(page).toHaveURL(/\/playlists\/playlist-/);
	await expect(page.getByRole('textbox', { name: 'Playlist name' })).toHaveValue(
		'Falling Up essentials (copy)'
	);
	await expect(page.getByText(/^PLAYLIST · 19 SONGS/)).toBeVisible();
	await expect(rowAt(page, 0).locator('.handle')).toBeVisible();
});

test('Export: Download is a link to the file, named after the playlist', async ({ page }) => {
	await openExport(page);
	await expect(
		page.getByText('Falling Up.m3u8 · 15 LINES · #EXTINF WITH ARTIST - TITLE')
	).toBeVisible();
	const download = page.getByRole('link', { name: 'Download' });
	await expect(download).toHaveAttribute('download', 'Falling Up.m3u8');
	await expect(download).toHaveAttribute('href', /^data:audio\/x-mpegurl/);
	const href = (await download.getAttribute('href')) ?? '';
	expect(decodeURIComponent(href.slice(href.indexOf(',') + 1))).toMatch(/^#EXTM3U\n#EXTINF:\d+,/);
	await page.getByRole('button', { name: 'Leave it' }).click();
	await expect(page.getByRole('button', { name: 'Play', exact: true })).toBeVisible();
});

test('Save here meets the file there, asks, and Replace it saves', async ({ page }) => {
	await openExport(page);
	await expect(page.getByText('music / Playlists')).toBeVisible();
	await page.getByRole('button', { name: 'Save here' }).click();
	const ask = page.getByRole('alertdialog', { name: 'The file is there already' });
	await expect(ask).toContainText('Falling Up.m3u8 is already there.');
	await expect(ask).toContainText('a file playlist of 148 songs, last changed 3 weeks ago');
	await expect(ask.getByRole('button', { name: 'Save as Falling Up 2.m3u8' })).toBeVisible();
	await ask.getByRole('button', { name: 'Replace it' }).click();
	await expect(page.getByText('Saved Falling Up.m3u8')).toBeVisible();
	await expect(page.getByText(/In music \/ Playlists\. It shows under From files/)).toBeVisible();
	await page.getByRole('button', { name: 'See it in Playlists' }).click();
	await expect(page).toHaveURL(/\/playlists(\?|$)/);
	await expect(
		page
			.getByRole('region', { name: 'From files' })
			.getByRole('link', { name: /Falling Up 15 SONGS/ })
	).toBeVisible();
});

test('Keep it leaves the file alone; Save as takes the next free name', async ({ page }) => {
	await openExport(page);
	await page.getByRole('button', { name: 'Save here' }).click();
	await page.getByRole('button', { name: 'Keep it' }).click();
	await expect(page.getByRole('button', { name: 'Save here' })).toBeVisible();
	await page.getByRole('button', { name: 'Save here' }).click();
	await page.getByRole('button', { name: 'Save as Falling Up 2.m3u8' }).click();
	await expect(page.getByText('Saved Falling Up 2.m3u8')).toBeVisible();
});

test('on a phone: the file page, its card under the buttons, and Download only', async ({
	page
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await openEssentials(page);
	await expect(page.getByText('FILE · 19 SONGS · 1 H 22 · EVERYONE')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Copy to a playlist' })).toHaveCount(0);
	const pill = page.getByRole('button', { name: /3 NOT FOUND/ });
	await pill.click();
	await expect(missing(page)).toContainText('Fangs! has 04 Golden Arrows.flac.');
	const card = await missing(page).boundingBox();
	const play = await page.getByRole('button', { name: 'Play', exact: true }).boundingBox();
	expect(card?.y ?? 0).toBeGreaterThan(play?.y ?? Infinity);
	expect(card?.width ?? 0).toBeLessThanOrEqual(390 - 32);

	await page.getByRole('button', { name: 'More for this playlist' }).click();
	await page.getByRole('menuitem', { name: 'Export .m3u' }).click();
	await expect(page.getByRole('link', { name: 'Download' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Save here' })).toHaveCount(0);
});
