import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ColumnsPref } from '#lib/library/source.ts';
import { defaultColumns } from './columns.ts';
import { ColumnsView, columnsSaveMs, undoResetMs, type ColumnsHome } from './columnsView.svelte.ts';

/** A home that counts what is sent, and lets a test speak for another device. */
function home(): ColumnsHome & {
	sent: ColumnsPref[];
	remote: (value: ColumnsPref) => void;
} {
	const sent: ColumnsPref[] = [];
	let heard: (value: ColumnsPref | null) => void = () => undefined;
	let waiting: ColumnsPref | null = null;
	return {
		sent,
		remote: (value) => {
			heard(value);
		},
		prefs: {
			get: () => Promise.resolve(null) as never,
			put: (_key, value) => {
				waiting = value as ColumnsPref;
			},
			flush: () => {
				if (waiting !== null) {
					sent.push(waiting);
					waiting = null;
				}
				return Promise.resolve();
			}
		},
		watchPref: (_key, listener) => {
			heard = listener;
		}
	};
}

const moved: ColumnsPref = {
	...defaultColumns,
	order: ['number', 'title', 'genre', 'artist', 'rating', 'plays', 'lastPlayed', 'format']
};
const movedAgain: ColumnsPref = {
	...defaultColumns,
	order: ['number', 'title', 'genre', 'rating', 'artist', 'plays', 'lastPlayed', 'format']
};

describe('ColumnsView', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it('saves a drop at once, once', () => {
		const at = home();
		const view = new ColumnsView(at);
		view.reorder(moved);
		expect(at.sent).toEqual([moved]);
		expect(view.pref).toBe(moved);
	});

	it('saves nothing for a drop in place', () => {
		const at = home();
		const view = new ColumnsView(at);
		view.reorder(view.pref);
		expect(at.sent).toEqual([]);
	});

	it('saves a run of keys once, after the last', async () => {
		const at = home();
		const view = new ColumnsView(at);
		view.reorder(moved, { debounce: true });
		await vi.advanceTimersByTimeAsync(columnsSaveMs - 1);
		view.reorder(movedAgain, { debounce: true });
		await vi.advanceTimersByTimeAsync(columnsSaveMs - 1);
		expect(at.sent).toEqual([]);
		await vi.advanceTimersByTimeAsync(1);
		expect(at.sent).toEqual([movedAgain]);
	});

	it('holds another device’s change during a drag, and applies it after', () => {
		const at = home();
		const view = new ColumnsView(at);
		view.hold(true);
		at.remote(moved);
		expect(view.pref).toBe(defaultColumns);
		view.hold(false);
		expect(view.pref.order).toEqual(moved.order);
	});

	it('offers to undo a reset for six seconds, as a new change', async () => {
		const at = home();
		const view = new ColumnsView(at);
		view.reorder(moved);
		view.resetToDefaults();
		expect(view.pref).toBe(defaultColumns);
		expect(view.undoable).toBe(moved);
		view.undoReset();
		expect(view.pref).toBe(moved);
		expect(view.undoable).toBeNull();

		view.resetToDefaults();
		await vi.advanceTimersByTimeAsync(undoResetMs);
		expect(view.undoable).toBeNull();
	});

	it('has nothing to undo when the reset changed nothing', () => {
		const view = new ColumnsView(home());
		view.resetToDefaults();
		expect(view.undoable).toBeNull();
	});
});
