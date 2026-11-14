import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** A song's row, by its track ID. */
const row = (page: Page, id: string) => page.locator(`[data-track="${id}"]`).first();

/** The toast above the player island, for playlists. */
const toast = (page: Page) =>
	page.getByRole('status').filter({ hasText: /added to|already in|has it/ });

/** Add to playlist, open beside a menu or over the bar. */
const picker = (page: Page) => page.getByRole('menu', { name: 'Add to playlist' });

/** Ticks songs on an album page by their IDs. */
async function pick(page: Page, ids: readonly string[]): Promise<void> {
	for (const id of ids) {
		await row(page, id).click({ modifiers: ['ControlOrMeta'] });
	}
	await expect(page.getByRole('toolbar')).toContainText(`${String(ids.length)} selected`);
}

test("a song goes into a playlist from its row's menu, beside it", async ({ page }) => {
	await page.goto(fixtures('/album/zero-96'));
	await expect(row(page, 'zero-96-1')).toBeVisible();
	await page.getByRole('button', { name: 'More for Signal One' }).click();
	const item = page.getByRole('menuitem', { name: 'Add to playlist' });
	await item.click();
	await expect(picker(page)).toBeVisible();
	// Beside the menu: both stay open.
	await expect(page.getByRole('menu', { name: 'Signal One' })).toBeVisible();
	await picker(page)
		.getByRole('menuitem', { name: /Slow mornings/ })
		.click();

	await expect(toast(page)).toContainText('1 song added to Slow mornings');
	await expect(picker(page)).toHaveCount(0);
	await expect(page.getByRole('menu', { name: 'Signal One' })).toHaveCount(0);

	// Now it says so, with a tick.
	await page.getByRole('button', { name: 'More for Signal One' }).click();
	await page.getByRole('menuitem', { name: 'Add to playlist' }).press('ArrowRight');
	await expect(picker(page).getByRole('menuitem', { name: /Slow mornings/ })).toContainText(
		'ALREADY IN IT'
	);
	// ← goes back to its item, Esc then closes the menu.
	await page.keyboard.press('ArrowLeft');
	await expect(picker(page)).toHaveCount(0);
	await expect(page.getByRole('menuitem', { name: 'Add to playlist' })).toBeFocused();
});

test('songs picked add what the playlist lacks, then again, then Undo takes out exactly that', async ({
	page
}) => {
	await page.goto(fixtures('/album/orbitals-14'));
	await expect(row(page, 'orbitals-14-3')).toBeVisible();
	await pick(page, ['orbitals-14-3', 'orbitals-14-4', 'orbitals-14-5']);
	const addTo = page.getByRole('toolbar').getByRole('button', { name: 'Add to…' });
	await addTo.click();
	await expect(picker(page)).toContainText('ADD 3 SONGS TO');
	const falling = picker(page).getByRole('menuitem', { name: /Falling Up/ });
	await expect(falling).toContainText('1 OF THE 3 ALREADY IN IT');
	await falling.click();

	await expect(toast(page)).toContainText('2 songs added to Falling Up · 1 was already in it');
	await toast(page).getByRole('button', { name: 'Add it again' }).click();
	await expect(toast(page)).toContainText('1 song added to Falling Up');
	await toast(page).getByRole('button', { name: 'Undo' }).click();
	await expect(toast(page)).toHaveCount(0);

	// The two from the first add stay; the copy Add it again put in went.
	await addTo.click();
	await expect(picker(page).getByRole('menuitem', { name: /Falling Up/ })).toContainText(
		/Falling Up\s*ALREADY IN IT/
	);
});

test('New playlist… names one in the menu and puts the song in it', async ({ page }) => {
	await page.goto(fixtures('/album/zero-96'));
	await expect(row(page, 'zero-96-1')).toBeVisible();
	await page.getByRole('button', { name: 'More for Signal One' }).click();
	await page.getByRole('menuitem', { name: 'Add to playlist' }).click();
	await picker(page).getByRole('menuitem', { name: 'New playlist…' }).click();
	const name = picker(page).getByRole('textbox', { name: 'Playlist name' });
	await expect(name).toBeFocused();
	await name.fill('Road trip');
	await name.press('Enter');

	await expect(toast(page)).toContainText('1 song added to Road trip');
	await page.getByRole('button', { name: 'More for Signal One' }).click();
	await page.getByRole('menuitem', { name: 'Add to playlist' }).click();
	await expect(picker(page).getByRole('menuitem', { name: /Road trip/ })).toContainText(
		'ALREADY IN IT'
	);
});

test('on a phone, Add to playlist is a sheet from the selection bar’s More', async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto(fixtures('/album/orbitals-14'));
	await expect(row(page, 'orbitals-14-4')).toBeVisible();
	await pick(page, ['orbitals-14-4', 'orbitals-14-5']);
	await page.getByRole('toolbar').getByRole('button', { name: 'More' }).click();
	await page.getByRole('menuitem', { name: 'Add to playlist…' }).click();

	const sheet = page.getByRole('dialog', { name: 'Add to playlist' });
	await expect(sheet).toBeVisible();
	await expect(sheet).toContainText('ADD 2 SONGS TO');
	// From the foot of the screen, once it has risen.
	await expect
		.poll(async () => {
			const box = await sheet.boundingBox();
			return box === null ? 0 : Math.round(box.y + box.height);
		})
		.toBe(844);
	expect((await sheet.boundingBox())?.y ?? 0).toBeGreaterThan(300);
	await sheet.getByRole('menuitem', { name: /Falling Up/ }).click();

	await expect(sheet).toHaveCount(0);
	await expect(toast(page)).toContainText('2 songs added to Falling Up');
});
