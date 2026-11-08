import { describe, expect, it } from 'vitest';
import { isThemeChoice, resolveTheme, type Theme, type ThemeChoice } from './theme.svelte.ts';

describe('resolveTheme', () => {
	const cases: [string, ThemeChoice | null, boolean, Theme][] = [
		['Light on a dark system', 'light', false, 'light'],
		['Light on a light system', 'light', true, 'light'],
		['Dark on a dark system', 'dark', false, 'dark'],
		['Dark on a light system', 'dark', true, 'dark'],
		['System on a dark system', 'system', false, 'dark'],
		['System on a light system', 'system', true, 'light'],
		['signed out on a dark system', null, false, 'dark'],
		['signed out on a light system', null, true, 'light']
	];

	it.each(cases)('%s', (_, choice, prefersLight, want) => {
		expect(resolveTheme(choice, prefersLight)).toBe(want);
	});
});

describe('isThemeChoice', () => {
	it('takes the three choices and nothing else', () => {
		expect(['system', 'light', 'dark'].every(isThemeChoice)).toBe(true);
		expect([null, undefined, '', 'Light', 'sepia', 1].some(isThemeChoice)).toBe(false);
	});
});
