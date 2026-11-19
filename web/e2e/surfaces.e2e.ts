import type { Locator, Page } from '@playwright/test';
import { expect, fixtures, landed, test } from './fixture';
import { slowClock, slowMotion } from './motion';

/**
 * Surfaces in the page (plan 071's fourth milestone, A: Bud), on the fixtures. A surface is one box at
 * its true size and radius, born at the control that opened it (or at the floor when none is left on
 * screen), and its words never move: the box clips them. It becomes its next face by reshaping the
 * same box in place, and leaves by giving way where it stands, under whatever comes next. Each test
 * holds the structure on a clock slowed almost to a stop, never a lucky frame or a timing.
 */

test.beforeEach(async ({ page }) => {
	await slowClock(page);
});

interface Box {
	x: number;
	y: number;
	width: number;
	height: number;
}

/** The box a surface in flight is drawn with: the newest, when several are in the air. */
function flightBox(page: Page): Promise<Box | null> {
	return page.evaluate(() => {
		const all = [...document.querySelectorAll('[data-surface-box]')].filter(
			(box) => box.closest('[data-surface-leaving]') === null
		);
		const box = all.at(-1)?.getBoundingClientRect();
		return box === undefined ? null : { x: box.x, y: box.y, width: box.width, height: box.height };
	});
}

/** The box of a surface giving way, by its layer. */
function leavingBox(page: Page): Promise<Box | null> {
	return page.evaluate(() => {
		const box = document
			.querySelector('[data-surface-leaving] [data-surface-box]')
			?.getBoundingClientRect();
		return box === undefined ? null : { x: box.x, y: box.y, width: box.width, height: box.height };
	});
}

async function rectOf(locator: Locator): Promise<Box> {
	const box = await locator.boundingBox();
	if (box === null) {
		throw new Error('not on screen');
	}
	return box;
}

/** Nothing in flight: no box of the engine's own, nothing giving way. */
async function settled(page: Page): Promise<void> {
	await page.waitForFunction(
		() =>
			document.querySelectorAll('[data-surface-box], [data-surface-leaving]').length === 0 &&
			[...document.querySelectorAll<HTMLElement>('[data-surface]')].every(
				(surface) => surface.style.clipPath === ''
			),
		null,
		{ timeout: 10_000 }
	);
}

const near = (a: number, b: number, within = 2): boolean => Math.abs(a - b) <= within;

const albumMore = (page: Page): Locator =>
	page.getByRole('button', { name: 'More for Tin Lantern', exact: true });

async function openAlbum(page: Page, query = 'gainAlbum=200'): Promise<void> {
	await page.goto(fixtures(`/album/tin-lantern-78?${query}`));
	await expect(albumMore(page)).toBeVisible();
	await landed(page);
	// The hero's words are set in Geist once it has loaded, which moves the ⋯ beside them.
	await page.evaluate(() => document.fonts.ready.then(() => undefined));
}

test('a menu is born at its ⋯, at the ⋯’s size, and grows into itself', async ({ page }) => {
	await openAlbum(page);
	await slowMotion(page, 2000);
	const dots = await rectOf(albumMore(page));
	await albumMore(page).click();
	const born = await flightBox(page);
	expect(born).not.toBeNull();
	if (born !== null) {
		// The ⋯ itself: its place and its size, nowhere near the menu's.
		expect(near(born.x, dots.x, 4) && near(born.y, dots.y, 4)).toBe(true);
		expect(near(born.width, dots.width, 4) && near(born.height, dots.height, 4)).toBe(true);
	}
	await slowMotion(page, 1);
	await settled(page);
	const menu = await rectOf(page.getByRole('menu', { name: 'Tin Lantern' }));
	expect(menu.width).toBeGreaterThan(born?.width ?? 0);
	expect(menu.height).toBeGreaterThan(born?.height ?? 0);
});

test('Write ReplayGain… is the menu’s own box reshaped, from where the menu was drawn', async ({
	page
}) => {
	await openAlbum(page);
	await albumMore(page).click();
	const menu = page.getByRole('menu', { name: 'Tin Lantern' });
	await settled(page);
	const drawn = await rectOf(menu);
	const surface = await menu.elementHandle();
	await slowMotion(page, 2000);
	await page.getByRole('menuitem', { name: /Write ReplayGain/ }).click();
	// It starts where the menu was actually drawn, never where it was asked for.
	const start = await flightBox(page);
	expect(start).not.toBeNull();
	if (start !== null) {
		expect(near(start.x, drawn.x) && near(start.y, drawn.y)).toBe(true);
		expect(near(start.width, drawn.width) && near(start.height, drawn.height)).toBe(true);
	}
	await slowMotion(page, 1);
	await settled(page);
	// One surface: the same element now holds the confirm.
	const confirm = page.getByRole('dialog', { name: 'Write ReplayGain' });
	expect(await confirm.evaluate((element, same) => element === same, surface)).toBe(true);
	await expect(confirm.getByTestId('gain-confirm')).toBeVisible();
});

test('a menu gives way where it stands, never back into its ⋯', async ({ page }) => {
	await openAlbum(page);
	await albumMore(page).click();
	const menu = page.getByRole('menu', { name: 'Tin Lantern' });
	await settled(page);
	const drawn = await rectOf(menu);
	await slowMotion(page, 2000);
	await page.keyboard.press('Escape');
	await expect(menu).toHaveCount(0);
	const leaving = await leavingBox(page);
	expect(leaving).not.toBeNull();
	if (leaving !== null) {
		expect(near(leaving.x, drawn.x) && near(leaving.y, drawn.y)).toBe(true);
		expect(near(leaving.width, drawn.width) && near(leaving.height, drawn.height)).toBe(true);
	}
	await slowMotion(page, 1);
	await settled(page);
});

test('a menu opened again while it gives way takes over from where it had got to', async ({
	page
}) => {
	await openAlbum(page);
	await albumMore(page).click();
	await settled(page);
	await slowMotion(page, 2000);
	await page.keyboard.press('Escape');
	await expect(page.locator('[data-surface-leaving]')).toHaveCount(1);
	await albumMore(page).click();
	// Not born twice: what was giving way is gone, handed over to the menu.
	await expect(page.locator('[data-surface-leaving]')).toHaveCount(0);
	// Grown on from the box it had: never the ⋯'s size again (and maybe at rest already).
	const box = await flightBox(page);
	const dots = await rectOf(albumMore(page));
	expect(box === null || box.width > dots.width * 3).toBe(true);
	await slowMotion(page, 1);
	await settled(page);
	await expect(page.getByRole('menu', { name: 'Tin Lantern' })).toBeVisible();
});

test('a submenu is born at its item’s chevron, beside its menu, never over it', async ({
	page
}) => {
	await page.goto(fixtures('/album/zero-96'));
	await page.getByRole('button', { name: 'More for Signal One' }).click();
	const item = page.getByRole('menuitem', { name: 'Add to playlist' });
	await settled(page);
	const row = await rectOf(item);
	await slowMotion(page, 2000);
	await item.click();
	const born = await flightBox(page);
	expect(born).not.toBeNull();
	if (born !== null) {
		// At the chevron's end of the row, on whichever side the submenu opens.
		const right = near(born.x + born.width, row.x + row.width, 3);
		expect(right || near(born.x, row.x, 3)).toBe(true);
		expect(born.width).toBeLessThan(row.width / 4);
		expect(near(born.y, row.y, 3)).toBe(true);
	}
	await slowMotion(page, 1);
	await settled(page);
	const picker = await rectOf(page.getByRole('menu', { name: 'Add to playlist' }));
	expect(picker.x >= row.x + row.width || picker.x + picker.width <= row.x).toBe(true);
});

test('a toast is born at the floor, and Undo makes it what it did, in place', async ({ page }) => {
	await page.goto(fixtures('/album/glasshouse-66'));
	await page.getByRole('button', { name: 'More for Low Tide' }).click();
	const trash = page.getByRole('menuitem', { name: /Move to Trash/ });
	await settled(page);
	await slowMotion(page, 2000);
	await trash.click();
	const toast = page.getByRole('status').filter({ hasText: 'moved to Trash' });
	await expect(toast).toHaveCount(1);
	const born = await flightBox(page);
	const rest = await rectOf(toast);
	expect(born).not.toBeNull();
	if (born !== null) {
		// Its own width, no height yet, standing on its own foot.
		expect(near(born.width, rest.width, 3)).toBe(true);
		// No height but its own edges.
		expect(born.height).toBeLessThanOrEqual(3);
		expect(near(born.y, rest.y + rest.height, 3)).toBe(true);
	}
	await slowMotion(page, 1);
	await settled(page);
	const before = await toast.elementHandle();
	await toast.getByRole('button', { name: 'Undo' }).click();
	const after = page.getByRole('status').filter({ hasText: 'Brought back' });
	await expect(after).toContainText('Low Tide');
	expect(await after.evaluate((element, same) => element === same, before)).toBe(true);
	await settled(page);
});

test('a second toast is born in front of the first, which gives way under it', async ({ page }) => {
	await page.goto(fixtures('/album/glasshouse-66'));
	await page.getByRole('button', { name: 'More for Low Tide' }).click();
	await page.getByRole('menuitem', { name: /Move to Trash/ }).click();
	await expect(page.getByRole('status').filter({ hasText: 'moved to Trash' })).toHaveCount(1);
	await settled(page);
	await page.getByRole('button', { name: 'More for Panes' }).click();
	await settled(page);
	await slowMotion(page, 2000);
	await page.getByRole('menuitem', { name: /Move to Trash/ }).click();
	await expect(page.getByRole('status').filter({ hasText: 'Panes' })).toHaveCount(1);
	// The first gives way first in its layer, so the new one, later, is drawn in front of it.
	const order = await page.evaluate(() => {
		const toast = [...document.querySelectorAll('.toast')].find((each) =>
			each.textContent?.includes('Panes')
		);
		const leaving = [...document.querySelectorAll('[data-surface-leaving]')].find((each) =>
			each.textContent?.includes('Low Tide')
		);
		if (toast === undefined || leaving === undefined) {
			return 'missing';
		}
		return leaving.compareDocumentPosition(toast) & Node.DOCUMENT_POSITION_FOLLOWING
			? 'behind'
			: 'in front';
	});
	expect(order).toBe('behind');
	await slowMotion(page, 1);
	await settled(page);
});

test('the selection bar is born at the floor, and its count is re-pressed in place', async ({
	page
}) => {
	await page.goto(fixtures('/album/orbitals-14'));
	const row = (id: string) => page.locator(`[data-track="${id}"]`).first();
	await expect(row('orbitals-14-4')).toBeVisible();
	await slowMotion(page, 2000);
	await row('orbitals-14-4').click({ modifiers: ['ControlOrMeta'] });
	const bar = page.getByRole('toolbar');
	await expect(bar).toContainText('1 selected');
	const born = await flightBox(page);
	const rest = await rectOf(bar);
	expect(born).not.toBeNull();
	expect(born?.height ?? 99).toBeLessThanOrEqual(3);
	expect(near(born?.y ?? 0, rest.y + rest.height, 3)).toBe(true);
	await slowMotion(page, 1);
	await settled(page);
	await slowMotion(page, 2000);
	await row('orbitals-14-5').click({ modifiers: ['ControlOrMeta'] });
	await expect(bar).toContainText('2 selected');
	// The old count gives way as a picture while the new rises into its place.
	await expect(bar.locator('.count[aria-hidden="true"]')).toHaveCount(1);
	await slowMotion(page, 1);
	await expect(bar.locator('.count[aria-hidden="true"]')).toHaveCount(0);
});

test('closing the tag editor lays the page again in front of it', async ({ page }) => {
	await page.goto(fixtures('/album/glasshouse-66'));
	await landed(page);
	const edit = page.getByRole('button', { name: 'Edit tags', exact: true });
	const button = await rectOf(edit);
	await slowMotion(page, 2000);
	await edit.click();
	const editor = page.getByRole('region', { name: 'Editing songs' });
	await expect(editor).toHaveCount(1);
	const born = await flightBox(page);
	expect(near(born?.x ?? 0, button.x, 4) && near(born?.width ?? 0, button.width, 4)).toBe(true);
	await slowMotion(page, 1);
	await settled(page);
	await slowMotion(page, 2000);
	await page.getByRole('button', { name: 'Leave it', exact: true }).click();
	await expect(editor).toHaveCount(0);
	// Under the page, which comes up in front of it: the In-Front Rule.
	const stack = await page.evaluate(() => {
		const main = document.querySelector('.shell > main');
		const leaving = document.querySelector('.shell > [data-surface-leaving]');
		const arriving = [...document.querySelectorAll<HTMLElement>('main [data-morph-part]')].some(
			(unit) => unit.style.opacity !== ''
		);
		return {
			under: leaving !== null && leaving.nextElementSibling === main,
			arriving
		};
	});
	expect(stack).toEqual({ under: true, arriving: true });
	await slowMotion(page, 1);
	await settled(page);
	await landed(page);
});

test('on a phone nothing is a sheet: the menu becomes its confirm in place, clear of the floor', async ({
	page
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	await openAlbum(page, 'gainAlbum=100');
	await albumMore(page).click();
	const menu = page.getByRole('menu', { name: 'Tin Lantern' });
	await settled(page);
	const surface = await menu.elementHandle();
	await page.getByRole('menuitem', { name: /Write ReplayGain/ }).click();
	await settled(page);
	const confirm = page.getByRole('dialog', { name: 'Write ReplayGain' });
	expect(await confirm.evaluate((element, same) => element === same, surface)).toBe(true);
	const box = await rectOf(confirm);
	// The phone's whole width less its gutters, and never down at the foot of the screen.
	expect(near(box.x, 16) && near(box.width, 358)).toBe(true);
	expect(box.y + box.height).toBeLessThan(844 - 8);
	await expect(page.locator('.scrim')).toHaveCount(0);
});

test('with reduced motion a surface blends in where it rests, its shape set at once', async ({
	page
}) => {
	await page.emulateMedia({ reducedMotion: 'reduce' });
	await openAlbum(page);
	await slowMotion(page, 2000);
	await albumMore(page).click();
	const menu = page.getByRole('menu', { name: 'Tin Lantern' });
	await expect(menu).toHaveCount(1);
	const box = await flightBox(page);
	const rest = await rectOf(menu);
	expect(near(box?.x ?? 0, rest.x) && near(box?.width ?? 0, rest.width)).toBe(true);
	expect(near(box?.height ?? 0, rest.height)).toBe(true);
	await slowMotion(page, 1);
	await settled(page);
});

test('at 4K a menu takes the page’s size step', async ({ page }) => {
	await page.setViewportSize({ width: 3840, height: 2160 });
	await openAlbum(page);
	await albumMore(page).click();
	await settled(page);
	const menu = await rectOf(page.getByRole('menu', { name: 'Tin Lantern' }));
	expect(near(menu.width, 375)).toBe(true);
});
