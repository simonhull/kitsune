import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** Opens Health on the fixtures with the Metadata card starting as `state`, and brings it on screen. */
async function health(page: Page, state: string, query = ''): Promise<void> {
	await page.goto(fixtures(`/health?metadata=${state}${query}`));
	const card = page.getByTestId('metadata');
	await expect(card).toBeVisible();
	await card.scrollIntoViewIfNeeded();
}

/** Whether the page scrolls sideways: it never may. */
async function overflows(page: Page): Promise<boolean> {
	return page.evaluate(
		() => document.documentElement.scrollWidth > document.documentElement.clientWidth
	);
}

test('a new server offers lookups with what they send, and Turn on reshapes the card in place', async ({
	page
}) => {
	await health(page, 'off');
	const card = page.getByTestId('metadata');
	await expect(card).toContainText('ARTIST ART AND BIOS · OFF');
	await expect(card).toContainText('Fetch art and bios for 351 artists?');
	await expect(card).toContainText('MUSICBRAINZ IDS, AND NOTHING ELSE');
	await expect(card).toContainText('NAMES, PATHS, PLAYS OR PEOPLE');
	await expect(card.getByRole('link', { name: /FANART.TV NEEDS YOURS/ })).toHaveAttribute(
		'href',
		'/server#art'
	);

	const before = await card.elementHandle();
	await card.getByRole('button', { name: 'Turn on lookups' }).click();
	await expect(card).toContainText('ARTIST ART AND BIOS · FETCHING');
	await expect(card).toContainText('of 351 artists looked up');
	await expect(card.getByRole('button', { name: 'Turn on lookups' })).toHaveCount(0);
	// The same card, reshaped: not a new one put in its place.
	expect(await before?.evaluate((element) => element.isConnected)).toBe(true);
	expect(await before?.evaluate((element) => element.textContent.includes('looked up'))).toBe(true);
	// Once it has come to rest, nothing of the engine is left.
	await expect(page.locator('[data-surface-box]')).toHaveCount(0);

	// And the setting is on in Server › Settings.
	await card.getByRole('link', { name: 'NEEDS A KEY ›' }).click();
	await expect(page).toHaveURL(/\/server#art$/);
	await expect(page.getByRole('checkbox', { name: /Look up art and bios/ })).toBeChecked();
});

test('a pass going says how far, and fanart.tv without its key leads to Settings', async ({
	page
}) => {
	await health(page, 'running');
	const card = page.getByTestId('metadata');
	await expect(card).toContainText('ARTIST ART AND BIOS · FETCHING');
	await expect(card).toContainText(/NOW · .+ · \d+ OF 140 · MUSIC KEEPS PLAYING/);
	await expect(card.getByRole('progressbar')).toHaveAttribute('aria-valuemax', '140');
	const fanart = card.locator('[data-source=fanarttv]');
	await expect(fanart.getByRole('link', { name: 'NEEDS A KEY ›' })).toHaveAttribute(
		'href',
		'/server#art'
	);
	await expect(card.locator('[data-source=musicbrainz]')).toContainText('ON');
	await expect(card.locator('[data-source=wikipedia]')).toContainText('ON');
});

test('a refused key is said in the server’s words', async ({ page }) => {
	await health(page, 'refused');
	const card = page.getByTestId('metadata');
	await expect(card).toContainText('ARTIST ART AND BIOS · UP TO DATE');
	const fanart = card.locator('[data-source=fanarttv]');
	await expect(fanart.getByRole('link', { name: 'REFUSED THE KEY ›' })).toBeVisible();
	await expect(fanart).toContainText('fanart.tv refused the key · TODAY');
});

test('a source resting says until when, and why', async ({ page }) => {
	await health(page, 'resting');
	const wiki = page.getByTestId('metadata').locator('[data-source=wikipedia]');
	await expect(wiki).toContainText(/RESTING · AGAIN \d\d:\d\d/);
	await expect(wiki).toContainText('Wikipedia answered 503 · TODAY');
});

test('up to date, with the next refresh, the way to Matching and the Credits', async ({ page }) => {
	await health(page, 'done', '&matching=board');
	const card = page.getByTestId('metadata');
	await expect(card).toContainText('ARTIST ART AND BIOS · UP TO DATE');
	await expect(card).toContainText('312');
	await expect(card).toContainText('of 418 artists have art');
	await expect(card).toContainText(/39 FOUND NOTHING YET · ASKED AGAIN LATER · NEXT REFRESH/);
	await expect(card.locator('[data-source=fanarttv]')).toContainText('your own key too');

	await expect(card.getByRole('link', { name: 'fanart.tv' })).toHaveAttribute(
		'href',
		'https://fanart.tv'
	);
	await expect(card.getByRole('link', { name: 'Wikipedia' })).toBeVisible();
	await expect(card.getByRole('link', { name: 'CC BY-SA 4.0' })).toHaveAttribute(
		'href',
		'https://creativecommons.org/licenses/by-sa/4.0/'
	);
	await expect(card.getByRole('link', { name: 'Wikimedia Commons' })).toBeVisible();
	await expect(card.getByRole('link', { name: 'MusicBrainz', exact: true })).toBeVisible();

	await card.getByRole('link', { name: /67 ALBUM ARTISTS HAVE NO MUSICBRAINZ ID/ }).click();
	await expect(page).toHaveURL(/\/health\/matching/);
});

test('a pass that ends is read on screen, and the card says up to date', async ({ page }) => {
	await health(page, 'running', '&metadataArtist=20');
	const card = page.getByTestId('metadata');
	await expect(card).toContainText('ARTIST ART AND BIOS · UP TO DATE', { timeout: 10_000 });
	await expect(card).toContainText('of 418 artists have art');
});

test('only administrators see the card', async ({ page }) => {
	await page.goto(fixtures('/health?metadata=running&role=user'));
	await expect(page.getByRole('heading').first()).toBeVisible();
	await expect(page.getByTestId('metadata')).toHaveCount(0);
});

test('Settings adds, replaces and clears fanart.tv’s keys, and never shows one', async ({
	page
}) => {
	const typed = 'f00dfeedc0ffee0123456789abcdef01';
	await page.goto(fixtures('/server?metadata=running'));
	const section = page.locator('#art');
	const project = section.getByTestId('key-fanarttv.api_key');
	await expect(project).toContainText('Not set');
	await expect(section).toContainText('Needs a project key. Off until it has one.');

	await project.getByRole('button', { name: 'Add key' }).click();
	const field = project.getByLabel('fanart.tv project key');
	await expect(field).toBeFocused();
	await expect(field).toHaveAttribute('type', 'password');
	await field.fill(typed);
	await expect(section).toContainText('Thumbs, backdrops, logos, banners and disc art.');
	await section.getByRole('button', { name: 'Save' }).click();
	await expect(section).toContainText('SAVED');
	await expect(project).toContainText('Set · never shown');
	await expect(project.getByRole('textbox')).toHaveCount(0);
	expect(await page.content()).not.toContain(typed);

	// Replace opens an empty field; Undo puts it back as it was.
	await project.getByRole('button', { name: 'Replace' }).click();
	await expect(project.getByLabel('fanart.tv project key')).toHaveValue('');
	await section.getByRole('button', { name: 'Undo' }).click();
	await expect(project).toContainText('Set · never shown');

	// Clear says what saving will do, and Save clears it.
	await project.getByRole('button', { name: 'Clear' }).click();
	await expect(project).toContainText('Cleared when you save');
	await section.getByRole('button', { name: 'Save' }).click();
	await expect(project).toContainText('Not set');
	await expect(project.getByRole('button', { name: 'Add key' })).toBeVisible();

	// An empty key isn't saved.
	const personal = section.getByTestId('key-fanarttv.client_key');
	await personal.getByRole('button', { name: 'Add key' }).click();
	await section.getByRole('button', { name: 'Save' }).click();
	await expect(section.getByRole('alert')).toContainText('Paste the key, or keep the one set.');
});

test('a key saved in Settings takes fanart.tv out of needing one on Health', async ({ page }) => {
	await page.goto(fixtures('/server?metadata=running'));
	const section = page.locator('#art');
	const project = section.getByTestId('key-fanarttv.api_key');
	await project.getByRole('button', { name: 'Add key' }).click();
	await project.getByLabel('fanart.tv project key').fill('a-key-nobody-sees');
	await section.getByRole('button', { name: 'Save' }).click();
	await expect(project).toContainText('Set · never shown');

	await page.locator('a.item.health').click();
	const fanart = page.getByTestId('metadata').locator('[data-source=fanarttv]');
	await expect(fanart).toContainText('ON');
	await expect(fanart.getByRole('link')).toHaveCount(0);
});

test('the sources dim under the master switch, and the language is saved', async ({ page }) => {
	await page.goto(fixtures('/server?metadata=done'));
	const section = page.locator('#art');
	const subs = section.locator('.subs');
	await expect(subs).not.toHaveClass(/dim/);
	await section.getByText('Look up art and bios', { exact: true }).click();
	await expect(subs).toHaveClass(/dim/);
	await expect(section).toContainText('UNSAVED');
	await section.getByRole('combobox', { name: 'Bio language' }).selectOption('de');
	await section.getByRole('button', { name: 'Save' }).click();
	await expect(section).toContainText('SAVED');
	await expect(section.getByRole('checkbox', { name: /Look up art and bios/ })).not.toBeChecked();
	await expect(section.getByRole('combobox', { name: 'Bio language' })).toHaveValue('de');
});

test('the card and the rows fit a phone at 390', async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await health(page, 'off');
	const card = page.getByTestId('metadata');
	await expect(card).toContainText('MUSICBRAINZ, WIKIMEDIA, FANART.TV');
	await expect(card).not.toContainText('ONLY ADMINS');
	const go = card.getByRole('button', { name: 'Turn on lookups' });
	const [goBox, cardBox] = [await go.boundingBox(), await card.boundingBox()];
	expect(goBox !== null && cardBox !== null && goBox.width > cardBox.width - 60).toBe(true);
	expect(await overflows(page)).toBe(false);
	await go.click();
	await expect(card).toContainText(/NOW · .+ · \d+ OF 351/);
	await expect(card).not.toContainText('MUSIC KEEPS PLAYING');
	await expect(card).toContainText('MATCH THEIR ALBUMS ›');
	expect(await overflows(page)).toBe(false);

	await page.goto(fixtures('/server?metadata=running'));
	const section = page.locator('#art');
	await expect(section).toContainText('NEW ART IN 2 DAYS, NOT 7');
	await expect(section.getByText('KITSUNE SHIPS NONE')).toBeHidden();
	expect(await overflows(page)).toBe(false);
});

test('the card and the rows hold at 4K', async ({ page }) => {
	await page.setViewportSize({ width: 3840, height: 2160 });
	await health(page, 'running');
	const card = page.getByTestId('metadata');
	await expect(card).toContainText('ONLY ADMINS');
	const name = card.locator('[data-source=fanarttv] .name');
	const what = card.locator('[data-source=fanarttv] .what');
	const [nameBox, whatBox] = [await name.boundingBox(), await what.boundingBox()];
	// Each source's note beside its name, not under it.
	expect(nameBox !== null && whatBox !== null && whatBox.x > nameBox.x + nameBox.width).toBe(true);
	expect(await overflows(page)).toBe(false);

	await page.goto(fixtures('/server?metadata=done'));
	const section = page.locator('#art');
	const master = section.getByText('Look up art and bios', { exact: true });
	const key = section.getByText('fanart.tv project key', { exact: true });
	const [masterBox, keyBox] = [await master.boundingBox(), await key.boundingBox()];
	// Two columns: the switches beside the keys.
	expect(masterBox !== null && keyBox !== null && keyBox.x > masterBox.x + 400).toBe(true);
	expect(await overflows(page)).toBe(false);
});

test('a pass through albums says which, counts the archive’s covers, and Last.fm without its key leads to it', async ({
	page
}) => {
	await health(page, 'albums', '&lastfm=unset');
	const card = page.getByTestId('metadata');
	await expect(card).toContainText(
		/NOW · .+ · THE RECORD · \d+ OF 96 ALBUMS · MUSIC KEEPS PLAYING/
	);
	await expect(card.getByTestId('albums-line')).toContainText(
		/\d+ ALBUM COVERS? FROM THE COVER ART ARCHIVE · \d+ ALBUMS HAVE NO COVER/
	);
	await expect(card.locator('[data-source=coverartarchive]')).toContainText('ON');
	await expect(card.locator('[data-source=coverartarchive]')).toContainText(
		'covers your files lack, back covers · no key'
	);
	const lastfm = card.locator('[data-source=lastfm]');
	await expect(lastfm).toContainText('bios Wikipedia lacks');
	await lastfm.getByRole('link', { name: 'NEEDS A KEY ›' }).click();
	await expect(page).toHaveURL(/\/server#lastfm$/);
	await expect(page.locator('#art')).toContainText(
		'Needs the Last.fm API key above. Off until it has one.'
	);
});

test('the Credits name Last.fm and the Cover Art Archive', async ({ page }) => {
	await health(page, 'done');
	const card = page.getByTestId('metadata');
	await expect(card.locator('[data-source=lastfm]')).toContainText('ON');
	await expect(card.getByRole('link', { name: 'Last.fm', exact: true })).toHaveAttribute(
		'href',
		'https://www.last.fm'
	);
	await expect(card.getByRole('link', { name: 'CC BY-SA', exact: true })).toHaveAttribute(
		'href',
		'https://creativecommons.org/licenses/by-sa/3.0/'
	);
	await expect(card.getByRole('link', { name: 'Cover Art Archive' })).toHaveAttribute(
		'href',
		'https://coverartarchive.org'
	);
});

test('Settings switches Last.fm’s bios and the Cover Art Archive, and Health follows', async ({
	page
}) => {
	await page.goto(fixtures('/server?metadata=done'));
	const section = page.locator('#art');
	const lastfm = section.getByRole('checkbox', { name: /Last\.fm/ });
	const archive = section.getByRole('checkbox', { name: /Cover Art Archive/ });
	await expect(lastfm).toBeChecked();
	await expect(archive).toBeChecked();
	await expect(section).toContainText('Bios Wikipedia doesn’t have, with the Last.fm key above.');
	await expect(section).toContainText(
		'Covers for albums with none in their files, and back covers. No key.'
	);
	await section.getByText('Cover Art Archive', { exact: true }).click();
	await expect(section).toContainText('UNSAVED');
	await section.getByRole('button', { name: 'Save' }).click();
	await expect(section).toContainText('SAVED');
	await expect(archive).not.toBeChecked();

	await page.locator('a.item.health').click();
	const caa = page.getByTestId('metadata').locator('[data-source=coverartarchive]');
	await expect(caa).toContainText('OFF');
	await expect(page.getByTestId('metadata').locator('[data-source=lastfm]')).toContainText('ON');
});

test('a pass through albums fits a phone at 390', async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await health(page, 'albums', '&lastfm=unset');
	const card = page.getByTestId('metadata');
	await expect(card).toContainText(/NOW · .+ · \d+ OF 96 ALBUMS/);
	await expect(card.getByTestId('albums-line')).toContainText(/COVERS? FROM THE ARCHIVE/);
	await expect(card.locator('[data-source=lastfm]').getByRole('link')).toBeVisible();
	expect(await overflows(page)).toBe(false);
});
