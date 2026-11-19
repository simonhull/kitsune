import type { Page } from '@playwright/test';
import { expect, runs, test } from './fixture.ts';
import { atRest, mountedAlbums, openLibrary } from './pages.ts';

/** How many songs the timed playlist holds: a long one, as a person's own grow. */
const songs = 500;

/** Makes a playlist of the library's first songs, through the API, and returns its ID. */
async function makePlaylist(page: Page, name: string, count: number): Promise<string> {
	const index = await page.request.get('/api/v1/library/index');
	const { artists } = (await index.json()) as {
		artists: { albums: { tracks: { id: string }[] }[] }[];
	};
	const tracks = artists
		.flatMap(({ albums }) => albums.flatMap(({ tracks: songsOf }) => songsOf.map(({ id }) => id)))
		.slice(0, count);
	const made = await page.request.post('/api/v1/me/playlists', {
		headers: { 'X-Kitsune-Request': '1' },
		data: { name, tracks }
	});
	expect(made.status()).toBe(201);
	const { playlist } = (await made.json()) as { playlist: { id: string } };
	return playlist.id;
}

test('Playlists: open one of 500 songs from its card', async ({ page, perf }) => {
	const id = await makePlaylist(page, `Perf ${String(Date.now())}`, songs);
	const open = perf.samples('open-playlist');
	for (let i = 0; i < runs; i++) {
		// A fresh page each time, so nothing of the playlist is held: the card's read is timed too.
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
});

test('Library: add a song to a playlist from its menu, press to the toast', async ({
	page,
	perf
}) => {
	const id = await makePlaylist(page, `Perf adds ${String(Date.now())}`, 0);
	await openLibrary(page);
	const add = perf.samples('add-to-playlist');
	const used = new Set<string>();
	for (let i = 0; i < runs; i++) {
		const album = (await mountedAlbums(page)).find(({ id: each }) => !used.has(each));
		if (album === undefined) {
			throw new Error('no album left on screen');
		}
		used.add(album.id);
		const row = page.locator(`main .row[data-track="${album.firstTrack}"]`);
		await row.scrollIntoViewIfNeeded();
		await row.click({ button: 'right' });
		await page.getByRole('menuitem', { name: /Add to playlist/ }).click();
		const item = page.locator(`[data-playlist="${id}"]`);
		await item.waitFor();
		const saved = page.waitForResponse(
			(r) =>
				r.url().endsWith(`/api/v1/me/playlists/${id}/tracks`) && r.request().method() === 'POST'
		);
		// The press to the toast: nothing waits on the server for it.
		await perf.time(
			add,
			'click',
			{ kind: 'selector', selector: '.toast[role="status"]:not([aria-hidden])' },
			() => item.click()
		);
		expect((await saved).status()).toBe(200);
		// The live toast, never the picture of one giving way. Undo leaves it saying what it did:
		// dismissed, the next run starts with none.
		const toast = page.locator('.toast:not([aria-hidden])');
		await toast.locator('button', { hasText: 'Undo' }).click();
		await toast.getByRole('button', { name: 'Dismiss' }).click();
		// Every toast gone, the pictures of those giving way too.
		await page.waitForFunction(() => document.querySelector('.toast') === null);
		await page.locator('.floating').first().waitFor({ state: 'detached' });
	}
});
