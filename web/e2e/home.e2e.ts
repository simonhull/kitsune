import { expect, fixtures, test } from './fixture';

/** Home as a launchpad, on the fixtures: a way back in, two shuffles, and rows of covers. */

test('holding nothing, the card is the newest album, and its Play plays it', async ({ page }) => {
	await page.goto(fixtures('/'));
	const start = page.getByRole('region', { name: 'Start' });
	await expect(start.getByText('Just added', { exact: true })).toBeVisible();
	await expect(start.getByRole('link', { name: /^Glasshouse, / })).toBeVisible();
	// The card has it: the row beside it doesn't say it twice.
	const added = page.getByRole('region', { name: 'Just added' });
	await expect(added.getByRole('link', { name: 'All albums' }).first()).toBeAttached();
	await expect(added.locator('a[href="/album/glasshouse-66"]')).toHaveCount(0);

	await start.getByRole('button', { name: 'Play Glasshouse' }).click();
	const island = page.locator('aside[aria-label="Player"]');
	await expect(island.getByRole('button', { name: 'Pause' })).toBeVisible();
	await expect(start.getByText('Playing now', { exact: true })).toBeVisible();
});

test('a remembered record is the card, and its Play plays it in place', async ({ page }) => {
	await page.goto(fixtures('/?playback=phone'));
	const island = page.locator('aside[aria-label="Player"]');
	const song = (await island.locator('.song').textContent()) ?? '';
	expect(song).not.toBe('');
	const start = page.getByRole('region', { name: 'Start' });
	await expect(start.getByText('Pick up where you left off', { exact: true })).toBeVisible();
	await expect(start.getByRole('link', { name: new RegExp(`^${song}, `) })).toBeVisible();

	await start.getByRole('button', { name: `Play ${song}` }).click();
	await expect(island.getByRole('button', { name: 'Pause', exact: true })).toBeVisible();
	await expect(start.getByText('Playing now', { exact: true })).toBeVisible();
	await start.getByRole('button', { name: `Pause ${song}` }).click();
	await expect(island.getByRole('button', { name: 'Play', exact: true })).toBeVisible();
});

test('a cover’s Play plays that album, and the card takes it from the row', async ({ page }) => {
	await page.goto(fixtures('/'));
	const played = page.getByRole('region', { name: 'Recently played' });
	const cover = played.locator('.cover').first();
	const title = (await cover.locator('.title').textContent()) ?? '';
	await cover.hover();
	await cover.getByRole('button', { name: `Play ${title}` }).click();
	const island = page.locator('aside[aria-label="Player"]');
	await expect(island.getByRole('button', { name: 'Pause' })).toBeVisible();
	const start = page.getByRole('region', { name: 'Start' });
	await expect(start.getByText('Playing now', { exact: true })).toBeVisible();
	await expect(played.locator('.cover .title').getByText(title, { exact: true })).toHaveCount(0);
});

test('a cover opens its album', async ({ page }) => {
	await page.goto(fixtures('/'));
	const cover = page.getByRole('region', { name: 'On repeat' }).locator('.cover').first();
	await expect(cover.locator('.extra')).toHaveText(/^\d+ plays?$/);
	await cover.locator('a.open').click();
	await expect(page).toHaveURL(/\/album\//);
});

for (const name of ['Shuffle everything', 'Shuffle your favourites']) {
	test(`${name} starts a shuffle`, async ({ page }) => {
		await page.goto(fixtures('/'));
		await page.getByRole('button', { name }).click();
		const island = page.locator('aside[aria-label="Player"]');
		await expect(island.getByRole('button', { name: 'Pause' })).toBeVisible();
		await page.locator('a[href="/now-playing"]').first().click();
		await expect(page.getByRole('heading', { name: 'Up next' })).toBeVisible();
	});
}

test('someone who has played nothing gets no history rows, not empty ones', async ({ page }) => {
	await page.goto(fixtures('/?history=none'));
	await expect(page.getByRole('region', { name: 'Just added' })).toBeVisible();
	await expect(page.getByRole('region', { name: 'Recently played' })).toHaveCount(0);
	await expect(page.getByRole('region', { name: 'On repeat' })).toHaveCount(0);
});

test.describe('on a phone', () => {
	test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

	test('the rows scroll, and a cover has no play on it', async ({ page }) => {
		await page.goto(fixtures('/'));
		const added = page.getByRole('region', { name: 'Just added' });
		await expect(added.getByRole('link', { name: 'All albums' })).toBeVisible();
		const row = added.locator('.covers');
		expect(await row.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(true);
		await expect(added.locator('.cover-play').first()).toBeHidden();
	});
});
