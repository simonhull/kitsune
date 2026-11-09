import type { Page } from '@playwright/test';
import { percentile } from './budgets.ts';
import { expect, runs, test } from './fixture.ts';
import { albumTracks, mountedAlbums, nextLoudAlbum, openLibrary, playing } from './pages.ts';

/**
 * How long before its end a song is moved to: time for the next one to be fetched on the side,
 * as it is 15 s ahead in a song played through, without waiting out every song.
 */
const leadSeconds = 3;
/**
 * How much slower, in ms at the median, the change of album may be than a change of song. With
 * the list's own work taken out it measured the same 1.5 to 2 ms slower: that is the app moving to
 * another album, which any album change pays, and this is the margin over it.
 */
const albumOverTrackMs = 3;

/** Moves the song sounding to {@link leadSeconds} before its end. */
async function toEnd(page: Page): Promise<void> {
	await page.evaluate((lead) => {
		window.perf.toEnd(lead);
	}, leadSeconds);
}

test('Library: a song runs into the next, and an album into the next album', async ({
	page,
	perf
}) => {
	await openLibrary(page);
	const track = perf.samples('track-change');
	const album = perf.samples('album-change');
	const used = new Set<string>();
	const island = page.locator('aside[aria-label="Player"]');
	/** Plays a song from its row in the list, untimed: the press is play-library's to measure. */
	const startAt = async (id: string): Promise<void> => {
		const row = page.locator(`main .row[data-track="${id}"]`);
		await row.scrollIntoViewIfNeeded();
		await page.evaluate((wait) => {
			window.perf.arm(wait);
		}, playing(id));
		await row.locator('.play').click();
		await page.evaluate(() => window.perf.settle('click', 10_000));
	};
	let timed = 0;
	while (timed < runs) {
		const listed = await nextLoudAlbum(page, perf.manifest, used);
		// The album after it must be mounted too, to know its first song.
		await page.locator(`main .row[data-track="${listed.firstTrack}"]`).scrollIntoViewIfNeeded();
		const shown = await mountedAlbums(page);
		const following = shown.at(shown.findIndex(({ id }) => id === listed.id) + 1);
		const tracks = await albumTracks(page, listed.id);
		const [first, second] = tracks;
		const last = tracks.at(-1);
		if (
			first === undefined ||
			second === undefined ||
			last === undefined ||
			following === undefined
		) {
			// One song has no change of song to time against the change of album; and an album with
			// nothing mounted after it can't say where the list goes. The next loud album will do.
			continue;
		}
		// Each change goes into a song with real audio, a loud album's first or second, so the two
		// differ only in what comes before them: the next song, or the next album.
		await startAt(first);
		// Within the album: the next song was fetched on the side, as any next song is.
		await perf.time(track, 'ended', playing(second), () => toEnd(page));
		if (last !== second) {
			await startAt(last);
		}
		// Into the next album down the list: it was put after the last song as that began.
		await perf.time(album, 'ended', playing(following.firstTrack), () => toEnd(page));
		await island.locator('button.pause').click();
		timed += 1;
	}
	const over = percentile(album.values, 50) - percentile(track.values, 50);
	expect
		.soft(over, `the change of album, ${over.toFixed(1)} ms slower than a change of song`)
		.toBeLessThanOrEqual(albumOverTrackMs);
});
