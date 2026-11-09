import { Channel } from '#lib/morph/channel.ts';
import { springParams } from '#lib/morph/choreography.ts';

/** The edge heading somewhere gets there first, so the pill stretches and then catches up. */
const leading = springParams({ duration: 260, bounce: 0.04 });
const trailing = springParams({ duration: 460, bounce: 0 });

/**
 * Keeps the nav's pill under its current item, flowing there when that changes. With no current
 * item it goes away where it stands, and comes back at the next one: nothing flows out of nowhere.
 */
export function flowingPill(nav: HTMLElement, item: string | null) {
	const pill = nav.querySelector<HTMLElement>('.pill');
	const left = new Channel(0);
	const right = new Channel(0);
	let shown = item;
	let frame = 0;
	let last = 0;

	function write(): void {
		if (pill !== null) {
			pill.style.transform = `translateX(${left.value.toFixed(2)}px)`;
			pill.style.width = `${(right.value - left.value).toFixed(2)}px`;
		}
	}

	function current(): { from: number; to: number } | null {
		const item = nav.querySelector<HTMLElement>('[aria-current="page"]');
		return item === null ? null : { from: item.offsetLeft, to: item.offsetLeft + item.offsetWidth };
	}

	function show(visible: boolean): void {
		if (pill !== null) {
			pill.style.opacity = visible ? '1' : '0';
		}
	}

	function settle(): void {
		const place = current();
		if (place !== null) {
			left.value = left.target = place.from;
			right.value = right.target = place.to;
			left.velocity = right.velocity = 0;
			write();
		}
	}

	function tick(now: number): void {
		// rAF's timestamp is the frame's start, which can come before the `performance.now()` that
		// queued it: a negative step would run the springs backwards for a frame.
		const seconds = Math.min(Math.max(0, now - last) / 1000, 0.05);
		last = now;
		const movingRight = right.target >= right.value;
		left.step(movingRight ? trailing : leading, seconds);
		right.step(movingRight ? leading : trailing, seconds);
		write();
		frame = left.settled(0.3) && right.settled(0.3) ? 0 : requestAnimationFrame(tick);
	}

	function flow(from: string | null): void {
		const place = current();
		show(place !== null);
		if (place === null) {
			return;
		}
		if (from === null || matchMedia('(prefers-reduced-motion: reduce)').matches) {
			settle();
			return;
		}
		left.target = place.from;
		right.target = place.to;
		if (frame === 0) {
			last = performance.now();
			frame = requestAnimationFrame(tick);
		}
	}

	// A resize or the font arriving moves the items: nothing about that should animate.
	const observer = new ResizeObserver(() => {
		if (frame === 0) {
			settle();
		}
	});
	observer.observe(nav);
	settle();
	show(item !== null);

	return {
		update(next: string | null) {
			if (next !== shown) {
				const from = shown;
				shown = next;
				// `aria-current` changes in the same flush as this: wait for it.
				queueMicrotask(() => {
					flow(from);
				});
			}
		},
		destroy() {
			observer.disconnect();
			cancelAnimationFrame(frame);
		}
	};
}
