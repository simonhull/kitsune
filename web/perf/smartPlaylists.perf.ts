import type { Page } from '@playwright/test';
import { expect, runs, test } from './fixture.ts';
import { atRest } from './pages.ts';

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

/** The library's first songs, by ID. */
async function firstSongs(page: Page, count: number): Promise<string[]> {
	const index = await page.request.get('/api/v1/library/index');
	const { artists } = (await index.json()) as {
		artists: { albums: { tracks: { id: string }[] }[] }[];
	};
	return artists
		.flatMap(({ albums }) => albums.flatMap(({ tracks }) => tracks.map(({ id }) => id)))
		.slice(0, count);
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

test('Rules editor: press a star, to the count changed', async ({ page, perf }) => {
	// Ten songs at 5★ and ten at 3★, so "Rating is at least" counts 20 at ★★★ and 10 at ★★★★★.
	const songs = await firstSongs(page, 20);
	for (const [i, song] of songs.entries()) {
		const rated = await page.request.put(`/api/v1/me/ratings/tracks/${song}`, {
			headers: writes,
			data: { stars: i < 10 ? 5 : 3 }
		});
		expect(rated.status()).toBe(204);
	}
	const id = await makeSmart(page, `Perf rules ${String(Date.now())}`, {
		match: 'all',
		rules: [{ field: 'rating', op: 'atLeast', value: 6 }]
	});
	await page.goto(`/playlists/${id}`);
	await page.locator('main section.songs .row[data-track]').first().waitFor();
	await page.getByRole('button', { name: 'Edit rules' }).click();
	const count = '[data-testid="smart-count"]';
	await page.locator(`${count}[data-count="20"]`).waitFor();
	await atRest(page);
	const edit = perf.samples('edit-rule');
	const rules = page.getByRole('region', { name: 'Rules' });
	for (let i = 0; i < runs; i++) {
		// Up to ★★★★★ and back down to ★★★, each press timed to its new count.
		const [star, counted] = i % 2 === 0 ? [5, '10'] : [3, '20'];
		const press = rules.getByRole('button', { name: new RegExp(`^${String(star)} stars`) });
		await perf.time(
			edit,
			'click',
			{ kind: 'attribute', selector: count, name: 'data-count', value: counted },
			() => press.click()
		);
	}
	await page.getByRole('button', { name: 'Leave it' }).click();
	await remove(page, id);
	for (const song of songs) {
		await page.request.delete(`/api/v1/me/ratings/tracks/${song}`, { headers: writes });
	}
});
