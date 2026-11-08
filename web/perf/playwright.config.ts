import { defineConfig } from '@playwright/test';
import { join } from 'node:path';
import { libraries, perfDir } from './servers.ts';

/**
 * The perf suite: one project per library size, one worker, so nothing else competes with what's
 * timed. `KITSUNE_PERF_CHROMIUM` runs a Chromium of your own instead of Playwright's download.
 */
export default defineConfig({
	testDir: '.',
	testMatch: '*.perf.ts',
	outputDir: join(perfDir, 'test-results'),
	globalSetup: './setup.ts',
	workers: 1,
	fullyParallel: false,
	retries: 0,
	timeout: 10 * 60_000,
	reporter: [['list']],
	use: {
		viewport: { width: 1440, height: 900 },
		launchOptions: {
			executablePath: process.env['KITSUNE_PERF_CHROMIUM'] || undefined,
			// Sound would only be noise on a runner; muting leaves the media pipeline as it is.
			args: ['--mute-audio']
		}
	},
	projects: libraries.map((library) => ({
		name: library.name,
		use: { baseURL: `http://127.0.0.1:${String(library.port)}` }
	}))
});
