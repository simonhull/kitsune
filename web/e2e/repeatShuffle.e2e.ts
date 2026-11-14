import type { Locator, Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/** Opens the newest album and plays it from its first song: an album's own queue, in order. */
async function playAlbum(page: Page): Promise<{ island: Locator; order: string[] }> {
	await page.goto(fixtures('/'));
	const card = page.getByRole('region', { name: 'Just added' }).locator('.cover').first();
	await card.click();
	const rows = page.locator('[data-track]');
	await expect(rows.first()).toBeVisible();
	const order = (
		await rows.evaluateAll((all) => all.map((row) => row.getAttribute('aria-label')))
	).filter((title): title is string => title !== null);
	const first = order[0] ?? '';
	await rows
		.first()
		.getByRole('button', { name: `Play ${first}` })
		.click();
	const island = page.locator('aside[aria-label="Player"]');
	await expect(island.locator('.song')).toHaveText(first);
	// Landed: while it rises, a pointer aimed at a control ends up beside it.
	await expect(island).not.toHaveClass(/arrive/);
	return { island, order };
}

/** Takes focus off whatever has it, so the page's own keys answer. */
async function letGo(page: Page): Promise<void> {
	await page.evaluate(() => {
		if (document.activeElement instanceof HTMLElement) {
			document.activeElement.blur();
		}
	});
}

/** Up next's songs on Now playing, as its pills name them. */
async function upNext(page: Page): Promise<string[]> {
	return page.getByRole('region', { name: 'Up next' }).locator('.pill-name').allTextContents();
}

/** Seeks to a few seconds before the end, on Now playing's scrubber. */
async function nearTheEnd(page: Page): Promise<void> {
	const track = page.getByRole('slider', { name: 'Seek' });
	const box = await track.boundingBox();
	if (box === null) {
		throw new Error('no scrubber');
	}
	await page.mouse.click(box.x + box.width * 0.995, box.y + box.height / 2);
}

test('Shuffle reorders what comes next, and off goes back to the album from the song playing', async ({
	page
}) => {
	const { order } = await playAlbum(page);
	expect(order.length).toBeGreaterThan(4);
	await page.keyboard.press('n');
	await expect(page).toHaveURL(/\/now-playing/);
	await expect.poll(() => upNext(page)).toEqual(order.slice(1, 4));

	const shuffle = page.getByRole('button', { name: 'Shuffle', exact: true });
	await expect(shuffle).toHaveAttribute('aria-pressed', 'false');
	await shuffle.click();
	await expect(shuffle).toHaveAttribute('aria-pressed', 'true');
	const line = page.getByRole('region', { name: 'Up next' }).locator('.next-label');
	await expect(line).toHaveText(/^UP NEXT · SHUFFLED FROM /);
	let shuffled = await upNext(page);
	// Off goes on in album order after the song playing: from the album's last song, nothing
	// does. A fresh seed until the shuffle leads with another, so the test never rests on luck.
	for (let tries = 0; shuffled[0] === order.at(-1) && tries < 20; tries++) {
		await shuffle.click();
		await expect(shuffle).toHaveAttribute('aria-pressed', 'false');
		await shuffle.click();
		await expect(shuffle).toHaveAttribute('aria-pressed', 'true');
		await expect(line).toHaveText(/^UP NEXT · SHUFFLED FROM /);
		shuffled = await upNext(page);
	}
	expect(shuffled[0]).not.toBe(order.at(-1));
	expect(shuffled.length).toBe(3);

	// Next plays from the shuffled order: Shift+→ from the page, off the toolbar's own arrows.
	await letGo(page);
	await page.keyboard.press('Shift+ArrowRight');
	await expect(page.getByRole('heading', { level: 1 })).toHaveText(shuffled[0] ?? '');

	// Off, from S: on through the album in order from the song playing, leaving out what played.
	await letGo(page);
	await page.keyboard.press('s');
	await expect(shuffle).toHaveAttribute('aria-pressed', 'false');
	await expect(line).toHaveText('UP NEXT');
	const at = order.indexOf(shuffled[0] ?? '');
	const expected = order
		.slice(at + 1)
		.filter((title) => title !== order[0])
		.slice(0, 3);
	await expect.poll(() => upNext(page)).toEqual(expected);
});

test('Repeat one plays the song again at its end; Repeat all goes round to the first song', async ({
	page
}) => {
	const { order } = await playAlbum(page);
	await page.keyboard.press('n');
	const repeat = page.getByRole('button', { name: /^Repeat: / });
	await expect(repeat).toHaveAccessibleName('Repeat: off');
	await page.keyboard.press('r');
	await expect(repeat).toHaveAccessibleName('Repeat: all');
	await repeat.click();
	await expect(repeat).toHaveAccessibleName('Repeat: this song');
	await expect(page.getByText('REPEATING THIS SONG')).toBeVisible();

	await nearTheEnd(page);
	const seek = page.getByRole('slider', { name: 'Seek' });
	await expect(seek).toHaveAttribute('aria-valuetext', /^0:0\d of/, { timeout: 15_000 });
	await expect(page.getByRole('heading', { level: 1 })).toHaveText(order[0] ?? '');

	// All: the last song, then round to the first.
	await repeat.click();
	await repeat.click();
	await expect(repeat).toHaveAccessibleName('Repeat: all');
	// Off the toolbar, whose arrows are its own: the page's Shift+→ skips.
	await letGo(page);
	for (let step = 1; step < order.length; step += 1) {
		await page.keyboard.press('Shift+ArrowRight');
		await expect(page.getByRole('heading', { level: 1 })).toHaveText(order[step] ?? '');
	}
	await nearTheEnd(page);
	await expect(page.getByRole('heading', { level: 1 })).toHaveText(order[0] ?? '', {
		timeout: 15_000
	});
	await expect(page.getByRole('button', { name: 'Pause', exact: true })).toBeVisible();
});

test('a refresh keeps Shuffle and Repeat', async ({ page }) => {
	const { island } = await playAlbum(page);
	await island.getByRole('button', { name: 'Shuffle', exact: true }).click();
	await island.getByRole('button', { name: /^Repeat: / }).click();
	await island.getByRole('button', { name: 'Pause', exact: true }).click();
	// The pause saves within a moment.
	await page.waitForTimeout(1500);
	// Back on the fixtures (a reload of the album page would leave them), as a fresh load.
	await page.goto(fixtures('/'));
	await expect(island.getByRole('button', { name: 'Play', exact: true })).toBeVisible();
	await expect(island.getByRole('button', { name: 'Shuffle', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await expect(island.getByRole('button', { name: /^Repeat: / })).toHaveAccessibleName(
		'Repeat: all'
	);
});

test('the island is one Tab stop: arrows go along it, and Tab leaves the stars for Shuffle', async ({
	page
}) => {
	const { island } = await playAlbum(page);
	const toolbar = island.getByRole('toolbar', { name: 'Player controls' });
	const items = toolbar.locator('[data-roam]');
	await expect(items).toHaveCount(7);
	await expect(toolbar.locator('[data-roam][tabindex="0"]')).toHaveCount(1);

	await page.keyboard.press('Shift');
	await toolbar.locator('[data-roam][tabindex="0"]').focus();
	await expect(island.locator('a.now')).toBeFocused();
	await page.keyboard.press('ArrowRight');
	const stars = island.getByRole('slider', { name: /^Rating of/ });
	await expect(stars).toBeFocused();
	// Inside the stars, → rates as it always did.
	const before = Number(await stars.getAttribute('aria-valuenow'));
	await page.keyboard.press(before >= 5 ? 'ArrowLeft' : 'ArrowRight');
	await expect(stars).toHaveAttribute('aria-valuenow', String(before + (before >= 5 ? -0.5 : 0.5)));
	await page.keyboard.press('Tab');
	const shuffle = island.getByRole('button', { name: 'Shuffle', exact: true });
	await expect(shuffle).toBeFocused();
	await page.keyboard.press('End');
	await expect(island.getByRole('button', { name: 'Next' })).toBeFocused();
	await page.keyboard.press('ArrowLeft');
	await expect(island.getByRole('button', { name: 'Pause' })).toBeFocused();
	await page.keyboard.press('Home');
	await expect(island.locator('a.now')).toBeFocused();
	// The focus print is the keyboard's alone.
	await page.keyboard.press('End');
	const next = island.getByRole('button', { name: 'Next' });
	const ring = (where: Locator): Promise<string> =>
		where.evaluate((el) => getComputedStyle(el, '::after').boxShadow);
	await expect.poll(() => ring(next)).not.toBe('none');
	await page.mouse.click(5, 300);
	await next.click();
	await expect.poll(() => ring(next)).toBe('none');
});

test('the player keys: Space, seek, stars and N', async ({ page }) => {
	const { island } = await playAlbum(page);
	await letGo(page);
	await page.keyboard.press('3');
	await expect(island.getByRole('slider', { name: /^Rating of/ })).toHaveAttribute(
		'aria-valuenow',
		'3'
	);
	await page.keyboard.press('0');
	await expect(island.getByRole('slider', { name: /^Rating of/ })).toHaveAttribute(
		'aria-valuenow',
		'0'
	);
	await page.keyboard.press('Space');
	await expect(island.getByRole('button', { name: 'Play', exact: true })).toBeVisible();
	await page.keyboard.press('ArrowRight');
	await page.keyboard.press('ArrowRight');
	await expect(island.locator('.time')).toHaveText(/^0:2\d \//);
	await page.keyboard.press('ArrowLeft');
	await expect(island.locator('.time')).toHaveText(/^0:1\d \//);
	await page.keyboard.press('n');
	await expect(page).toHaveURL(/\/now-playing/);
	await page.keyboard.press('n');
	await expect(page).not.toHaveURL(/\/now-playing/);
});

test('hover brings the wash up behind a control, and reduced motion moves no glyph', async ({
	page
}) => {
	const { island } = await playAlbum(page);
	const next = island.getByRole('button', { name: 'Next' });
	const wash = (): Promise<string> =>
		next.evaluate((el) => getComputedStyle(el, '::before').opacity);
	await expect.poll(wash).toBe('0');
	await next.hover();
	await expect.poll(wash).toBe('1');
	const moved = (): Promise<string> =>
		next.evaluate((el) => getComputedStyle(el.querySelector('svg') ?? el).translate);
	await expect.poll(moved).toBe('2px');

	await page.emulateMedia({ reducedMotion: 'reduce' });
	await page.mouse.move(0, 0);
	await next.hover();
	await expect.poll(wash).toBe('1');
	await expect.poll(moved).toBe('none');
});
