<script lang="ts">
	import { floorElements, floors } from '#lib/shell/floor.svelte.ts';
	import { railHere } from './libraryRows.ts';
	import { fold, letterAt } from './railFit.ts';

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
			: railHere(
					folded.shown.map((index) => letters[index] ?? ''),
					here
				)
	);

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
	{#each letters as letter, index (letter)}
		<a
			href={`#${letter === '#' ? 'other' : letter}`}
			class:quiet={quiet.has(letter)}
			class:folded={!shown.has(index)}
			aria-current={letter === lit ? 'true' : undefined}
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
		<span class="bubble" aria-hidden="true" style:top={`${String(held.y)}px`}>{held.letter}</span>
	{/if}
</nav>

<style>
	.rail {
		--room: calc(100dvh - 216px);
		--step: min(25px, calc(var(--room) / var(--letters)));

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

	/* The board's pill, 22 tall in a 25 step: the step shrinks in a short window, and it shrinks too. */
	a::before {
		content: '';
		position: absolute;
		inset: 1.5px 0;
		border-radius: var(--radius-pill);
	}

	a:hover {
		color: var(--fox-text);
	}

	.quiet,
	.quiet:hover {
		color: color-mix(in srgb, var(--text-label) 45%, var(--ground));
	}

	[aria-current='true'] {
		color: var(--fox-ink);
		font-weight: 700;
	}

	[aria-current='true']:hover {
		color: var(--fox-ink);
	}

	[aria-current='true']::before {
		z-index: -1;
		background: var(--fox);
	}

	/* Folded, a slot is shorter than a line: the lit letter's pill keeps one, centred on its slot. */
	.folding [aria-current='true']::before {
		inset: calc((var(--step) - 18px) / 2) 0;
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
		z-index: -1;
		inset: -4px 0;
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

	/* The letter under the finger, big beside it, where the thumb doesn't cover it. */
	.bubble {
		position: absolute;
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
		transform: translateY(-50%);
		pointer-events: none;
		animation: rise var(--morph-fast) var(--curve-settle);
	}

	@keyframes rise {
		from {
			opacity: 0;
			transform: translate(12px, -50%) scale(0.6);
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

			top: 216px;
			right: 2px;
			width: 36px;
			touch-action: none;
		}

		a::before {
			inset: 0;
		}
	}
</style>
