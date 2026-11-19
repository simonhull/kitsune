/**
 * Which place on a page a shared element occupies: a room's `card` or `row`, a record's `hero`, a
 * song's 40 px `thumb` in a list, and the shell's own places.
 */
export type MorphSlot =
	'card' | 'row' | 'thumb' | 'deck' | 'hero' | 'island' | 'poster' | 'door' | 'nav';

/**
 * What a shared element is, which decides how it travels. A `surface` changes its true size and
 * radius: the album's stage becoming the Continue card. A `field` does too, but only its look goes
 * with it, never what's in it: the nav's search field becoming the Search page's. A `strip` is
 * the page's own on both sides and never flies: the Server tools' tabs, whose fill flows.
 */
export type MorphKind = 'cover' | 'title' | 'control' | 'bar' | 'surface' | 'field' | 'strip';

export interface MorphParams {
	/** Shared across pages: `cover:<albumId>`, `title:<albumId>`, `song`, `control:pause`… */
	id: string;
	slot: MorphSlot;
	kind: MorphKind;
	/**
	 * Other names for the same element: the album's artist line is also that artist's name
	 * (`name:<artistId>`), which their page's headline answers to.
	 */
	also?: readonly string[];
}

export interface MorphEntry {
	id: string;
	slot: MorphSlot;
	kind: MorphKind;
	element: HTMLElement;
}

/** Every entry an element registers: one for its id, and one for each other name it has. */
function entriesOf(params: MorphParams, element: HTMLElement): MorphEntry[] {
	const { slot, kind } = params;
	return [params.id, ...(params.also ?? [])].map((id) => ({ id, slot, kind, element }));
}

type Listener = (entry: MorphEntry) => void;

function inDocumentOrder(a: MorphEntry, b: MorphEntry): number {
	return a.element.compareDocumentPosition(b.element) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1;
}

/** Every shared element on screen. Registering is all a page ever does for a morph. */
class Registry {
	/** By id: a long list registers thousands, and a journey only ever wants a few ids. */
	readonly #byId = new Map<string, Set<MorphEntry>>();
	readonly #listeners = new Set<Listener>();

	add(entry: MorphEntry): void {
		const known = this.#byId.get(entry.id);
		if (known === undefined) {
			this.#byId.set(entry.id, new Set([entry]));
		} else {
			known.add(entry);
		}
		for (const listener of this.#listeners) {
			listener(entry);
		}
	}

	remove(entry: MorphEntry): void {
		const known = this.#byId.get(entry.id);
		known?.delete(entry);
		if (known?.size === 0) {
			this.#byId.delete(entry.id);
		}
	}

	/** Returns every live entry in document order. */
	list(): MorphEntry[] {
		return [...this.#byId.values()].flatMap((entries) => [...entries]).sort(inDocumentOrder);
	}

	/** Returns the live entries with these ids, in document order. */
	of(ids: Iterable<string>): MorphEntry[] {
		return [...new Set(ids)].flatMap((id) => [...(this.#byId.get(id) ?? [])]).sort(inDocumentOrder);
	}

	/** Calls `listener` whenever an element mounts. Returns the unsubscribe function. */
	subscribe(listener: Listener): () => void {
		this.#listeners.add(listener);
		return () => this.#listeners.delete(listener);
	}
}

export const registry = new Registry();

/**
 * Registers an element as shared. It never touches the element's styles: pages know nothing
 * about the engine.
 */
export function morph(element: HTMLElement, params: MorphParams) {
	let entries = entriesOf(params, element);
	for (const entry of entries) {
		registry.add(entry);
	}

	return {
		update(next: MorphParams) {
			for (const entry of entries) {
				registry.remove(entry);
			}
			entries = entriesOf(next, element);
			for (const entry of entries) {
				registry.add(entry);
			}
		},
		destroy() {
			for (const entry of entries) {
				registry.remove(entry);
			}
		}
	};
}
