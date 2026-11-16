import { runs, test } from './fixture.ts';
import { atRest } from './pages.ts';

test('Organize: open the page from Server, to the template painted', async ({ page, perf }) => {
	const open = perf.samples('open-organize');
	for (let i = 0; i < runs; i++) {
		await page.goto('/server');
		const link = page.getByRole('link', { name: 'ORGANIZE FILES ›' });
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
