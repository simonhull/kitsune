import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** Opens Organize on the fixtures, with each album of a run taking `albumMs`. */
async function open(page: Page, albumMs = 400): Promise<void> {
	await page.goto(fixtures(`/server/organize?organizeAlbum=${String(albumMs)}`));
	await expect(page.getByRole('heading', { name: 'Organize.' })).toBeVisible();
	await expect(page.getByTestId('example')).toHaveCount(3);
}

async function preview(page: Page): Promise<void> {
	await page.getByRole('button', { name: 'Preview', exact: true }).click();
	await expect(page.getByRole('article').first()).toBeVisible();
}

test('editing the template changes the example as it is typed', async ({ page }) => {
	await open(page);
	const fangs = page.getByTestId('example').filter({ hasText: 'Golden Arrows' });
	await expect(fangs).toContainText('IN PLACE');

	await page.getByRole('button', { name: 'Presets' }).click();
	await page.getByRole('menuitem', { name: /Artist \/ Album \/ NN Title/ }).click();
	await expect(fangs).toContainText('NEW FOLDER');
	await expect(fangs.getByTestId('example-new')).toContainText(
		'Falling Up/Fangs!/05 Golden Arrows.flac'
	);

	// A field typed after a <, from the list.
	const field = page.getByRole('combobox', { name: 'Template', exact: true });
	await field.focus();
	await page.keyboard.press('Backspace');
	await page.keyboard.type('<tit');
	await expect(page.getByRole('option', { name: /Title/ })).toBeVisible();
	await page.keyboard.press('Enter');
	await expect(fangs.getByTestId('example-new')).toContainText('05 Golden Arrows.flac');

	// A broken one is marked where it breaks, and Preview waits for it.
	await page.keyboard.type(' <Tilte>');
	await expect(page.getByRole('alert')).toContainText('NO FIELD IS CALLED "TILTE"');
	await expect(page.getByRole('button', { name: 'Preview', exact: true })).toBeDisabled();
});

test('the preview filters to its conflicts, and one says why', async ({ page }) => {
	await open(page);
	await preview(page);
	await expect(page.getByTestId('counts')).toContainText('29 WOULD MOVE · 3 IN CONFLICT');

	await page.getByRole('button', { name: /^Conflicts/ }).click();
	const deluxe = page.getByRole('article', { name: 'Wish Upon a Blackstar (Deluxe)' });
	await expect(deluxe).toBeVisible();
	await expect(page.getByRole('article', { name: 'Church Music' })).toHaveCount(0);

	await deluxe.getByRole('button', { name: 'WHY ›' }).last().click();
	const card = deluxe.getByRole('region', { name: 'Why it stays' });
	await expect(card).toContainText('2 songs want');
	await expect(card).toContainText('They look like the same recording.');
	await expect(card).toContainText('ALREADY THERE');
	// Duplicates is plan 069's: until then its way in is marked SOON.
	await expect(card.getByRole('button', { name: 'Compare in Duplicates ›' })).toBeDisabled();
	await expect(card).toContainText('SOON');

	const braid = page.getByRole('article', { name: 'Frame & Canvas' });
	await braid.getByRole('button', { name: 'WHY ›' }).click();
	await expect(braid).toContainText('A file Kitsune doesn’t play is already there');

	await page.getByRole('textbox', { name: 'Find in the preview' }).fill('braid');
	await expect(deluxe).toHaveCount(0);
	await expect(braid).toBeVisible();
});

test('a run stops at an album’s end, and Undo puts it back', async ({ page }) => {
	await open(page, 700);
	await preview(page);
	await page.getByRole('button', { name: 'Run it' }).click();
	await expect(page.getByTestId('run-head')).toContainText('Moving');
	await expect(page.getByText('MOVING', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Stop', exact: true }).click();

	const done = page.getByTestId('done');
	await expect(done).toContainText('STOPPED');
	await expect(done).toContainText(/Moved \d+ songs?/);

	await done.getByRole('button', { name: 'Undo this run' }).click();
	await done.getByRole('button', { name: 'Put them back' }).click();
	await expect(page.getByTestId('done')).toContainText(/Put back \d+ songs?/, { timeout: 10_000 });
	await expect(page.getByText('UNDONE', { exact: true })).toBeVisible();
});

test('a whole run finishes, and moved songs preview as in place', async ({ page }) => {
	await open(page, 30);
	await preview(page);
	await page.getByRole('button', { name: 'Run it' }).click();
	await expect(page.getByTestId('done')).toContainText('FINISHED');
	await expect(page.getByTestId('done')).toContainText('MOVED 29 SONGS INTO');
	await page.getByRole('button', { name: 'Preview', exact: true }).click();
	await expect(page.getByTestId('counts')).toContainText('0 WOULD MOVE');
	await expect(page.getByText('Nothing here.')).toBeVisible();
});

test('an album’s ⋯ organizes its files, or says they are in place', async ({ page }) => {
	await page.goto(fixtures('/album/glasshouse-66'));
	await page.getByRole('button', { name: 'More for Glasshouse', exact: true }).click();
	await page.getByRole('menuitem', { name: /Organize files…/ }).click();
	await expect(page).toHaveURL(/\/server\/organize\?preview=/);
	await expect(page.getByText('Organizing 10 songs from Glasshouse')).toBeVisible();
	await expect(page.getByRole('article', { name: 'Glasshouse' })).toBeVisible();

	await page.getByRole('button', { name: 'Show the whole library' }).click();
	await expect(page.getByText('Organizing 10 songs from Glasshouse')).toHaveCount(0);
	await expect(page.getByRole('article', { name: 'Church Music' })).toBeVisible();
});

test('an album already in place gets a toast', async ({ page }) => {
	await page.goto(fixtures('/album/night-ferry-94'));
	await page.getByRole('button', { name: 'More for Night Ferry', exact: true }).click();
	await page.getByRole('menuitem', { name: /Organize files…/ }).click();
	const toast = page.getByRole('status').filter({ hasText: 'already where the template' });
	await expect(toast).toContainText('These 8 songs are already where the template puts them.');
	await toast.getByRole('link', { name: 'See the template ›' }).click();
	await expect(page.getByRole('heading', { name: 'Organize.' })).toBeVisible();
});

test('on a phone the preview is one column, Run it the width of the page', async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto(fixtures('/server/organize'));
	await expect(page.getByTestId('example')).toHaveCount(1);
	await preview(page);
	const run = page.getByRole('button', { name: /Run it · move 29 songs/ });
	await expect(run).toBeVisible();
	const box = await run.boundingBox();
	expect(box?.width ?? 0).toBeGreaterThan(340);
	const width = await page.evaluate(() => document.documentElement.scrollWidth);
	expect(width).toBeLessThanOrEqual(390);
});

test('with tag writes off it previews, and nothing runs', async ({ page }) => {
	await page.goto(fixtures('/server/organize?tagWrites=off'));
	await preview(page);
	await expect(page.getByText('Tag writes are off, so nothing on disk changes.')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Run it' })).toHaveCount(0);
});
