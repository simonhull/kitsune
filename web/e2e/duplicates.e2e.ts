import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** Opens Duplicates on the fixtures. */
async function open(page: Page, query = ''): Promise<void> {
	await page.goto(fixtures(`/server/duplicates${query}`));
	await expect(page.getByRole('heading', { name: 'Duplicates.' })).toBeVisible();
	await expect(page.getByRole('main').getByTestId('counts')).toContainText('12 GROUPS');
}

const groups = (page: Page) => page.getByRole('list', { name: 'Groups' }).getByRole('listitem');

test('the list puts the album pair first, and its filters narrow it', async ({ page }) => {
	await open(page);
	await expect(page.getByRole('main').getByTestId('counts')).toContainText(
		'12 GROUPS · 24 SONGS · 1 ALBUM PAIR'
	);
	await expect(page.getByRole('article', { name: /^Album pair/ })).toContainText(
		'Every song of Wish Upon a Blackstar is also in Wish Upon a Blackstar (Deluxe)'
	);
	// The pair's four groups are in its card: the list holds the other eight, the surest first.
	await expect(groups(page)).toHaveCount(8);
	await expect(groups(page).first()).toContainText('Bittersweet');

	await page.getByRole('button', { name: /^Probably/ }).click();
	await expect(groups(page)).toHaveCount(2);
	await expect(page.getByRole('article', { name: /^Album pair/ })).toHaveCount(0);
	await page.getByRole('button', { name: /^Album pairs/ }).click();
	await expect(groups(page)).toHaveCount(0);
	await expect(page.getByRole('article', { name: /^Album pair/ })).toBeVisible();
});

test('a group compares its copies, and the keeper can change', async ({ page }) => {
	await open(page);
	await page.getByRole('link', { name: 'Compare Creed' }).click();
	await expect(page.getByRole('heading', { name: 'Creed.' })).toBeVisible();
	const liturgy = page.getByRole('region', { name: 'A Liturgy, a Legacy & a Ragamuffin Band' });
	const songs = page.getByRole('region', { name: 'Songs', exact: true });
	await expect(liturgy).toContainText('KEEP THIS ONE');
	await expect(liturgy.getByTestId('why')).toHaveText('ORIGINAL ALBUM');

	await songs.getByRole('button', { name: 'Keep this one instead' }).click();
	await expect(songs).toContainText('KEEP THIS ONE');
	await expect(songs.getByTestId('why')).toHaveText(
		'YOUR CHOICE · THE OTHER IS ON THE ORIGINAL ALBUM'
	);
	await expect(page.getByRole('button', { name: 'Keep Songs, trash the other' })).toBeVisible();

	// Play on a copy plays that copy.
	await liturgy.getByRole('button', { name: /^Play the copy/ }).click();
	await expect(page.getByRole('button', { name: 'Pause', exact: true }).first()).toBeVisible();
});

test('the confirm says what moves, and a merge leaves the list for the Trash', async ({ page }) => {
	await open(page);
	await page.getByRole('link', { name: 'Compare Creed' }).click();
	await page.getByRole('button', { name: 'Keep A Liturgy, trash the other' }).click();
	const confirm = page.getByRole('main').getByTestId('confirm');
	await expect(confirm).toContainText('Merge into A Liturgy, then trash the copy from Songs?');
	await expect(confirm).toContainText('2 plays and their listens, so A Liturgy has 3 plays');
	await expect(confirm).toContainText('Stars: your 5★ stays (Songs had none)');
	await expect(confirm).toContainText('Restoring it brings back the file, not its plays or stars.');

	await confirm.getByRole('button', { name: 'Keep both' }).click();
	await expect(confirm).toHaveCount(0);
	await page.getByRole('button', { name: 'Keep A Liturgy, trash the other' }).click();
	await page
		.getByRole('main')
		.getByTestId('confirm')
		.getByRole('button', { name: 'Merge and trash' })
		.click();

	await expect(page.getByRole('heading', { name: 'Duplicates.' })).toBeVisible();
	const toast = page.getByRole('status').filter({ hasText: 'Kept Creed' });
	await expect(toast).toContainText(
		'Kept Creed · A Liturgy, a Legacy & a Ragamuffin Band. 1 copy moved to the Trash'
	);
	await expect(page.getByRole('main').getByTestId('counts')).toContainText('11 GROUPS');
	await expect(page.getByRole('link', { name: 'Compare Creed' })).toHaveCount(0);
	await toast.getByRole('link', { name: 'See the Trash' }).click();
	await expect(page.getByText('Songs', { exact: true }).first()).toBeVisible();
	await expect(page.getByText('Creed', { exact: true }).first()).toBeVisible();
});

test('Not duplicates hides a group, and Bring back returns it', async ({ page }) => {
	await open(page);
	await page.getByRole('link', { name: 'Compare Sacrifice' }).click();
	await page.getByRole('button', { name: 'Not duplicates' }).click();
	await expect(page.getByRole('main').getByTestId('counts')).toContainText('11 GROUPS');
	await expect(page.getByRole('link', { name: 'Compare Sacrifice' })).toHaveCount(0);

	await page.getByRole('button', { name: /^Dismissed/ }).click();
	await expect(groups(page)).toHaveCount(1);
	await groups(page).getByRole('button', { name: 'Bring back' }).click();
	await expect(groups(page)).toHaveCount(0);
	await expect(page.getByRole('main').getByTestId('counts')).toContainText('12 GROUPS');
});

test('an album pair matches its track lists, and merges in one go', async ({ page }) => {
	await open(page);
	await page.getByRole('link', { name: 'Compare albums ›' }).click();
	await expect(page.getByRole('heading', { name: 'Album pair.' })).toBeVisible();
	const rows = page.getByRole('list', { name: 'Track lists' }).getByRole('listitem');
	await expect(rows).toHaveCount(5);
	await expect(rows.last()).toContainText('ONLY IN THIS ONE');
	await expect(page.getByRole('region', { name: 'Wish Upon a Blackstar (Deluxe)' })).toContainText(
		'KEEP THIS ONE'
	);

	await page.getByRole('button', { name: /^Keep Wish Upon a Blackstar \(Deluxe\)/ }).click();
	const confirm = page.getByRole('main').getByTestId('confirm');
	await expect(confirm).toContainText(
		'Merge 4 songs into Wish Upon a Blackstar (Deluxe) and trash the others?'
	);
	await expect(confirm).toContainText('The songs only one album has stay where they are');
	await confirm.getByRole('button', { name: 'Merge and trash' }).click();
	await expect(page.getByRole('main').getByTestId('counts')).toContainText('8 GROUPS · 16 SONGS');
	await expect(page.getByRole('article', { name: /^Album pair/ })).toHaveCount(0);
});

test('with tag writes off a group compares, and nothing merges', async ({ page }) => {
	await page.goto(fixtures('/server/duplicates/dup-creed?tagWrites=off'));
	await expect(page.getByRole('heading', { name: 'Creed.' })).toBeVisible();
	await expect(page.getByText('Tag writes are off, so nothing on disk changes.')).toBeVisible();
	await expect(page.getByRole('button', { name: /trash the other/ })).toHaveCount(0);
});

test('on a phone the copies stack, keeper first, and the confirm is a sheet', async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto(fixtures('/server/duplicates/dup-creed'));
	const copies = page.getByRole('main').getByTestId('copy');
	await expect(copies.first()).toContainText('KEEP THIS ONE');
	await expect(copies.first()).toContainText('ORIGINAL ALBUM');
	await page.getByRole('button', { name: 'Keep A Liturgy, trash the other' }).click();
	const sheet = page.getByRole('dialog', { name: 'Merge' });
	await expect(sheet).toContainText('WHAT MOVES');
	const merge = sheet.getByRole('button', { name: 'Merge and trash' });
	// The width of the sheet, less its gutter.
	expect((await merge.boundingBox())?.width ?? 0).toBeGreaterThan(320);
	const width = await page.evaluate(() => document.documentElement.scrollWidth);
	expect(width).toBeLessThanOrEqual(390);
});
