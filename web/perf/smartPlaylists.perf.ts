import type { Page } from '@playwright/test';
import { expect, runs, test } from './fixture.ts';
import { albumTracks, atRest } from './pages.ts';

const writes = { 'X-Kitsune-Request': '1' };

/** Makes a smart playlist through the API, and returns its ID. */
async function makeSmart(page: Page, name: string, rules: unknown): Promise<string> {
	const made = await page.request.post('/api/v1/me/playlists', {
		headers: writes,
		data: { name, rules, seed: 1 }
	});
	expect(made.status()).toBe(201);
	const { playlist } = (await made.json()) as { playlist: { id: string } };
	return playlist.id;
}

async function remove(page: Page, id: string): Promise<void> {
	const gone = await page.request.delete(`/api/v1/me/playlists/${id}`, { headers: writes });
	expect(gone.status()).toBe(204);
}

/** Rules that choose songs rated at least halves. */
function ratedAtLeast(halves: number): unknown {
	return { match: 'all', rules: [{ field: 'rating', op: 'atLeast', value: halves }] };
}

/** How many songs the rules choose now, as the server counts them. */
async function counted(page: Page, rules: unknown): Promise<string> {
	const preview = await page.request.post('/api/v1/me/playlists/preview', {
		headers: writes,
		data: { rules }
	});
	expect(preview.status()).toBe(200);
	const { songCount } = (await preview.json()) as { songCount: number };
	return String(songCount);
}

test('Playlists: open a smart playlist of 500 songs from its card', async ({ page, perf }) => {
	// Worked out over the whole library each time it's read: a text rule, then the 500 least
	// recently played.
	const id = await makeSmart(page, `Perf smart ${String(Date.now())}`, {
		match: 'all',
		rules: [{ field: 'title', op: 'contains', value: 'a' }],
		limit: { count: 500, unit: 'songs', by: 'leastRecent' },
		order: 'chosen'
	});
	const open = perf.samples('open-smart-playlist');
	for (let i = 0; i < runs; i++) {
		await page.goto('/playlists');
		const card = page.locator(`a.card[href="/playlists/${id}"]`);
		await card.waitFor();
		await atRest(page);
		await perf.time(
			open,
			'click',
			{
				kind: 'selector',
				selector: 'main section.songs .row[data-track]',
				path: `/playlists/${id}`
			},
			() => card.click()
		);
	}
	await remove(page, id);
});

/**
 * Rates the first song of the Library's first album 4★, so "Rating is at least" counts more at
 * ★★★ than at ★★★★★ whatever else this run has rated: the generated library comes unrated.
 */
async function rateOne(page: Page): Promise<void> {
	const listed = await page.request.get('/api/v1/albums?sort=name&limit=1');
	const { albums } = (await listed.json()) as { albums: { id: string }[] };
	const album = albums[0];
	if (album === undefined) {
		throw new Error('no album to rate');
	}
	const [song] = await albumTracks(page, album.id);
	if (song === undefined) {
		throw new Error(`no song on ${album.id}`);
	}
	const rated = await page.request.put(`/api/v1/me/ratings/tracks/${song}`, {
		headers: writes,
		data: { stars: 4 }
	});
	expect(rated.status()).toBe(204);
}

test('Rules editor: press a star, to the count changed', async ({ page, perf }) => {
	// The server says how many songs each rating chooses.
	await rateOne(page);
	const [atThree, atFive] = [
		await counted(page, ratedAtLeast(6)),
		await counted(page, ratedAtLeast(10))
	];
	expect(atThree).not.toBe(atFive);
	const id = await makeSmart(page, `Perf rules ${String(Date.now())}`, ratedAtLeast(6));
	await page.goto(`/playlists/${id}`);
	await page.locator('main section.songs .row[data-track]').first().waitFor();
	await page.getByRole('button', { name: 'Edit rules' }).click();
	const count = '[data-testid="smart-count"]';
	await page.locator(`${count}[data-count="${atThree}"]`).waitFor();
	await atRest(page);
	const edit = perf.samples('edit-rule');
	const rules = page.getByRole('region', { name: 'Rules' });
	for (let i = 0; i < runs; i++) {
		// Up to ★★★★★ and back down to ★★★, each press timed to its new count.
		const [star, want] = i % 2 === 0 ? [5, atFive] : [3, atThree];
		const press = rules.getByRole('button', { name: new RegExp(`^${String(star)} stars`) });
		await perf.time(
			edit,
			'click',
			{ kind: 'attribute', selector: count, name: 'data-count', value: want },
			() => press.click()
		);
	}
	await page.getByRole('button', { name: 'Leave it' }).click();
	await remove(page, id);
});
