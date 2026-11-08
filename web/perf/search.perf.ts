import { runs, test } from './fixture.ts';

test('Search: a keystroke to its results', async ({ page, perf }) => {
	await page.goto('/search');
	const box = page.getByRole('textbox', { name: 'Search your library' });
	await box.waitFor();
	const search = perf.samples('search');
	// Words the library is named from, a different one every run.
	const words = [
		...new Set(
			perf.manifest.loud.flatMap(({ title }) => title.split(' ')).map((w) => w.toLowerCase())
		)
	].filter((word) => word.length >= 4);
	if (words.length < runs) {
		throw new Error(`only ${String(words.length)} words to search for`);
	}
	for (const word of words.slice(0, runs)) {
		const typed = word.slice(0, -1);
		await box.fill(typed);
		await page.locator(`.search[data-answered="${typed}"]`).waitFor();
		await perf.time(
			search,
			'keydown',
			{ kind: 'selector', selector: `.search[data-answered="${word}"] .found` },
			() => page.keyboard.press(word.slice(-1))
		);
	}
});
