import type { Page } from '@playwright/test';
import { expect, runs, test } from './fixture.ts';
import { atRest, openLibrary } from './pages.ts';
import type { Wait } from './probe.ts';

const header = 'main .columns-head .header';

/** The Library header's columns, in order. */
function lineup(page: Page): Promise<string[]> {
	return page.$$eval(`${header} > [data-col]`, (cells) =>
		cells.map((cell) => cell.getAttribute('data-col') ?? '')
	);
}

/** Where a header cell's middle is, on screen. */
async function middle(page: Page, id: string): Promise<{ x: number; y: number }> {
	const box = await page.locator(`${header} > [data-col="${id}"]`).boundingBox();
	if (box === null) {
		throw new Error(`no header cell for ${id}`);
	}
	return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

function inOrder(order: string[]): Wait {
	return { kind: 'columns', selector: header, order };
}

function moved(order: readonly string[], id: string, to: number): string[] {
	const out = order.filter((each) => each !== id);
	out.splice(to, 0, id);
	return out;
}

// Wide enough for every default column, so a move crosses several.
test.use({ viewport: { width: 1920, height: 1080 } });

test('Library: move a column', async ({ page, perf }) => {
	await page.request.put('/api/v1/me/prefs/library.columns', {
		data: {
			order: ['number', 'title', 'artist', 'genre', 'rating', 'plays', 'lastPlayed', 'format'],
			artistWhenDifferent: true
		},
		headers: { 'X-Kitsune-Request': '1' }
	});
	await openLibrary(page);
	await atRest(page);
	const drag = perf.samples('column-drag');
	const drop = perf.samples('column-drop');
	const key = perf.samples('column-key');
	const start = await lineup(page);
	const id = 'genre';

	for (let i = 0; i < runs; i++) {
		// A 2 s sweep across every column shown, there and back, and a drop where it began.
		const from = await middle(page, id);
		const first = await middle(page, start.at(0) ?? id);
		const last = await middle(page, start.at(-1) ?? id);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 12, from.y);
		const frames = page.evaluate(() => window.perf.frames(2000));
		const until = Date.now() + 2000;
		let t = 0;
		while (Date.now() < until) {
			// Out to the last, back past the first, and home, once a second.
			const phase = (t % 60) / 60;
			const x =
				phase < 0.5
					? first.x + (last.x - first.x) * (phase * 2)
					: last.x - (last.x - first.x) * ((phase - 0.5) * 2);
			await page.mouse.move(x, from.y);
			t += 1;
		}
		drag.add(Math.max(...(await frames)));
		await page.keyboard.press('Escape');
		await page.mouse.up();
		await atRest(page);
	}
	expect(await lineup(page)).toEqual(start);

	for (let i = 0; i < runs; i++) {
		// Over one place to the right, then back.
		const order = await lineup(page);
		const at = order.indexOf(id);
		const to = i % 2 === 0 ? at + 1 : at - 1;
		const target = order[to] ?? id;
		const from = await middle(page, id);
		const over = await middle(page, target);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + (to > at ? 12 : -12), from.y);
		await page.mouse.move(over.x + (to > at ? 8 : -8), from.y, { steps: 6 });
		await page.evaluate(
			() =>
				new Promise<void>((done) => {
					requestAnimationFrame(() => {
						requestAnimationFrame(() => {
							done();
						});
					});
				})
		);
		await perf.time(drop, 'pointerup', inOrder(moved(order, id, to)), () => page.mouse.up());
		await atRest(page);
	}
	expect(await lineup(page)).toEqual(start);

	const label = page.locator(`${header} > [data-col="${id}"] .label`);
	await label.focus();
	for (let i = 0; i < runs; i++) {
		const order = await lineup(page);
		const at = order.indexOf(id);
		const right = i % 2 === 0;
		await perf.time(key, 'keydown', inOrder(moved(order, id, right ? at + 1 : at - 1)), () =>
			page.keyboard.press(right ? 'Alt+ArrowRight' : 'Alt+ArrowLeft')
		);
		await atRest(page);
	}
	expect(await lineup(page)).toEqual(start);
	// The moves are saved: what was there to begin with, after the last.
	await page.waitForTimeout(1200);
});
