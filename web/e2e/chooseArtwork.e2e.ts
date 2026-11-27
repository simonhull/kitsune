import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/**
 * Choose artwork (plan 072's fourth phase), on the fixtures: the artist hero's ⋯ buds its menu for
 * administrators, Choose artwork… reshapes it into the picker, and a pin develops the new picture
 * on the stage behind. Low Tide Choir has candidates from fanart.tv and Commons; Ana Lindqvist only
 * her folder's portrait and no MusicBrainz ID.
 */

const choir = '/artists/artist-low-tide-choir';
const stage = (page: Page) => page.locator('.shell > .backdrop [data-morph-stage="artist"]');
const picker = (page: Page) => page.getByTestId('artwork-picker');
const tile = (page: Page, id: string) => picker(page).locator(`[data-candidate="${id}"]`);

async function openPicker(page: Page, query = ''): Promise<void> {
	await page.goto(fixtures(`${choir}${query}`));
	await page.getByRole('button', { name: 'More for Low Tide Choir' }).click();
	const menu = page.getByRole('menu', { name: 'More for Low Tide Choir' });
	await expect(menu).toContainText('ARTWORK · ADMINS');
	await menu.getByRole('menuitem', { name: 'Choose artwork…' }).click();
	await expect(picker(page)).toBeVisible();
}

test('the ⋯ buds its menu, and Choose artwork… reshapes it in place into the picker', async ({
	page
}) => {
	await openPicker(page);
	// One surface, its face changed: the menu is gone, the picker in its place.
	await expect(page.locator('[data-surface][role="menu"]')).toHaveCount(0);
	await expect(page.getByRole('dialog', { name: 'Choose artwork' })).toBeVisible();
	await expect(picker(page)).toContainText('LOW TIDE CHOIR · BY MUSICBRAINZ ID 6A1B…5F');
	await expect(page.getByRole('tab', { name: 'Background' })).toHaveAttribute(
		'aria-selected',
		'true'
	);
	const chosen = tile(page, 'fanart:bg-dusk');
	await expect(chosen).toContainText('CHOSEN · AUTOMATIC');
	await expect(chosen.locator('.ring')).toBeVisible();
	await expect(tile(page, 'fanart:bg-crowd')).toContainText('FANART.TV · 1280 × 720');
	await expect(tile(page, 'fanart:bg-crowd')).toContainText('NO WORDS');
	await expect(picker(page).getByRole('button', { name: 'Automatic ✓' })).toBeDisabled();
	await expect(picker(page)).toContainText('FILES FIRST, THEN FANART.TV, THEN THE REST');
	await expect(picker(page).getByRole('button', { name: 'Upload an image…' })).toBeVisible();
	await picker(page).getByRole('button', { name: 'Close the picker' }).click();
	await expect(picker(page)).toHaveCount(0);
});

test('each kind has its own candidates, a Commons portrait credited by its author', async ({
	page
}) => {
	await openPicker(page);
	await page.getByRole('tab', { name: 'Thumb' }).click();
	const commons = tile(page, 'wikimedia:Low_Tide_Choir_2019.jpg');
	await expect(commons).toContainText('COMMONS · K. ARNESEN');
	await expect(commons.getByRole('link', { name: 'CC BY-SA 4.0 ›' })).toHaveAttribute(
		'href',
		'https://creativecommons.org/licenses/by-sa/4.0/'
	);
	await page.getByRole('tab', { name: 'Logo' }).click();
	await expect(tile(page, 'fanart:lg-fox')).toContainText('TRANSPARENT PNG');
	// The strip's arrows move between the kinds.
	await page.getByRole('tab', { name: 'Logo' }).press('ArrowRight');
	await expect(page.getByRole('tab', { name: 'Banner' })).toHaveAttribute('aria-selected', 'true');
	await expect(tile(page, 'fanart:bn-sky')).toBeVisible();
});

test('pinning rings and stamps it, the stage develops it, and Automatic unpins', async ({
	page
}) => {
	await openPicker(page);
	const before = await stage(page).locator('[data-picture]').first().getAttribute('data-picture');
	const stageTile = tile(page, 'fanart:bg-stage');
	await stageTile.getByRole('button').click();
	await expect(stageTile).toContainText('PINNED BY SIMON · TODAY');
	await expect(stageTile.locator('.stamp')).toHaveText('PINNED');
	await expect(tile(page, 'fanart:bg-dusk')).not.toContainText('CHOSEN');
	// The tiles stay where they were: the ring moved, not the list.
	await expect(picker(page).locator('[data-candidate]').nth(1)).toHaveAttribute(
		'data-candidate',
		'fanart:bg-stage'
	);
	await expect(page.getByTestId('artwork-toast')).toContainText(
		'Pinned Low Tide Choir’s background'
	);
	const pinned = await stageTile.locator('[data-picture]').getAttribute('data-picture');
	await expect(
		stage(page)
			.locator(`[data-picture="${String(pinned)}"]`)
			.first()
	).toBeAttached();
	expect(pinned).not.toBe(before);
	await expect(picker(page)).toContainText('PINNED: A REFRESH NEVER CHANGES IT');

	await picker(page).getByRole('button', { name: 'Automatic', exact: true }).click();
	await expect(tile(page, 'fanart:bg-dusk')).toContainText('CHOSEN · AUTOMATIC');
	await expect(page.getByTestId('artwork-toast')).toContainText('background is automatic again');
	await expect(
		stage(page)
			.locator(`[data-picture="${String(before)}"]`)
			.first()
	).toBeAttached();
});

test('a pin that is already there opens pinned', async ({ page }) => {
	await openPicker(page, '?artwork=pinned');
	await expect(tile(page, 'fanart:bg-stage')).toContainText('PINNED BY SIMON · TODAY');
	await expect(picker(page).getByRole('button', { name: 'Automatic', exact: true })).toBeEnabled();
});

test('Not this artist… reshapes into its confirm; Keep it goes back, and confirming holds them', async ({
	page
}) => {
	await openPicker(page);
	await picker(page).getByRole('button', { name: 'Not this artist…' }).click();
	const confirm = page.getByTestId('not-this-artist');
	await expect(confirm).toContainText('NOT THIS ARTIST · LOW TIDE CHOIR');
	await expect(confirm).toContainText('Take this art and bio away?');
	await expect(confirm).toContainText('the MusicBrainz ID 6a1b…5f');
	await expect(picker(page)).toHaveCount(0);
	await confirm.getByRole('button', { name: 'Keep it' }).click();
	await expect(picker(page)).toBeVisible();

	await picker(page).getByRole('button', { name: 'Not this artist…' }).click();
	await page
		.getByTestId('not-this-artist')
		.getByRole('button', { name: 'Not this artist' })
		.click();
	await expect(page.getByTestId('not-this-artist')).toHaveCount(0);
	await expect(page.getByTestId('artwork-toast')).toContainText('Low Tide Choir is held back');
	// What fanart.tv gave is gone: the hero's thumb is the initial, the bio too.
	await expect(page.locator('.shell > main .artist .head .initial')).toHaveText('L');
	await page.getByRole('button', { name: 'More for Low Tide Choir' }).click();
	await page.getByRole('menuitem', { name: 'Choose artwork…' }).click();
	await expect(picker(page)).toContainText('HELD BACK BY NOT THIS ARTIST');
	await expect(picker(page).getByRole('button', { name: 'Not this artist…' })).toHaveCount(0);
});

test('an uploaded image is pinned and shown; a file it can’t take says why', async ({ page }) => {
	await openPicker(page);
	const png = Buffer.from(
		'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNgYPj/HwAEAQH/ra1NOAAAAABJRU5ErkJggg==',
		'base64'
	);
	const input = picker(page).locator('input[type="file"]');
	await input.setInputFiles({
		name: 'notes.txt',
		mimeType: 'text/plain',
		buffer: Buffer.from('x')
	});
	await expect(page.getByTestId('artwork-toast')).toContainText(
		'The image must be JPEG, PNG or WebP'
	);
	await input.setInputFiles({ name: 'band.png', mimeType: 'image/png', buffer: png });
	await expect(page.getByTestId('artwork-toast')).toContainText(
		'Pinned your image as Low Tide Choir’s background'
	);
	const upload = picker(page).locator('[data-candidate^="upload:"]');
	await expect(upload).toContainText('UPLOADED');
	await expect(upload).toContainText('PINNED BY SIMON');
});

test('Look again now has no surface of its own: the menu closes and a toast says so', async ({
	page
}) => {
	await page.goto(fixtures(choir));
	await page.getByRole('button', { name: 'More for Low Tide Choir' }).click();
	await page.getByRole('menuitem', { name: 'Look again now' }).click();
	await expect(page.locator('[data-surface][role="menu"]')).toHaveCount(0);
	await expect(page.getByTestId('artwork-toast')).toContainText('Looking again for Low Tide Choir');
});

test('a source that failed or is off says so, and the first read says it is asking', async ({
	page
}) => {
	await openPicker(page, '?artwork=failed');
	await expect(picker(page)).toContainText('FANART.TV ANSWERED 503');
	await page.getByRole('tab', { name: 'Thumb' }).click();
	await expect(tile(page, 'wikimedia:Low_Tide_Choir_2019.jpg')).toBeVisible();
	await page.goto(fixtures(`${choir}?artwork=off&artworkMs=800`));
	await page.getByRole('button', { name: 'More for Low Tide Choir' }).click();
	await page.getByRole('menuitem', { name: 'Choose artwork…' }).click();
	await expect(picker(page)).toContainText('Asking the sources…');
	await expect(picker(page)).toContainText('FANART.TV IS OFF');
});

test('an artist with no MusicBrainz ID offers only Choose artwork, its folder’s portrait chosen', async ({
	page
}) => {
	await page.goto(fixtures('/artists/artist-ana-lindqvist'));
	await page.getByRole('button', { name: 'More for Ana Lindqvist' }).click();
	await expect(page.getByRole('menuitem')).toHaveCount(1);
	await page.getByRole('menuitem', { name: 'Choose artwork…' }).click();
	await page.getByRole('tab', { name: 'Thumb' }).click();
	await expect(tile(page, 'folder')).toContainText('YOUR FILES');
	await expect(tile(page, 'folder')).toContainText('CHOSEN · AUTOMATIC');
	await expect(picker(page)).toContainText('NO MUSICBRAINZ ID');
});

test('someone who isn’t an administrator never sees the ⋯', async ({ page }) => {
	await page.goto(fixtures(`${choir}?role=user`));
	await expect(page.locator('.shell > main h1')).toHaveText('Low Tide Choir');
	await expect(page.getByRole('button', { name: 'More for Low Tide Choir' })).toHaveCount(0);
});

test('on a phone the menu buds upward and the picker is it at the phone’s width, two across', async ({
	page
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto(fixtures(choir));
	const more = page.getByRole('button', { name: 'More for Low Tide Choir' });
	await more.click();
	const menu = page.getByRole('menu', { name: 'More for Low Tide Choir' });
	const [moreBox, menuBox] = [await more.boundingBox(), await menu.boundingBox()];
	expect(moreBox !== null && menuBox !== null && menuBox.y + menuBox.height <= moreBox.y).toBe(
		true
	);
	await menu.getByRole('menuitem', { name: 'Choose artwork…' }).click();
	const dialog = page.getByRole('dialog', { name: 'Choose artwork' });
	await expect(picker(page)).toContainText('LOW TIDE CHOIR · 6A1B…5F');
	const box = await dialog.boundingBox();
	expect(box !== null && Math.round(box.x) === 16 && Math.round(box.width) === 358).toBe(true);
	const tiles = picker(page).locator('[data-candidate]');
	const [a, b, c] = [
		await tiles.nth(0).boundingBox(),
		await tiles.nth(1).boundingBox(),
		await tiles.nth(2).boundingBox()
	];
	expect(a !== null && b !== null && c !== null && a.y === b.y && c.y > a.y).toBe(true);
	// Clear of the island.
	const player = page.getByRole('complementary', { name: 'Player' });
	const island = (await player.count()) > 0 ? await player.boundingBox() : null;
	expect(island === null || (box !== null && box.y + box.height <= island.y)).toBe(true);
	await expect(page.getByTestId('artwork-picker').locator('.auto-note')).toHaveCount(0);
});

test('at 4K the picker is on the size step, anchored to the ⋯’s side', async ({ page }) => {
	await page.setViewportSize({ width: 3840, height: 2160 });
	await openPicker(page);
	const dialog = await page.getByRole('dialog', { name: 'Choose artwork' }).boundingBox();
	const more = await page.getByRole('button', { name: 'More for Low Tide Choir' }).boundingBox();
	const step = await page.evaluate(() =>
		Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--step'))
	);
	expect(step).toBeGreaterThan(1);
	expect(dialog !== null && Math.abs(dialog.width - 880 * step) < 2).toBe(true);
	expect(
		dialog !== null &&
			more !== null &&
			Math.abs(dialog.x + dialog.width - (more.x + more.width)) < 2
	).toBe(true);
});
