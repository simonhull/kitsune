/** What the dev HUD shows about the last morph. */
export const hud = $state<{ startMs: number | null }>({ startMs: null });

let clickedAt = 0;

/** Marks the input that started a navigation. */
export function markClick(at: number): void {
	clickedAt = at;
}

/** Records that a morph's first frame is on its way. */
export function reportStart(): void {
	if (clickedAt !== 0) {
		hud.startMs = performance.now() - clickedAt;
		clickedAt = 0;
	}
}
