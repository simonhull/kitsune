import { runs, test } from './fixture.ts';
import { libraryRows, menu, nextLoudAlbum, openLibrary, playing } from './pages.ts';

test('Library: play a song, open the menus', async ({ page, perf }) => {
	await openLibrary(page);
	const play = perf.samples('play-library');
	const playCell = perf.samples('play-library-cell');
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

		// Anywhere on a row plays it: the same, from a cell that isn't the title. Last, so its
		// song's loading is over before the next round's menus are timed.
		const other = await nextLoudAlbum(page, perf.manifest, used);
		const cell = page.locator(`main .row[data-track="${other.firstTrack}"] .time`);
		await cell.scrollIntoViewIfNeeded();
		await perf.time(playCell, 'click', playing(other.firstTrack), () => cell.click());
		await page.locator('aside[aria-label="Player"] button.pause').click();
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

test('Library: change the order', async ({ page, perf }) => {
	await openLibrary(page);
	const sort = perf.samples('sort-library');
	// Who heads the list each way: the index comes A to Z, so Z to A starts with its last.
	const [first, last] = await page.evaluate(async () => {
		const response = await fetch('/api/v1/library/index');
		const { artists } = (await response.json()) as { artists: { name: string }[] };
		return [artists.at(0)?.name ?? '', artists.at(-1)?.name ?? ''];
	});
	await page.locator('main button.sort[aria-haspopup="menu"]').click();
	await page.locator('.floating.placed').waitFor();
	// Name is the artists' first order: picking it again turns the list round, and back. Each time
	// the list starts again from its top, under the artist who heads it now.
	const name = page.locator('.floating.placed [role="menuitemradio"]').first();
	for (let i = 0; i < runs * 2; i++) {
		const heading = i % 2 === 0 ? last : first;
		await perf.time(
			sort,
			'click',
			{
				kind: 'selector',
				selector: `main .list > section.artist:first-of-type[aria-label=${JSON.stringify(heading)}]`
			},
			() => name.click()
		);
	}
	// An even number of turns leaves the default for the tests after this one.
});
