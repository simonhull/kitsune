import { test as base, expect } from '@playwright/test';

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
