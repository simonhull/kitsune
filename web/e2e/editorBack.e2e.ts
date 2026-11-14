import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** The fixtures' album whose files the editor reads. */
const album = 'Glasshouse';
/** The Albums page, where the album was opened from. */
const onAlbums = /\/albums(\?|$)/;

/** Goes from Albums to the album, so Back from the album has somewhere to go. */
async function openAlbum(page: Page): Promise<void> {
	await page.goto(fixtures('/'));
	await page.getByRole('link', { name: 'Albums', exact: true }).click();
	await expect(page).toHaveURL(/\/albums/);
	await page.locator('a[href^="/album/glasshouse-66"]').first().click();
	await expect(page.getByRole('heading', { name: album, exact: true })).toBeVisible();
}

/** Opens the tag editor and returns the first title field, read. */
async function openEditor(page: Page): ReturnType<Page['locator']> {
	await page.getByRole('button', { name: 'Edit tags', exact: true }).click();
	const title = page.getByRole('textbox', { name: /^Title of / }).first();
	await expect(title).toBeVisible();
	await expect(title).not.toHaveValue('');
	return title;
}

test('Back from the tag editor closes it, drops its edits and stays on the album', async ({
	page
}) => {
	await openAlbum(page);
	const albumUrl = page.url();
	const title = await openEditor(page);
	const was = await title.inputValue();
	await title.fill('Not kept');

	await page.goBack();
	await expect(page.getByRole('textbox', { name: /^Title of / })).toHaveCount(0);
	expect(page.url()).toBe(albumUrl);
	await expect(page.getByRole('heading', { name: album, exact: true })).toBeVisible();
	const edit = page.getByRole('button', { name: 'Edit tags', exact: true });
	await expect(edit).toBeFocused();

	// Forward doesn't bring a dropped editor back.
	await page.goForward();
	await expect(edit).toBeVisible();
	await expect(page.getByRole('textbox', { name: /^Title of / })).toHaveCount(0);
	await expect.poll(() => page.url()).toBe(albumUrl);

	// The edit went with it.
	await expect(await openEditor(page)).toHaveValue(was);

	// And Back still closes, then leaves for Albums.
	await page.goBack();
	await expect(edit).toBeVisible();
	await page.goBack();
	await expect(page).toHaveURL(onAlbums);
});

test('Back after Leave it goes to the page before the album, not to the editor', async ({
	page
}) => {
	await openAlbum(page);
	await openEditor(page);
	await page.getByRole('button', { name: 'Leave it', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Edit tags', exact: true })).toBeFocused();

	await page.goBack();
	await expect(page).toHaveURL(onAlbums);
});

test('Back after Write goes to the page before the album, not to the editor', async ({ page }) => {
	await openAlbum(page);
	const title = await openEditor(page);
	await title.fill(`${await title.inputValue()} (live)`);
	await page.getByRole('button', { name: /^Write to files?$/ }).click();
	await expect(page.getByText('Tags written.')).toBeAttached();
	await expect(page.getByRole('textbox', { name: /^Title of / })).toHaveCount(0);

	await page.goBack();
	await expect(page).toHaveURL(onAlbums);
});

test('leaving the album by a link drops the editor’s edits', async ({ page }) => {
	await openAlbum(page);
	const title = await openEditor(page);
	const was = await title.inputValue();
	await title.fill('Not kept');
	await page.getByRole('link', { name: 'Library', exact: true }).click();
	await expect(page).toHaveURL(/\/library/);

	await page.goBack();
	await expect(page.getByRole('button', { name: 'Edit tags', exact: true })).toBeVisible();
	await expect(page.getByRole('textbox', { name: /^Title of / })).toHaveCount(0);
	await expect(await openEditor(page)).toHaveValue(was);
});

test('Back from Edit N songs closes it and stays on the Library', async ({ page }) => {
	await page.goto(fixtures('/'));
	await page.getByRole('link', { name: 'Library', exact: true }).click();
	const row = page.locator('[data-track]').first();
	await row.getByRole('checkbox', { name: /^Select / }).click();
	await page.getByRole('button', { name: 'Edit 1 song', exact: true }).click();
	const editing = page.getByRole('region', { name: 'Editing songs' });
	await expect(editing).toBeVisible();

	await page.goBack();
	await expect(editing).toHaveCount(0);
	await expect(page).toHaveURL(/\/library/);
	await expect(row).toBeVisible();

	// Leave it takes its entry off too: Back then leaves the Library. The song is still picked.
	await page.getByRole('button', { name: 'Edit 1 song', exact: true }).click();
	await expect(editing).toBeVisible();
	await editing.getByRole('button', { name: 'Leave it', exact: true }).click();
	await expect(editing).toHaveCount(0);
	await page.goBack();
	await expect(page).not.toHaveURL(/\/library/);
});
