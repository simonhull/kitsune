const waiting = new Set<() => void>();
let moving = false;
let holds = 0;

function runWaiting(): void {
	if (moving || holds > 0) {
		return;
	}
	const due = [...waiting];
	waiting.clear();
	for (const run of due) {
		run();
	}
}

/** Told by the engine whenever a morph starts and when the last thing has come to rest. */
export function markMoving(now: boolean): void {
	moving = now;
	runWaiting();
}

/**
 * Counts as moving until released, however the engine comes and goes meanwhile: for something
 * longer than a morph, which holds the page still through one, or through none if it's cut short.
 */
export function holdSettled(): () => void {
	holds += 1;
	let held = true;
	return () => {
		if (held) {
			held = false;
			holds -= 1;
			runWaiting();
		}
	};
}

/**
 * Runs `work` once nothing is in flight, now if nothing is. Whatever changes the shell must wait
 * here: a morph reads the page it leaves, so a swap in mid-air tears. Returns the cancel.
 */
export function whenSettled(work: () => void): () => void {
	if (!moving && holds === 0) {
		work();
		return () => undefined;
	}
	waiting.add(work);
	return () => waiting.delete(work);
}
