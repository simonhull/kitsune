import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// A single-page client: the Go server hands every unknown path to index.html.
			adapter: adapter({ fallback: 'index.html' })
		})
	],
	test: {
		include: ['src/**/*.test.ts'],
		environment: 'node'
	}
});
