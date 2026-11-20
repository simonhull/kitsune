import type { Locator, Page } from '@playwright/test';
import { expect, fixtures, landed, test } from './fixture';
import { expectBorn as bornAt, expectInFront, seen, slowClock, slowMotion, watch } from './motion';

/**
 * The rooms, from the nav (plan 071's second milestone): every pair of the nav's eight rooms unfolds
 * from what was pressed. Each journey is held to its structure, never to pixels or a lucky frame:
 * the room arriving is drawn in front of the room left, its headline is born at the press and grows
 * along one line into place, it lands with nothing left over, and a press in mid-flight takes over
 * from wherever things are, never snapping and never queueing.
 */

test.beforeEach(async ({ page }) => {
	await slowClock(page);
});

const rooms = {
	home: '/',
	library: '/library',
	albums: '/albums',
	artists: '/artists',
	playlists: '/playlists',
	search: '/search',
	you: '/you',
	health: '/health'
} as const;

type Room = keyof typeof rooms;

/** Presses `press` and holds the room's headline to its birth there: see motion.ts. */
const expectBorn = (page: Page, press: Locator, to: Room): Promise<void> =>
	bornAt(page, press, rooms[to]);

/** A room's place in the nav: its item, the fox for You, the field for Search. */
const place = (page: Page, room: Room): Locator =>
	page.locator(`nav[aria-label="Main"] [data-room="${room}"]`);

/** Waits for the room on screen to be the one asked for, by the page's own location. */
async function at(page: Page, room: Room): Promise<void> {
	await page.waitForFunction((path) => location.pathname === path, rooms[room]);
}

/** Opens a room the plain way and waits for it to be still. */
async function open(page: Page, room: Room, query = ''): Promise<void> {
	await page.goto(fixtures(`${rooms[room]}${query}`));
	await page.locator('nav[aria-label="Main"] [data-room="home"]').waitFor();
	await landed(page);
}

/**
 * Presses a room's place in the nav and holds the journey to the rule: an unfold, drawn under the
 * room arriving, nothing of a page put in the air, landed, and the pill where the room says.
 */
async function unfolds(page: Page, to: Room): Promise<void> {
	await watch(page);
	await place(page, to).click();
	await at(page, to);
	await expectInFront(page);
	await landed(page);
	const { ghostPages, airPages, pictures, journeys } = await seen(page);
	expect(ghostPages, `into ${to}`).toBeGreaterThan(0);
	expect(journeys, `into ${to}`).toEqual(journeys.map(() => 'unfold'));
	expect(airPages).toBe(0);
	expect(pictures, 'never the blend').toBe(0);
	await expectPill(page, to);
}

/** The pill sits under the room's item, or has given way where the room has none. */
async function expectPill(page: Page, room: Room): Promise<void> {
	const pill = await page.evaluate((name) => {
		const shown = document.querySelector<HTMLElement>('nav[aria-label="Main"] .pill');
		const item = document.querySelector(`nav[aria-label="Main"] [data-room="${name}"]`);
		const box = shown?.getBoundingClientRect();
		const under = item?.getBoundingClientRect();
		return {
			opacity: shown === null || shown === undefined ? '' : getComputedStyle(shown).opacity,
			left: box?.left ?? 0,
			right: box?.right ?? 0,
			itemLeft: under?.left ?? 0,
			itemRight: under?.right ?? 0
		};
	}, room);
	if (['search', 'you', 'health'].includes(room)) {
		// No item holds it: it has given way, or is giving way where it stood.
		await expect
			.poll(() =>
				page.evaluate(() =>
					Number(getComputedStyle(document.querySelector('nav .pill') ?? document.body).opacity)
				)
			)
			.toBe(0);
		return;
	}
	expect(Number(pill.opacity)).toBe(1);
	// The pill's two springs take a moment after the room lands; nothing else is still moving.
	await expect
		.poll(() =>
			page.evaluate((name) => {
				const box = document.querySelector('nav .pill')?.getBoundingClientRect();
				const item = document.querySelector(`nav [data-room="${name}"]`)?.getBoundingClientRect();
				return Math.round(Math.abs((box?.left ?? 0) - (item?.left ?? 0)));
			}, room)
		)
		.toBeLessThanOrEqual(1);
}

/**
 * Goes into `to` slowed down and, once the room is under way, presses `back`: the room returned to
 * takes over from the picture of itself that was giving way, its headline at its own size, never
 * born again; then it lands, with nothing left of the other.
 */
async function turnsBack(page: Page, to: Room, back: Room): Promise<void> {
	await slowMotion(page, 20);
	await place(page, to).click();
	await at(page, to);
	// Under way: the room left is a picture giving way, and the room arriving is coming in.
	await page.waitForFunction(
		() =>
			(document.querySelector('.shell > .ghost-layer')?.children.length ?? 0) > 0 &&
			[...document.querySelectorAll<HTMLElement>('.shell > main [style*="opacity"]')].some(
				(unit) => Number(unit.style.opacity) > 0.05 && Number(unit.style.opacity) < 0.95
			)
	);
	await place(page, back).click();
	await at(page, back);
	// Its own picture is taken over, not left beside it: only the room just left is under it now.
	expect(
		await page.locator('.shell > .ghost-layer > [data-morph-ghost="page"]').count(),
		'the room gone back to goes on from its own picture'
	).toBe(1);
	const headline = await page.evaluate(() => {
		const unit = document.querySelector<HTMLElement>('.shell > main [data-morph-headline]');
		return unit === null ? null : /scale\(([\d.]+)\)/u.exec(unit.style.transform)?.[1];
	});
	if (headline !== null && headline !== undefined) {
		expect(
			Number(headline),
			'at its own size: a room gone back to is not born again'
		).toBeGreaterThan(0.95);
	}
	await slowMotion(page, 1);
	await landed(page);
	await expect(page.locator('.shell > .ghost-layer > *')).toHaveCount(0);
}

test.describe('every pair of the nav’s rooms unfolds', () => {
	const all = Object.keys(rooms) as Room[];
	for (const from of all) {
		test(`from ${from}, into each of the other seven and back`, async ({ page }) => {
			await open(page, from);
			for (const to of all.filter((room) => room !== from)) {
				await unfolds(page, to);
				await unfolds(page, from);
			}
		});
	}
});

test.describe('the leads', () => {
	test('Home → Library: “Library” is born at its item, and the playing album’s cover travels both ways', async ({
		page
	}) => {
		// The Library lands where it was left: leave it at the playing album's artist, under L.
		await open(page, 'library');
		await page.locator('nav[aria-label="Jump to a letter"] a', { hasText: /^L$/u }).click();
		await expect(
			page.locator('.shell > main [data-morph-key="cover:glasshouse-66"]')
		).toBeVisible();
		await unfolds(page, 'home');
		await page
			.getByRole('region', { name: 'Start' })
			.getByRole('button', { name: 'Play Glasshouse' })
			.click();
		await expect(page.locator('.shell [data-island-content]')).toBeVisible();
		await slowMotion(page, 20);
		await place(page, 'library').click();
		// The cover leaves the Continue card for the album's heading in the list, in the air.
		await page.waitForFunction(
			() => (document.querySelector('.shell > .morph-layer')?.children.length ?? 0) > 0
		);
		await expect(
			page.locator('.shell > main [data-morph-key="cover:glasshouse-66"] [data-morph-hidden]')
		).toBeAttached();
		await slowMotion(page, 1);
		await landed(page);
		await place(page, 'home').click();
		await page.waitForFunction(
			() => (document.querySelector('.shell > .morph-layer')?.children.length ?? 0) > 0
		);
		// And back, onto a Glasshouse cover Home shows, under its flier until it lands.
		await expect(page.locator('.shell > main [data-morph-hidden]')).toHaveCount(1);
		await landed(page);
		await expectBorn(page, place(page, 'library'), 'library');
	});

	test('each room’s headline is born at its item and grows along one line into place', async ({
		page
	}) => {
		await open(page, 'home');
		for (const to of ['albums', 'artists', 'playlists', 'health', 'library', 'home'] as const) {
			await expectBorn(page, place(page, to), to);
		}
	});

	test('into Search the nav’s field reshapes into the page’s, under the nav’s glass', async ({
		page
	}) => {
		await open(page, 'albums');
		await watch(page);
		await slowMotion(page, 20);
		await place(page, 'search').click();
		await at(page, 'search');
		// The field and its words come out from under the bar, not in the air over it.
		await page.waitForFunction(
			() => (document.querySelector('.shell > .born-layer')?.children.length ?? 0) >= 2
		);
		await expect(page.locator('.shell > main .box[data-morph-hidden]')).toBeAttached();
		await expectInFront(page);
		await slowMotion(page, 1);
		await landed(page);
		await expect(page.locator('.shell > main .box')).toBeVisible();
		// The nav keeps its own field: it was born from, never taken.
		await expect(place(page, 'search')).toBeVisible();
		expect((await seen(page)).journeys).toEqual(['unfold']);
		// Out of Search, the field goes with its page, never back into the nav.
		await watch(page);
		await place(page, 'artists').click();
		await at(page, 'artists');
		expect(await page.locator('.shell > .born-layer > *, .shell > .morph-layer > *').count()).toBe(
			0
		);
		await landed(page);
	});

	test('into You the name is born at the fox, which takes its ring as the pill goes', async ({
		page
	}) => {
		await open(page, 'library');
		await expectBorn(page, place(page, 'you'), 'you');
		await expect(place(page, 'you')).toHaveAttribute('aria-current', 'page');
		await expectPill(page, 'you');
		// Out of You the pill comes back at its item, without flowing out of nowhere.
		await unfolds(page, 'artists');
		await expect(place(page, 'you')).not.toHaveAttribute('aria-current', 'page');
	});
});

test.describe('presses that come quickly', () => {
	test('turned back mid-flight, the room left takes over from where it had got to', async ({
		page
	}) => {
		await open(page, 'home');
		await turnsBack(page, 'library', 'home');
		await turnsBack(page, 'albums', 'home');
		await open(page, 'you');
		await turnsBack(page, 'search', 'you');
	});

	test('a second and third press retarget, and it settles on the last', async ({ page }) => {
		await open(page, 'home');
		await watch(page);
		await slowMotion(page, 20);
		await place(page, 'library').click();
		await at(page, 'library');
		await place(page, 'albums').click();
		await at(page, 'albums');
		// The half-built Library gives way from the pose it reached: a picture of it, still there.
		await page.waitForFunction(
			() =>
				document.querySelectorAll('.shell > .ghost-layer > [data-morph-ghost="page"]').length >= 2
		);
		await place(page, 'artists').click();
		await at(page, 'artists');
		await slowMotion(page, 1);
		await landed(page);
		const { journeys, pictures } = await seen(page);
		expect(journeys).toEqual(['unfold', 'unfold', 'unfold']);
		expect(pictures).toBe(0);
		await expectPill(page, 'artists');
	});

	test('two presses in one moment: the later one wins, and nothing queues', async ({ page }) => {
		await open(page, 'albums');
		await page.evaluate(() => {
			for (const room of ['library', 'playlists']) {
				document.querySelector<HTMLElement>(`nav [data-room="${room}"]`)?.click();
			}
		});
		await at(page, 'playlists');
		await landed(page);
		await expectPill(page, 'playlists');
		await expect(page.locator('.shell > .ghost-layer > *')).toHaveCount(0);
	});

	test('the same pill twice: at the top only the pill presses; scrolled, the room is laid again', async ({
		page
	}) => {
		await open(page, 'albums');
		await watch(page);
		const pill = page.locator('nav[aria-label="Main"] .pill');
		await place(page, 'albums').click();
		expect(await pill.evaluate((element) => element.getAnimations().length)).toBeGreaterThan(0);
		await at(page, 'albums');
		expect((await seen(page)).ghostPages).toBe(0);
		await landed(page);

		await page.mouse.wheel(0, 1600);
		await page.waitForFunction(() => scrollY > 400);
		await watch(page);
		await place(page, 'albums').click();
		await page.waitForFunction(() => scrollY === 0);
		await landed(page);
		const { ghostPages, pictures } = await seen(page);
		expect(ghostPages, 'laid again over a picture of itself').toBe(1);
		expect(pictures).toBe(0);
		await at(page, 'albums');
		await expectPill(page, 'albums');
	});
});

test.describe('wherever it is seen', () => {
	test('at 390 the rooms unfold from the compact nav, and You → Playlists from its link', async ({
		page
	}) => {
		await page.setViewportSize({ width: 390, height: 844 });
		await open(page, 'home');
		await expectBorn(page, place(page, 'library'), 'library');
		await unfolds(page, 'albums');
		await unfolds(page, 'home');
		await unfolds(page, 'you');
		const link = page.locator('.shell > main a', { hasText: 'Your playlists' });
		await watch(page);
		await expectBorn(page, link, 'playlists');
		expect((await seen(page)).journeys).toEqual(['unfold']);
		await unfolds(page, 'home');
		await turnsBack(page, 'library', 'home');
	});

	test('at 3840 the same choreography, on the step’s sizes', async ({ page }) => {
		await page.setViewportSize({ width: 3840, height: 2160 });
		await open(page, 'home');
		await expectBorn(page, place(page, 'library'), 'library');
		await unfolds(page, 'search');
		await unfolds(page, 'health');
		await turnsBack(page, 'albums', 'health');
	});

	test('with the server slow, nothing waits for it', async ({ page }) => {
		await open(page, 'home', '?latency=300');
		await unfolds(page, 'library');
		await unfolds(page, 'search');
		await unfolds(page, 'you');
		await turnsBack(page, 'artists', 'you');
	});

	test('with reduced motion it is the blend, still drawn in front, the pill straight to its item', async ({
		page
	}) => {
		await page.emulateMedia({ reducedMotion: 'reduce' });
		await open(page, 'home');
		for (const to of ['library', 'search', 'albums'] as const) {
			await watch(page);
			await place(page, to).click();
			await at(page, to);
			await landed(page);
			const { pictures, airPages, ghostPages } = await seen(page);
			expect(pictures, `into ${to}`).toBe(1);
			expect(airPages + ghostPages).toBe(0);
			await expectPill(page, to);
		}
	});

	test('pressed again and again, it leaves nothing behind', async ({ page }) => {
		await open(page, 'home');
		const order = ['library', 'albums', 'search', 'you', 'artists', 'playlists', 'health', 'home'];
		for (const to of [...order, ...order] as Room[]) {
			await place(page, to).click();
			await at(page, to);
		}
		await landed(page);
		await expect(page.locator('.shell > .ghost-layer > *')).toHaveCount(0);
		await expect(page.locator('.shell > .born-layer > *')).toHaveCount(0);
		await expect(page.locator('.shell > main')).not.toHaveAttribute('data-morph-waiting');
	});
});
