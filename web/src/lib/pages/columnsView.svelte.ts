import { library } from '#lib/library/library.svelte.ts';
import type { PrefsStore } from '#lib/library/prefs.ts';
import type { ColumnsPref, IndexField } from '#lib/library/source.ts';
import type { ColumnId } from '#lib/library/source.ts';
import { defaultColumns, fieldsFor, resized, savedColumns } from './columns.ts';

/**
 * How long a resize or a run of Alt+arrow moves waits for the next before it's saved: one write for
 * a drag, or for a run of keys.
 */
export const columnsSaveMs = 400;
/** How long Reset offers to undo itself. */
export const undoResetMs = 6000;

/** Runs `then` once the next frame has painted, or in a moment where nothing paints. */
function afterPaint(then: () => void): void {
	if (typeof requestAnimationFrame === 'function') {
		requestAnimationFrame(() => {
			setTimeout(then, 0);
		});
	} else {
		setTimeout(then, 0);
	}
}

/** What the view needs of the library: its preferences, and word of another device's change. */
export interface ColumnsHome {
	prefs: Pick<PrefsStore, 'get' | 'put' | 'flush'>;
	watchPref(key: 'library.columns', heard: (value: ColumnsPref | null) => void): void;
}

/**
 * The person's song columns. They start as the defaults and become what the server has once it
 * answers; a change shows at once and saves quietly.
 */
export class ColumnsView {
	pref = $state.raw<ColumnsPref>(defaultColumns);
	/** Set once the server has answered, or failed to: before that the list waits, not to ask twice. */
	known = $state(false);
	/** What Reset replaced, while it can still be put back. */
	undoable = $state.raw<ColumnsPref | null>(null);
	readonly fields: IndexField[] = $derived(fieldsFor(this.pref.order));
	readonly #home: ColumnsHome;
	#asked: Promise<void> | null = null;
	#saving: ReturnType<typeof setTimeout> | undefined;
	#undoing: ReturnType<typeof setTimeout> | undefined;
	/** A column is being dragged: another device's change waits for the drop. */
	#holding = false;
	#held: { value: ColumnsPref | null } | null = null;

	constructor(home: ColumnsHome) {
		this.#home = home;
		// Columns chosen on another device follow here, as the theme does.
		home.watchPref('library.columns', (saved) => {
			if (this.#holding) {
				this.#held = { value: saved };
				return;
			}
			const next = savedColumns(saved);
			// Our own save comes back as an event: the same value redraws nothing.
			if (JSON.stringify(next) !== JSON.stringify(this.pref)) {
				this.pref = next;
			}
			this.known = true;
		});
	}

	/** Asks the server, once a session. */
	load(): Promise<void> {
		this.#asked ??= this.#home.prefs.get('library.columns').then((saved) => {
			// A change made while the answer was on its way is newer than it.
			if (!this.known) {
				this.pref = savedColumns(saved);
				this.known = true;
			}
		});
		return this.#asked;
	}

	set(next: ColumnsPref): void {
		clearTimeout(this.#saving);
		this.pref = next;
		this.known = true;
		this.#home.prefs.put('library.columns', next);
	}

	/**
	 * A new order: a drop saves at once, and a run of keys once it stops. The same order again (a
	 * drop in place) saves nothing. A drop's request goes once its frame has painted, so the work of
	 * sending it stays off the frame that shows the new order.
	 */
	reorder(next: ColumnsPref, { debounce = false }: { debounce?: boolean } = {}): void {
		if (next === this.pref) {
			return;
		}
		this.pref = next;
		this.known = true;
		clearTimeout(this.#saving);
		if (debounce) {
			this.#saving = setTimeout(() => {
				this.#home.prefs.put('library.columns', this.pref);
				void this.#home.prefs.flush();
			}, columnsSaveMs);
		} else {
			this.#home.prefs.put('library.columns', this.pref);
			afterPaint(() => void this.#home.prefs.flush());
		}
	}

	/** A column dragged or nudged to a width, at the base step: shown at once, saved once it settles. */
	resize(id: ColumnId, width: number): void {
		this.pref = resized(this.pref, id, width);
		this.known = true;
		clearTimeout(this.#saving);
		this.#saving = setTimeout(() => {
			this.#home.prefs.put('library.columns', this.pref);
		}, columnsSaveMs);
	}

	/** Holds another device's change while a column is under the pointer, and applies it after. */
	hold(holding: boolean): void {
		this.#holding = holding;
		const held = this.#held;
		if (!holding && held !== null) {
			this.#held = null;
			this.pref = savedColumns(held.value);
			this.known = true;
		}
	}

	/** Reset to defaults, from the menu: what it replaced can be put back for six seconds. */
	resetToDefaults(): void {
		const was = this.pref;
		this.set(defaultColumns);
		clearTimeout(this.#undoing);
		this.undoable = was === defaultColumns ? null : was;
		this.#undoing = setTimeout(() => {
			this.undoable = null;
		}, undoResetMs);
	}

	/** Puts back what Reset replaced: a new change, saved like any other. */
	undoReset(): void {
		const was = this.undoable;
		clearTimeout(this.#undoing);
		this.undoable = null;
		if (was !== null) {
			this.set(was);
		}
	}

	/** Signing out: back to the defaults, to ask again for the next person. */
	reset(): void {
		clearTimeout(this.#saving);
		clearTimeout(this.#undoing);
		this.pref = defaultColumns;
		this.known = false;
		this.undoable = null;
		this.#asked = null;
		this.#holding = false;
		this.#held = null;
	}
}

export const columnsView = new ColumnsView({
	prefs: library.prefs,
	watchPref: (key, heard) => {
		library.watchPref(key, heard);
	}
});
