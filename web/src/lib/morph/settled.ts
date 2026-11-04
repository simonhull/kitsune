const waiting = new Set<() => void>();
let moving = false;

/** Told by the engine whenever a morph starts and when the last thing has come to rest. */
export function markMoving(now: boolean): void {
	moving = now;
	if (!now) {
		const due = [...waiting];
		waiting.clear();
		for (const run of due) {
			run();
		}
	}
}

/**
 * Runs `work` once nothing is in flight, now if nothing is. Whatever changes the shell must wait
 * here: a morph reads the page it leaves, so a swap in mid-air tears. Returns the cancel.
 */
export function whenSettled(work: () => void): () => void {
	if (!moving) {
		work();
		return () => undefined;
	}
	waiting.add(work);
	return () => waiting.delete(work);
}
