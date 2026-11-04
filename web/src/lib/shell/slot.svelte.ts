interface SlotRect {
	left: number;
	top: number;
	width: number;
}

/** Where Now playing wants the island to sit, in viewport coordinates. */
export const islandSlot = $state<{ rect: SlotRect | null }>({ rect: null });

/** Marks the element the island should take the place of. */
export function reserveIslandSlot(element: HTMLElement): { destroy: () => void } {
	function measure(): void {
		const { left, top, width } = element.getBoundingClientRect();
		islandSlot.rect = { left, top, width };
	}

	const observer = new ResizeObserver(measure);
	observer.observe(element);
	observer.observe(document.documentElement);
	measure();

	return {
		destroy() {
			observer.disconnect();
			islandSlot.rect = null;
		}
	};
}
