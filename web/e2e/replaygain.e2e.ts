import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** Opens Health on the fixtures, with each album of a ReplayGain run taking `albumMs`. */
async function health(page: Page, albumMs = 400, query = ''): Promise<void> {
	await page.goto(fixtures(`/health?gainAlbum=${String(albumMs)}${query}`));
	await expect(page.getByTestId('replaygain')).toBeVisible();
}

test('the card writes, stops at an album’s end, and Undo takes it out', async ({ page }) => {
	await health(page, 700);
	const card = page.getByTestId('replaygain');
	await expect(card).toContainText('826');
	await expect(card).toContainText('songs are levelled only in Kitsune');
	await expect(card).toContainText('ON 10 ALBUMS · FLAC AND MP3');
	await expect(card).toContainText('+ 7 MORE');

	await card.getByRole('button', { name: 'Write ReplayGain to 826 files' }).click();
	await expect(page.getByTestId('gain-head')).toContainText('Writing');
	await expect(card.getByText('WRITING', { exact: true })).toBeVisible();
	await expect(page.getByRole('link', { name: /^Writing ReplayGain:/ })).toContainText(
		'REPLAYGAIN ·'
	);
	await card.getByRole('button', { name: 'Stop', exact: true }).click();

	const done = page.getByTestId('gain-done');
	await expect(done).toContainText('STOPPED');
	await expect(done).toContainText(/Wrote ReplayGain to \d+ songs?/);
	await expect(page.getByRole('link', { name: /^Writing ReplayGain:/ })).toHaveCount(0);

	await done.getByRole('button', { name: 'Undo this run' }).click();
	await expect(done).toContainText(/Take ReplayGain out of \d+ files?\?/);
	await done.getByRole('button', { name: 'Take it out' }).click();
	await expect(page.getByTestId('gain-done')).toContainText(/Took ReplayGain out of \d+ files?/, {
		timeout: 10_000
	});
	await expect(card.getByText('UNDONE', { exact: true })).toBeVisible();
});

test('a whole run finishes, and names the songs it left and why', async ({ page }) => {
	await health(page, 30);
	const card = page.getByTestId('replaygain');
	await card.getByRole('button', { name: 'Write ReplayGain to 826 files' }).click();
	const done = page.getByTestId('gain-done');
	await expect(done).toContainText('FINISHED');
	await expect(done).toContainText('Wrote ReplayGain to 821 songs');
	await expect(card).toContainText('LEFT AS THEY WERE · 5');
	await expect(card).toContainText('CAN’T BE WRITTEN');
	await expect(card).toContainText('Compilations/Rayman 3/041 - Hoodlum Hideout.flac');
	await expect(card).toContainText('+ 2 MORE');
	await expect(card).toContainText('Wrote 821 songs, 5 left');
	await expect(card.getByRole('button', { name: 'Undo: Wrote 821 songs, 5 left' })).toBeVisible();
});

test('a run outlives the page, and the nav’s note leads back to it', async ({ page }) => {
	await health(page, 30_000);
	await page
		.getByTestId('replaygain')
		.getByRole('button', { name: 'Write ReplayGain to 826 files' })
		.click();
	await expect(page.getByTestId('gain-head')).toContainText('Writing 0 of 826');
	await page.getByRole('link', { name: 'Albums', exact: true }).click();
	await expect(page).toHaveURL(/\/albums/);
	await page.getByRole('link', { name: /^Writing ReplayGain:/ }).click();
	await expect(page).toHaveURL(/\/health/);
	// Once the page it left has gone, the head is the run's, still going.
	await expect(page.getByTestId('gain-head')).toHaveCount(1);
	await expect(page.getByTestId('gain-head')).toContainText('Writing 0 of 826');
});

test('the songs still being measured are said, and wait', async ({ page }) => {
	await health(page, 400, '&gain=measuring');
	await expect(page.getByTestId('replaygain')).toContainText(
		'212 STILL BEING MEASURED · WRITE THEM ONCE THEY ARE'
	);
});

test('with tag writes off the card says so in place of the button', async ({ page }) => {
	await health(page, 400, '&tagWrites=off');
	const card = page.getByTestId('replaygain');
	await expect(card).toContainText('Tag writes are off, so nothing on disk changes.');
	await expect(card.getByRole('link', { name: 'Server › Library' })).toBeVisible();
	await expect(card.getByRole('button', { name: /Write ReplayGain/ })).toHaveCount(0);
});

test('with everything written the card goes quiet, and keeps Undo', async ({ page }) => {
	await health(page, 30, '&gain=written');
	const card = page.getByTestId('replaygain');
	await expect(card).toContainText('Every song carries its own ReplayGain');
	await card.getByRole('button', { name: 'Undo: Wrote 818 songs' }).click();
	await card.getByRole('button', { name: 'Take it out' }).click();
	await expect(page.getByTestId('gain-done')).toContainText('Took ReplayGain out of 818 files');
});

test('with nothing to write and no runs there is no card', async ({ page }) => {
	await page.goto(fixtures('/health?gain=none'));
	await expect(page.getByRole('heading', { name: /WAV files/ })).toBeVisible();
	await expect(page.getByTestId('replaygain')).toHaveCount(0);
});

test('someone who isn’t an administrator sees no card and no menu item', async ({ page }) => {
	await page.goto(fixtures('/album/tin-lantern-78?role=user'));
	await page.getByRole('button', { name: 'More for Tin Lantern', exact: true }).click();
	await expect(page.getByRole('menuitem', { name: 'Shuffle' })).toBeVisible();
	await expect(page.getByRole('menuitem', { name: /Write ReplayGain/ })).toHaveCount(0);
});

/** Shows the album's ReplayGain column, from the columns menu on its heading. */
async function showGain(page: Page): Promise<void> {
	await page.getByText('TITLE', { exact: true }).first().click({ button: 'right' });
	await page.getByRole('menuitemcheckbox', { name: 'ReplayGain' }).check();
	await page.keyboard.press('Escape');
}

test('an album’s ⋯ writes its measured songs, and the toast’s Undo takes it out', async ({
	page
}) => {
	await page.goto(fixtures('/album/tin-lantern-78?gainAlbum=200'));
	await showGain(page);
	const first = page.getByRole('group', { name: 'Hollow Rivers', exact: true });
	await expect(first).toContainText('≈');

	await page.getByRole('button', { name: 'More for Tin Lantern', exact: true }).click();
	const item = page.getByRole('menuitem', { name: /Write ReplayGain…/ });
	await expect(item).toContainText('8 SONGS MEASURED · WRITES THEM TO THE FILES');
	await item.click();
	const confirm = page.getByTestId('gain-confirm');
	await expect(confirm).toContainText('WRITE REPLAYGAIN · 8 FILES · FLAC');
	await expect(confirm).toContainText('−8.62 DB · PEAK 1.000');

	// Leave them writes nothing.
	await confirm.getByRole('button', { name: 'Leave them' }).click();
	await expect(confirm).toHaveCount(0);
	await expect(first).toContainText('≈');

	await page.getByRole('button', { name: 'More for Tin Lantern', exact: true }).click();
	await page.getByRole('menuitem', { name: /Write ReplayGain…/ }).click();
	await page.getByRole('button', { name: 'Write 8 files' }).click();
	const toast = page.getByTestId('gain-toast');
	await expect(toast).toContainText('Wrote ReplayGain to 8 files of Tin Lantern');
	await expect(first).not.toContainText('≈');
	await expect(first).toContainText('dB');

	// Written, there's nothing left for the ⋯ to write.
	await page.getByRole('button', { name: 'More for Tin Lantern', exact: true }).click();
	await expect(page.getByRole('menuitem', { name: 'Shuffle' })).toBeVisible();
	await expect(page.getByRole('menuitem', { name: /Write ReplayGain/ })).toHaveCount(0);
	await page.keyboard.press('Escape');

	await toast.getByRole('button', { name: 'Undo' }).click();
	await expect(page.getByTestId('gain-toast')).toContainText(
		'Took ReplayGain out of 8 files of Tin Lantern'
	);
	await expect(first).toContainText('≈');
});

test.describe('on a phone', () => {
	test.use({ viewport: { width: 390, height: 844 } });

	test('the card stacks, and its button is the card’s width', async ({ page }) => {
		await health(page);
		const card = page.getByTestId('replaygain');
		await expect(card).toContainText('BIGGEST: RAYMAN 3 – THE COMPLETE SOUNDTRACK (292)');
		const button = card.getByRole('button', { name: 'Write ReplayGain to 826 files' });
		const [cardBox, buttonBox] = await Promise.all([card.boundingBox(), button.boundingBox()]);
		expect(buttonBox?.width ?? 0).toBeGreaterThan((cardBox?.width ?? 0) - 60);
		await button.click();
		await expect(page.getByTestId('gain-head')).toContainText('Writing');
		await card.getByRole('button', { name: 'Stop', exact: true }).click();
		await expect(page.getByTestId('gain-done')).toContainText('STOPPED');
	});

	test('the album’s confirm is a sheet from the bottom', async ({ page }) => {
		await page.goto(fixtures('/album/tin-lantern-78?gainAlbum=100'));
		await page.getByRole('button', { name: 'More for Tin Lantern', exact: true }).click();
		await page.getByRole('menuitem', { name: /Write ReplayGain…/ }).click();
		const sheet = page.getByRole('dialog', { name: 'Write ReplayGain' });
		await expect(sheet).toBeVisible();
		// Once it has risen into place: flush with the foot of the screen, and its whole width.
		await expect
			.poll(async () => {
				const box = await sheet.boundingBox();
				return [
					Math.round(box?.x ?? -1),
					Math.round((box?.y ?? 0) + (box?.height ?? 0)),
					(box?.width ?? 0) >= 360
				];
			})
			.toEqual([0, 844, true]);
		await sheet.getByRole('button', { name: 'Write 8 files' }).click();
		await expect(page.getByTestId('gain-toast')).toContainText(
			'Wrote ReplayGain to 8 files of Tin Lantern'
		);
	});
});
