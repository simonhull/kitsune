/**
 * What the suite puts in the page before the app loads. It never changes what the app does: it
 * notes when input arrived, when each `<audio>` element starts playing, and when a frame showing
 * what a test waits for has been painted, all on the page's own clock (`performance.now()`).
 *
 * "Painted" is the end of the first frame whose rendering includes the change: the predicate is
 * checked in every `requestAnimationFrame` callback, and once it holds, a message posted from that
 * callback is delivered after the frame's style, layout and paint, which is when the time is taken.
 */

/** The probe's face in the page, as `window.perf`. */
export interface Probe {
	/** Starts waiting for `what`, before the input that should bring it. */
	arm(what: Wait): void;
	/**
	 * Resolves with the ms from the latest `input` event (its `timeStamp`, when it reached the page)
	 * to what was armed, or rejects after `timeoutMs`.
	 */
	settle(input: InputType, timeoutMs: number): Promise<number>;
	/** Resolves with the length of every frame, in ms, over the next `ms`. */
	frames(ms: number): Promise<number[]>;
	/** Moves the song sounding to `seconds` before its end, as if it had played that far. */
	toEnd(seconds: number): void;
}

/**
 * What a measurement can start at: an input event, the start of the page load, or the latest end
 * of a song (an `<audio>` element's `ended`), for what plays next without anyone asking.
 */
export type InputType = 'click' | 'contextmenu' | 'keydown' | 'pointerup' | 'navigation' | 'ended';

/** Where a test leaves a {@link Wait} for the next page load to arm at its very start. */
export const armOnLoadKey = 'perf:arm';

/**
 * What a test can wait for. Each but `playing` is a check the probe runs once a frame, and with a
 * `path`, holds only on that page.
 */
export type Wait =
	| { kind: 'selector'; selector: string; path?: string }
	/** An element is there and an `img` in it has decoded, as the cover on an album page. */
	| { kind: 'loaded'; selector: string; image: string; path?: string }
	| { kind: 'attribute'; selector: string; name: string; value: string; path?: string }
	/**
	 * A list's columns are in `order`: its header's cells (under `selector`), and the first and the
	 * last of its rows on screen.
	 */
	| { kind: 'columns'; selector: string; order: string[]; path?: string }
	/** An `<audio>` element whose source has `src` in it fires `playing`: sound is coming out. */
	| { kind: 'playing'; src: string };

declare global {
	interface Window {
		perf: Probe;
	}
}

/** Runs in the page, so it must stand alone: no imports, nothing from the module around it. */
export function installProbe(): void {
	// The default keeps 250 entries, and a long run fetches more than that.
	performance.setResourceTimingBufferSize(100_000);
	const input: Record<string, number> = {};
	for (const type of ['click', 'contextmenu', 'keydown', 'pointerup']) {
		addEventListener(
			type,
			(event) => {
				input[type] = event.timeStamp;
			},
			{ capture: true }
		);
	}

	const waiting = new Map<string, ((at: number) => void)[]>();
	const elements: HTMLAudioElement[] = [];
	const NativeAudio = window.Audio;
	function ProbedAudio(src?: string): HTMLAudioElement {
		const element = new NativeAudio(src);
		elements.push(element);
		element.addEventListener('ended', (event) => {
			input['ended'] = event.timeStamp;
		});
		element.addEventListener('playing', () => {
			const at = performance.now();
			for (const [want, resolvers] of waiting) {
				if (element.src.includes(want)) {
					waiting.delete(want);
					for (const resolve of resolvers) {
						resolve(at);
					}
				}
			}
		});
		return element;
	}
	ProbedAudio.prototype = NativeAudio.prototype;
	window.Audio = ProbedAudio as unknown as typeof Audio;

	function holds(what: Exclude<Wait, { kind: 'playing' }>): boolean {
		const element = document.querySelector(what.selector);
		if (element === null || (what.path !== undefined && location.pathname !== what.path)) {
			return false;
		}
		switch (what.kind) {
			case 'selector':
				return true;
			case 'loaded': {
				const images = [...element.querySelectorAll<HTMLImageElement>(what.image)];
				return images.some((img) => img.complete && img.naturalWidth > 0);
			}
			case 'attribute':
				return element.getAttribute(what.name) === what.value;
			case 'columns': {
				const want = what.order.join(' ');
				const said = (line: Element): string =>
					[...line.querySelectorAll(':scope > [data-col]')]
						.map((cell) => cell.getAttribute('data-col'))
						.join(' ');
				const rows = [...document.querySelectorAll('.row[data-track]')].filter((row) => {
					const { top, bottom } = row.getBoundingClientRect();
					return bottom > 0 && top < innerHeight;
				});
				const first = rows.at(0);
				const last = rows.at(-1);
				return (
					said(element) === want &&
					first !== undefined &&
					last !== undefined &&
					said(first) === want &&
					said(last) === want
				);
			}
		}
	}

	function afterPaint(): Promise<number> {
		return new Promise((resolve) => {
			const channel = new MessageChannel();
			channel.port1.onmessage = () => {
				resolve(performance.now());
			};
			channel.port2.postMessage(null);
		});
	}

	function painted(what: Exclude<Wait, { kind: 'playing' }>): Promise<number> {
		return new Promise((resolve) => {
			const frame = () => {
				if (holds(what)) {
					void afterPaint().then(resolve);
				} else {
					requestAnimationFrame(frame);
				}
			};
			requestAnimationFrame(frame);
		});
	}

	function playing(src: string): Promise<number> {
		return new Promise((resolve) => {
			waiting.set(src, [...(waiting.get(src) ?? []), resolve]);
		});
	}

	let armed: Promise<number> | null = null;
	window.perf = {
		arm(what) {
			armed = what.kind === 'playing' ? playing(what.src) : painted(what);
		},
		async settle(type, timeoutMs) {
			const wanted = armed;
			armed = null;
			if (wanted === null) {
				throw new Error('nothing armed');
			}
			let timer: ReturnType<typeof setTimeout> | undefined;
			const timeout = new Promise<never>((_, reject) => {
				timer = setTimeout(() => {
					reject(new Error(`not there after ${String(timeoutMs)} ms`));
				}, timeoutMs);
			});
			const at = await Promise.race([wanted, timeout]);
			clearTimeout(timer);
			const from = type === 'navigation' ? 0 : input[type];
			if (from === undefined) {
				throw new Error(`no ${type} event seen`);
			}
			return at - from;
		},
		frames(ms) {
			return new Promise((resolve) => {
				const gaps: number[] = [];
				let last = 0;
				let end = 0;
				const frame = (now: number) => {
					if (last === 0) {
						end = now + ms;
					} else {
						gaps.push(now - last);
					}
					last = now;
					if (now < end) {
						requestAnimationFrame(frame);
					} else {
						resolve(gaps);
					}
				};
				requestAnimationFrame(frame);
			});
		},
		toEnd(seconds) {
			const element = elements.find((each) => !each.paused);
			if (element === undefined) {
				throw new Error('nothing is sounding');
			}
			element.currentTime = Math.max(0, element.duration - seconds);
		}
	};

	// The key is spelled out: this function runs in the page, without the module's constants.
	const onLoad = sessionStorage.getItem('perf:arm');
	if (onLoad !== null) {
		sessionStorage.removeItem('perf:arm');
		window.perf.arm(JSON.parse(onLoad) as Wait);
	}
}
