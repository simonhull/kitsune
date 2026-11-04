import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { library } from '#lib/library/library.svelte.ts';
import type { AlbumEntry } from '#lib/library/source.ts';
import { gridWindow } from './gridWindow.svelte';

function albums(count: number): AlbumEntry[] {
	return Array.from({ length: count }, (_, index) => ({
		id: `a${String(index)}`,
		title: 'T',
		artist: 'A',
		trackCount: 1,
		compilation: false
	}));
}

beforeEach(() => {
	vi.stubGlobal('innerHeight', 900);
	library.albums = albums(927);
	gridWindow.reset();
	// Five columns of 12 rows, 300 + 20 each: a full page and its gap are 3840 tall.
	gridWindow.measure = { columns: 5, rowHeight: 300, rowGap: 20 };
});

afterEach(() => {
	vi.unstubAllGlobals();
	library.albums = [];
	gridWindow.reset();
});

describe('gridWindow', () => {
	it('counts pages, the last one short', () => {
		expect(gridWindow.pageCount).toBe(16);
		expect(gridWindow.height(0)).toBe(3820);
		// 27 cards left: six rows.
		expect(gridWindow.height(15)).toBe(1900);
	});

	it('follows the viewport, and mounts only what it reaches until the landing is over', () => {
		gridWindow.track(-3840 * 5 - 1000, 0);
		expect([gridWindow.first, gridWindow.last]).toEqual([5, 5]);
		expect(gridWindow.mounted(4)).toBe(false);
		expect(gridWindow.mounted(5)).toBe(true);
		gridWindow.wide = true;
		expect(gridWindow.mounted(4)).toBe(true);
		expect(gridWindow.mounted(6)).toBe(true);
		expect(gridWindow.mounted(7)).toBe(false);
	});

	it('holds both pages at a seam', () => {
		gridWindow.track(-3840 * 5 + 200, 0);
		expect([gridWindow.first, gridWindow.last]).toEqual([4, 5]);
	});

	it('stays in bounds', () => {
		gridWindow.track(500, 0);
		expect([gridWindow.first, gridWindow.last]).toEqual([0, 0]);
		gridWindow.track(-1e7, 0);
		expect([gridWindow.first, gridWindow.last]).toEqual([15, 15]);
	});

	it('aims at the page holding an album', () => {
		gridWindow.aim('a130');
		expect([gridWindow.first, gridWindow.last]).toEqual([2, 2]);
		gridWindow.aim('nobody');
		expect([gridWindow.first, gridWindow.last]).toEqual([2, 2]);
	});
});
