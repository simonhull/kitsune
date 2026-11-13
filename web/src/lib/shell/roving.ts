/**
 * A toolbar that is one Tab stop: its items, each marked `data-roam`, take ← and → between them,
 * and Home and End to either end. Only the last one focused is in the Tab order, so Tab comes back
 * to it. A slider among them keeps its own arrows, Home and End, so its value moves as it always
 * did; from a slider, Tab and Shift+Tab go on to the item beside it, and only from the others does
 * Tab leave the toolbar.
 */

/** The items as they show now: one hidden at this width isn't one to land on. */
function itemsOf(node: HTMLElement): HTMLElement[] {
	return [...node.querySelectorAll<HTMLElement>('[data-roam]')].filter(
		(item) => item.getClientRects().length > 0
	);
}

/** Where focus goes from `at` on `key`, among `count` items, or null for a key that isn't ours. */
export function roamTo(
	key: string,
	at: number,
	count: number,
	{ slider, shift }: { slider: boolean; shift: boolean }
): number | null {
	if (slider) {
		if (key !== 'Tab') {
			return null;
		}
		const to = at + (shift ? -1 : 1);
		return to >= 0 && to < count ? to : null;
	}
	switch (key) {
		case 'ArrowLeft':
			return Math.max(0, at - 1);
		case 'ArrowRight':
			return Math.min(count - 1, at + 1);
		case 'Home':
			return 0;
		case 'End':
			return count - 1;
		default:
			return null;
	}
}

export function roving(node: HTMLElement): { destroy: () => void } {
	/** The item that holds the Tab stop. */
	let current: HTMLElement | null = null;

	function settle(): void {
		const items = [...node.querySelectorAll<HTMLElement>('[data-roam]')];
		const shown = itemsOf(node);
		if (current === null || !shown.includes(current)) {
			current = shown.at(0) ?? null;
		}
		for (const item of items) {
			item.tabIndex = item === current ? 0 : -1;
		}
	}

	function onfocusin(event: FocusEvent): void {
		const item = event.target instanceof HTMLElement ? event.target.closest('[data-roam]') : null;
		if (item instanceof HTMLElement && node.contains(item)) {
			current = item;
			settle();
		}
	}

	function onkeydown(event: KeyboardEvent): void {
		if (event.altKey || event.ctrlKey || event.metaKey || event.defaultPrevented) {
			return;
		}
		const items = itemsOf(node);
		const from = event.target instanceof HTMLElement ? event.target.closest('[data-roam]') : null;
		const at = from instanceof HTMLElement ? items.indexOf(from) : -1;
		if (at < 0) {
			return;
		}
		const slider = from?.getAttribute('role') === 'slider';
		if (!slider && event.shiftKey) {
			return;
		}
		const to = roamTo(event.key, at, items.length, { slider, shift: event.shiftKey });
		const target = to === null ? undefined : items.at(to);
		if (target === undefined) {
			return;
		}
		event.preventDefault();
		current = target;
		settle();
		target.focus();
	}

	// Items come and go with the toolbar's own children, and show or hide with the width: the Tab
	// stop stays on one that shows. Only the toolbar's children are watched, so a glyph changing
	// inside a button (Play to Pause) reads no layout.
	const watch = new MutationObserver(settle);
	watch.observe(node, { childList: true });
	let frame = 0;
	const onresize = (): void => {
		cancelAnimationFrame(frame);
		frame = requestAnimationFrame(settle);
	};
	// The island mounts on a first play: what shows is read a frame later, off that path.
	for (const [at, item] of [...node.querySelectorAll<HTMLElement>('[data-roam]')].entries()) {
		item.tabIndex = at === 0 ? 0 : -1;
	}
	frame = requestAnimationFrame(settle);
	node.addEventListener('focusin', onfocusin);
	node.addEventListener('keydown', onkeydown);
	addEventListener('resize', onresize);
	return {
		destroy() {
			watch.disconnect();
			cancelAnimationFrame(frame);
			node.removeEventListener('focusin', onfocusin);
			node.removeEventListener('keydown', onkeydown);
			removeEventListener('resize', onresize);
		}
	};
}
