import { runs, test } from './fixture.ts';
import { albumPage, albumTracks, nextLoudAlbum, openLibrary, playing } from './pages.ts';

test('Album: open it, play, skip, heart and star', async ({ page, perf }) => {
	await openLibrary(page);
	const open = perf.samples('open-album');
	const play = perf.samples('play-album');
	const next = perf.samples('next');
	const previous = perf.samples('previous');
	const heart = perf.samples('heart');
	const heartServer = perf.samples('heart-server');
	const star = perf.samples('star');
	const used = new Set<string>();
	const island = page.locator('aside[aria-label="Player"]');
	for (let i = 0; i < runs; i++) {
		const album = await nextLoudAlbum(page, perf.manifest, used);
		const tracks = await albumTracks(page, album.id);
		const [first, second, third] = tracks;
		if (first === undefined || second === undefined || third === undefined) {
			throw new Error(`${album.title} has fewer than three songs`);
		}
		const art = page.locator(`main a.art[href="/album/${album.id}"]`);
		await art.scrollIntoViewIfNeeded();
		await perf.time(open, 'click', albumPage(album.id), () => art.click());

		const rows = page.locator('main section[aria-label="Tracks"] .row');
		await perf.time(play, 'click', playing(first), () => rows.nth(0).locator('.play').click());
		await perf.time(next, 'click', playing(second), () =>
			island.getByRole('button', { name: 'Next' }).click()
		);
		// Within three seconds of its start, Previous goes back a song rather than to the start.
		await perf.time(previous, 'click', playing(first), () =>
			island.getByRole('button', { name: 'Previous' }).click()
		);
		await island.locator('button.pause').click();

		const row = rows.nth(2);
		const heartButton = row.locator('button.heart');
		await row.hover();
		await perf.time(
			heart,
			'click',
			{
				kind: 'attribute',
				selector: `main section[aria-label="Tracks"] .row:nth-child(3) button.heart`,
				name: 'aria-pressed',
				value: 'true'
			},
			() => heartButton.click()
		);
		heartServer.add(
			await page.evaluate(async (id) => {
				for (let tries = 0; tries < 500; tries++) {
					const entry = performance
						.getEntriesByType('resource')
						.find((each) => each.name.includes(`/me/hearts/tracks/${id}`));
					if (entry instanceof PerformanceResourceTiming && entry.responseEnd > 0) {
						return entry.responseEnd - entry.startTime;
					}
					await new Promise((done) => setTimeout(done, 20));
				}
				throw new Error(`no request hearted ${id}`);
			}, third)
		);
		await perf.time(
			star,
			'click',
			{
				kind: 'attribute',
				selector: `main section[aria-label="Tracks"] .row:nth-child(3) .stars button:nth-child(4)`,
				name: 'aria-pressed',
				value: 'true'
			},
			() => row.locator('.stars button').nth(3).click()
		);
		await page.goBack();
		await page.locator('main .row[data-track]').first().waitFor();
	}
});
