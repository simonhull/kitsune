import { runs, test } from './fixture.ts';
import { nextLoudAlbum, openLibrary } from './pages.ts';
import type { Wait } from './probe.ts';

/** The island, back from a remembered record: paused, so its button offers Play. */
const restoredIsland: Wait = {
	kind: 'selector',
	selector: 'aside[aria-label="Player"] button.pause[aria-label="Play"]'
};

/**
 * Loads Home with a record saved, and times the page load to the island restored, paused. The
 * record is the one this run leaves: a song played and paused here first, as a person would, so
 * there's always one to come back to.
 */
test('Resume: page load to the restored island', async ({ page, perf }) => {
	await openLibrary(page);
	const album = await nextLoudAlbum(page, perf.manifest, new Set());
	const row = page.locator(`main .row[data-track="${album.firstTrack}"]`);
	await row.scrollIntoViewIfNeeded();
	await row.locator('.play').click();
	const island = page.locator('aside[aria-label="Player"]');
	await island.locator('button.pause[aria-label="Pause"]').waitFor();
	// A pause is saved a moment later.
	const saved = page.waitForResponse(
		(response) =>
			response.url().endsWith('/api/v1/me/playback') && response.request().method() === 'PUT'
	);
	await island.locator('button.pause').click();
	await saved;
	const restore = perf.samples('resume-island');
	for (let i = 0; i < runs; i++) {
		await perf.load(restore, '/', restoredIsland);
	}
});
