import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** Opens Glasshouse from Albums, so Back from it has somewhere to go. */
async function openAlbum(page: Page): Promise<void> {
	await page.goto(fixtures('/'));
	await page.getByRole('link', { name: 'Albums', exact: true }).click();
	await page.locator('a[href^="/album/glasshouse-66"]').first().click();
	await expect(page.getByRole('heading', { name: 'Glasshouse', exact: true })).toBeVisible();
}

/** The editor, open over the page. */
const editor = (page: Page) => page.getByRole('region', { name: 'Editing songs' });

async function editAlbum(page: Page): Promise<void> {
	await page.getByRole('button', { name: 'Edit tags', exact: true }).click();
	await expect(editor(page).getByText('No changes yet')).toBeVisible();
}

/** Picks the first Library song of each album, scrolling the list until it shows. */
async function pickInLibrary(page: Page, albums: readonly string[]): Promise<void> {
	await page.goto(fixtures('/'));
	await page.getByRole('link', { name: 'Library', exact: true }).click();
	await expect(page.locator('[data-track]').first()).toBeVisible();
	for (const album of albums) {
		const row = page.locator(`[data-track^="${album}-"]`).first();
		while ((await row.count()) === 0) {
			await page.mouse.wheel(0, 400);
		}
		await row.scrollIntoViewIfNeeded();
		await row.getByRole('checkbox', { name: /^Select / }).click();
	}
}

test('Edit tags opens one album with every song ticked, edits rows, ticked songs and one song', async ({
	page
}) => {
	await openAlbum(page);
	await editAlbum(page);
	const songs = editor(page).getByRole('region', { name: 'Songs in this edit' });
	await expect(songs.getByRole('checkbox', { name: 'Every song' })).toBeChecked();
	await expect(songs.getByText('All 10 songs')).toBeVisible();
	await expect(editor(page).getByRole('tab', { name: 'Matches' })).toBeVisible();

	// A title is renamed in its row.
	await songs.getByRole('textbox', { name: 'Title of Panes' }).fill('Panes (live)');
	await expect(editor(page)).toContainText('1 change · Panes’s title');

	// Only the ticked songs take a shared field.
	await songs.getByRole('checkbox', { name: 'Every song' }).click();
	await songs.getByRole('checkbox', { name: 'Edit Breakwater' }).check();
	await songs.getByRole('checkbox', { name: 'Edit Salt in the Lungs' }).check();
	await expect(songs.getByText('2 of 10 songs')).toBeVisible();
	await editor(page).getByRole('textbox', { name: 'Year' }).fill('1999');
	await expect(editor(page)).toContainText('2 changes · across 3 files');

	// › opens one song's every tag; a raw tag is edited there; back to all of them.
	await songs.getByRole('button', { name: 'Every tag of Ribbons' }).click();
	const one = editor(page).getByRole('group', { name: 'Every tag of Ribbons' });
	await expect(one).toBeVisible();
	await one.getByRole('textbox', { name: 'ENCODED-BY' }).fill('Kitsune');
	await one.getByRole('button', { name: 'All 10 songs' }).click();
	await expect(one).toHaveCount(0);
	await expect(editor(page)).toContainText('3 changes · across 4 files');

	await editor(page).getByRole('button', { name: 'Write to files' }).click();
	await expect(editor(page)).toHaveCount(0);
	await expect(page.getByText('Tags written.')).toBeAttached();

	// Read back: the ticked songs took the year, the others kept theirs, and the rest held.
	await editAlbum(page);
	const date = async (title: string): Promise<string> => {
		await songs.getByRole('button', { name: `Every tag of ${title}` }).click();
		const field = editor(page).getByRole('group', { name: `Every tag of ${title}` });
		return field.getByRole('textbox', { name: 'Date', exact: true }).inputValue();
	};
	expect(await date('Breakwater')).toBe('1999');
	expect(await date('Small Weather')).toBe('2024-03-12');
	await songs.getByRole('button', { name: 'Every tag of Ribbons' }).click();
	await expect(
		editor(page)
			.getByRole('group', { name: 'Every tag of Ribbons' })
			.getByRole('textbox', { name: 'ENCODED-BY' })
	).toHaveValue('Kitsune');
	await expect(songs.getByRole('textbox', { name: 'Title of Panes (live)' })).toBeVisible();
});

test('a song’s Edit tags… opens its album with it alone ticked and its every tag open', async ({
	page
}) => {
	await openAlbum(page);
	await page.getByRole('button', { name: 'More for Low Tide' }).click();
	await page.getByRole('menuitem', { name: 'Edit tags…' }).click();
	await expect(editor(page).getByRole('group', { name: 'Every tag of Low Tide' })).toBeVisible();
	const songs = editor(page).getByRole('region', { name: 'Songs in this edit' });
	await expect(songs.getByText('1 of 10 songs')).toBeVisible();
	await expect(songs.getByRole('checkbox', { name: 'Edit Low Tide' })).toBeChecked();
});

test('songs picked across albums edit together, without Matches, and both albums take it', async ({
	page
}) => {
	await pickInLibrary(page, ['harbour-lights-6', 'night-ferry-94']);
	await page.getByRole('button', { name: 'Edit 2 songs', exact: true }).click();
	await expect(editor(page)).toContainText('EDITING · 2 SONGS FROM 2 ALBUMS');
	await expect(editor(page).getByText('Harbour Lights', { exact: true })).toBeVisible();
	await expect(editor(page).getByText('Night Ferry', { exact: true })).toBeVisible();
	await expect(editor(page).getByRole('tab', { name: 'Matches' })).toHaveCount(0);
	await expect(editor(page).getByRole('textbox', { name: 'Album', exact: true })).toHaveAttribute(
		'placeholder',
		'Mixed'
	);

	// Unticking one album's songs leaves the other's: one album can be matched.
	const songs = editor(page).getByRole('region', { name: 'Songs in this edit' });
	await songs.getByRole('checkbox').nth(2).uncheck();
	await expect(editor(page).getByRole('tab', { name: 'Matches' })).toBeVisible();
	await songs.getByRole('checkbox').nth(2).check();

	await editor(page).getByRole('textbox', { name: 'Comment' }).fill('Both');
	await editor(page).getByRole('button', { name: 'Write to files' }).click();
	await expect(page.getByText('Tags written.')).toBeAttached();

	// The songs are still picked: the editor reads both files back as written.
	await page.getByRole('button', { name: 'Edit 2 songs', exact: true }).click();
	await expect(editor(page).getByRole('textbox', { name: 'Comment' })).toHaveValue('Both');
});

test('Esc leaves the editor and drops its edits, and Back after it leaves the album', async ({
	page
}) => {
	await openAlbum(page);
	await editAlbum(page);
	await editor(page).getByRole('textbox', { name: 'Title of Panes' }).fill('Not kept');
	await page.keyboard.press('Escape');
	await expect(editor(page)).toHaveCount(0);
	await expect(page.getByRole('button', { name: 'Edit tags', exact: true })).toBeFocused();
	await editAlbum(page);
	await expect(editor(page).getByRole('textbox', { name: 'Title of Panes' })).toHaveValue('Panes');
	await page.keyboard.press('Escape');
	await page.goBack();
	await expect(page).toHaveURL(/\/albums(\?|$)/);
});

test.describe('on a phone', () => {
	test.use({ viewport: { width: 390, height: 844 } });

	test('Songs, Tags and More are tabs, and Write says just that', async ({ page }) => {
		await page.goto(fixtures('/album/glasshouse-66'));
		await expect(page.getByRole('heading', { name: 'Glasshouse', exact: true })).toBeVisible();
		await editAlbum(page);
		const tabs = editor(page).getByRole('tablist', { name: 'Songs or tags' });
		await expect(tabs.getByRole('tab', { name: 'Tags' })).toHaveAttribute('aria-selected', 'true');
		await expect(editor(page).getByRole('textbox', { name: 'Year' })).toBeVisible();
		await expect(editor(page).getByRole('textbox', { name: 'Title of Panes' })).toBeHidden();
		await expect(editor(page).getByRole('button', { name: 'Write', exact: true })).toBeVisible();

		await tabs.getByRole('tab', { name: 'Songs · 10 of 10' }).click();
		await expect(editor(page).getByRole('textbox', { name: 'Title of Panes' })).toBeVisible();
		await expect(editor(page).getByRole('textbox', { name: 'Year' })).toHaveCount(0);

		await tabs.getByRole('tab', { name: 'More' }).click();
		await expect(editor(page).getByRole('switch', { name: /^Exclude all 10/ })).toBeVisible();
		await expect(editor(page).getByRole('button', { name: 'Find matches' })).toBeVisible();
	});
});
