import { runs, test } from './fixture.ts';
import { albumPage, albumTracks, atRest, nextLoudAlbum, openLibrary, playing } from './pages.ts';

test('Album: open it, play, skip, and star from the island and the row', async ({ page, perf }) => {
	// A song's stars are its Rating column: the person's.
	const columns = {
		order: ['artist', 'genre', 'rating', 'plays', 'lastPlayed', 'format'],
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
	const islandStar = perf.samples('island-star');
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

		// The island's stars, where the heart was, rate the song playing: the right half of the
		// fourth sets 4.
		const islandStars = island.locator('.rating[role="slider"]');
		const islandBox = await islandStars.boundingBox();
		if (islandBox === null) {
			throw new Error(`no rating on the island for ${first}`);
		}
		await perf.time(
			islandStar,
			'click',
			{
				kind: 'attribute',
				selector: 'aside[aria-label="Player"] .rating[role="slider"]',
				name: 'aria-valuenow',
				value: '4'
			},
			() => islandStars.click({ position: { x: islandBox.width * 0.75, y: islandBox.height / 2 } })
		);

		const row = page.locator(thirdRow);
		await row.hover();
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
