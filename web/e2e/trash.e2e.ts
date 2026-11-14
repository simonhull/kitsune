import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** A song's row on the album page, by its title. */
const row = (page: Page, title: string) => page.locator(`[data-track][aria-label="${title}"]`);

/** The toast above the player island. */
const toast = (page: Page) => page.getByRole('status').filter({ hasText: 'moved to Trash' });

test('a song moved to Trash from its menu folds away, and Undo brings it back in place', async ({
	page
}) => {
	await page.goto(fixtures('/album/glasshouse-66'));
	await expect(row(page, 'Low Tide')).toBeVisible();
	await page.getByRole('button', { name: 'More for Low Tide' }).click();
	const item = page.getByRole('menuitem', { name: /Move to Trash/ });
	await expect(item).toContainText('DEL');
	await item.click();

	await expect(row(page, 'Low Tide')).toHaveCount(0);
	await expect(toast(page)).toContainText('Low Tide moved to Trash');
	await toast(page).getByRole('button', { name: 'Undo' }).click();

	await expect(row(page, 'Low Tide')).toBeVisible();
	await expect(toast(page)).toHaveCount(0);
	// In place: between Panes and Ribbons, as it was.
	const titles = await page
		.locator('[data-track]')
		.evaluateAll((rows) => rows.map((each) => each.getAttribute('aria-label')));
	expect(titles.slice(titles.indexOf('Panes'), titles.indexOf('Panes') + 3)).toEqual([
		'Panes',
		'Low Tide',
		'Ribbons'
	]);
});

test('an album moved to Trash leaves the grid, with its songs counted', async ({ page }) => {
	await page.goto(fixtures('/albums'));
	const card = page.locator('a[href^="/album/after-hours"]').first();
	await expect(card).toBeVisible();
	await card.click({ button: 'right' });
	await page.getByRole('menuitem', { name: 'Move album to Trash' }).click();

	await expect(toast(page)).toContainText('After Hours · 8 songs moved to Trash');
	await expect(page.locator('a[href^="/album/after-hours"]')).toHaveCount(0);
});

test('songs picked go to Trash with the Delete key, and the Trash page lists them', async ({
	page
}) => {
	await page.goto(fixtures('/album/glasshouse-66'));
	for (const title of ['Breakwater', 'Small Weather']) {
		await row(page, title).getByRole('checkbox').check();
	}
	await expect(page.getByRole('toolbar')).toContainText('2 selected');
	await expect(
		page.getByRole('toolbar').getByRole('button', { name: 'Move 2 songs to Trash' })
	).toBeVisible();
	await page.keyboard.press('Delete');

	await expect(toast(page)).toContainText('2 songs moved to Trash');
	await expect(row(page, 'Breakwater')).toHaveCount(0);
	await expect(row(page, 'Small Weather')).toHaveCount(0);
	await expect(page.getByRole('toolbar')).toHaveCount(0);

	await page.getByRole('link', { name: 'Search' }).first().click();
	// Delete in a text field is the field's own, songs picked or not.
	const field = page.getByRole('textbox', { name: 'Search your library' });
	await field.fill('Light');
	await expect(page.getByRole('main').getByRole('status')).toContainText(/\d+ tracks/);
	const found = page.getByRole('main').locator('[data-track]').first();
	await expect(found).toBeVisible();
	const id = (await found.getAttribute('data-track')) ?? '';
	const picked = page.locator(`[data-track="${id}"]`);
	// The results arrive while the page does: pick once they've settled.
	await expect(async () => {
		if ((await page.getByRole('toolbar').count()) === 0) {
			await picked.click({ modifiers: ['ControlOrMeta'], timeout: 1000 });
		}
		await expect(page.getByRole('toolbar')).toContainText('1 selected', { timeout: 500 });
	}).toPass();
	await field.focus();
	await field.press('Home');
	await field.press('Delete');
	await expect(field).toHaveValue('ight');
	await expect(picked).toBeVisible();
	// The toast up is still the one before: nothing new moved.
	await expect(toast(page).filter({ hasText: '1 song' })).toHaveCount(0);

	// On by links, since a fixture page loaded afresh forgets what moved.
	await page.locator('a[href="/you"]').first().click();
	await page.locator('a[href="/server"]').first().click();
	await page.getByRole('link', { name: '2 songs in Trash' }).first().click();
	await expect(page.getByRole('heading', { name: 'Trash.' })).toBeVisible();
	const group = page.getByRole('region', { name: 'Glasshouse' });
	await expect(group).toContainText('Breakwater');
	await expect(group).toContainText('Small Weather');
	await expect(group).toContainText('30 DAYS LEFT');
});

test('the Trash page restores a song, and empties only on a second press', async ({ page }) => {
	await page.goto(fixtures('/server/trash?trash=board'));
	await expect(page.getByText('6 SONGS ·', { exact: false })).toBeVisible();
	await expect(page.getByText('GONE IN 4 DAYS').first()).toBeVisible();

	await page.getByRole('button', { name: 'Restore Amber Light' }).click();
	await expect(page.getByRole('region', { name: 'Night Ferry' })).toHaveCount(0);
	await expect(page.getByText('5 SONGS ·', { exact: false })).toBeVisible();

	await page.getByRole('button', { name: 'Empty Trash…' }).click();
	await expect(page.getByText('Delete 5 songs for good? This can’t be undone.')).toBeVisible();
	await page.getByRole('button', { name: 'Keep them' }).click();
	await expect(page.getByRole('region', { name: 'Tidewrack' })).toBeVisible();

	await page.getByRole('button', { name: 'Empty Trash…' }).click();
	await page.getByRole('button', { name: 'Delete for good' }).click();
	await expect(page.getByText('Nothing in Trash. Songs moved here stay 30 days.')).toBeVisible();

	// The song restored is back on its album.
	await page.getByRole('link', { name: 'Albums', exact: true }).click();
	await page.locator('a[href^="/album/night-ferry-94"]').first().click();
	await expect(row(page, 'Amber Light')).toBeVisible();
});

test('someone who is not an administrator sees none of it', async ({ page }) => {
	await page.goto(fixtures('/album/glasshouse-66?role=user'));
	await expect(row(page, 'Low Tide')).toBeVisible();
	await page.getByRole('button', { name: 'More for Low Tide' }).click();
	await expect(page.getByRole('menu')).toBeVisible();
	await expect(page.getByRole('menuitem', { name: /Trash/ })).toHaveCount(0);
	await page.keyboard.press('Escape');

	await row(page, 'Low Tide').getByRole('checkbox').check();
	await expect(page.getByRole('toolbar')).toBeVisible();
	await expect(page.getByRole('toolbar').getByRole('button', { name: /Trash/ })).toHaveCount(0);
	await page.keyboard.press('Delete');
	await expect(row(page, 'Low Tide')).toBeVisible();
	await expect(toast(page)).toHaveCount(0);
});

test('with tag writes off, an administrator sees none of it either', async ({ page }) => {
	await page.goto(fixtures('/album/glasshouse-66?tagWrites=off'));
	await page.getByRole('button', { name: 'More for Low Tide' }).click();
	await expect(page.getByRole('menu')).toBeVisible();
	await expect(page.getByRole('menuitem', { name: /Trash/ })).toHaveCount(0);
});
