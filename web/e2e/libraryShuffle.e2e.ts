import type { Locator, Page } from '@playwright/test';
import { expect, fixtures, test } from './fixture';

/**
 * Shuffle pressed while the Library plays on, album by album: it shuffles the Library, and off
 * goes back on through it from the song playing. The queue's state is read from the record the
 * fixtures keep in sessionStorage, which is what the player saves of it.
 */

interface Saved {
	kind: string;
	tracks: string[];
	index: number;
	continues?: string;
	shuffle?: { id: string };
}

/** The record the player last saved: its queue as track IDs. */
function saved(page: Page): Promise<Saved | null> {
	return page.evaluate(() => {
		const raw = sessionStorage.getItem('kitsune:fixtures:playback');
		return raw === null ? null : (JSON.parse(raw) as { save: Saved }).save;
	});
}

/** The fixtures' track IDs are their album's, then a number. */
const albumOf = (trackId: string): string => trackId.slice(0, trackId.lastIndexOf('-'));

/** The Library's songs on screen, in its order: their IDs and titles. */
async function openLibrary(page: Page): Promise<{ ids: string[]; titles: string[] }> {
	await page.goto(fixtures('/'));
	await page.getByRole('link', { name: 'Library', exact: true }).click();
	const rows = page.locator('[data-track]');
	await expect(rows.nth(6)).toBeVisible();
	const read = (name: string): Promise<string[]> =>
		rows.evaluateAll((all, attr) => all.map((row) => row.getAttribute(attr) ?? ''), name);
	return { ids: await read('data-track'), titles: await read('aria-label') };
}

/** Plays a song from the Library's list, and waits for the island to have landed. */
async function playFromLibrary(page: Page, id: string, title: string): Promise<Locator> {
	const row = page.locator(`[data-track="${id}"]`);
	await row.getByRole('button', { name: `Play ${title}` }).click();
	const island = page.locator('aside[aria-label="Player"]');
	await expect(island.locator('.song')).toHaveText(title);
	await expect(island).not.toHaveClass(/arrive/);
	return island;
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
function upNext(page: Page): Promise<string[]> {
	return page.getByRole('region', { name: 'Up next' }).locator('.pill-name').allTextContents();
}

/** A shuffle session's Up next on Now playing: the songs it has queued and picked, by ID. */
function sessionNext(page: Page): Promise<string[]> {
	return page
		.getByRole('region', { name: 'Up next' })
		.locator('.slot [data-track]')
		.evaluateAll((all) => all.map((row) => row.getAttribute('data-track') ?? ''));
}

test('Shuffle on a song from the Library shuffles the Library, and off goes on through it from the song playing', async ({
	page
}) => {
	const { ids, titles } = await openLibrary(page);
	// A song with three more of its album after it: Up next is those, in the Library's order.
	const start = ids.findIndex((id, at) =>
		ids.slice(at + 1, at + 4).every((each) => albumOf(each) === albumOf(id))
	);
	expect(start).toBeGreaterThanOrEqual(0);
	const libraryNext = ids[start + 1] ?? '';
	const island = await playFromLibrary(page, ids[start] ?? '', titles[start] ?? '');
	await page.keyboard.press('n');
	await expect(page).toHaveURL(/\/now-playing/);
	await expect.poll(() => upNext(page)).toEqual(titles.slice(start + 1, start + 4));

	const shuffle = island.getByRole('button', { name: 'Shuffle', exact: true });
	const heading = page.getByRole('heading', { level: 1 });
	const session = page.getByText('NOW PLAYING · SHUFFLING THE LIBRARY');
	/** Turns it on, and waits for the session's songs to be up next. */
	const on = async (): Promise<string[]> => {
		await shuffle.click();
		await expect(shuffle).toHaveAttribute('aria-pressed', 'true');
		await expect(session.first()).toBeVisible();
		await expect.poll(async () => (await sessionNext(page)).length).toBeGreaterThan(2);
		return sessionNext(page);
	};
	let coming = await on();
	// The song playing never stopped, and the queue is a session's now, saved as one.
	await expect(heading).toHaveText(titles[start] ?? '');
	await expect(island.getByRole('button', { name: 'Pause', exact: true })).toBeVisible();
	await expect.poll(async () => (await saved(page))?.shuffle?.id).toMatch(/^shuffle-/);
	// The session leads with a song drawn at random: one that is the Library's next song by luck
	// says nothing, so it's drawn again until it isn't.
	for (let tries = 0; coming[0] === libraryNext && tries < 20; tries++) {
		await shuffle.click();
		await expect(shuffle).toHaveAttribute('aria-pressed', 'false');
		coming = await on();
	}
	const [playing = ''] = coming;
	expect(playing).not.toBe(libraryNext);

	// Next plays the session's song, not the Library's next one.
	await island.getByRole('button', { name: 'Next', exact: true }).click();
	await expect(session.first()).toBeVisible();
	await expect
		.poll(async () => {
			const record = await saved(page);
			return record?.kind === 'shuffle' ? record.tracks.at(record.index) : undefined;
		})
		.toBe(playing);
	const title = (await heading.textContent()) ?? '';

	// Off, from S: on through the Library from the song playing, its album first, in order.
	await letGo(page);
	await page.keyboard.press('s');
	await expect(shuffle).toHaveAttribute('aria-pressed', 'false');
	await expect(session).toHaveCount(0);
	await expect(heading).toHaveText(title);
	await expect(island.getByRole('button', { name: 'Pause', exact: true })).toBeVisible();
	await expect.poll(async () => (await saved(page))?.kind).not.toBe('shuffle');
	const back = await saved(page);
	expect(back?.shuffle).toBeUndefined();
	expect(back?.tracks.at(back.index)).toBe(playing);
	const ahead = back?.tracks.slice(back.index + 1) ?? [];
	const ownAlbum = ahead.filter((id) => albumOf(id) === albumOf(playing));
	const number = (id: string): number => Number(id.slice(id.lastIndexOf('-') + 1));
	// What's ahead of its album is the rest of it, in order, and the Library goes on after it.
	expect(ahead.slice(0, ownAlbum.length)).toEqual(ownAlbum);
	expect(ownAlbum.map(number)).toEqual(ownAlbum.map(number).toSorted((a, b) => a - b));
	expect(ownAlbum.every((id) => number(id) > number(playing))).toBe(true);
	expect([albumOf(playing), ...ahead.map(albumOf)]).toContain(back?.continues);
});

test('off on the last song of an album goes on to the next album in the Library', async ({
	page
}) => {
	const { ids, titles } = await openLibrary(page);
	// The first album's last song: the Library's next songs are the next album's, wholly on screen.
	const last = ids.findIndex((id, at) => albumOf(id) !== albumOf(ids[at + 1] ?? id));
	expect(last).toBeGreaterThanOrEqual(0);
	const nextAlbum = albumOf(ids[last + 1] ?? '');
	const end = ids.findIndex((id, at) => at > last && albumOf(id) !== nextAlbum);
	expect(end).toBeGreaterThan(last + 1);
	const following = titles.slice(last + 1, Math.min(end, last + 4));
	const island = await playFromLibrary(page, ids[last] ?? '', titles[last] ?? '');
	await page.keyboard.press('n');
	await expect(page).toHaveURL(/\/now-playing/);
	await expect.poll(() => upNext(page)).toEqual(following);

	const shuffle = island.getByRole('button', { name: 'Shuffle', exact: true });
	await shuffle.click();
	await expect(page.getByText('NOW PLAYING · SHUFFLING THE LIBRARY')).toBeVisible();
	await expect.poll(async () => (await saved(page))?.shuffle?.id).toMatch(/^shuffle-/);

	await shuffle.click();
	await expect(shuffle).toHaveAttribute('aria-pressed', 'false');
	await expect(page.getByRole('heading', { level: 1 })).toHaveText(titles[last] ?? '');
	await expect.poll(() => upNext(page)).toEqual(following);
	await expect
		.poll(async () => {
			const record = await saved(page);
			return record?.kind === 'shuffle' ? null : record?.tracks.slice(record.index);
		})
		.toEqual(expect.arrayContaining([ids[last], ids[last + 1]]));
	const back = await saved(page);
	expect(back?.tracks.slice(back.index, back.index + 2)).toEqual([ids[last], ids[last + 1]]);
	expect(back?.continues).toBe(nextAlbum);
});

test('Now playing in a shuffle names its album, its artist is a link, and the song playing has a ⋯ menu', async ({
	page
}) => {
	const { ids, titles } = await openLibrary(page);
	const title = titles[0] ?? '';
	const island = await playFromLibrary(page, ids[0] ?? '', title);
	await page.keyboard.press('n');
	await expect(page).toHaveURL(/\/now-playing/);
	const shuffle = island.getByRole('button', { name: 'Shuffle', exact: true });
	await shuffle.click();
	await expect(page.getByText('NOW PLAYING · SHUFFLING THE LIBRARY').first()).toBeVisible();

	// The top line keeps the album while shuffling.
	await expect(
		page.locator('.where').getByRole('link', { name: /^Go to the album / })
	).toBeVisible();

	// The song playing heads Up next, and its ⋯ opens the menu any song's does, with nothing to
	// take out of the queue.
	const upNextRegion = page.getByRole('region', { name: 'Up next' });
	await upNextRegion.getByRole('button', { name: `More for ${title}`, exact: true }).click();
	const menu = page.getByRole('menu', { name: title, exact: true });
	await expect(menu).toBeVisible();
	await expect(menu.getByRole('menuitem', { name: 'Go to album' })).toBeVisible();
	await expect(menu.getByRole('menuitem', { name: 'Remove from queue' })).toHaveCount(0);
	await menu.getByRole('menuitem', { name: 'Go to artist' }).click();
	await expect(page).toHaveURL(/\/artists\//);

	// Back on Now playing, the artist's name is the way to the artist too.
	await page.goBack();
	await expect(page).toHaveURL(/\/now-playing/);
	await page.locator('.names').getByRole('link').first().click();
	await expect(page).toHaveURL(/\/artists\//);
});
