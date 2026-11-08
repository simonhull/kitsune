import { describe, expect, it } from 'vitest';
import { rawColors } from './rawColors.ts';

const components = import.meta.glob<string>('/src/**/*.svelte', {
	query: '?raw',
	import: 'default',
	eager: true
});
const sheets = import.meta.glob<string>(['/src/**/*.css', '!/src/lib/styles/tokens.css'], {
	query: '?raw',
	import: 'default',
	eager: true
});

describe('rawColors', () => {
	it('finds hex, rgb() and hsl() in a style block', () => {
		const source = [
			'<div class="a"></div>',
			'<style>',
			'\t.a {',
			'\t\tcolor: #f28c28;',
			'\t\tbackground: rgb(255 255 255 / 8%);',
			'\t\tborder-color: hsl(30 80% 50%);',
			'\t\tbox-shadow: 0 1px 2px rgba(0, 0, 0, 0.5);',
			'\t\toutline-color: #FFF;',
			'\t}',
			'</style>'
		].join('\n');
		expect(rawColors(source).map((found) => found.line)).toEqual([4, 5, 6, 7, 8]);
	});

	it('finds them in a style attribute', () => {
		expect(rawColors('<span style="color: #a64b00">x</span>')).toEqual([
			{ line: 1, text: '<span style="color: #a64b00">x</span>' }
		]);
		expect(rawColors(`<span style:color='rgb(1 2 3)'>x</span>`)).toHaveLength(1);
	});

	it('leaves tokens, mixes of tokens, scripts and markup alone', () => {
		const source = [
			'<script lang="ts">',
			"\tconst fox = '#f28c28';",
			'</script>',
			'{#each items as item (item)}<a href="#L" style:background={palette.field}>x</a>{/each}',
			'<style>',
			'\t.a {',
			'\t\tcolor: var(--fox-text);',
			'\t\tbackground: color-mix(in srgb, var(--ground) 30%, transparent);',
			'\t}',
			'</style>'
		].join('\n');
		expect(rawColors(source)).toEqual([]);
	});

	it('lets a line through that says why, on it or just above', () => {
		const source = [
			'<style>',
			'\t/* raw-color: a mask reads only alpha. */',
			'\t.a { mask-image: linear-gradient(#000, transparent); }',
			'\t.b { color: #000; /* raw-color: a test. */ }',
			'\t.c { margin: 0; }',
			'\t.d { color: #000; }',
			'</style>'
		].join('\n');
		expect(rawColors(source)).toEqual([{ line: 6, text: '.d { color: #000; }' }]);
	});

	it('reads all of a stylesheet', () => {
		expect(rawColors('html {\n\tbackground: #0a0908;\n}', true)).toEqual([
			{ line: 2, text: 'background: #0a0908;' }
		]);
	});
});

describe('the web client', () => {
	it('found its components', () => {
		expect(Object.keys(components).length).toBeGreaterThan(40);
	});

	it.each(Object.entries(components))('%s reads every color from a token', (_, source) => {
		expect(rawColors(source)).toEqual([]);
	});

	it.each(Object.entries(sheets))('%s reads every color from a token', (_, source) => {
		expect(rawColors(source, true)).toEqual([]);
	});
});
