import { test as base, expect, type Page } from '@playwright/test';

/** A page that fails its test on any uncaught error or console error the app reports. */
export const test = base.extend({
	page: async ({ page }, use) => {
		const errors: string[] = [];
		page.on('pageerror', (error) => errors.push(error.stack ?? error.message));
		page.on('console', (message) => {
			if (message.type() === 'error') errors.push(message.text());
		});
		await use(page);
		expect(errors, 'the page reported errors').toEqual([]);
	}
});

export { expect };

/** Opens the app on its fixtures. Navigate onward by clicking: a goto would leave them. */
export const fixtures = (path = '/'): string =>
	`${path}${path.includes('?') ? '&' : '?'}source=fixtures`;

/**
 * Waits for the journey to the page on screen to come to rest: nothing of the page left under it,
 * nothing in the air or coming out from under the nav, nothing hidden under a flier, and no unit still on its way in. Read a page's
 * geometry only after this: units arriving are still moving.
 */
export async function landed(page: Page): Promise<void> {
	await page.waitForFunction(
		() => {
			const shell = document.querySelector('.shell');
			const ghost = shell?.querySelector(':scope > .ghost-layer')?.children.length ?? 1;
			const air = shell?.querySelector(':scope > .morph-layer')?.children.length ?? 1;
			const born = shell?.querySelector(':scope > .born-layer')?.children.length ?? 1;
			const hidden = document.querySelectorAll('[data-morph-hidden]').length;
			const moving = [...document.querySelectorAll('.shell > main [data-morph-part]')].filter(
				(unit) =>
					unit instanceof HTMLElement && (unit.style.opacity !== '' || unit.style.transform !== '')
			).length;
			return ghost + air + born + hidden + moving === 0;
		},
		null,
		{ timeout: 10_000 }
	);
}
