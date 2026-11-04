/** Which place on a page a shared element occupies. */
export type MorphSlot = 'card' | 'hero' | 'island' | 'poster';

/** What a shared element is, which decides how it travels. */
export type MorphKind = 'cover' | 'title';

export interface MorphParams {
	/** Shared across pages: `cover:<albumId>` or `title:<albumId>`. */
	id: string;
	slot: MorphSlot;
	kind: MorphKind;
}

export interface MorphEntry extends MorphParams {
	element: HTMLElement;
}

type Listener = (entry: MorphEntry) => void;

/** Every shared element on screen. Registering is all a page ever does for a morph. */
class Registry {
	readonly #entries = new Set<MorphEntry>();
	readonly #listeners = new Set<Listener>();

	add(entry: MorphEntry): void {
		this.#entries.add(entry);
		for (const listener of this.#listeners) {
			listener(entry);
		}
	}

	remove(entry: MorphEntry): void {
		this.#entries.delete(entry);
	}

	/** Returns every live entry in document order. */
	list(): MorphEntry[] {
		return [...this.#entries].sort((a, b) =>
			a.element.compareDocumentPosition(b.element) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1
		);
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
	let entry: MorphEntry = { ...params, element };
	registry.add(entry);

	return {
		update(next: MorphParams) {
			registry.remove(entry);
			entry = { ...next, element };
			registry.add(entry);
		},
		destroy() {
			registry.remove(entry);
		}
	};
}
