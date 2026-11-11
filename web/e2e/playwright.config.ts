import { defineConfig } from '@playwright/test';

/**
 * The behavior suite: the built client on its fixtures, in a browser. Every page error fails the
 * test that caused it. `KITSUNE_E2E_CHROMIUM` runs a Chromium of your own instead of Playwright's.
 */
export default defineConfig({
	testDir: '.',
	testMatch: '*.e2e.ts',
	fullyParallel: true,
	retries: 0,
	reporter: [['list']],
	webServer: {
		command: 'bun run build && bun run preview -- --host 127.0.0.1 --port 4180 --strictPort',
		port: 4180,
		reuseExistingServer: !process.env['CI'],
		timeout: 180_000
	},
	use: {
		baseURL: 'http://127.0.0.1:4180',
		viewport: { width: 1440, height: 900 },
		launchOptions: {
			executablePath: process.env['KITSUNE_E2E_CHROMIUM'] || undefined,
			args: ['--mute-audio']
		}
	}
});
