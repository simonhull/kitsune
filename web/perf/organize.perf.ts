import type { Page } from '@playwright/test';
import { expect, runs, test } from './fixture.ts';
import { atRest, nextLoudAlbum, openLibrary } from './pages.ts';
import type { Sound } from './probe.ts';

/** What the `<audio>` element that plays a track's stream says of itself now. */
async function sound(page: Page, trackId: string): Promise<Sound> {
	return page.evaluate((src) => window.perf.sound(src), `/tracks/${trackId}/stream`);
}

/** Waits for the run `id` to end, and returns how. */
async function ended(page: Page, id: string): Promise<{ state: string; moved: number }> {
	for (;;) {
		const response = await page.request.get(`/api/v1/admin/organize/runs/${id}`);
		expect(response.status()).toBe(200);
		const { run } = (await response.json()) as { run: { state: string; moved: number } };
		if (run.state !== 'running') {
			return run;
		}
		await page.waitForTimeout(50);
	}
}

// Nothing on the play path waits on a run: a song whose file moves while it plays plays on,
// since a stream opens by the song's ID, never its path. Undo puts the file back.
test('Organize: a song keeps playing while its file moves', async ({ page, perf }) => {
	await openLibrary(page);
	const album = await nextLoudAlbum(page, perf.manifest, new Set());
	const row = page.locator(`main .row[data-track="${album.firstTrack}"]`);
	await row.scrollIntoViewIfNeeded();
	await row.locator('.play').click();
	await expect.poll(async () => (await sound(page, album.firstTrack)).at).toBeGreaterThan(0.5);
	const headers = { 'X-Kitsune-Request': '1' };
	const preview = await page.request.post('/api/v1/admin/organize/preview', {
		data: { template: 'Moved while playing/<Album>/<Title>', tracks: [album.firstTrack] },
		headers
	});
	expect(preview.status()).toBe(200);
	const { id, counts } = (await preview.json()) as { id: string; counts: { moves: number } };
	expect(counts.moves).toBe(1);
	const started = await page.request.post('/api/v1/admin/organize/runs', {
		data: { previewId: id },
		headers
	});
	expect(started.status()).toBe(202);
	const run = (await started.json()) as { id: string };
	expect((await ended(page, run.id)).moved).toBe(1);
	const before = await sound(page, album.firstTrack);
	await page.waitForTimeout(1500);
	const after = await sound(page, album.firstTrack);
	expect(after.error).toBe(false);
	expect(after.paused).toBe(false);
	expect(after.at).toBeGreaterThan(before.at);
	await page.locator('aside[aria-label="Player"] button.pause').click();
	const undo = await page.request.post(`/api/v1/admin/organize/runs/${run.id}/undo`, { headers });
	expect(undo.status()).toBe(202);
	const back = (await undo.json()) as { id: string };
	expect((await ended(page, back.id)).moved).toBe(1);
});

test('Organize: open the page from Server, to the template painted', async ({ page, perf }) => {
	const open = perf.samples('open-organize');
	for (let i = 0; i < runs; i++) {
		await page.goto('/server');
		const link = page.getByRole('link', { name: 'Organize where they sit' });
		await link.waitFor();
		await atRest(page);
		await perf.time(
			open,
			'click',
			{ kind: 'selector', selector: 'main .field.main .chip', path: '/server/organize' },
			() => link.click()
		);
	}
});

test('Organize: press Preview, to the first album rows', async ({ page, perf }) => {
	const paint = perf.samples('preview-paint');
	for (let i = 0; i < runs; i++) {
		// A fresh page each time, so the press previews the whole library from nothing.
		await page.goto('/server/organize');
		const button = page.getByRole('button', { name: 'Preview', exact: true });
		await button.waitFor();
		await atRest(page);
		await perf.time(paint, 'click', { kind: 'selector', selector: 'main article .file' }, () =>
			button.click()
		);
	}
});
