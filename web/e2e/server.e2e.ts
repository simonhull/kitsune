import type { Locator, Page } from '@playwright/test';
import { expect, fixtures, landed, test } from './fixture';
import { expectBorn, expectInFront, seen, slowClock, slowMotion, watch } from './motion';

/**
 * The Server pages and tools, and the door (plan 071's fifth milestone). The Server family are rooms:
 * each unfolds from what was pressed in the page (a row, a crumb, a tab, a button), its headline born
 * there; the tools' tabs are one strip that never leaves, only its fill flowing; a group's title and
 * Matching's number travel between the two pages that say them. Signing out, the fox leaves its seat
 * in the nav for the door while the frame dissolves where it stands. Each journey is held to its
 * structure, never to a lucky frame.
 */

test.beforeEach(async ({ page }) => {
	await slowClock(page);
});

const main = (page: Page): Locator => page.locator('.shell > main');

/** A link in the page, by where it goes: the page's own, never the nav's. */
const link = (page: Page, path: string): Locator =>
	main(page).locator(`a[href="${path}"]`).locator('visible=true').first();

async function open(page: Page, path: string): Promise<void> {
	await page.goto(fixtures(path));
	await main(page).locator('h1').first().waitFor();
	await landed(page);
}

async function at(page: Page, path: string): Promise<void> {
	await page.waitForFunction((to) => location.pathname === to, path);
}

/** Presses `press` and holds the journey to the rule: `name`, in front, landed, never the blend. */
async function goes(page: Page, press: Locator, path: string, name = 'unfold'): Promise<void> {
	await watch(page);
	await press.click();
	await at(page, path);
	await expectInFront(page);
	await landed(page);
	const { ghostPages, airPages, pictures, journeys } = await seen(page);
	expect(ghostPages, `into ${path}`).toBeGreaterThan(0);
	expect(journeys, `into ${path}`).toEqual(journeys.map(() => name));
	expect(airPages).toBe(0);
	expect(pictures, 'never the blend').toBe(0);
}

/** The strip of the tools' tabs on the page on screen. */
const strip = (page: Page): Locator => main(page).locator('nav[aria-label="Library tools"]');
const tab = (page: Page, name: string): Locator => strip(page).getByRole('link', { name });

test.describe('the Server family unfolds from the press', () => {
	test('Server ⇄ Trash and Organize, by their rows and crumbs', async ({ page }) => {
		await open(page, '/server');
		await expectBorn(page, link(page, '/server/trash'), '/server/trash');
		await goes(page, main(page).getByRole('link', { name: 'SERVER › TRASH' }), '/server');
		await expectBorn(page, link(page, '/server/organize'), '/server/organize');
		await goes(page, main(page).getByRole('link', { name: 'YOU › SERVER' }), '/server');
		await goes(page, link(page, '/server/organize'), '/server/organize');
		await expectBorn(page, main(page).getByRole('link', { name: 'YOU › SERVER' }), '/server');
	});

	test('You ⇄ Server, Server → Health and back, and the nav from every one of them', async ({
		page
	}) => {
		await open(page, '/you');
		await expectBorn(page, main(page).getByRole('link', { name: /Server settings/u }), '/server');
		await expectBorn(page, main(page).getByRole('link', { name: 'YOU › SERVER' }), '/you');
		await goes(page, main(page).getByRole('link', { name: /Server settings/u }), '/server');
		await goes(page, link(page, '/health'), '/health');
		for (const path of ['/server/trash', '/server/organize', '/server/duplicates']) {
			await open(page, path);
			await goes(page, page.locator('nav[aria-label="Main"] [data-room="home"]'), '/');
			await goes(page, page.locator('nav[aria-label="Main"] [data-room="you"]'), '/you');
		}
	});

	test('Health → Server by ReplayGain’s link, when tag writes are off', async ({ page }) => {
		await open(page, '/health?tagWrites=off');
		const library = main(page).getByRole('link', { name: /Server › Library/u });
		await expectBorn(page, library, '/server');
	});

	test('after a merge the list is born at ‹ All duplicates, and its toast’s link unfolds Trash', async ({
		page
	}) => {
		await open(page, '/server/duplicates');
		await main(page).getByRole('link', { name: 'Compare Creed' }).click();
		await at(page, '/server/duplicates/dup-creed');
		await landed(page);
		await main(page).getByRole('button', { name: 'Keep A Liturgy, trash the other' }).click();
		const merge = main(page)
			.getByTestId('confirm')
			.getByRole('button', { name: 'Merge and trash' });
		await merge.scrollIntoViewIfNeeded();
		await watch(page);
		// The group is gone, so no row holds its title: Duplicates is born where ‹ All duplicates was.
		await expectBorn(
			page,
			merge,
			'/server/duplicates',
			main(page).getByRole('link', { name: '‹ ALL DUPLICATES' })
		);
		expect((await seen(page)).journeys).toEqual(['unfold']);
		await goes(page, main(page).getByRole('link', { name: 'See the Trash' }), '/server/trash');
	});
});

test.describe('Organize and Duplicates share one strip of tabs', () => {
	test('the strip never leaves: it is on screen from the first frame, and only its fill flows', async ({
		page
	}) => {
		await open(page, '/server/organize');
		const before = await strip(page).boundingBox();
		await slowMotion(page, 20);
		await watch(page);
		await tab(page, 'Duplicates').click();
		await at(page, '/server/duplicates');
		// Under way: the page left is a picture giving way, without a strip of its own in it.
		await page.waitForFunction(
			() => (document.querySelector('.shell > .ghost-layer')?.children.length ?? 0) > 0
		);
		await expect(strip(page)).toHaveAttribute('data-strip-flowing');
		// On its way: the fill has left Organize's tab and not yet reached Duplicates'.
		await page.waitForFunction(() => {
			const shown = document.querySelector('.shell > main nav[aria-label="Library tools"]');
			const fill = shown?.querySelector('[data-strip-fill]')?.getBoundingClientRect();
			const tabs = [...(shown?.querySelectorAll('a') ?? [])].map((a) => a.getBoundingClientRect());
			return (fill?.left ?? 0) > (tabs[0]?.left ?? 0) + 2;
		});
		const mid = await page.evaluate(() => {
			const shown = document.querySelector('.shell > main nav[aria-label="Library tools"]');
			const fill = shown?.querySelector('[data-strip-fill]')?.getBoundingClientRect();
			const tabs = [...(shown?.querySelectorAll('a') ?? [])].map((a) => a.getBoundingClientRect());
			const ghost = document.querySelector('.shell > .ghost-layer nav[aria-label="Library tools"]');
			return {
				opacity: shown === null || shown === undefined ? 0 : getComputedStyle(shown).opacity,
				ghostShown: ghost === null ? 0 : Number(getComputedStyle(ghost).opacity),
				fillLeft: fill?.left ?? 0,
				organizeLeft: tabs[0]?.left ?? 0,
				duplicatesLeft: tabs[1]?.left ?? 0
			};
		});
		expect(Number(mid.opacity), 'the strip is the page’s own, in full').toBe(1);
		expect(mid.ghostShown, 'nothing of the strip gives way').toBe(0);
		// The fill is on its way from Organize's tab to Duplicates', not at either end.
		expect(mid.fillLeft).toBeGreaterThan(mid.organizeLeft);
		expect(mid.fillLeft).toBeLessThan(mid.duplicatesLeft);
		await slowMotion(page, 1);
		await landed(page);
		await expect(strip(page)).not.toHaveAttribute('data-strip-flowing');
		const after = await strip(page).boundingBox();
		expect(Math.abs((after?.y ?? 0) - (before?.y ?? 0)), 'one place on both pages').toBeLessThan(1);
		expect((await seen(page)).journeys).toEqual(['unfold']);
		await expectBorn(page, tab(page, 'Organize'), '/server/organize');
	});

	test('pressed three times quickly, the fill turns where it is and settles on the last', async ({
		page
	}) => {
		await open(page, '/server/organize');
		await watch(page);
		await slowMotion(page, 10);
		await tab(page, 'Duplicates').click();
		await at(page, '/server/duplicates');
		await tab(page, 'Organize').click();
		await at(page, '/server/organize');
		await tab(page, 'Duplicates').click();
		await at(page, '/server/duplicates');
		await slowMotion(page, 1);
		await landed(page);
		expect((await seen(page)).journeys).toEqual(['unfold', 'unfold', 'unfold']);
		await expect(tab(page, 'Duplicates')).toHaveAttribute('aria-current', 'page');
		await expect(page.locator('.shell [data-strip-flowing]')).toHaveCount(0);
	});
});

test.describe('a group, and what both pages say', () => {
	test('a group’s title leaves its row for the headline, and goes back into the row', async ({
		page
	}) => {
		await open(page, '/server/duplicates');
		const compare = main(page).getByRole('link', { name: 'Compare Creed' });
		await watch(page);
		await slowMotion(page, 20);
		await compare.click();
		await at(page, '/server/duplicates/dup-creed');
		// The title is in the air, and its headline under it until it lands; nothing is born.
		await page.waitForFunction(
			() => (document.querySelector('.shell > .morph-layer')?.children.length ?? 0) > 0
		);
		await expect(main(page).locator('h1 [data-morph-hidden]')).toBeAttached();
		await slowMotion(page, 1);
		await landed(page);
		expect((await seen(page)).journeys).toEqual(['unfold']);
		await expect(main(page).locator('h1')).toContainText('Creed');

		await watch(page);
		await slowMotion(page, 20);
		await main(page).getByRole('link', { name: '‹ ALL DUPLICATES' }).click();
		await at(page, '/server/duplicates');
		await page.waitForFunction(
			() => (document.querySelector('.shell > .morph-layer')?.children.length ?? 0) > 0
		);
		await expect(main(page).locator('.groups [data-morph-hidden]')).toHaveCount(1);
		await slowMotion(page, 1);
		await landed(page);
		expect((await seen(page)).journeys).toEqual(['unfold']);
	});

	test('a group to its album pair is re-pressed in place', async ({ page }) => {
		await open(page, '/server/duplicates/dup-blackstar-1');
		await goes(
			page,
			main(page).getByRole('link', { name: 'COMPARE THE ALBUMS ›' }),
			'/server/duplicates/dup-blackstar-pair',
			'duplicate-to-duplicate'
		);
	});

	test('Organize’s Compare opens a group born at the button', async ({ page }) => {
		await open(page, '/server/organize');
		await main(page).getByRole('button', { name: 'Preview', exact: true }).click();
		await main(page)
			.getByRole('button', { name: /^Conflicts/u })
			.click();
		const deluxe = main(page).getByRole('article', { name: 'Wish Upon a Blackstar (Deluxe)' });
		await deluxe.getByRole('button', { name: 'WHY ›' }).last().click();
		await landed(page);
		// A button whose code navigates is the press: the group's headline is born there.
		await expectBorn(
			page,
			deluxe.getByRole('button', { name: 'Compare in Duplicates ›' }),
			'/server/duplicates/dup-best-its-gonna-get'
		);
	});

	test('Health ⇄ Matching: the number leaves the card for the headline, and goes back', async ({
		page
	}) => {
		await open(page, '/health?matching=board');
		const card = main(page).locator('[aria-labelledby="matching-card"]');
		await expect(card).toBeVisible();
		await watch(page);
		await slowMotion(page, 20);
		await card.getByRole('link', { name: 'Work through them' }).click();
		await at(page, '/health/matching');
		await page.waitForFunction(
			() => (document.querySelector('.shell > .morph-layer')?.children.length ?? 0) > 0
		);
		await expect(main(page).locator('h1 [data-morph-hidden]')).toBeAttached();
		await slowMotion(page, 1);
		await landed(page);
		expect((await seen(page)).journeys).toEqual(['unfold']);

		await watch(page);
		await slowMotion(page, 20);
		await main(page)
			.getByRole('link', { name: /HEALTH ›/u })
			.click();
		await at(page, '/health');
		await page.waitForFunction(
			() => (document.querySelector('.shell > .morph-layer')?.children.length ?? 0) > 0
		);
		await expect(card.locator('[data-morph-hidden]')).toHaveCount(1);
		await slowMotion(page, 1);
		await landed(page);
		expect((await seen(page)).journeys).toEqual(['unfold']);
	});
});

/** Where the fox is drawn, its middle, in the viewport. */
async function foxAt(
	page: Page,
	selector: string
): Promise<{ x: number; y: number; size: number }> {
	return page.evaluate((query) => {
		const box = document.querySelector(query)?.getBoundingClientRect();
		return {
			x: (box?.left ?? 0) + (box?.width ?? 0) / 2,
			y: (box?.top ?? 0) + (box?.height ?? 0) / 2,
			size: box?.width ?? 0
		};
	}, selector);
}

test.describe('the door closes', () => {
	test('Sign out: the button says so, then the fox leaves the nav for the door as the frame dissolves', async ({
		page
	}) => {
		// A server that takes a moment to answer, so the button is seen saying so.
		await page.goto(fixtures('/you?playback=phone&latency=300'));
		await page.locator('.shell [data-island-content]').waitFor();
		await landed(page);
		await watch(page);
		await slowMotion(page, 20);
		const seat = await foxAt(page, 'nav[aria-label="Main"] .fox');
		await main(page).getByRole('button', { name: 'Sign out' }).click();
		await expect(main(page).getByRole('button', { name: 'Signing out…' })).toBeVisible();
		await at(page, '/login');
		// In the air: the fox, at first where its seat was; under the door, the frame's picture.
		await page.waitForFunction(
			() => (document.querySelector('.shell > .morph-layer')?.children.length ?? 0) > 0
		);
		const flying = await foxAt(page, '.shell > .morph-layer > *');
		expect(flying.size).toBeLessThan(88);
		expect(Math.hypot(flying.x - seat.x, flying.y - seat.y)).toBeLessThan(200);
		await expect(page.locator('.shell > .ghost-layer > [data-morph-ghost="frame"]')).toHaveCount(2);
		// The real nav is gone at once, not back and up: its picture dissolves where it stood.
		await expect(page.locator('.shell > header.bar')).toHaveCSS('opacity', '0');
		await expect(main(page).locator('.fox[data-morph-hidden]')).toBeAttached();
		await expectInFront(page);
		await slowMotion(page, 1);
		await landed(page);
		const { journeys, pictures, airPages } = await seen(page);
		expect(journeys).toEqual(['door-closes']);
		expect(pictures + airPages).toBe(0);
		await expect(page.locator('.shell > header.bar')).not.toHaveAttribute('data-morph-held');
		await expect(page.getByLabel('Name')).toBeVisible();
	});

	test('a session ending on Now playing: the Play button becomes the fox', async ({ page }) => {
		await page.goto(fixtures('/now-playing?playback=phone'));
		await page.locator('.shell [data-island-content]').waitFor();
		await landed(page);
		const play = await foxAt(
			page,
			'.shell aside [data-morph-key], .shell aside .play, .shell aside button[aria-label^="P"]'
		);
		await watch(page);
		await slowMotion(page, 20);
		await page.evaluate(() => dispatchEvent(new Event('kitsune:session-ends')));
		await at(page, '/login');
		await page.waitForFunction(
			() => (document.querySelector('.shell > .morph-layer')?.children.length ?? 0) > 0
		);
		const flying = await foxAt(page, '.shell > .morph-layer > *');
		expect(flying.size).toBeGreaterThan(0);
		expect(play.size).toBeGreaterThan(0);
		await slowMotion(page, 1);
		await landed(page);
		expect((await seen(page)).journeys).toEqual(['door-closes-on-the-moment']);
	});

	test('a session ending anywhere closes the door at once, from wherever it is', async ({
		page
	}) => {
		await open(page, '/server/trash');
		await watch(page);
		await page.evaluate(() => dispatchEvent(new Event('kitsune:session-ends')));
		await at(page, '/login');
		await landed(page);
		expect((await seen(page)).journeys).toEqual(['door-closes']);
	});

	test('through the door again, to the Server page the session ended on', async ({ page }) => {
		await page.goto(fixtures('/login?setup=done&next=%2Fserver%2Ftrash'));
		await page.getByLabel('Name').fill('simon');
		await page.getByLabel('Password').fill('a password');
		await watch(page);
		await page.getByRole('button', { name: 'Sign in' }).click();
		await at(page, '/server/trash');
		await expectInFront(page);
		await landed(page);
		expect((await seen(page)).journeys).toEqual(['login-to-page']);
	});
});

test.describe('presses that come quickly', () => {
	test('Server → Trash, then its crumb mid-flight: Server takes over from where it had got to', async ({
		page
	}) => {
		await open(page, '/server');
		await slowMotion(page, 20);
		await link(page, '/server/trash').click();
		await at(page, '/server/trash');
		await page.waitForFunction(
			() => (document.querySelector('.shell > .ghost-layer')?.children.length ?? 0) > 0
		);
		await main(page).getByRole('link', { name: 'SERVER › TRASH' }).click();
		await at(page, '/server');
		const headline = await page.evaluate(() => {
			const unit = document.querySelector<HTMLElement>('.shell > main [data-morph-headline]');
			return unit === null ? null : /scale\(([\d.]+)\)/u.exec(unit.style.transform)?.[1];
		});
		if (headline !== null && headline !== undefined) {
			expect(Number(headline), 'not born again').toBeGreaterThan(0.95);
		}
		await slowMotion(page, 1);
		await landed(page);
		await expect(page.locator('.shell > .ghost-layer > *')).toHaveCount(0);
	});

	test('You → Server, then Trash while Server is still gathering', async ({ page }) => {
		await open(page, '/you');
		await watch(page);
		await slowMotion(page, 10);
		await main(page)
			.getByRole('link', { name: /Server settings/u })
			.click();
		await at(page, '/server');
		await link(page, '/server/trash').click();
		await at(page, '/server/trash');
		await slowMotion(page, 1);
		await landed(page);
		expect((await seen(page)).journeys).toEqual(['unfold', 'unfold']);
	});
});

test.describe('wherever it is seen', () => {
	test('at 390 Trash is born at Server’s compact link, and the tabs keep their flow', async ({
		page
	}) => {
		await page.setViewportSize({ width: 390, height: 844 });
		await open(page, '/server');
		await expectBorn(page, link(page, '/server/trash'), '/server/trash');
		await goes(page, main(page).getByRole('link', { name: 'SERVER › TRASH' }), '/server');
		await open(page, '/server/organize');
		await goes(page, tab(page, 'Duplicates'), '/server/duplicates');
		await open(page, '/you');
		await goes(page, main(page).getByRole('button', { name: 'Sign out' }), '/login', 'door-closes');
	});

	test('at 3840 the same, on the step’s sizes', async ({ page }) => {
		await page.setViewportSize({ width: 3840, height: 2160 });
		await open(page, '/server');
		await expectBorn(page, link(page, '/server/trash'), '/server/trash');
		await open(page, '/server/organize');
		await goes(page, tab(page, 'Duplicates'), '/server/duplicates');
		await open(page, '/you');
		await goes(page, main(page).getByRole('button', { name: 'Sign out' }), '/login', 'door-closes');
	});

	test('with the server slow, nothing waits for it', async ({ page }) => {
		await open(page, '/server?latency=300');
		await goes(page, link(page, '/server/trash'), '/server/trash');
		await goes(page, main(page).getByRole('link', { name: 'SERVER › TRASH' }), '/server');
		await goes(page, link(page, '/server/organize'), '/server/organize');
		await goes(page, tab(page, 'Duplicates'), '/server/duplicates');
	});

	test('with reduced motion every pair is the blend, the door too', async ({ page }) => {
		await page.emulateMedia({ reducedMotion: 'reduce' });
		await open(page, '/server');
		for (const [press, path] of [
			[link(page, '/server/trash'), '/server/trash'],
			[main(page).getByRole('link', { name: 'SERVER › TRASH' }), '/server'],
			[link(page, '/server/organize'), '/server/organize'],
			[tab(page, 'Duplicates'), '/server/duplicates']
		] as const) {
			await watch(page);
			await press.click();
			await at(page, path);
			await landed(page);
			const { pictures, airPages, ghostPages } = await seen(page);
			expect(pictures, `into ${path}`).toBe(1);
			expect(airPages + ghostPages).toBe(0);
		}
		await open(page, '/you');
		await watch(page);
		await main(page).getByRole('button', { name: 'Sign out' }).click();
		await at(page, '/login');
		await landed(page);
		expect((await seen(page)).pictures).toBe(1);
	});
});
