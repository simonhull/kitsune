import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';

export default defineConfig(({ mode }) => {
	// Same origin as the client, so the session cookie just works in dev and preview.
	const api = loadEnv(mode, '.', 'KITSUNE_')['KITSUNE_API'] ?? 'http://localhost:4533';
	const proxy = { '/api': api };

	return {
		server: { proxy },
		preview: { proxy },
		plugins: [
			sveltekit({
				compilerOptions: {
					// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
					runes: ({ filename }) =>
						filename.split(/[/\\]/).includes('node_modules') ? undefined : true
				},
				// A single-page client: the Go server hands every unknown path to index.html.
				adapter: adapter({ fallback: 'index.html' }),
				// An open tab learns of a new deploy, and takes it on its next navigation (lib/shell/freshness.ts).
				version: { pollInterval: 5 * 60_000 }
			})
		],
		test: {
			include: ['src/**/*.test.ts'],
			environment: 'node'
		}
	};
});
