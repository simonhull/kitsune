import { expect, fixtures, test } from './fixture';

test('a hearted song with no stars joins 4 and up once hearts count as 4', async ({ page }) => {
	await page.goto(fixtures('/you'));
	const card = page.getByRole('region', { name: 'Hearts' });
	await expect(card.getByText('OFF', { exact: true })).toBeVisible();
	await card.getByRole('slider', { name: 'A heart counts as' }).focus();
	await page.keyboard.press('4');
	await expect(card.getByText('4★ WHEN UNRATED')).toBeVisible();

	await page.getByRole('link', { name: 'Library', exact: true }).click();
	await page.getByRole('button', { name: '4 stars and up, hearts included' }).click();
	// Hearted, never rated: there by its heart alone, and counted as such.
	const row = page.locator('[data-track]', { hasText: 'What the Water Kept' });
	await expect(row).toBeVisible();
	await expect(page.getByText(/· 2 BY HEART/)).toBeAttached();
	// Its ⋯ menu shows the heart's stars, and says where they came from.
	await row.getByRole('button', { name: 'More for What the Water Kept' }).click();
	await expect(page.getByRole('slider', { name: 'Rating of What the Water Kept' })).toHaveAttribute(
		'aria-valuetext',
		'4 stars from your heart. Rate it yourself to change it.'
	);
});
