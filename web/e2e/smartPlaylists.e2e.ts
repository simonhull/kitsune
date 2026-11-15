import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** The live card's count, desktop or phone. */
const count = (page: Page) => page.getByTestId('smart-count');

/** A row of the playlist on screen, by its place, from 0. */
const rowAt = (page: Page, at: number) => page.locator('section.songs .slot [data-track]').nth(at);

/** The rules editor's Rules card. */
const rules = (page: Page) => page.getByRole('region', { name: 'Rules' });

async function openRested(page: Page): Promise<void> {
	await page.goto(fixtures('/playlists'));
	await page.getByRole('link', { name: /Rested and rated/ }).click();
	await expect(page).toHaveURL(/\/playlists\/rested-and-rated/);
	await expect(rowAt(page, 0)).toBeVisible();
}

test('a smart card wears its stamp, and New smart playlist makes one from a rule and its count', async ({
	page
}) => {
	await page.goto(fixtures('/playlists'));
	await expect(page.getByText('3 PLAYLISTS · 1 SMART · YOURS ALONE')).toBeVisible();
	const card = page.getByRole('link', { name: /Rested and rated/ });
	await expect(card).toContainText('SMART');
	await expect(card).toContainText(/3 SONGS · \d+ MIN · LIVE/);

	await page.getByRole('link', { name: /New smart playlist/ }).click();
	await expect(page).toHaveURL(/\/playlists\/smart/);
	await expect(page.getByRole('textbox', { name: 'Playlist name' })).toHaveValue(
		'New smart playlist'
	);
	await expect(rules(page).getByRole('combobox', { name: 'Field' })).toHaveValue('rating');
	await expect(count(page)).toHaveText('8');
	await expect(page.getByText('1 rule · not saved yet')).toBeVisible();

	await page.getByRole('textbox', { name: 'Playlist name' }).fill('Four and up');
	await page.getByRole('button', { name: 'Save rules' }).click();
	await expect(page).toHaveURL(/\/playlists\/playlist-/);
	await expect(page.getByRole('textbox', { name: 'Playlist name' })).toHaveValue('Four and up');
	await expect(page.getByText(/^SMART PLAYLIST · 8 SONGS · 4 ALBUMS · .* · LIVE$/)).toBeVisible();
	await expect(page.getByLabel('Its rules')).toContainText(/SONGS WHERE ALL OF\s*★★★★ and up/);
	await expect(page.locator('.hero > .mosaic')).toContainText('SMART');
	await expect(rowAt(page, 7)).toBeVisible();
});

test('Leave it on a new smart playlist makes nothing', async ({ page }) => {
	await page.goto(fixtures('/playlists'));
	await page.getByRole('link', { name: /New smart playlist/ }).click();
	await expect(count(page)).toHaveText('8');
	await page.getByRole('button', { name: 'Leave it' }).click();
	await expect(page).toHaveURL(/\/playlists(\?|$)/);
	// Once the editor's page has gone: a page leaving stays a moment as the next arrives.
	await expect(page.getByText('3 PLAYLISTS · 1 SMART · YOURS ALONE')).toHaveCount(1);
});

test('Edit rules: a star moves the count, a group comes and goes, and Esc leaves it unchanged', async ({
	page
}) => {
	await openRested(page);
	const sentence = page.getByLabel('Its rules');
	await expect(sentence).toContainText('★★★ and up');
	await page.getByRole('button', { name: 'Edit rules' }).click();
	await expect(page.getByText('EDITING THE RULES · SMART PLAYLIST')).toBeVisible();
	await expect(count(page)).toHaveText('3');
	await expect(page.getByText('Nothing changed')).toBeVisible();

	await rules(page)
		.getByRole('button', { name: /^5 stars/ })
		.click();
	await expect(count(page)).toHaveText('2');
	await expect(page.getByText('5 rules · not saved yet')).toBeVisible();
	await expect(page.getByText(/1 FEWER THAN SAVED/)).toBeVisible();
	// A second press on the same star takes off half.
	await rules(page)
		.getByRole('button', { name: /^5 stars/ })
		.click();
	await expect(rules(page).getByRole('group', { name: 'Rating 4½★' })).toBeVisible();

	await expect(rules(page).getByRole('button', { name: 'Group', exact: true })).toHaveCount(0);
	await rules(page).getByRole('button', { name: 'Take the group out' }).click();
	await rules(page).getByRole('button', { name: 'Group', exact: true }).click();
	await expect(rules(page).getByText('of these', { exact: true })).toBeVisible();
	await expect(rules(page).getByText('NOT COUNTED YET')).toBeVisible();
	await rules(page).getByRole('button', { name: 'Rule in this group' }).click();
	await expect(rules(page).getByText('NOT COUNTED YET')).toHaveCount(2);

	await page.keyboard.press('Escape');
	await expect(page.getByText('EDITING THE RULES · SMART PLAYLIST')).toHaveCount(0);
	await expect(page).toHaveURL(/\/playlists\/rested-and-rated/);
	await expect(sentence).toContainText('★★★ and up');
	await expect(sentence).toContainText('either from 2011 or before or 10+ plays');
	await expect(rowAt(page, 2)).toBeVisible();
});

test('Back leaves the rules editor and stays on the playlist; Save rules keeps the change', async ({
	page
}) => {
	await openRested(page);
	await page.getByRole('button', { name: 'Edit rules' }).click();
	await rules(page)
		.getByRole('button', { name: /^5 stars/ })
		.click();
	await page.goBack();
	await expect(page.getByText('EDITING THE RULES · SMART PLAYLIST')).toHaveCount(0);
	await expect(page).toHaveURL(/\/playlists\/rested-and-rated/);
	await expect(page.getByLabel('Its rules')).toContainText('★★★ and up');

	await page.getByRole('button', { name: 'Edit rules' }).click();
	await rules(page)
		.getByRole('button', { name: /^5 stars/ })
		.click();
	await expect(count(page)).toHaveText('2');
	await page.getByRole('button', { name: 'Save rules' }).click();
	await expect(page.getByLabel('Its rules')).toContainText('★★★★★ and up');
	await expect(page.getByText(/^SMART PLAYLIST · 2 SONGS/)).toBeVisible();
});

test('the limit switch keeps at most so many, and only a limit offers how they were chosen', async ({
	page
}) => {
	await page.goto(fixtures('/playlists'));
	await page.getByRole('link', { name: /New smart playlist/ }).click();
	await expect(count(page)).toHaveText('8');
	const order = page.getByRole('combobox', { name: 'Order' });
	await expect(order.locator('option[value="chosen"]')).toHaveCount(0);

	await page.getByRole('switch', { name: 'Limit' }).click();
	await expect(order).toHaveValue('chosen');
	await page.getByRole('spinbutton', { name: 'How many' }).fill('3');
	await expect(count(page)).toHaveText('3');
	await expect(page.getByText(/8 MATCH, THE LIMIT KEEPS 3/)).toBeVisible();

	await page.getByRole('switch', { name: 'Limit' }).click();
	await expect(count(page)).toHaveText('8');
	await expect(order).toHaveValue('library');
	await expect(order.locator('option[value="chosen"]')).toHaveCount(0);
});

test('Copy to a playlist lands on a regular copy of the songs as they are now', async ({
	page
}) => {
	await openRested(page);
	await page.getByRole('button', { name: 'More for this playlist' }).click();
	await page.getByRole('menuitem', { name: 'Copy to a playlist' }).click();
	await expect(page).toHaveURL(/\/playlists\/playlist-/);
	await expect(page.getByRole('textbox', { name: 'Playlist name' })).toHaveValue(
		'Rested and rated (copy)'
	);
	await expect(page.getByText(/^PLAYLIST · 3 SONGS/)).toBeVisible();
	await expect(rowAt(page, 0).locator('.handle')).toBeVisible();
});

test('a smart page has no grip, no Del and no Take out; a row plays on through, and Shuffle plays its songs', async ({
	page
}) => {
	await openRested(page);
	await expect(page.getByText(/THE RULES SET THE ORDER, SO NOTHING DRAGS/)).toBeVisible();
	await expect(page.locator('section.songs .handle')).toHaveCount(0);
	await expect(page.locator('.header .rule')).toHaveText(['RATING', 'PLAYS', 'LAST PLAYED']);

	const first = (await rowAt(page, 0).getAttribute('aria-label')) ?? '';
	await rowAt(page, 0).click({ modifiers: ['ControlOrMeta'] });
	await page.keyboard.press('Delete');
	await expect(rowAt(page, 0)).toHaveAttribute('aria-label', first);
	await expect(page.locator('section.songs .slot')).toHaveCount(3);
	await page.keyboard.press('Escape');

	const second = (await rowAt(page, 1).getAttribute('aria-label')) ?? '';
	await page.getByRole('button', { name: `More for ${second}` }).click();
	await expect(page.getByRole('menuitem', { name: /Take out of playlist/ })).toHaveCount(0);
	await page.keyboard.press('Escape');

	await rowAt(page, 1)
		.getByRole('button', { name: `Play ${second}` })
		.click();
	const island = page.locator('aside[aria-label="Player"]');
	await expect(island.locator('.song')).toHaveText(second);

	await page.getByRole('button', { name: 'Shuffle', exact: true }).first().click();
	const songs = await page
		.locator('section.songs .slot [data-track]')
		.evaluateAll((rows) => rows.map((row) => row.getAttribute('aria-label') ?? ''));
	await expect
		.poll(async () => songs.includes((await island.locator('.song').textContent()) ?? ''))
		.toBe(true);
});

test('Save as smart playlist from the Library’s ★ 4 and up opens the editor on that rule', async ({
	page
}) => {
	await page.goto(fixtures('/library'));
	await expect(page.getByRole('link', { name: 'Save as smart playlist' })).toHaveCount(0);
	await page.getByRole('button', { name: '★ 4 and up' }).click();
	await page.getByRole('link', { name: 'Save as smart playlist' }).click();
	await expect(page).toHaveURL(/\/playlists\/smart/);
	await expect(rules(page).getByRole('group', { name: 'Rating 4★' })).toBeVisible();
	await expect(count(page)).toHaveText('8');
	await page.getByRole('button', { name: 'Leave it' }).click();
	await expect(page).toHaveURL(/\/library/);
});

test('the rules on a phone: the count rides on top, each rule a card, the limit folded', async ({
	page
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await openRested(page);
	await expect(page.getByText(/^SMART · 3 SONGS · .* · LIVE$/)).toBeVisible();
	await page.getByRole('button', { name: /Edit rules ›/ }).click();
	await expect(count(page)).toHaveText('3');
	await expect(page.getByRole('link', { name: 'See them ›' })).toBeVisible();
	await expect(page.getByText('BACK LEAVES IT')).toBeVisible();
	await expect(page.getByRole('switch', { name: 'Limit' })).toHaveCount(0);
	await page.getByRole('button', { name: 'Limit · 50 songs ›' }).click();
	await expect(page.getByRole('switch', { name: 'Limit' })).toBeVisible();
	await rules(page)
		.getByRole('button', { name: /^5 stars/ })
		.click();
	await expect(count(page)).toHaveText('2');
	const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
	expect(overflow).toBeLessThanOrEqual(0);
	await page.getByRole('button', { name: 'Leave it' }).click();
	await expect(page.getByText(/^SMART · 3 SONGS/)).toBeVisible();
});
