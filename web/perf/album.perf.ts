import { runs, test } from './fixture.ts';
import { albumPage, albumTracks, atRest, nextLoudAlbum, openLibrary, playing } from './pages.ts';

test('Album: open it, play, skip, heart and star', async ({ page, perf }) => {
	// Hearts and stars on a song are its Heart and Rating columns: the person's, here with Heart on.
	const columns = {
		order: ['artist', 'genre', 'rating', 'plays', 'lastPlayed', 'format', 'heart'],
		artistWhenDifferent: true
	};
	await page.request.put('/api/v1/me/prefs/library.columns', {
		data: columns,
		headers: { 'X-Kitsune-Request': '1' }
	});
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
		// Someone looks before they press: the page has landed, and only the press is timed.
		await atRest(page);

		const rows = page.locator('main section[aria-label="Tracks"] .row');
		const thirdRow = `main section[aria-label="Tracks"] .row[data-track="${third}"]`;
		await perf.time(play, 'click', playing(first), () => rows.nth(0).locator('.play').click());
		await perf.time(next, 'click', playing(second), () =>
			island.getByRole('button', { name: 'Next' }).click()
		);
		// Within three seconds of its start, Previous goes back a song rather than to the start.
		await perf.time(previous, 'click', playing(first), () =>
			island.getByRole('button', { name: 'Previous' }).click()
		);
		await island.locator('button.pause').click();

		const row = page.locator(thirdRow);
		const heartButton = row.locator('button.heart');
		await row.hover();
		await perf.time(
			heart,
			'click',
			{
				kind: 'attribute',
				selector: `${thirdRow} button.heart`,
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
		// The rating is one slider: the right half of its fourth star sets 4.
		const rating = row.locator('.rate[role="slider"]');
		const box = await rating.boundingBox();
		if (box === null) {
			throw new Error(`no rating on ${third}`);
		}
		await perf.time(
			star,
			'click',
			{
				kind: 'attribute',
				selector: `${thirdRow} .rate[role="slider"]`,
				name: 'aria-valuenow',
				value: '4'
			},
			() => rating.click({ position: { x: box.width * 0.75, y: box.height / 2 } })
		);
		await page.goBack();
		await page.locator('main .row[data-track]').first().waitFor();
	}
});
