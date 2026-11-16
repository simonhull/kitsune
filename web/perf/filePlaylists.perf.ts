import type { Page } from '@playwright/test';
import { expect, runs, test } from './fixture.ts';
import { atRest } from './pages.ts';

const writes = { 'X-Kitsune-Request': '1' };

/** How many songs the timed file holds: as many as the regular playlist the suite opens. */
const songs = 500;

/** The file the suite saves into the library, replaced each run, so the library keeps one. */
const fileName = 'Kitsune perf.m3u8';

/** Makes a regular playlist of the library's first songs, and returns its ID. */
async function makePlaylist(page: Page, name: string, count: number): Promise<string> {
	const index = await page.request.get('/api/v1/library/index');
	const { artists } = (await index.json()) as {
		artists: { albums: { tracks: { id: string }[] }[] }[];
	};
	const tracks = artists
		.flatMap(({ albums }) => albums.flatMap(({ tracks: of }) => of.map(({ id }) => id)))
		.slice(0, count);
	const made = await page.request.post('/api/v1/me/playlists', {
		headers: writes,
		data: { name, tracks }
	});
	expect(made.status()).toBe(201);
	const { playlist } = (await made.json()) as { playlist: { id: string } };
	return playlist.id;
}

async function remove(page: Page, id: string): Promise<void> {
	const gone = await page.request.delete(`/api/v1/me/playlists/${id}`, { headers: writes });
	expect(gone.status()).toBe(204);
}

/**
 * Saves a playlist of 500 songs as a file at the top of the library, which the server reads at
 * once, and returns the file playlist's ID.
 */
async function makeFilePlaylist(page: Page): Promise<string> {
	const settings = await page.request.get('/api/v1/admin/settings');
	const { settings: values } = (await settings.json()) as {
		settings: { 'library.roots': { value: string[] } };
	};
	const [root] = values['library.roots'].value;
	expect(root).toBeDefined();
	const own = await makePlaylist(page, `Perf file ${String(Date.now())}`, songs);
	const saved = await page.request.post(`/api/v1/admin/playlists/${own}/save`, {
		headers: writes,
		data: { root, folder: '', name: fileName, replace: true }
	});
	expect(saved.status()).toBe(200);
	await remove(page, own);
	const listed = await page.request.get('/api/v1/me/playlists');
	const { playlists } = (await listed.json()) as {
		playlists: { id: string; source: string; file?: { path: string } }[];
	};
	const file = playlists.find((p) => p.source === 'file' && p.file?.path === fileName);
	expect(file).toBeDefined();
	return file?.id ?? '';
}

test('Playlists: open a file playlist of 500 songs from its card', async ({ page, perf }) => {
	const id = await makeFilePlaylist(page);
	const open = perf.samples('open-file-playlist');
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

test('Export: press Download, to the file arriving', async ({ page, perf }) => {
	const id = await makePlaylist(page, `Perf export ${String(Date.now())}`, songs);
	await page.goto(`/playlists/${id}`);
	await page.locator('main section.songs .row[data-track]').first().waitFor();
	await page.getByRole('button', { name: 'More for this playlist' }).click();
	await page.getByRole('menuitem', { name: 'Export .m3u' }).click();
	const link = page.getByRole('link', { name: 'Download' });
	await link.waitFor();
	await atRest(page);
	const press = perf.samples('export-download');
	for (let i = 0; i < runs; i++) {
		// The download starts once the response's headers are in: the server has written its first
		// byte, every song of the playlist read and its file begun.
		const started = page.waitForEvent('download');
		const at = performance.now();
		await link.click();
		const download = await started;
		press.add(performance.now() - at);
		await download.cancel();
	}
	await remove(page, id);
});
