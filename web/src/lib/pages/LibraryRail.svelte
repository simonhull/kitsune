<script lang="ts">
	import { untrack } from 'svelte';
	import { floorElements, floors } from '#lib/shell/floor.svelte.ts';
	import { railLit } from '#lib/library/libraryOrder.ts';
	import { fold, letterAt } from './railFit.ts';
	import { inkChanges, RailGoo, slotCenter, type GooGeometry, type InkRange } from './railGoo.ts';

	interface Props {
		letters: readonly string[];
		/** Letters with nothing shown under them: grey, and a jump to one goes on to the next. */
		quiet: ReadonlySet<string>;
		/** The letter you're at. */
		here: string | undefined;
		onjump: (letter: string) => void;
	}

	let { letters, quiet, here, onjump }: Props = $props();

	/** The space the rail keeps between its last letter and the island under it. */
	const clearance = 12;
	/** Folded, the pill keeps a line's height whatever the slot's. */
	const foldedPill = 18;
	/** Under reduced motion, the old pill fades out where it was over this many ms. */
	const ghostFade = 140;

	let rail: HTMLElement | undefined = $state();
	/** The height the rail has, from its top to the screen's foot or the highest island on it. */
	let room: number | undefined = $state();
	/** A finger on the rail: the letter under it, and how far down the rail it is. */
	let held: { letter: string; y: number } | undefined = $state();

	const folded = $derived(
		fold(letters.length, room === undefined ? Infinity : room / letters.length)
	);
	const shown = $derived(new Set(folded.shown));
	/** Lit is a letter the rail draws: the one you're at, or the drawn one before it. */
	const lit = $derived(
		here === undefined
			? undefined
			: railLit(
					folded.shown.map((index) => letters[index] ?? ''),
					here
				)
	);
	const litIndex = $derived(lit === undefined ? -1 : letters.indexOf(lit));

	function measure(): void {
		if (rail === undefined) {
			return;
		}
		const viewport = window.visualViewport;
		let bottom = viewport === null ? window.innerHeight : viewport.offsetTop + viewport.height;
		for (const element of floorElements) {
			const { top, height } = element.getBoundingClientRect();
			if (height > 0) {
				bottom = Math.min(bottom, top);
			}
		}
		room = Math.max(0, bottom - clearance - rail.getBoundingClientRect().top);
	}

	$effect(() => {
		// Read here so a new floor, or one that moved, measures again.
		void floors.moved;
		void floorElements.size;
		measure();
	});

	$effect(() => {
		const viewport = window.visualViewport;
		window.addEventListener('resize', measure);
		viewport?.addEventListener('resize', measure);
		return () => {
			window.removeEventListener('resize', measure);
			viewport?.removeEventListener('resize', measure);
		};
	});

	/* The goo (plan 054): the pill is its own element, moved by transforms from a frame loop. */

	const anchors: HTMLAnchorElement[] = [];
	let pill: HTMLElement | undefined = $state();
	let capTop: HTMLElement | undefined;
	let body: HTMLElement | undefined;
	let capBottom: HTMLElement | undefined;
	let drop: HTMLElement | undefined;
	let ghost: HTMLElement | undefined;
	/** The bubble's letter: null once the bubble has gone. */
	let glyph: HTMLElement | null | undefined = $state();

	let goo: RailGoo | undefined;
	let frame = 0;
	let last = 0;
	let ink: InkRange = { from: 0, to: -1 };
	let dropShowing = false;

	/** The rail's measures, read only when it changes size, never per frame. */
	function geometry(): GooGeometry | undefined {
		if (rail === undefined || letters.length === 0) {
			return undefined;
		}
		const step = rail.clientHeight / letters.length;
		const inset = Number.parseFloat(getComputedStyle(rail).getPropertyValue('--pill-inset')) || 0;
		const pill = folded.shown.length < letters.length ? foldedPill : step - 2 * inset;
		return { step, pill: Math.max(1, pill) };
	}

	function write(): void {
		if (goo === undefined) {
			return;
		}
		const { center, length, width, lean, squash } = goo.shape();
		const cap = goo.geometry.pill / 2;
		if (pill !== undefined) {
			pill.style.transform = `translateY(${center.toFixed(2)}px) scaleX(${width.toFixed(3)}) skewX(${lean.toFixed(2)}deg)`;
		}
		if (capTop !== undefined) {
			capTop.style.transform = `translateY(${(-length / 2).toFixed(2)}px) scaleY(${squash.toFixed(3)})`;
		}
		if (capBottom !== undefined) {
			capBottom.style.transform = `translateY(${(length / 2 - cap * squash).toFixed(2)}px) scaleY(${squash.toFixed(3)})`;
		}
		if (body !== undefined) {
			const tall = Math.max(0, length - 2 * cap + 1);
			body.style.transform = `translateY(${(-length / 2 + cap - 0.5).toFixed(2)}px) scaleY(${tall.toFixed(2)})`;
		}
		const shed = goo.dropShape();
		if (drop !== undefined && (shed !== undefined || dropShowing)) {
			dropShowing = shed !== undefined;
			drop.style.transform =
				shed === undefined
					? ''
					: `translateY(${(shed.center - 4).toFixed(2)}px) scale(${shed.scale.toFixed(3)})`;
			drop.style.opacity = shed === undefined ? '0' : String(Math.min(1, shed.scale * 1.5));
		}
		if (glyph) {
			glyph.style.transform = `scale(${goo.glyph.value.toFixed(3)})`;
		}
		paintInk(goo.inked());
	}

	/** Letters are ink while the pill covers their middle: only the ends of that run ever change. */
	function paintInk(next: InkRange): void {
		for (const [index, on] of inkChanges(ink, next, (index) => shown.has(index))) {
			anchors[index]?.toggleAttribute('data-ink', on);
		}
		ink = next;
	}

	function tick(now: number): void {
		if (goo === undefined) {
			frame = 0;
			return;
		}
		// rAF's timestamp is the frame's start, which can come before the `performance.now()` that
		// queued it: the goo clamps the step at 0, so a spring never runs backwards.
		goo.step((now - last) / 1000);
		last = now;
		if (goo.resting()) {
			goo.place(slotCenter(Math.max(0, litIndex), goo.geometry.step));
			frame = 0;
		} else {
			frame = requestAnimationFrame(tick);
		}
		write();
	}

	function start(): void {
		if (frame === 0 && goo !== undefined) {
			last = performance.now();
			frame = requestAnimationFrame(tick);
		}
	}

	/** Puts the pill on the lit letter without moving there: at first, and when the rail is resized. */
	function settle(): void {
		const measures = geometry();
		if (measures === undefined || litIndex < 0) {
			return;
		}
		cancelAnimationFrame(frame);
		frame = 0;
		const center = slotCenter(litIndex, measures.step);
		if (goo === undefined) {
			goo = new RailGoo(measures, center);
		} else {
			goo.place(center, measures);
		}
		for (const element of [capTop, capBottom]) {
			element?.style.setProperty('height', `${String(measures.pill / 2)}px`);
		}
		ghost?.style.setProperty('height', `${String(measures.pill)}px`);
		for (const anchor of anchors) {
			anchor.removeAttribute('data-ink');
		}
		ink = { from: 0, to: -1 };
		write();
	}

	function fadeGhost(center: number): void {
		if (ghost === undefined || goo === undefined) {
			return;
		}
		ghost.style.transform = `translateY(${(center - goo.geometry.pill / 2).toFixed(2)}px)`;
		ghost.animate([{ opacity: 1 }, { opacity: 0 }], { duration: ghostFade, easing: 'ease-out' });
	}

	// A resize, a fold or new letters move the slots: nothing about that animates.
	$effect(() => {
		if (rail === undefined) {
			return;
		}
		const observer = new ResizeObserver(() => {
			settle();
		});
		observer.observe(rail);
		return () => {
			observer.disconnect();
			cancelAnimationFrame(frame);
			frame = 0;
		};
	});

	$effect(() => {
		void letters;
		void folded;
		untrack(settle);
	});

	let shownIndex = -1;
	$effect(() => {
		const index = litIndex;
		untrack(() => {
			const from = shownIndex;
			shownIndex = index;
			if (index < 0) {
				paintInk({ from: 0, to: -1 });
				return;
			}
			if (goo === undefined) {
				return;
			}
			if (from < 0) {
				settle();
				return;
			}
			goo.reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
			const { ghost: old } = goo.retarget(slotCenter(index, goo.geometry.step), performance.now());
			if (old === undefined) {
				start();
			} else {
				fadeGhost(old);
				write();
			}
		});
	});

	// Under a finger the goo tracks without wobbling; on release the wobble plays once.
	$effect(() => {
		const holding = held !== undefined;
		untrack(() => {
			if (goo !== undefined) {
				goo.held = holding;
				start();
			}
		});
	});

	$effect(() => {
		if (held?.letter !== undefined && glyph) {
			untrack(() => {
				goo?.squish();
				start();
			});
		}
	});

	/** A finger slides down the rail, jumping as it goes, so a touch here must not scroll the page. */
	function slide({ clientY }: PointerEvent): void {
		if (rail === undefined) {
			return;
		}
		const { top, height } = rail.getBoundingClientRect();
		const y = clientY - top;
		const letter = letters.at(letterAt(y, height, letters.length));
		if (letter === undefined) {
			return;
		}
		const changed = held?.letter !== letter;
		held = { letter, y: Math.min(height, Math.max(0, y)) };
		if (changed) {
			onjump(letter);
		}
	}

	function onpointerdown(event: PointerEvent): void {
		if (event.pointerType !== 'mouse') {
			rail?.setPointerCapture(event.pointerId);
			held = undefined;
			slide(event);
		}
	}

	function onpointermove(event: PointerEvent): void {
		if (rail?.hasPointerCapture(event.pointerId) === true) {
			slide(event);
		}
	}

	function letGo(): void {
		held = undefined;
	}
</script>

<nav
	class="rail"
	aria-label="Jump to a letter"
	class:folding={folded.shown.length < letters.length}
	style:--letters={letters.length}
	style:--room={room === undefined ? undefined : `${String(room)}px`}
	bind:this={rail}
	{onpointerdown}
	{onpointermove}
	onpointerup={letGo}
	onpointercancel={letGo}
	onlostpointercapture={letGo}
>
	<span class="ghost" aria-hidden="true" bind:this={ghost}></span>
	<span class="drop" aria-hidden="true" bind:this={drop}></span>
	<span class="pill" class:gone={lit === undefined} aria-hidden="true" bind:this={pill}
		><i class="top" bind:this={capTop}></i><i class="body" bind:this={body}></i><i
			class="bottom"
			bind:this={capBottom}
		></i></span
	>
	{#each letters as letter, index (letter)}
		<a
			href={`#${letter === '#' ? 'other' : letter}`}
			class:quiet={quiet.has(letter)}
			class:folded={!shown.has(index)}
			aria-current={letter === lit ? 'true' : undefined}
			bind:this={anchors[index]}
			onclick={(event) => {
				event.preventDefault();
				onjump(letter);
			}}>{letter}</a
		>
	{/each}
	{#each folded.dots as dot (dot)}
		<span class="dot" aria-hidden="true" style:top={`calc(var(--step) * ${String(dot)})`}></span>
	{/each}
	{#if held !== undefined}
		<span class="bubble" aria-hidden="true" style:transform={`translateY(${String(held.y)}px)`}
			><span class="glyph" bind:this={glyph}>{held.letter}</span></span
		>
	{/if}
</nav>

<style>
	.rail {
		--room: calc(100dvh - 216px);
		--step: min(25px, calc(var(--room) / var(--letters)));
		/* The pill is 22 tall in a 25 step: it shrinks with the step in a short window. */
		--pill-inset: 1.5px;

		position: fixed;
		z-index: 1;
		top: 104px;
		right: 24px;
		width: 32px;
		display: flex;
		flex-direction: column;
		align-items: center;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	a {
		width: 28px;
		height: var(--step);
		flex: none;
		display: flex;
		align-items: center;
		justify-content: center;
		position: relative;
	}

	a:hover {
		color: var(--fox-text);
	}

	.quiet,
	.quiet:hover {
		color: color-mix(in srgb, var(--text-label) 45%, var(--ground));
	}

	/*
	 * Ink follows the goo, not aria-current: a letter is ink while the pill covers its middle. The
	 * frame loop sets `data-ink`, which the template doesn't know of.
	 */
	.rail :global(a[data-ink]),
	.rail :global(a[data-ink]:hover) {
		color: var(--fox-ink);
		font-weight: 700;
	}

	/*
	 * The pill: a wrapper at its centre, and two round ends with a body between, so the ends stay
	 * round as it stretches. Each is a small layer of its own for good, moved only by transforms.
	 */
	.pill,
	.ghost,
	.drop {
		position: absolute;
		top: 0;
		left: calc(50% - 14px);
		width: 28px;
		pointer-events: none;
	}

	.pill {
		height: 0;
		transform-origin: 14px 0;
		will-change: transform;
	}

	.pill.gone {
		visibility: hidden;
	}

	.pill > i {
		position: absolute;
		top: 0;
		left: 0;
		width: 28px;
		background: var(--fox);
		transform-origin: 0 0;
		will-change: transform;
	}

	.pill > .top {
		border-radius: var(--radius-pill) var(--radius-pill) 0 0;
	}

	.pill > .bottom {
		border-radius: 0 0 var(--radius-pill) var(--radius-pill);
	}

	.pill > .body {
		height: 1px;
	}

	/* Under reduced motion, the old pill fades where it was. */
	.ghost {
		border-radius: var(--radius-pill);
		background: var(--fox);
		opacity: 0;
		will-change: opacity;
	}

	/* The drop the pill sheds at the old letter on a calm hop. */
	.drop {
		left: calc(50% - 4px);
		width: 8px;
		height: 8px;
		border-radius: var(--radius-pill);
		background: var(--fox);
		opacity: 0;
		will-change: transform, opacity;
	}

	/* A letter the rail has no line for keeps its slot, so a finger or a click still finds it. */
	.folded {
		color: transparent;
	}

	.folded:hover {
		color: transparent;
	}

	/* Reached by the keyboard, it shows itself over the dots around it. */
	.folded:focus-visible {
		z-index: 1;
		color: var(--fox-text);
	}

	.folded:focus-visible::before {
		content: '';
		position: absolute;
		z-index: -1;
		inset: -4px 0;
		border-radius: var(--radius-pill);
		background: var(--ground);
	}

	.dot {
		position: absolute;
		left: 50%;
		width: 4px;
		height: 4px;
		border-radius: var(--radius-pill);
		background: currentcolor;
		transform: translate(-50%, -50%);
		pointer-events: none;
	}

	/* The letter under the finger, big beside it, where the thumb doesn't cover it. Moved by transform. */
	.bubble {
		position: absolute;
		top: -36px;
		right: calc(100% + 24px);
		width: 72px;
		height: 72px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		box-shadow: var(--shadow-float);
		font-family: var(--font-sans);
		font-size: 40px;
		font-weight: 800;
		line-height: 1;
		pointer-events: none;
		will-change: transform;
		animation: rise var(--morph-fast) var(--curve-settle);
	}

	.glyph {
		display: block;
		will-change: transform;
	}

	@keyframes rise {
		from {
			opacity: 0;
			translate: 12px 0;
			scale: 0.6;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.bubble {
			animation: none;
		}
	}

	@media (max-width: 1024px) {
		.rail {
			--room: calc(100dvh - 320px);
			--step: min(44px, calc(var(--room) / var(--letters)));
			--pill-inset: 0px;

			top: 216px;
			right: 2px;
			width: 36px;
			touch-action: none;
		}
	}
</style>
