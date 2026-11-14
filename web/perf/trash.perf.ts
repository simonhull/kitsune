import { expect, runs, test } from './fixture.ts';
import { mountedAlbums, openLibrary } from './pages.ts';

test('Library: move a song to Trash, and Undo', async ({ page, perf }) => {
	await openLibrary(page);
	const trash = perf.samples('trash-song');
	const used = new Set<string>();
	for (let i = 0; i < runs; i++) {
		// A song of a new album every time, its first: the album stays, with its others.
		const album = (await mountedAlbums(page)).find(({ id }) => !used.has(id));
		if (album === undefined) {
			throw new Error('no album left on screen');
		}
		used.add(album.id);
		const row = page.locator(`main .row[data-track="${album.firstTrack}"]`);
		await row.scrollIntoViewIfNeeded();
		await row.click({ button: 'right' });
		await page.locator('.floating.placed').waitFor();
		const item = page.getByRole('menuitem', { name: 'Move to Trash' });
		const moved = page.waitForResponse(
			(r) => r.url().endsWith('/api/v1/admin/trash') && r.request().method() === 'POST'
		);
		// The press to the frame the row starts folding in: nothing waits on the server for it.
		await perf.time(
			trash,
			'click',
			{ kind: 'attribute', selector: 'html', name: 'data-folding', value: '' },
			() => item.click()
		);
		expect((await moved).status()).toBe(200);
		// Undo, so every run leaves the library as it found it.
		const restored = page.waitForResponse((r) => r.url().endsWith('/api/v1/admin/trash/restore'));
		await page.locator('.toast button.undo').click();
		expect((await restored).status()).toBe(200);
		await row.waitFor();
		await page.locator('.floating').waitFor({ state: 'detached' });
	}
});
