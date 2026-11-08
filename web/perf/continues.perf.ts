import { runs, test } from './fixture.ts';
import { albumTracks, mountedAlbums, nextLoudAlbum, openLibrary, playing } from './pages.ts';

/**
 * How long before its end a song is moved to: time for the next one to be fetched on the side,
 * as it is 15 s ahead in a song played through, without waiting out every song.
 */
const leadSeconds = 3;

test('Library: a song runs into the next, and an album into the next album', async ({
	page,
	perf
}) => {
	await openLibrary(page);
	const track = perf.samples('track-change');
	const album = perf.samples('album-change');
	const used = new Set<string>();
	const island = page.locator('aside[aria-label="Player"]');
	for (let i = 0; i < runs; i++) {
		const listed = await nextLoudAlbum(page, perf.manifest, used);
		const shown = await mountedAlbums(page);
		const following = shown.at(shown.findIndex(({ id }) => id === listed.id) + 1);
		const tracks = await albumTracks(page, listed.id);
		const last = tracks.at(-1);
		const before = tracks.at(-2);
		if (following === undefined || last === undefined || before === undefined) {
			throw new Error(`${listed.title} has fewer than two songs, or nothing after it`);
		}
		const row = page.locator(`main .row[data-track="${before}"]`);
		await row.scrollIntoViewIfNeeded();
		// Played from the list, untimed: the press is play-library's to measure.
		await page.evaluate((wait) => {
			window.perf.arm(wait);
		}, playing(before));
		await row.locator('.play').click();
		await page.evaluate(() => window.perf.settle('click', 10_000));
		// Within the album: the next song was fetched on the side, as any next song is.
		await perf.time(track, 'ended', playing(last), () =>
			page.evaluate((lead) => {
				window.perf.toEnd(lead);
			}, leadSeconds)
		);
		// Into the next album down the list: it was put after the last song as that began.
		await perf.time(album, 'ended', playing(following.firstTrack), () =>
			page.evaluate((lead) => {
				window.perf.toEnd(lead);
			}, leadSeconds)
		);
		await island.locator('button.pause').click();
	}
});
