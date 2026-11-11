import { runs, test } from './fixture.ts';
import { atRest, openLibrary } from './pages.ts';
import type { Wait } from './probe.ts';

/** Any song starts: a shuffle's song is the shuffle's to choose. */
const anySong: Wait = { kind: 'playing', src: '/tracks/' };

test('Shuffle: start the library, then Next', async ({ page, perf }) => {
	await openLibrary(page);
	const start = perf.samples('play-shuffle');
	const next = perf.samples('next-shuffle');
	const island = page.locator('aside[aria-label="Player"]');
	const shuffle = page.getByRole('button', { name: 'Shuffle the library' });
	for (let i = 0; i < runs; i++) {
		// The first song is picked, and its album fetched, while the page sits: the press never waits.
		await page.waitForTimeout(300);
		// The disc has stamped in, and the last press's ring is gone: Playwright waits for a still
		// target before it clicks, so the probe is armed only once there is nothing left to wait for.
		await atRest(page);
		await perf.time(start, 'click', anySong, () => shuffle.click());
		// The server's session lands and the queue fills while the first song plays.
		await page.waitForTimeout(500);
		await perf.time(next, 'click', anySong, () =>
			island.getByRole('button', { name: 'Next' }).click()
		);
		await island.locator('button.pause').click();
	}
});
