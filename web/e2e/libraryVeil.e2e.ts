import { expect, fixtures, landed, test } from './fixture';

/**
 * The band over the Library's list, from the window's top to the columns' line, is one veil edge to
 * edge: a cover or an artist's name scrolled under it is hidden across the whole width, never only
 * over the songs, where a cover's shadow drew a pale box round the narrower one.
 */
test('the veil over the list spans the page, and the rail and filters stay over it', async ({
	page
}) => {
	await page.setViewportSize({ width: 1800, height: 900 });
	await page.goto(fixtures('/library'));
	await landed(page);
	await page.mouse.move(1200, 500);
	await page.mouse.wheel(0, 1500);
	await expect
		.poll(() => page.evaluate(() => document.scrollingElement?.scrollTop ?? 0))
		.toBeGreaterThan(1000);

	const hits = await page.evaluate(() => {
		const head = document.querySelector('.columns-head');
		const line = head === null ? 0 : head.getBoundingClientRect().bottom;
		const covers = document.querySelector('.albums')?.getBoundingClientRect().left ?? 0;
		// Below the nav, above the line: left of the covers, over a cover, and the window's edge.
		const y = line - 12;
		const at = (x: number): string => {
			const top = document.elementFromPoint(x, y);
			return top?.closest('.columns-head, .top, .rail') === null ? 'through' : 'veiled';
		};
		return [at(covers - 20), at(covers + 40), at(4)];
	});
	expect(hits).toEqual(['veiled', 'veiled', 'veiled']);

	await expect(page.locator('.top').getByRole('button', { name: 'Everything' })).toBeVisible();
	const rail = page.locator('.rail');
	const railTop = await rail.evaluate((el) => {
		const r = el.getBoundingClientRect();
		const top = document.elementFromPoint(r.left + r.width / 2, r.top + 6);
		return el.contains(top);
	});
	expect(railTop).toBe(true);
});
