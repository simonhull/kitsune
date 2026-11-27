import type { Page } from '@playwright/test';
import { expect, fixtures, landed, test } from './fixture';
import { journey, nav, slowClock } from './motion';

/**
 * Artist art, bios and album extras (plan 072's first phase), on the fixtures: Low Tide Choir has
 * everything, Ana Lindqvist only what her folder holds, Ines Duarte nothing and no albums of her
 * own. Pictures develop in place once decoded and never hold a journey; an `artists` event develops
 * them on an open page and moves nothing.
 */

test.beforeEach(async ({ page }) => {
	await slowClock(page);
});

const stage = (page: Page) => page.locator('.shell > .backdrop [data-morph-stage="artist"]');
const hero = (page: Page) => page.locator('.shell > main .artist .words');

/** Waits for every picture inside `scope` to have decoded. */
async function developed(page: Page, scope: string): Promise<void> {
	await page.waitForFunction((selector) => {
		const pictures = [...document.querySelectorAll(`${selector} [data-picture]`)];
		return pictures.length > 0 && pictures.every((each) => each.hasAttribute('data-picture-ready'));
	}, scope);
}

async function playGlasshouse(page: Page): Promise<void> {
	await page.goto(fixtures('/album/glasshouse-66'));
	await page.locator('.shell > main .hero button.play:not(:disabled)').click();
	await expect(page.getByRole('complementary', { name: 'Player' })).toBeVisible();
}

test.describe('the hero', () => {
	test('stands on the backdrop, with the round thumb, the credit and the logo off register', async ({
		page
	}) => {
		await page.goto(fixtures('/artists/artist-low-tide-choir'));
		await expect(page.locator('.shell > main h1')).toHaveText('Low Tide Choir');
		await expect(stage(page)).toHaveAttribute('data-backdrop', '');
		await developed(page, '.shell > .backdrop [data-morph-stage="artist"] .wide');
		// The ground colour is laid under the picture, so the first frame is never empty.
		await expect(stage(page)).toHaveCSS('background-color', 'rgb(42, 26, 51)');
		await expect(stage(page).locator('.logo .misprint')).toBeAttached();
		const thumb = hero(page).locator('.head .thumb');
		await expect(thumb).toHaveCSS('border-radius', '50%');
		await developed(page, '.shell > main .artist .head');
		await expect(hero(page).locator('.credit')).toContainText('PHOTO, BACKDROP AND LOGO');
		await expect(hero(page).getByRole('link', { name: 'FANART.TV ›' })).toHaveAttribute(
			'href',
			'https://fanart.tv/artist/low-tide-choir/'
		);
		// The name stands at the stage's foot, inside it.
		const name = await page.locator('.shell > main h1').boundingBox();
		const box = await stage(page).boundingBox();
		expect(name !== null && box !== null && name.y + name.height <= box.y + box.height).toBe(true);
	});

	test('with no backdrop, stands on the stage of the album they play most', async ({ page }) => {
		await page.goto(fixtures('/artists/artist-ana-lindqvist'));
		await expect(stage(page)).toBeVisible();
		await expect(stage(page)).not.toHaveAttribute('data-backdrop', '');
		await expect(stage(page).locator('.wide .wash')).toBeAttached();
		await developed(page, '.shell > main .artist .head');
		// Her folder's portrait asks for no credit.
		await expect(hero(page).locator('.credit')).toHaveCount(0);
	});

	test('with no art and no albums, there is no stage, and the thumb is the initial', async ({
		page
	}) => {
		await page.goto(fixtures('/artists/artist-ines-duarte'));
		await expect(page.locator('.shell > main h1')).toHaveText('Ines Duarte');
		await expect(page.locator('.shell > main .artist .meta')).toContainText('GUEST');
		await expect(stage(page)).toHaveCount(0);
		await expect(hero(page).locator('.head .initial')).toHaveText('I');
	});
});

test.describe('the bio', () => {
	test('shows its summary, its credit always, and More for the rest', async ({ page }) => {
		await page.goto(fixtures('/artists/artist-low-tide-choir'));
		const bio = page.getByRole('region', { name: 'About Low Tide Choir' });
		await bio.scrollIntoViewIfNeeded();
		await expect(bio.locator('.tag')).toHaveText('WIKIPEDIA');
		const credit = bio.locator('.credit');
		await expect(credit).toContainText('FROM WIKIPEDIA');
		await expect(credit).toContainText('SHORTENED FROM THE ORIGINAL');
		await expect(credit.getByRole('link', { name: 'CC BY-SA 4.0' })).toHaveAttribute(
			'href',
			'https://creativecommons.org/licenses/by-sa/4.0/'
		);
		await expect(credit.getByRole('link', { name: 'READ THE ARTICLE ›' })).toHaveAttribute(
			'href',
			'https://en.wikipedia.org/wiki/Low_Tide_Choir'
		);
		const rest = bio.locator('.rest');
		await expect(rest).toHaveAttribute('inert', '');
		await bio.getByRole('button', { name: 'More' }).click();
		await expect(rest).not.toHaveAttribute('inert', '');
		await expect(rest.getByText('ferry terminal', { exact: false })).toBeVisible();
		await bio.getByRole('button', { name: 'Less' }).click();
		await expect(rest).toHaveAttribute('inert', '');
	});

	test('from the library’s artist.nfo, it says YOUR FILES and is whole', async ({ page }) => {
		await page.goto(fixtures('/artists/artist-ana-lindqvist'));
		const bio = page.getByRole('region', { name: 'About Ana Lindqvist' });
		await expect(bio.locator('.tag')).toHaveText('YOUR FILES');
		await expect(bio.locator('.credit')).toHaveText('FROM ARTIST.NFO IN YOUR LIBRARY');
		await expect(bio.getByRole('button', { name: 'More' })).toHaveCount(0);
	});

	test('an artist with none has no card', async ({ page }) => {
		await page.goto(fixtures('/artists/artist-ines-duarte'));
		await expect(page.locator('.shell > main h1')).toHaveText('Ines Duarte');
		await expect(page.locator('.shell > main .bio')).toHaveCount(0);
	});
});

test.describe('thumbs and initials', () => {
	test('the Artists list draws each, round, with no grey silhouette', async ({ page }) => {
		await page.goto(fixtures('/artists'));
		const row = (name: string) =>
			page.locator('.shell > main li').filter({ has: page.getByText(name, { exact: true }) });
		await expect(row('Low Tide Choir').locator('.thumb [data-picture]')).toBeAttached();
		await expect(row('Kaito Ferreira').locator('.thumb .initial')).toHaveText('K');
		// Its own colour, the same each time.
		const colour = await row('Kaito Ferreira')
			.locator('.initial')
			.evaluate((initial) => getComputedStyle(initial).backgroundColor);
		expect(colour).not.toBe('rgba(0, 0, 0, 0)');
	});

	test('Search, the album’s artist line and Now playing’s name carry one', async ({ page }) => {
		await page.goto(fixtures('/search?q=low'));
		await expect(page.locator('.shell > main a.top .face .thumb [data-picture]')).toBeAttached();
		await page.goto(fixtures('/search?q=o'));
		const hit = page.locator('.shell > main a.artist', { hasText: 'Low Tide Choir' });
		await expect(hit.locator('.thumb [data-picture]')).toBeAttached();
		await expect(
			page.locator('.shell > main a.artist', { hasText: 'Kaito Ferreira' }).locator('.initial')
		).toHaveText('K');
		await playGlasshouse(page);
		await expect(page.locator('.shell > main .hero .artist .thumb [data-picture]')).toBeAttached();
		await page.keyboard.press('n');
		await expect(page).toHaveURL(/now-playing/);
		await expect(page.locator('.shell > main a.artist-link .thumb [data-picture]')).toBeAttached();
	});
});

test('an artists event develops the backdrop on an open page, and moves nothing', async ({
	page
}) => {
	await page.goto(fixtures('/artists/artist-low-tide-choir?art=late'));
	await expect(page.locator('.shell > main h1')).toHaveText('Low Tide Choir');
	await expect(hero(page).locator('.head .initial')).toHaveText('L');
	// Before: the stage of her album, under the initial.
	await expect(stage(page)).not.toHaveAttribute('data-backdrop', '');
	await landed(page);
	const before = await page.locator('.shell > main h1').boundingBox();
	await page.evaluate(() => dispatchEvent(new Event('kitsune:art-arrives')));
	await expect(stage(page)).toHaveAttribute('data-backdrop', '');
	// It develops: a print coming up, then itself.
	await expect(stage(page).locator('.wide img.fresh')).toBeAttached();
	await developed(page, '.shell > .backdrop [data-morph-stage="artist"] .wide');
	await expect(stage(page).locator('.wide img.fresh')).toHaveCount(0);
	await developed(page, '.shell > main .artist .head');
	const after = await page.locator('.shell > main h1').boundingBox();
	expect(after).toEqual(before);
	await expect(page.getByRole('region', { name: 'About Low Tide Choir' })).toBeAttached();
});

test.describe('Now playing', () => {
	test('stands on the artist’s backdrop, the disc out of its sleeve behind the poster', async ({
		page
	}) => {
		await playGlasshouse(page);
		await page.keyboard.press('n');
		await expect(page).toHaveURL(/now-playing/);
		await expect(stage(page)).toHaveAttribute('data-place', 'poster');
		// The cover's light is on the backdrop: Now playing's own stage steps aside.
		await expect(page.locator('.shell > .backdrop .poster.lit')).toBeAttached();
		const disc = page.locator('.shell > main .stack .disc');
		await expect(disc).toBeAttached();
		await landed(page);
		await expect(disc).not.toHaveClass(/sleeve/);
		const art = await page.locator('.shell > main .stack .art').boundingBox();
		const out = await disc.boundingBox();
		// Behind the poster, out toward the open side.
		expect(art !== null && out !== null && out.x + out.width > art.x + art.width).toBe(true);
	});

	test('with no disc art and no backdrop, it is as it was', async ({ page }) => {
		await page.goto(fixtures('/album/night-ferry-94'));
		await page.locator('.shell > main .hero button.play:not(:disabled)').click();
		await page.keyboard.press('n');
		await expect(page).toHaveURL(/now-playing/);
		await landed(page);
		await expect(page.locator('.shell > main .stack .disc')).toHaveCount(0);
		await expect(stage(page)).toHaveCount(0);
		await expect(page.locator('.shell > .backdrop [data-morph-stage="poster"]')).toBeVisible();
	});

	test('Now playing → Artist keeps the backdrop: it reshapes into the hero’s stage', async ({
		page
	}) => {
		await playGlasshouse(page);
		await page.keyboard.press('n');
		await landed(page);
		await stage(page).evaluate((element) => {
			element.dataset['kept'] = '';
		});
		await journey(
			page,
			() => page.locator('.shell > main a.artist-link').click(),
			/\/artists\/artist-low-tide-choir/
		);
		// The same element: it never left, so it never came up again.
		await expect(stage(page)).toHaveAttribute('data-kept', '');
		await expect(stage(page)).toHaveAttribute('data-place', 'hero');
		await expect(stage(page).locator('.light')).toHaveCSS('opacity', '0');
	});
});

test.describe('album extras', () => {
	test('the disc out of its sleeve behind the cover, and the Artwork strip', async ({ page }) => {
		await page.goto(fixtures('/album/glasshouse-66'));
		const disc = page.locator('.shell > main .hero .disc');
		await expect(disc).toBeAttached();
		await landed(page);
		const strip = page.getByRole('region', { name: 'Artwork' });
		await strip.scrollIntoViewIfNeeded();
		await expect(strip.locator('.count')).toHaveText('3 PICTURES · 1 DISC');
		await expect(strip.locator('.label')).toHaveText(['FRONT', 'BACK', 'DISC']);
		await expect(strip.locator('.from')).toHaveText([
			'DRAWN BY KITSUNE',
			'IN THE FILES',
			'FANART.TV'
		]);
		await developed(page, '.shell > main .strip');
	});

	test('a cover from the Cover Art Archive says so under it; no extras, no strip', async ({
		page
	}) => {
		await page.goto(fixtures('/album/night-ferry-94'));
		await expect(
			page.getByRole('link', { name: 'COVER FROM THE COVER ART ARCHIVE ›' })
		).toBeVisible();
		await expect(page.locator('.shell > main .hero .disc')).toHaveCount(0);
		await expect(page.getByRole('region', { name: 'Artwork' })).toHaveCount(0);
	});
});

test.describe('the journeys into an artist', () => {
	test('Artists → Artist: the stage comes up in front as the list falls back', async ({ page }) => {
		await page.goto(fixtures('/artists'));
		const row = page.locator('.shell > main li a', { hasText: 'Low Tide Choir' });
		await row.waitFor();
		await landed(page);
		await journey(page, () => row.click(), /\/artists\/artist-low-tide-choir/);
		await expect(stage(page)).toHaveAttribute('data-backdrop', '');
		await journey(page, () => nav(page, 'Artists').click(), /\/artists$/);
		await expect(stage(page)).toHaveCount(0);
	});

	test('Library → Artist and Search → Artist', async ({ page }) => {
		await page.goto(fixtures('/library'));
		// Every one of the fixtures' artists has a stage: a backdrop, or the stage of their album.
		const heading = page.locator('.shell > main h2 a').nth(1);
		await heading.waitFor();
		await landed(page);
		await journey(page, () => heading.click(), /\/artists\/.+/);
		await expect(stage(page)).toBeVisible();
		await page.goto(fixtures('/search?q=o'));
		const hit = page.locator('.shell > main a.artist', { hasText: 'Low Tide Choir' });
		await hit.waitFor();
		await landed(page);
		await journey(page, () => hit.click(), /\/artists\/artist-low-tide-choir/);
		await expect(stage(page)).toBeVisible();
	});

	test('Album → Artist: the album’s stage gives way under the artist’s', async ({ page }) => {
		await page.goto(fixtures('/album/glasshouse-66'));
		await landed(page);
		await page.evaluate(() => {
			const ghost = document.querySelector('.ghost-layer');
			const held = { album: 0 };
			Object.assign(window, { heldStages: held });
			new MutationObserver((changes) => {
				for (const change of changes) {
					for (const node of change.addedNodes) {
						if (node instanceof HTMLElement && node.classList.contains('album')) held.album += 1;
					}
				}
			}).observe(ghost ?? document.body, { childList: true });
		});
		await journey(
			page,
			() => page.locator('.shell > main .hero .name a').first().click(),
			/\/artists\/artist-low-tide-choir/
		);
		expect(
			await page.evaluate(
				() => (window as unknown as { heldStages: { album: number } }).heldStages.album
			)
		).toBe(1);
		await expect(stage(page)).toBeVisible();
	});
});

test.describe('wherever it is seen', () => {
	test('at 390 the square thumb is the stage, with the logo small at its top right', async ({
		page
	}) => {
		await page.setViewportSize({ width: 390, height: 844 });
		await page.goto(fixtures('/artists/artist-low-tide-choir'));
		await expect(stage(page)).toBeVisible();
		const box = await stage(page).boundingBox();
		expect(box?.height).toBeCloseTo(390, 0);
		await expect(stage(page).locator('.square [data-picture]')).toBeVisible();
		await expect(stage(page).locator('.wide')).toBeHidden();
		// The stage is the thumb, so the head has none of its own.
		await expect(hero(page).locator('.head .thumb')).toHaveCount(0);
		const logo = await stage(page).locator('.logo').boundingBox();
		expect(logo?.width).toBe(190);
		// Nothing wider than the window: no sideways scroll.
		expect(
			await page.evaluate(
				() => document.documentElement.scrollWidth - document.documentElement.clientWidth
			)
		).toBeLessThanOrEqual(0);
	});

	test('at 390 Now playing sets the poster left and the disc past the edge', async ({ page }) => {
		await page.setViewportSize({ width: 390, height: 844 });
		await playGlasshouse(page);
		await page.keyboard.press('n');
		await landed(page);
		const art = await page.locator('.shell > main .stack .art').boundingBox();
		expect(art?.x).toBeLessThan(40);
		expect(art?.width).toBeCloseTo(292, 0);
		// The disc comes out of its sleeve @560 into the moment, after the units land.
		const disc = page.locator('.shell > main .stack .disc');
		await expect
			.poll(async () => {
				const box = await disc.boundingBox();
				return box !== null && box.x + box.width > 390;
			})
			.toBe(true);
	});

	test('at 3840 the hero takes the size step', async ({ page }) => {
		await page.setViewportSize({ width: 3840, height: 2160 });
		await page.goto(fixtures('/artists/artist-low-tide-choir'));
		await developed(page, '.shell > .backdrop [data-morph-stage="artist"] .wide');
		const thumb = await hero(page).locator('.head .thumb').boundingBox();
		expect(thumb?.width).toBeCloseTo(140, 0);
		const logo = await stage(page).locator('.logo').boundingBox();
		expect(logo?.width).toBeCloseTo(650, 0);
		// The 3840 rendition is asked for (the fixtures draw one picture for every size).
		await expect(stage(page)).toBeVisible();
	});
});
