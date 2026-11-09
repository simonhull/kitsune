import { library } from '#lib/library/library.svelte.ts';
import type { ColumnsPref, IndexField } from '#lib/library/source.ts';
import type { ColumnId } from '#lib/library/source.ts';
import { defaultColumns, fieldsFor, resized, savedColumns } from './columns.ts';

/** How long a resize waits for the next before it's saved: one write for a drag, or a run of keys. */
const resizeSaveMs = 400;

/**
 * The person's song columns. They start as the defaults and become what the server has once it
 * answers; a change shows at once and saves quietly.
 */
class ColumnsView {
	pref = $state.raw<ColumnsPref>(defaultColumns);
	/** Set once the server has answered, or failed to: before that the list waits, not to ask twice. */
	known = $state(false);
	readonly fields: IndexField[] = $derived(fieldsFor(this.pref.order));
	#asked: Promise<void> | null = null;
	#saving: ReturnType<typeof setTimeout> | undefined;

	constructor() {
		// Columns chosen on another device follow here, as the theme does.
		library.watchPref('library.columns', (saved) => {
			this.pref = savedColumns(saved);
			this.known = true;
		});
	}

	/** Asks the server, once a session. */
	load(): Promise<void> {
		this.#asked ??= library.prefs.get('library.columns').then((saved) => {
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
		library.prefs.put('library.columns', next);
	}

	/** A column dragged or nudged to a width, at the base step: shown at once, saved once it settles. */
	resize(id: ColumnId, width: number): void {
		this.pref = resized(this.pref, id, width);
		this.known = true;
		clearTimeout(this.#saving);
		this.#saving = setTimeout(() => {
			library.prefs.put('library.columns', this.pref);
		}, resizeSaveMs);
	}

	reset(): void {
		clearTimeout(this.#saving);
		this.pref = defaultColumns;
		this.known = false;
		this.#asked = null;
	}
}

export const columnsView = new ColumnsView();
