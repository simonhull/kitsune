import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** Nothing on the page names a heart: no button, no image, no word. */
async function noHeart(page: Page): Promise<void> {
	await expect(page.getByRole('button', { name: /heart/i })).toHaveCount(0);
	await expect(page.getByRole('img', { name: /heart/i })).toHaveCount(0);
	await expect(page.getByText(/hearted|by heart|from your heart/i)).toHaveCount(0);
}

test('no heart anywhere: the Library, an album, the island and Now playing', async ({ page }) => {
	await page.goto(fixtures('/'));
	await page.getByRole('link', { name: 'Library', exact: true }).click();
	const row = page.locator('[data-track]').first();
	await expect(row).toBeVisible();
	await expect(page.getByRole('group', { name: 'Show' }).getByRole('button')).toHaveText([
		/Everything/,
		'★ 4 and up'
	]);
	await noHeart(page);

	const title = await row.getAttribute('aria-label');
	await row.getByRole('button', { name: `Play ${title}` }).click();
	const island = page.getByRole('complementary', { name: 'Player' });
	await expect(island.getByRole('slider', { name: `Rating of ${title}` })).toBeVisible();
	await noHeart(page);

	const albumLink = page.locator('.list a.title[href^="/album/"]').first();
	const album = (await albumLink.textContent())?.trim() ?? '';
	await albumLink.click();
	await expect(page.getByRole('heading', { name: album, exact: true })).toBeVisible();
	await noHeart(page);

	await page.locator('a[href="/now-playing"]').first().click();
	await expect(page.getByRole('slider', { name: `Rating of ${title}` }).first()).toBeVisible();
	await noHeart(page);
});

test('a song is rated from the island, and its row says so', async ({ page }) => {
	await page.goto(fixtures('/'));
	await page.getByRole('link', { name: 'Library', exact: true }).click();
	const row = page.locator('[data-track]').first();
	await expect(row).toBeVisible();
	const title = (await row.getAttribute('aria-label')) ?? '';
	await row.getByRole('button', { name: `Play ${title}` }).click();

	const stars = page
		.getByRole('complementary', { name: 'Player' })
		.getByRole('slider', { name: `Rating of ${title}` });
	await expect(stars).toBeVisible();
	// The island has landed: measured while it rises, the stars aren't where the click goes.
	await expect(page.getByRole('complementary', { name: 'Player' })).not.toHaveClass(/arrive/);
	// A star it doesn't have yet: the right half of the fourth is 4, of the third 3.
	const want = (await stars.getAttribute('aria-valuenow')) === '4' ? 3 : 4;
	const box = await stars.boundingBox();
	if (box === null) {
		throw new Error('the island has no stars');
	}
	await page.mouse.click(box.x + box.width * (want / 5 - 0.03), box.y + box.height / 2);
	await expect(stars).toHaveAttribute('aria-valuenow', String(want));
	// The row's menu has the same stars: one rating, wherever it shows.
	await row.getByRole('button', { name: `More for ${title}` }).click();
	await expect(
		page.getByRole('menu').getByRole('slider', { name: `Rating of ${title}` })
	).toHaveAttribute('aria-valuenow', String(want));
});

test('a phone’s island leaves the rating to Now playing', async ({ page }) => {
	await page.goto(fixtures('/'));
	await page.getByRole('link', { name: 'Library', exact: true }).click();
	const row = page.locator('[data-track]').first();
	const title = (await row.getAttribute('aria-label')) ?? '';
	await row.getByRole('button', { name: `Play ${title}` }).click();
	const island = page.getByRole('complementary', { name: 'Player' });
	await expect(island.getByRole('slider', { name: `Rating of ${title}` })).toBeVisible();
	await page.setViewportSize({ width: 390, height: 844 });
	await expect(island.getByRole('slider', { name: `Rating of ${title}` })).toBeHidden();
	await page.locator('a[href="/now-playing"]').first().click();
	await expect(page.getByRole('slider', { name: `Rating of ${title}` }).first()).toBeVisible();
});

test('the You page turns the hearts into stars once, and the card is gone', async ({ page }) => {
	await page.goto(fixtures('/you?lastfm=connected'));
	const card = page.getByRole('region', { name: 'Stars say it better.' });
	await expect(card).toBeVisible();
	await expect(card.getByText('You hearted 5 songs.')).toBeVisible();
	// Most of the hearted songs rated are 4★, so the card offers 4★.
	await expect(card.getByRole('radio', { name: '4 stars' })).toHaveAttribute(
		'aria-checked',
		'true'
	);
	await expect(card.getByText('What the Water Kept')).toBeVisible();
	await card.getByRole('radio', { name: '5 stars' }).click();
	await card.getByRole('button', { name: 'Rate 2 songs 5★' }).click();
	await expect(card).toHaveCount(0);

	// Away and back: the server says there's nothing left, so it's gone for good.
	await page.getByRole('link', { name: 'Library', exact: true }).click();
	await expect(page.locator('[data-track]').first()).toBeVisible();
	await page.getByRole('link', { name: 'You', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'HOW KITSUNE LOOKS' })).toBeVisible();
	await expect(page.getByRole('region', { name: 'Stars say it better.' })).toHaveCount(0);
});

test('Last.fm loves follow a love line, and leaving the hearts unrated is a choice too', async ({
	page
}) => {
	await page.goto(fixtures('/you?lastfm=connected'));
	await expect(page.getByText('Love songs rated')).toBeVisible();
	await expect(page.getByText('4★ AND UP')).toBeVisible();
	await expect(page.getByRole('checkbox', { name: 'Love on Last.fm' })).toBeChecked();
	// Glasshouse's 5, 4½, 5 and two 4s, and Tidewrack's and the others' 4s and 5s, and once the two
	// unrated hearts are 4★, them too.
	await expect(page.getByText(/That's \d+ songs once your hearts become stars\./)).toBeVisible();

	await page
		.getByRole('radiogroup', { name: 'Love songs rated from' })
		.getByRole('radio', { name: '5 stars' })
		.click();
	await expect(page.getByText('5★ AND UP')).toBeVisible();
	await expect(page.getByText('Rating a song 5★ or more loves it on Last.fm')).toBeVisible();

	const card = page.getByRole('region', { name: 'Stars say it better.' });
	await card.getByRole('button', { name: 'Leave them unrated' }).click();
	await expect(card).toHaveCount(0);
	await expect(page.getByText(/That's \d+ songs\. Loves already/)).toBeVisible();
});
