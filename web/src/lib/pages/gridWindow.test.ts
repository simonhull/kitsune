import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AlbumEntry } from '#lib/library/source.ts';
import { clearLine, createGridWindow, type GridWindow } from './gridWindow.svelte';

function albums(count: number): AlbumEntry[] {
	return Array.from({ length: count }, (_, index) => ({
		id: `a${String(index)}`,
		title: 'T',
		artist: 'A',
		trackCount: 1,
		compilation: false
	}));
}

let entries: AlbumEntry[];
let nearEnd: () => void;
let gridWindow: GridWindow;

beforeEach(() => {
	vi.stubGlobal('innerHeight', 900);
	entries = albums(927);
	nearEnd = vi.fn<() => void>();
	gridWindow = createGridWindow(() => entries, nearEnd);
	// Five columns of 12 rows, 300 + 20 each: a full page and its gap are 3840 tall.
	gridWindow.measure = { columns: 5, rowHeight: 300, rowGap: 20 };
});

afterEach(() => {
	vi.unstubAllGlobals();
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

	it('counts whatever its entries are now', () => {
		entries = albums(61);
		expect(gridWindow.pageCount).toBe(2);
		expect(gridWindow.height(1)).toBe(300);
	});

	it('says when the end comes near, and only then', () => {
		gridWindow.track(0, 5000);
		expect(nearEnd).not.toHaveBeenCalled();
		gridWindow.track(0, 2000);
		expect(nearEnd).toHaveBeenCalledOnce();
	});

	it('keeps each grid to itself', () => {
		const other = createGridWindow(() => albums(1));
		other.first = 3;
		expect(gridWindow.first).toBe(0);
		expect(other.pageCount).toBe(1);
	});

	it('finds the row an entry is on, across pages', () => {
		expect(gridWindow.offsetOf(0)).toBe(0);
		expect(gridWindow.offsetOf(4)).toBe(0);
		expect(gridWindow.offsetOf(5)).toBe(320);
		expect(gridWindow.offsetOf(60)).toBe(3840);
		expect(gridWindow.offsetOf(66)).toBe(3840 + 320);
	});

	it('can’t place an entry until something is measured', () => {
		gridWindow.reset();
		expect(gridWindow.offsetOf(10)).toBeUndefined();
	});

	it('follows the row at the top of the screen', () => {
		gridWindow.track(500, 9999);
		expect(gridWindow.reached).toBe(0);
		// The clear line falls in the second row of the second page.
		gridWindow.track(clearLine - 3840 - 330, 9999);
		expect(gridWindow.reached).toBe(69);
	});

	it('takes a row a fraction above the clear line as the row at the top', () => {
		gridWindow.track(clearLine - 3840 - 320 - 0.5, 9999);
		expect(gridWindow.reached).toBe(69);
	});

	it('counts the end of the grid as reached, though its row can’t rise that far', () => {
		gridWindow.track(-1e5, 800);
		expect(gridWindow.reached).toBe(926);
	});

	it('rewinds without forgetting its measure', () => {
		gridWindow.track(-3840 * 5, 9999);
		gridWindow.rewind();
		expect([gridWindow.first, gridWindow.last, gridWindow.reached]).toEqual([0, 0, 0]);
		expect(gridWindow.measure).not.toBeNull();
	});

	it('rounds a page up to whole rows, so a wide screen never cuts one short at a seam', () => {
		gridWindow.measure = { columns: 7, rowHeight: 300, rowGap: 20 };
		expect(gridWindow.pageSize).toBe(63);
		expect(gridWindow.pageCount).toBe(15);
		// Nine whole rows a page.
		expect(gridWindow.height(0)).toBe(9 * 300 + 8 * 20);
		expect(gridWindow.offsetOf(63)).toBe(9 * 320);
		gridWindow.reset();
		expect(gridWindow.pageSize).toBe(60);
	});

	it('keeps the album it aims at mounted when a measure changes the page size', () => {
		gridWindow.reset();
		gridWindow.aim('a125');
		expect(gridWindow.mounted(2)).toBe(true);
		gridWindow.measure = { columns: 7, rowHeight: 300, rowGap: 20 };
		// 125 is on the second page of 63.
		expect(gridWindow.mounted(1)).toBe(true);
		gridWindow.track(-1e7, 0);
		expect(gridWindow.mounted(1)).toBe(false);
	});

	it('aims at the page holding an album', () => {
		gridWindow.aim('a130');
		expect([gridWindow.first, gridWindow.last]).toEqual([2, 2]);
		gridWindow.aim('nobody');
		expect([gridWindow.first, gridWindow.last]).toEqual([2, 2]);
	});
});
