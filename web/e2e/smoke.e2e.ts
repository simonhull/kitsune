import { expect, fixtures, test } from './fixture';

test('the home page opens on the fixtures without an error', async ({ page }) => {
	await page.goto(fixtures('/'));
	await expect(page.getByRole('navigation', { name: 'Main' })).toBeVisible();
});

test('the library opens and a song plays', async ({ page }) => {
	await page.goto(fixtures('/'));
	await page.getByRole('link', { name: 'Library', exact: true }).click();

	const row = page.locator('[data-track]').first();
	await expect(row).toBeVisible();
	const title = await row.getAttribute('aria-label');
	expect(title).toBeTruthy();

	await row.getByRole('button', { name: `Play ${title}` }).click();
	await expect(page.locator('a.now').getByText(title ?? '', { exact: true })).toBeVisible();
});

test('now playing opens while a shuffle plays', async ({ page }) => {
	await page.goto(fixtures('/'));
	await page.getByRole('link', { name: 'Library', exact: true }).click();
	await page.getByRole('button', { name: 'Shuffle the library' }).click();

	await page.locator('a[href="/now-playing"]').first().click();
	await expect(page.getByRole('heading', { name: 'Up next' })).toBeVisible();
});

test('now playing survives a song queued twice', async ({ page }) => {
	await page.goto(fixtures('/'));
	await page.getByRole('link', { name: 'Library', exact: true }).click();

	const rows = page.locator('[data-track]');
	await expect(rows.nth(1)).toBeVisible();
	const first = await rows.nth(0).getAttribute('aria-label');
	const second = await rows.nth(1).getAttribute('aria-label');
	await rows
		.nth(0)
		.getByRole('button', { name: `Play ${first}` })
		.click();

	await rows
		.nth(1)
		.getByRole('button', { name: `More for ${second}` })
		.click();
	await page.getByRole('menuitem', { name: 'Play next' }).click();

	await page.locator('a[href="/now-playing"]').first().click();
	await expect(page.getByRole('region', { name: 'Up next' })).toBeVisible();
});
