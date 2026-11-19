import { tick, untrack } from 'svelte';

/** A surface that can become its next face in place. */
interface Reshapes {
	hold(): void;
	reshape(): void;
}

/**
 * Reshapes `surface` in place whenever `face` changes while `which` stays the same: Undo's toast
 * becoming what it did. A new `which` is a new surface, born on its own, so nothing is held for it.
 * Call while a component is set up.
 */
export function reshapeOn(surface: Reshapes, which: () => unknown, face: () => string): void {
	let shown = untrack(() => ({ which: which(), face: face() }));
	$effect.pre(() => {
		const next = { which: which(), face: face() };
		const was = shown;
		shown = next;
		if (next.which !== was.which || next.face === was.face) {
			return;
		}
		untrack(() => {
			surface.hold();
		});
		void tick().then(() => {
			surface.reshape();
		});
	});
}
