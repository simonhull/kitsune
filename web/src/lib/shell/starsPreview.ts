/**
 * A rating slider's hover preview, written to the DOM and nowhere else: a pointer crossing a list
 * of songs changes one attribute on one element, never Svelte state, so no row renders again.
 * StarsInput's styles draw what `data-preview` says, in halves.
 */
import { halfAt } from '#lib/library/stars.ts';

/** How far across `el` a pointer at `clientX` is, 0 to 1 inside it. */
export function fractionAcross(el: Element, clientX: number): number {
	const { left, width } = el.getBoundingClientRect();
	return width === 0 ? 0 : (clientX - left) / width;
}

/** The half under a pointer at `clientX` on the slider `el`. */
export function halfUnder(el: Element, clientX: number): number {
	return halfAt(fractionAcross(el, clientX));
}

/** Shows on `el` the rating a click would set. */
export function showPreview(el: HTMLElement, stars: number): void {
	const halves = String(stars * 2);
	if (el.dataset['preview'] !== halves) {
		el.dataset['preview'] = halves;
	}
}

/** Takes the preview off `el`, so it shows its rating again. */
export function dropPreview(el: HTMLElement | null | undefined): void {
	if (el?.dataset['preview'] !== undefined) {
		delete el.dataset['preview'];
	}
}

/** The rating a slider holds, as its `aria-valuenow` says: null for none. */
export function valueOf(el: Element): number | null {
	const now = Number(el.getAttribute('aria-valuenow'));
	return Number.isFinite(now) && now > 0 ? now : null;
}
