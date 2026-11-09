import { runs, test } from './fixture.ts';
import { albumTracks, playing } from './pages.ts';

/**
 * Plays albums from their covers on the Albums page, by artist, where the loud albums come first. The
 * pointer rests on each card first, as a person's does on the way to its play.
 */
test('Albums: play one from its cover', async ({ page, perf }) => {
	await page.goto('/albums?sort=artist');
	await page.locator('main .card').first().waitFor();
	const play = perf.samples('play-cover');
	const island = page.locator('aside[aria-label="Player"]');
	for (const { artist, title } of perf.manifest.loud.slice(0, runs)) {
		let id: string | undefined;
		for (let tries = 0; tries < 100 && id === undefined; tries++) {
			id = await page.evaluate(
				([wantArtist, wantTitle]) =>
					[...document.querySelectorAll('main .card')]
						.find(
							(card) =>
								card.querySelector('.title')?.textContent === wantTitle &&
								card.querySelector('.artist')?.textContent === wantArtist
						)
						?.querySelector('a.open')
						?.getAttribute('href')
						?.split('/')
						.pop(),
				[artist, title]
			);
			if (id === undefined) {
				await page.mouse.wheel(0, 600);
				await page.waitForTimeout(100);
			}
		}
		if (id === undefined) {
			throw new Error(`${title} by ${artist} isn't on the Albums page`);
		}
		const [first] = await albumTracks(page, id);
		if (first === undefined) {
			throw new Error(`${title} has no songs`);
		}
		const card = page.locator('main .card', { has: page.locator(`a.open[href="/album/${id}"]`) });
		await card.scrollIntoViewIfNeeded();
		await card.hover();
		await page.waitForTimeout(300);
		await perf.time(play, 'click', playing(first), () => card.locator('button.cover-play').click());
		await island.locator('button.pause').click();
	}
});
