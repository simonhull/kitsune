import { expect, fixtures, test } from './fixture';

/**
 * Resume playback, on the fixtures: their record lives in the tab's sessionStorage, so a reload of
 * a fixtures page comes back to it, as a reload comes back to the server's.
 */

test('there is no island until something plays', async ({ page }) => {
	await page.goto(fixtures('/'));
	await expect(page.getByRole('navigation', { name: 'Main' })).toBeVisible();
	const recent = page.getByRole('region', { name: 'Recently added' });
	await expect(recent.locator('.recent-row > .recent').first()).toBeVisible();
	const island = page.locator('aside[aria-label="Player"]');
	await expect(island).toHaveCount(0);

	await page.keyboard.press('Space');
	await expect(island).toHaveCount(0);

	const card = recent.locator('.recent-row > .recent').first();
	await card.hover();
	await card.getByRole('button', { name: /^Play / }).click();
	await expect(island).toBeVisible();
	await expect(island.getByRole('button', { name: 'Pause' })).toBeVisible();
});

test('a refresh comes back paused, where it was', async ({ page }) => {
	await page.goto(fixtures('/'));
	const recent = page.getByRole('region', { name: 'Recently added' });
	const card = recent.locator('.recent-row > .recent').first();
	await card.hover();
	await card.getByRole('button', { name: /^Play / }).click();
	const island = page.locator('aside[aria-label="Player"]');
	await expect(island.locator('.time')).toHaveText(/^0:0[5-9] \//, { timeout: 15_000 });
	await island.getByRole('button', { name: 'Pause' }).click();
	const paused = (await island.locator('.time').textContent())?.split(' / ')[0] ?? '';
	expect(paused).toMatch(/^0:\d\d$/);
	// The pause saves within a moment: give it that, as a person reaching for reload would.
	await page.waitForTimeout(600);

	await page.reload();
	await expect(island.getByRole('button', { name: 'Play', exact: true })).toBeVisible();
	await expect(island.locator('.time')).toHaveText(new RegExp(`^${paused} /`));

	await island.locator('a.now').click();
	await expect(page.getByRole('slider', { name: 'Seek' })).toHaveAttribute(
		'aria-valuetext',
		new RegExp(`^${paused} of`)
	);
	await page.getByRole('button', { name: 'Play', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Pause', exact: true })).toBeVisible();
});

test('a record from another device says so, until Play', async ({ page }) => {
	await page.goto(fixtures('/?playback=phone'));
	const island = page.locator('aside[aria-label="Player"]');
	const tag = island.getByText(/^FROM YOUR PHONE · 2 H AGO$/);
	await expect(tag).toBeVisible();
	await expect(island.getByRole('button', { name: 'Play', exact: true })).toBeVisible();
	await island.getByRole('button', { name: 'Play', exact: true }).click();
	await expect(island.getByRole('button', { name: 'Pause', exact: true })).toBeVisible();
	await expect(tag).toHaveCount(0);
});

test('a song gone from the library comes back as the one after it', async ({ page }) => {
	await page.goto(fixtures('/?playback=gone'));
	const island = page.locator('aside[aria-label="Player"]');
	await expect(island.getByText('THAT ONE’S GONE · NEXT IN LINE')).toBeVisible();
	await expect(island.locator('.song')).toHaveText('Breakwater');
	await expect(island.locator('.time')).toHaveText(/^0:00 \//);
});
