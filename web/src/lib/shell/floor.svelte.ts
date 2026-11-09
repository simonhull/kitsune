import { SvelteSet } from 'svelte/reactivity';

/**
 * What floats at the foot of the screen, the player island and the selection bar: anything fixed
 * that runs down the screen, like the Library's rail, stops above the highest of them. `moved`
 * counts their changes of size and the ends of their transitions, so a reader measures again.
 */
export const floors = $state<{ moved: number }>({ moved: 0 });
export const floorElements = new SvelteSet<HTMLElement>();

/** Marks an element as a floor while `on` (an island sunk out of the way is not one). */
export function floor(
	element: HTMLElement,
	on = true
): { update: (on?: boolean) => void; destroy: () => void } {
	function moved(): void {
		floors.moved++;
	}

	const observer = new ResizeObserver(moved);
	observer.observe(element);
	element.addEventListener('transitionend', moved);

	function set(value: boolean): void {
		if (value) {
			floorElements.add(element);
		} else {
			floorElements.delete(element);
		}
	}
	set(on);

	return {
		update(value = true) {
			set(value);
		},
		destroy() {
			observer.disconnect();
			element.removeEventListener('transitionend', moved);
			floorElements.delete(element);
		}
	};
}
