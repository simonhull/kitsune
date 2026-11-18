import type { Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';
import {
	arrive,
	expectInFront,
	expectLanded,
	journey,
	nav,
	seen,
	slowClock,
	turnsBack,
	watch
} from './motion';

/**
 * The journeys of daily use (plan 071's first milestone), on the fixtures. Each is held to three
 * things, read from the page's structure and never from pixels or a lucky frame: the page you go to
 * is drawn in front of the page you leave, the journey lands with nothing left in the air, and going
 * back while it flies turns what is in the air around rather than dropping it.
 */

test.beforeEach(async ({ page }) => {
	await slowClock(page);
});

async function openAlbumFromAlbums(page: Page, at = 1): Promise<void> {
	await page.locator('.shell > main .card a.open').nth(at).click();
	await expect(page).toHaveURL(/\/album\//);
	await expectLanded(page);
}

test.describe('the album’s door, and back to the grid', () => {
	test('Albums → Album opens in front of the grid, and Album → Albums grows the grid from it', async ({
		page
	}) => {
		await page.goto(fixtures('/albums'));
		await page.locator('.shell > main .card a.open').first().waitFor();
		await journey(
			page,
			() => page.locator('.shell > main .card a.open').nth(1).click(),
			/\/album\//
		);
		// The stage came up in front: the album's own.
		await expect(page.locator('.shell > main .hero h1')).toHaveText('Glasshouse');
		await journey(page, () => nav(page, 'Albums').click(), /\/albums/);
		await expect(page.locator('.shell > main [data-morph-key="card:glasshouse-66"]')).toBeVisible();
	});

	test('each turns back in mid-flight', async ({ page }) => {
		await page.goto(fixtures('/albums'));
		await page.locator('.shell > main .card a.open').first().waitFor();
		await turnsBack(page, () => page.locator('.shell > main .card a.open').nth(1).click());
		await openAlbumFromAlbums(page);
		await turnsBack(page, () => nav(page, 'Albums').click());
	});
});

test.describe('Album → Home', () => {
	test('the album Continue holds reshapes into its card', async ({ page }) => {
		await page.goto(fixtures('/'));
		const start = page.getByRole('region', { name: 'Start' });
		await start.getByRole('button', { name: 'Play Glasshouse' }).click();
		await start.locator('a.open').click();
		await expect(page).toHaveURL(/\/album\/glasshouse-66/);
		await expectLanded(page);
		await watch(page);
		await nav(page, 'Home').click();
		// The card's surface is what the album's stage lands on: hidden under its flier until then.
		await expect(
			page.locator('.shell > main [data-morph-key="feature"] .surface[data-morph-hidden]')
		).toBeAttached();
		await expectInFront(page);
		await expectLanded(page);
		expect((await seen(page)).ghostPages).toBeGreaterThan(0);
	});

	test('any other album is set among the others, first in Recently played', async ({ page }) => {
		await page.goto(fixtures('/'));
		const played = page.getByRole('region', { name: 'Recently played' });
		const first = played.locator('a.open').first();
		const title = (await first.locator('.title').textContent()) ?? '';
		await first.click();
		await expect(page).toHaveURL(/\/album\//);
		await expectLanded(page);
		await watch(page);
		await nav(page, 'Home').click();
		// It lands where Home holds it: the first cover in Recently played is the one it flies to.
		await expect(played.locator('.art').first()).toHaveAttribute('data-morph-hidden');
		await expectInFront(page);
		await expectLanded(page);
		await expect(played.locator('.title').first()).toHaveText(title);
	});

	test('an album played and left leads Recently played at once, and lands there', async ({
		page
	}) => {
		await page.goto(fixtures('/albums'));
		await page.locator('.shell > main .card a.open').first().waitFor();
		await openAlbumFromAlbums(page, 4);
		const title = (await page.locator('.shell > main .hero h1').textContent())?.trim() ?? '';
		await page.locator('.shell > main .hero .play').click();
		// Another album takes the player, so this one leaves Continue for the row.
		await arrive(page, () => nav(page, 'Albums').click(), /\/albums/);
		await openAlbumFromAlbums(page, 6);
		await page.locator('.shell > main .hero .play').click();
		await arrive(page, () => page.goBack().then(() => undefined), /\/albums/);
		await page.goBack();
		await expect(page.locator('.shell > main .hero h1')).toHaveText(title);
		await expectLanded(page);
		await nav(page, 'Home').click();
		const played = page.getByRole('region', { name: 'Recently played' });
		await expect(played.locator('.art').first()).toHaveAttribute('data-morph-hidden');
		await expectLanded(page);
		await expect(played.locator('.title').first()).toHaveText(title);
	});

	test('each turns back in mid-flight', async ({ page }) => {
		await page.goto(fixtures('/'));
		const start = page.getByRole('region', { name: 'Start' });
		await start.getByRole('button', { name: 'Play Glasshouse' }).click();
		await arrive(page, () => start.locator('a.open').click(), /\/album\//);
		await turnsBack(page, () => nav(page, 'Home').click());
		await arrive(page, () => nav(page, 'Home').click(), /^\/$/);
		const played = page.getByRole('region', { name: 'Recently played' });
		await arrive(page, () => played.locator('a.open').nth(1).click(), /\/album\//);
		await turnsBack(page, () => nav(page, 'Home').click());
	});
});

test.describe('the name: Album, Artists and Library to an artist, and back', () => {
	test('Album → Artist lets the name take the stage, and Artist → Album opens the door', async ({
		page
	}) => {
		await page.goto(fixtures('/albums'));
		await page.locator('.shell > main .card a.open').first().waitFor();
		await openAlbumFromAlbums(page);
		await journey(
			page,
			() => page.locator('.shell > main .hero .name a').first().click(),
			/\/artists\//
		);
		await expect(page.locator('.shell > main h1')).toHaveText('Low Tide Choir');
		await page.locator('.shell > main .shelves a.open').first().waitFor();
		await journey(
			page,
			() => page.locator('.shell > main .shelves a.open').first().click(),
			/\/album\//
		);
	});

	test('Artists → Artist opens the name’s door, and Artist → Artists sets it back in the roll', async ({
		page
	}) => {
		await page.goto(fixtures('/artists'));
		const link = page.locator('.shell > main li a').nth(5);
		const name = (await link.locator('.name').textContent()) ?? '';
		await journey(page, () => link.click(), /\/artists\/.+/);
		await expect(page.locator('.shell > main h1')).toHaveText(name);
		await journey(page, () => nav(page, 'Artists').click(), /\/artists$/);
	});

	test('Library → Artist from a heading, and back to the Library', async ({ page }) => {
		await page.goto(fixtures('/library'));
		await page.locator('.shell > main h2 a').nth(1).waitFor();
		await journey(page, () => page.locator('.shell > main h2 a').nth(1).click(), /\/artists\/.+/);
		await journey(page, () => nav(page, 'Library').click(), /\/library/);
	});

	test('Albums → Artist from a card’s menu, and back', async ({ page }) => {
		await page.goto(fixtures('/albums'));
		await page.locator('.shell > main .card').nth(1).click({ button: 'right' });
		await journey(
			page,
			() => page.getByRole('menuitem', { name: 'Go to artist' }).click(),
			/\/artists\/.+/
		);
		await journey(page, () => page.goBack().then(() => undefined), /\/albums/);
	});

	test('each turns back in mid-flight', async ({ page }) => {
		await page.goto(fixtures('/artists'));
		const row = page.locator('.shell > main li a').nth(5);
		await turnsBack(page, () => row.click());
		await arrive(page, () => row.click(), /\/artists\/.+/);
		await turnsBack(page, () => nav(page, 'Artists').click());
		const shelf = page.locator('.shell > main .shelves a.open').first();
		await shelf.waitFor();
		await turnsBack(page, () => shelf.click());
		await arrive(page, () => shelf.click(), /\/album\//);
		await turnsBack(page, () => page.locator('.shell > main .hero .name a').first().click());
		await arrive(page, () => nav(page, 'Albums').click(), /\/albums/);
		await page.locator('.shell > main .card').nth(1).click({ button: 'right' });
		await turnsBack(page, () => page.getByRole('menuitem', { name: 'Go to artist' }).click());
	});
});

test.describe('the mosaic: Playlists to a playlist, a playlist to an album, and back', () => {
	test('Playlists → Playlist opens the mosaic’s door, and Playlist → Playlists sets it in the wall', async ({
		page
	}) => {
		await page.goto(fixtures('/playlists'));
		const card = page.locator('.shell > main a.card[href]').first();
		const name = (await card.locator('.name').textContent()) ?? '';
		await journey(page, () => card.click(), /\/playlists\/.+/);
		await expect(page.locator('.shell > main .hero .name')).toHaveValue(name);
		await journey(page, () => nav(page, 'Playlists').click(), /\/playlists$/);
	});

	test('Playlist → Album opens the door from a song’s thumb, and Back gathers it to the song', async ({
		page
	}) => {
		await page.goto(fixtures('/playlists'));
		await arrive(
			page,
			() => page.locator('.shell > main a.card[href]').first().click(),
			/\/playlists\/.+/
		);
		const link = page.locator('.shell > main .row[data-track] a.link').first();
		await link.waitFor();
		await journey(page, () => link.click(), /\/album\//);
		await journey(page, () => page.goBack().then(() => undefined), /\/playlists\/.+/);
	});

	test('each turns back in mid-flight', async ({ page }) => {
		await page.goto(fixtures('/playlists'));
		const card = page.locator('.shell > main a.card[href]').first();
		await turnsBack(page, () => card.click());
		await arrive(page, () => card.click(), /\/playlists\/.+/);
		await turnsBack(page, () => nav(page, 'Playlists').click());
		const link = page.locator('.shell > main .row[data-track] a.link').first();
		await link.waitFor();
		await turnsBack(page, () => link.click());
		await arrive(page, () => link.click(), /\/album\//);
		await turnsBack(
			page,
			() => page.evaluate(() => history.back()),
			() => page.evaluate(() => history.forward())
		);
	});
});

test.describe('Album → Album', () => {
	test('is re-pressed in place: the new record comes up in front of the old, and turns back', async ({
		page
	}) => {
		await page.goto(fixtures('/albums'));
		await page.locator('.shell > main .card a.open').first().waitFor();
		await openAlbumFromAlbums(page);
		await arrive(page, () => nav(page, 'Library').click(), /\/library/);
		await arrive(page, () => page.locator('.shell > main a.art').nth(3).click(), /\/album\//);
		const second = await page.evaluate(() => location.pathname);
		await watch(page);
		await page.evaluate(() => history.go(-2));
		await expect(page).toHaveURL(/\/album\/glasshouse-66/);
		await expectInFront(page);
		await expectLanded(page);
		const { ghostPages } = await seen(page);
		expect(ghostPages).toBeGreaterThan(0);
		// Nothing travels on this one, so turning back is the old page taking over from its ghost.
		await page.evaluate(() => history.go(2));
		await expect(page).toHaveURL(second);
		await page.evaluate(() => history.go(-2));
		await page.waitForFunction(() => location.pathname.includes('glasshouse-66'));
		await expectLanded(page);
	});
});

test.describe('wherever it is seen', () => {
	test('at 390 px every door and gathering lands, and turns back', async ({ page }) => {
		await page.setViewportSize({ width: 390, height: 844 });
		await page.goto(fixtures('/'));
		const start = page.getByRole('region', { name: 'Start' });
		await start.getByRole('button', { name: 'Play Glasshouse' }).click();
		await journey(page, () => start.locator('a.open').click(), /\/album\/glasshouse-66/);
		await journey(page, () => nav(page, 'Home').click(), /\/$/);
		await page.goto(fixtures('/artists'));
		await turnsBack(page, () => page.locator('.shell > main li a').nth(5).click());
	});

	test('with the server slow, nothing waits for it', async ({ page }) => {
		await page.goto(fixtures('/albums?latency=300'));
		await page.locator('.shell > main .card a.open').first().waitFor();
		await journey(
			page,
			() => page.locator('.shell > main .card a.open').nth(1).click(),
			/\/album\//
		);
		await turnsBack(page, () => nav(page, 'Albums').click());
	});

	test('with reduced motion it is the blend, still drawn in front of the page left', async ({
		page
	}) => {
		await page.emulateMedia({ reducedMotion: 'reduce' });
		await page.goto(fixtures('/albums'));
		await page.locator('.shell > main .card a.open').first().waitFor();
		await watch(page);
		await page.locator('.shell > main .card a.open').nth(1).click();
		await expect(page).toHaveURL(/\/album\//);
		await expectLanded(page);
		const { pictures, airPages } = await seen(page);
		// A picture of everything went under the new page, and nothing went over it.
		expect(pictures).toBe(1);
		expect(airPages).toBe(0);
		await expect(page.locator('.shell > .morph-layer > *')).toHaveCount(0);
	});

	test('opened and gathered again and again, it leaves nothing behind', async ({ page }) => {
		await page.goto(fixtures('/albums'));
		await page.locator('.shell > main .card a.open').first().waitFor();
		for (let round = 0; round < 6; round += 1) {
			await page
				.locator('.shell > main .card a.open')
				.nth(round % 4)
				.click();
			await expect(page).toHaveURL(/\/album\//);
			await nav(page, 'Albums').click();
			await expect(page).toHaveURL(/\/albums/);
		}
		await expectLanded(page);
		await expect(page.locator('.shell > .ghost-layer > *')).toHaveCount(0);
	});
});
