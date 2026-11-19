import type { Locator, Page } from '@playwright/test';
import { expect, fixtures, landed, test } from './fixture';
import { expectInFront, nav, seen, slowClock, slowMotion, turnsBack, watch } from './motion';

/**
 * The player and Now playing (plan 071's third milestone), on the fixtures. Every way out of the
 * moment goes back into the hand: the poster lands wherever the page you go to holds the playing album
 * in view, else in the island's thumb, while the controls' pill shrinks back into the island and the
 * page is built in front. Every page the island shows on opens into it, and so does the door. Each is
 * held to its structure, never to pixels or a lucky frame.
 */

test.beforeEach(async ({ page }) => {
	await slowClock(page);
});

const island = (page: Page): Locator => page.locator('aside a[href="/now-playing"]').first();
const close = (page: Page): Locator => page.getByRole('link', { name: 'Close now playing' });

/** Waits for the page on screen to be `path`, by the page's own location. */
async function at(page: Page, path: string | RegExp): Promise<void> {
	const pattern = typeof path === 'string' ? `^${path.replaceAll('/', '\\/')}$` : path.source;
	await page.waitForFunction((source) => new RegExp(source).test(location.pathname), pattern);
}

/** Plays Glasshouse from Home's Start, the album Continue then holds. */
async function playGlasshouse(page: Page): Promise<void> {
	await page.goto(fixtures('/'));
	await page
		.getByRole('region', { name: 'Start' })
		.getByRole('button', { name: 'Play Glasshouse' })
		.click();
	await island(page).waitFor();
	await landed(page);
}

/** Opens Now playing from the island's cover and waits for it to be still. */
async function openMoment(page: Page): Promise<void> {
	await island(page).click();
	await at(page, '/now-playing');
	await landed(page);
}

/**
 * Presses something on Now playing, slowed, and says where the poster is landing: the element hidden
 * under its flier while it flies. Then lets it land, and holds the journey to the In-Front Rule.
 */
async function landsOn(
	page: Page,
	press: () => Promise<void>,
	to: string | RegExp,
	landing: Locator,
	journey = 'back-into-the-hand'
): Promise<void> {
	await watch(page);
	await slowMotion(page, 20);
	await press();
	await at(page, to);
	await expect(landing).toHaveAttribute('data-morph-hidden');
	await expectInFront(page);
	await slowMotion(page, 1);
	await landed(page);
	const { ghostPages, airPages, pictures, journeys } = await seen(page);
	expect(ghostPages).toBeGreaterThan(0);
	expect(journeys).toEqual([journey]);
	expect(airPages).toBe(0);
	expect(pictures, 'never the blend').toBe(0);
	await expect(landing).not.toHaveAttribute('data-morph-hidden');
}

/** Opens the moment from the island's cover and holds it to the rule. */
async function opens(page: Page, journey = 'into-now-playing'): Promise<void> {
	await watch(page);
	await island(page).click();
	await at(page, '/now-playing');
	await expectInFront(page);
	await landed(page);
	const { ghostPages, airPages, pictures, journeys } = await seen(page);
	expect(ghostPages).toBeGreaterThan(0);
	expect(journeys).toEqual([journey]);
	expect(airPages).toBe(0);
	expect(pictures, 'never the blend').toBe(0);
}

const continueCover = (page: Page): Locator =>
	page.locator('.shell > main [data-morph-key="feature"] .art');
const thumb = (page: Page): Locator => page.locator('aside .thumb');

test.describe('back into the hand', () => {
	test('Close → Home lands the poster in Continue, never retracing the way in', async ({
		page
	}) => {
		await playGlasshouse(page);
		await openMoment(page);
		await landsOn(page, () => close(page).click(), '/', continueCover(page));
		// The island's thumb was never a landing: it came in place with the island.
		await expect(thumb(page)).not.toHaveAttribute('data-morph-hidden');
	});

	test('into a room that holds it in view the poster lands there, else in the island’s thumb', async ({
		page
	}) => {
		await playGlasshouse(page);
		await nav(page, 'Albums').click();
		await at(page, '/albums');
		await landed(page);
		const cell = page.locator('.shell > main [data-morph-key="card:glasshouse-66"] .art');
		await expect(cell).toBeVisible();
		await openMoment(page);
		await landsOn(page, () => close(page).click(), '/albums', cell);

		// Artists holds no cover of it: the poster goes back into the hand.
		await nav(page, 'Artists').click();
		await at(page, '/artists');
		await landed(page);
		await openMoment(page);
		await landsOn(page, () => close(page).click(), '/artists', thumb(page));
	});

	test('the cover goes up to its album and becomes the hero', async ({ page }) => {
		await playGlasshouse(page);
		await openMoment(page);
		await landsOn(
			page,
			() => page.locator('.shell > main a.art').click(),
			'/album/glasshouse-66',
			page.locator('.shell > main .hero .art'),
			'now-playing-to-album'
		);
		await expect(page.locator('.shell > main .hero h1')).toHaveText('Glasshouse');
	});

	test('the artist’s name takes the stage on the way to the artist', async ({ page }) => {
		await playGlasshouse(page);
		await openMoment(page);
		const name = (await page.locator('.shell > main a.artist-link').textContent())?.trim() ?? '';
		await landsOn(
			page,
			() => page.locator('.shell > main a.artist-link').click(),
			/^\/artists\/.+/,
			page.locator('.shell > main h1'),
			'now-playing-to-artist'
		);
		await expect(page.locator('.shell > main h1')).toHaveText(name);
	});

	test('the nav comes back where it stands: a fade, never down from where it went', async ({
		page
	}) => {
		await playGlasshouse(page);
		await openMoment(page);
		await close(page).click();
		await at(page, '/');
		const bar = page.locator('header.bar');
		// Its shape is set at once, unseen; only its opacity moves, a beat after the moment's top line.
		await page.waitForFunction(() =>
			(document.querySelector('header.bar')?.getAnimations() ?? []).some(
				(motion) => motion instanceof CSSTransition && motion.transitionProperty === 'opacity'
			)
		);
		const moving = await bar.evaluate((element) =>
			[element, ...element.querySelectorAll('*')].flatMap((each) =>
				each
					.getAnimations()
					.flatMap((motion) =>
						motion instanceof CSSTransition
							? [{ property: motion.transitionProperty, delay: motion.effect?.getTiming().delay }]
							: []
					)
			)
		);
		expect(moving).toEqual([{ property: 'opacity', delay: 120 }]);
		expect(await bar.evaluate((element) => getComputedStyle(element).transform)).toBe('none');
		await landed(page);
		await expect(bar).not.toHaveAttribute('data-returning');
	});
});

test.describe('into the moment', () => {
	test('opens from every page the island shows on', async ({ page }) => {
		for (const path of [
			'/library',
			'/albums',
			'/artists',
			'/playlists',
			'/search',
			'/you',
			'/health',
			'/server'
		]) {
			await page.goto(fixtures(`${path}?playback=phone`));
			await island(page).waitFor();
			await landed(page);
			await opens(page);
			await expect(page.locator('.shell > main h1')).toBeVisible();
		}
	});

	test('opens from a record, and from its album’s hero when it is on screen', async ({ page }) => {
		await page.goto(fixtures('/artists?playback=phone'));
		await island(page).waitFor();
		await page.locator('.shell > main li a').first().click();
		await at(page, /^\/artists\/.+/);
		await landed(page);
		await opens(page);

		await playGlasshouse(page);
		await page.getByRole('region', { name: 'Start' }).locator('a.open').click();
		await at(page, '/album/glasshouse-66');
		await landed(page);
		await watch(page);
		await slowMotion(page, 20);
		await island(page).click();
		await at(page, '/now-playing');
		// The hero is what rises: it is the source, hidden in the picture of the album left.
		await expect(
			page.locator('.shell > .ghost-layer [data-morph-source="cover:glasshouse-66"]')
		).toBeAttached();
		await slowMotion(page, 1);
		await landed(page);
		expect((await seen(page)).journeys).toEqual(['album-to-now-playing']);
	});

	test('the door opens on the moment when a session ended there', async ({ page }) => {
		await page.goto(fixtures('/login?setup=done&playback=phone&next=%2Fnow-playing'));
		await page.getByLabel('Name').fill('simon');
		await page.getByLabel('Password').fill('a password');
		await watch(page);
		await page.getByRole('button', { name: 'Sign in' }).click();
		await at(page, '/now-playing');
		await expectInFront(page);
		await landed(page);
		const { ghostPages, airPages, pictures, journeys } = await seen(page);
		expect(ghostPages).toBeGreaterThan(0);
		expect(journeys).toEqual(['login-to-now-playing']);
		expect(airPages + pictures).toBe(0);
		await expect(page.locator('.shell > main h1')).toBeVisible();
	});
});

test.describe('presses that come quickly', () => {
	test('the island’s cover pressed while the poster is on its way down turns it back', async ({
		page
	}) => {
		await playGlasshouse(page);
		await openMoment(page);
		await turnsBack(
			page,
			() => close(page).click(),
			() => island(page).click()
		);
		await expect(page.locator('.shell > main h1')).toBeVisible();
	});

	test('Close pressed while the moment is still opening sends the poster into the hand from there', async ({
		page
	}) => {
		await playGlasshouse(page);
		await turnsBack(
			page,
			() => island(page).click(),
			() => close(page).click()
		);
	});

	test('Close, the cover, Close again: it settles on the last, and leaves nothing behind', async ({
		page
	}) => {
		await playGlasshouse(page);
		await openMoment(page);
		await watch(page);
		await slowMotion(page, 20);
		const inAir = () =>
			page.waitForFunction(
				() => (document.querySelector('.shell > .morph-layer')?.children.length ?? 0) > 0
			);
		await close(page).click();
		await at(page, '/');
		await inAir();
		await island(page).click();
		await at(page, '/now-playing');
		await inAir();
		await close(page).click();
		await at(page, '/');
		await slowMotion(page, 1);
		await landed(page);
		const { journeys, pictures } = await seen(page);
		expect(journeys).toEqual(['back-into-the-hand', 'into-now-playing', 'back-into-the-hand']);
		expect(pictures).toBe(0);
		await expect(page.locator('.shell > .ghost-layer > *')).toHaveCount(0);
		await expect(page.locator('[data-morph-hidden]')).toHaveCount(0);
		await expect(page.locator('header.bar')).not.toHaveAttribute('data-returning');
	});
});

test.describe('wherever it is seen', () => {
	test('at 390 Close → Home lands in Continue, Home → Now playing opens, and each turns back', async ({
		page
	}) => {
		await page.setViewportSize({ width: 390, height: 844 });
		await playGlasshouse(page);
		await opens(page);
		await landsOn(page, () => close(page).click(), '/', continueCover(page));
		await turnsBack(page, () => island(page).click());
	});

	// Two tests, not one: a 4K frame is costly, and one test carrying both ways and a turn ran out of
	// time under a full suite's workers.
	test('at 3840 the same choreography, on the step’s sizes', async ({ page }) => {
		await page.setViewportSize({ width: 3840, height: 2160 });
		await playGlasshouse(page);
		await opens(page);
		await landsOn(page, () => close(page).click(), '/', continueCover(page));
	});

	test('at 3840 Close turns back in mid-flight', async ({ page }) => {
		await page.setViewportSize({ width: 3840, height: 2160 });
		await playGlasshouse(page);
		await openMoment(page);
		// Close is a step back: turning round is the step forward again.
		await turnsBack(
			page,
			() => close(page).click(),
			() => page.evaluate(() => history.forward())
		);
	});

	test('with the server slow, nothing waits for it', async ({ page }) => {
		await page.goto(fixtures('/?latency=300'));
		await page
			.getByRole('region', { name: 'Start' })
			.getByRole('button', { name: 'Play Glasshouse' })
			.click();
		await island(page).waitFor();
		await landed(page);
		await opens(page);
		await landsOn(
			page,
			() => page.locator('.shell > main a.art').click(),
			'/album/glasshouse-66',
			page.locator('.shell > main .hero .art'),
			'now-playing-to-album'
		);
		await opens(page, 'album-to-now-playing');
		await turnsBack(
			page,
			() => close(page).click(),
			() => page.evaluate(() => history.forward())
		);
	});

	test('with reduced motion both ways are the blend, drawn in front, the island at once', async ({
		page
	}) => {
		await page.emulateMedia({ reducedMotion: 'reduce' });
		await playGlasshouse(page);
		for (const [press, to] of [
			[() => island(page).click(), '/now-playing'],
			[() => close(page).click(), '/']
		] as const) {
			await watch(page);
			await press();
			await at(page, to);
			// The island takes its new shape at once: nothing of its glass is moving.
			expect(
				await page
					.locator('.shell:not([data-morph-ghost] *) > aside [data-island-glass]')
					.evaluate((glass) => glass.getAnimations().length)
			).toBe(0);
			await landed(page);
			const { pictures, airPages, ghostPages } = await seen(page);
			expect(pictures, `into ${to}`).toBe(1);
			expect(airPages + ghostPages).toBe(0);
		}
	});
});
