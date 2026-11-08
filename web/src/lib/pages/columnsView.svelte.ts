import { library } from '#lib/library/library.svelte.ts';
import type { ColumnsPref, IndexField } from '#lib/library/source.ts';
import { defaultColumns, fieldsFor, savedColumns } from './columns.ts';

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
		this.pref = next;
		this.known = true;
		library.prefs.put('library.columns', next);
	}

	reset(): void {
		this.pref = defaultColumns;
		this.known = false;
		this.#asked = null;
	}
}

export const columnsView = new ColumnsView();
