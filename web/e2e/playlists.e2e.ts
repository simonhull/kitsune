import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** The songs of the playlist on screen, in order, by title. */
async function titles(page: Page): Promise<string[]> {
	return page
		.locator('section.songs .slot [data-track]')
		.evaluateAll((rows) => rows.map((row) => row.getAttribute('aria-label') ?? ''));
}

/** A row of the playlist on screen, by its place, from 0. */
const rowAt = (page: Page, at: number) => page.locator('section.songs .slot [data-track]').nth(at);

async function openFallingUp(page: Page): Promise<void> {
	await page.goto(fixtures('/playlists'));
	await page.getByRole('link', { name: /Falling Up/ }).click();
	await expect(page).toHaveURL(/\/playlists\/falling-up/);
	await expect(rowAt(page, 0)).toBeVisible();
}

test('the Playlists page lists them and makes one in place, named New playlist when left blank', async ({
	page
}) => {
	await page.goto(fixtures('/playlists'));
	await expect(page.getByRole('heading', { name: 'Playlists', exact: true })).toBeVisible();
	await expect(page.getByText('5 PLAYLISTS · 1 SMART · 2 FROM FILES')).toBeVisible();
	await expect(page.getByRole('link', { name: /Falling Up.*15 SONGS/ })).toBeVisible();

	await page.getByRole('button', { name: 'New playlist' }).click();
	await page.getByLabel('NAME IT').fill('Road trip');
	await page.getByRole('button', { name: 'Make it' }).click();
	await expect(page.getByRole('link', { name: /Road trip.*EMPTY/ })).toBeVisible();

	await page.getByRole('button', { name: 'New playlist' }).click();
	await page.keyboard.press('Enter');
	await expect(page.getByRole('link', { name: /^New playlist/ })).toBeVisible();
	await expect(page.getByText('7 PLAYLISTS · 1 SMART · 2 FROM FILES')).toBeVisible();

	// Leave it puts the dashed card back, making nothing.
	await page.getByRole('button', { name: 'New playlist' }).click();
	await page.getByRole('button', { name: 'Leave it' }).click();
	await expect(page.getByText('7 PLAYLISTS · 1 SMART · 2 FROM FILES')).toBeVisible();
});

test('a playlist renames in place: Enter saves, Esc puts the name back', async ({ page }) => {
	await openFallingUp(page);
	await expect(
		page.getByText(/PLAYLIST · 15 SONGS · 5 ALBUMS · .* · FROM NAVIDROME/)
	).toBeVisible();
	const name = page.getByRole('textbox', { name: 'Playlist name' });
	await name.fill('Falling Down');
	await name.press('Escape');
	await expect(name).toHaveValue('Falling Up');
	await name.fill('Falling Down');
	await name.press('Enter');
	await expect(name).toHaveValue('Falling Down');
	await page.goto(fixtures(new URL(page.url()).pathname));
	await expect(page.getByRole('textbox', { name: 'Playlist name' })).toHaveValue('Falling Down');
});

test('a song dragged by its grip lands where the fox line says, and stays there after a reload', async ({
	page
}) => {
	await openFallingUp(page);
	const before = await titles(page);
	const grip = rowAt(page, 0).locator('.handle');
	const from = await grip.boundingBox();
	const target = await rowAt(page, 3).boundingBox();
	if (from === null || target === null) {
		throw new Error('no rows to drag between');
	}
	await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
	await page.mouse.down();
	await page.mouse.move(from.x + from.width / 2, target.y + target.height / 2, { steps: 6 });
	await expect(page.locator('.slot.moving')).toHaveCount(1);
	await expect(page.locator('.slot.below')).toHaveCount(1);
	await page.mouse.up();

	const moved = [...before.slice(1, 4), before[0], ...before.slice(4)];
	await expect.poll(() => titles(page)).toEqual(moved);
	await page.goto(fixtures(new URL(page.url()).pathname));
	await expect(rowAt(page, 0)).toBeVisible();
	await expect.poll(() => titles(page)).toEqual(moved);
});

test('Alt+↓ moves the focused song, and Del takes it out with an Undo', async ({ page }) => {
	await openFallingUp(page);
	const before = await titles(page);
	await rowAt(page, 0)
		.getByRole('button', { name: `Play ${before[0] ?? ''}` })
		.focus();
	await page.keyboard.press('Alt+ArrowDown');
	await expect.poll(() => titles(page)).toEqual([before[1], before[0], ...before.slice(2)]);

	await rowAt(page, 0)
		.getByRole('button', { name: `Play ${before[1] ?? ''}` })
		.focus();
	await page.keyboard.press('Delete');
	await expect.poll(() => titles(page)).toEqual([before[0], ...before.slice(2)]);
	const toast = page.getByRole('status').filter({ hasText: 'taken out of Falling Up' });
	await expect(toast).toContainText(`${before[1] ?? ''} taken out of Falling Up`);
	await toast.getByRole('button', { name: 'Undo' }).click();
	await expect.poll(() => titles(page)).toEqual([before[1], before[0], ...before.slice(2)]);
	await page.goto(fixtures(new URL(page.url()).pathname));
	await expect(rowAt(page, 0)).toBeVisible();
	await expect.poll(() => titles(page)).toEqual([before[1], before[0], ...before.slice(2)]);
});

test('a row plays from itself on through the playlist, and Shuffle plays its songs', async ({
	page
}) => {
	await openFallingUp(page);
	const [, second] = await titles(page);
	await rowAt(page, 1)
		.getByRole('button', { name: `Play ${second ?? ''}` })
		.click();
	const island = page.locator('aside[aria-label="Player"]');
	await expect(island.locator('.song')).toHaveText(second ?? '');

	await page.getByRole('button', { name: 'Shuffle', exact: true }).first().click();
	const songs = await titles(page);
	await expect
		.poll(async () => songs.includes((await island.locator('.song').textContent()) ?? ''))
		.toBe(true);
});

test('Delete playlist asks in place first, and Keep it keeps it', async ({ page }) => {
	await openFallingUp(page);
	await page.getByRole('button', { name: 'More for this playlist' }).click();
	await page.getByRole('menuitem', { name: 'Delete playlist' }).click();
	const confirm = page.getByRole('group', { name: 'Delete the playlist' });
	await expect(confirm).toContainText('Its songs stay in your library');
	await confirm.getByRole('button', { name: 'Keep it' }).click();
	await expect(confirm).toHaveCount(0);

	await page.getByRole('button', { name: 'More for this playlist' }).click();
	await page.getByRole('menuitem', { name: 'Delete playlist' }).click();
	await page.getByRole('button', { name: 'Delete playlist' }).click();
	await expect(page).toHaveURL(/\/playlists(\?|$)/);
	await expect(page.getByRole('link', { name: /Falling Up/ })).toHaveCount(0);
	await expect(page.getByText('4 PLAYLISTS · 1 SMART · 2 FROM FILES')).toBeVisible();
});

test('a playlist on a phone: two lines a song, the grip on each, the buttons under the name', async ({
	page
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await openFallingUp(page);
	await expect(page.getByText('PLAYLIST · 15 SONGS · 1 H')).toBeVisible();
	await expect(rowAt(page, 0).locator('.handle')).toBeVisible();
	await expect(page.locator('.header[data-song-header]')).toHaveCount(0);
	const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
	expect(overflow).toBeLessThanOrEqual(0);
});
