import { runs, test } from './fixture.ts';
import { libraryRows, menu, nextLoudAlbum, openLibrary, playing } from './pages.ts';

test('Library: play a song, open the menus', async ({ page, perf }) => {
	await openLibrary(page);
	const play = perf.samples('play-library');
	const rowMenu = perf.samples('row-menu');
	const columns = perf.samples('columns-menu');
	const used = new Set<string>();
	for (let i = 0; i < runs; i++) {
		// A new album every time: the Library fetches the album of a song before it plays it.
		const album = await nextLoudAlbum(page, perf.manifest, used);
		const row = page.locator(`main .row[data-track="${album.firstTrack}"]`);
		await row.scrollIntoViewIfNeeded();
		await perf.time(play, 'click', playing(album.firstTrack), () => row.locator('.play').click());
		await page.locator('aside[aria-label="Player"] button.pause').click();

		await perf.time(rowMenu, 'contextmenu', menu(), () => row.click({ button: 'right' }));
		await page.keyboard.press('Escape');
		await page.locator('.floating').waitFor({ state: 'detached' });

		const header = page.locator('main .columns-head .header');
		await perf.time(columns, 'contextmenu', menu('Columns'), () =>
			header.click({ button: 'right', position: { x: 200, y: 10 } })
		);
		await page.keyboard.press('Escape');
		await page.locator('.floating').waitFor({ state: 'detached' });
	}
});

test('Library: first rows from a page load', async ({ page, perf }) => {
	await page.goto('/');
	const paint = perf.samples('library-paint');
	for (let i = 0; i < runs; i++) {
		await perf.load(paint, '/library', libraryRows);
	}
});

test('Library: frames during a fling', async ({ page, perf }) => {
	await openLibrary(page);
	const scroll = perf.samples('scroll');
	await page.mouse.move(900, 500);
	for (let i = 0; i < runs; i++) {
		const frames = page.evaluate(() => window.perf.frames(2000));
		const until = Date.now() + 2000;
		// Down, then back up, so a short library never runs out of list.
		const step = i % 2 === 0 ? 240 : -240;
		while (Date.now() < until) {
			await page.mouse.wheel(0, step);
		}
		scroll.add(Math.max(...(await frames)));
		await page.waitForTimeout(300);
	}
});
