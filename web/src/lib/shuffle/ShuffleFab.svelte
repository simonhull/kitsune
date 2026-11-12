<!--
	The Library's Shuffle, as the Shuffle FAB board has it: a fox disc in the corner, on the island's
	line, that stamps in, sinks under a press and plays on the click. What it shuffles and by what
	are on a card that opens on hover, keyboard focus or a long press; a click or a tap always plays.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import { cubicOut } from 'svelte/easing';
	import { fade } from 'svelte/transition';
	import { resolve } from '$app/paths';
	import { heartPath } from '#lib/shell/Icon.svelte';
	import Icon from '#lib/shell/Icon.svelte';
	import type { LibraryFilter } from '#lib/pages/libraryRows.ts';
	import { shuffles, type ShuffleSource } from './session.svelte.ts';
	import { shuffleSettings } from './settings.svelte.ts';
	import { fabWords, settingsLine, songsLine } from './words.ts';

	interface Props {
		/** What a press shuffles. */
		source: ShuffleSource;
		/** The filter it stands for: its tag and its words. */
		filter: LibraryFilter;
		/** How many songs that is, for the card. */
		count: number;
		/** Tucked away: the selection bar's Shuffle is the one on screen, or the tag editor's layer is. */
		away?: boolean;
	}

	let { source, filter, count, away = false }: Props = $props();

	/** Hover waits this long before the card opens, so passing over never flashes it. */
	const openAfterMs = 150;
	/** And this long after the pointer leaves both before it goes. */
	const closeAfterMs = 200;
	/** A touch held this long opens the card instead of shuffling. */
	const longPressMs = 450;

	const words = $derived(fabWords(filter));
	/** The You page's shuffle settings. */
	const settingsHref = resolve('/you#shuffle');
	const id = $props.id();

	let open = $state(false);
	/** The disc is pressed down: set on the frame of the press, with no transition. */
	let down = $state(false);
	/** Counts presses, so each one's flourish starts afresh. */
	let fired = $state(0);
	/** Not drawn yet: mounted tucked away, it waits for its first arrival rather than tucking. */
	let shown = $state(false);
	$effect(() => {
		if (!away) {
			shown = true;
		} else {
			open = false;
		}
	});

	let openTimer: ReturnType<typeof setTimeout> | undefined;
	let closeTimer: ReturnType<typeof setTimeout> | undefined;
	let longTimer: ReturnType<typeof setTimeout> | undefined;
	/** A long press opened the card: the click it ends in doesn't shuffle. */
	let longPressed = false;

	let button: HTMLButtonElement | undefined = $state();
	let card: HTMLElement | undefined = $state();

	function clearTimers(): void {
		clearTimeout(openTimer);
		clearTimeout(closeTimer);
		clearTimeout(longTimer);
	}
	onDestroy(clearTimers);

	function prime(): void {
		void shuffles.prime(source);
	}

	function show(): void {
		clearTimeout(closeTimer);
		if (!away) {
			open = true;
		}
	}

	function hide(now: boolean): void {
		clearTimeout(openTimer);
		clearTimeout(closeTimer);
		if (now) {
			open = false;
		} else {
			closeTimer = setTimeout(() => {
				open = false;
			}, closeAfterMs);
		}
	}

	/** Hover over the disc or the card: the card opens after a moment, and stays while over either. */
	function enter(event: PointerEvent): void {
		if (event.pointerType === 'touch') {
			return;
		}
		clearTimeout(closeTimer);
		if (!open) {
			clearTimeout(openTimer);
			openTimer = setTimeout(show, openAfterMs);
		}
	}

	function leave(event: PointerEvent): void {
		if (event.pointerType === 'touch') {
			return;
		}
		hide(false);
	}

	function press(event: PointerEvent): void {
		if (event.button !== 0) {
			return;
		}
		down = true;
		longPressed = false;
		if (event.pointerType === 'touch') {
			prime();
			clearTimeout(longTimer);
			longTimer = setTimeout(() => {
				longPressed = true;
				down = false;
				show();
			}, longPressMs);
		}
	}

	function release(): void {
		clearTimeout(longTimer);
		down = false;
	}

	/** The song starts here, from what hovering primed; everything after it is off the play path. */
	function shuffle(event: MouseEvent): void {
		if (longPressed) {
			longPressed = false;
			event.preventDefault();
			return;
		}
		shuffles.start(source);
		void shuffles.prime(source);
		fired++;
		hide(true);
	}

	function focused(): void {
		prime();
		if (button?.matches(':focus-visible') === true) {
			show();
		}
	}

	/** Focus leaving for the card keeps it; leaving both closes it. */
	function blurred(event: FocusEvent): void {
		const to = event.relatedTarget;
		if (to instanceof Node && (button?.contains(to) === true || card?.contains(to) === true)) {
			return;
		}
		hide(false);
	}

	function onKey(event: KeyboardEvent): void {
		if (event.key === 'Escape' && open) {
			button?.focus();
			hide(true);
		}
	}

	/** A tap anywhere else closes a card a long press opened. */
	$effect(() => {
		if (!open) {
			return;
		}
		const outside = (event: PointerEvent): void => {
			const target = event.target;
			if (
				target instanceof Node &&
				button?.contains(target) !== true &&
				card?.contains(target) !== true
			) {
				hide(true);
			}
		};
		document.addEventListener('pointerdown', outside, true);
		return () => {
			document.removeEventListener('pointerdown', outside, true);
		};
	});
</script>

<button
	bind:this={button}
	type="button"
	class="fab"
	class:arrive={shown && !away}
	class:away={shown && away}
	class:unseen={!shown}
	class:down
	aria-label={words.name}
	aria-expanded={open}
	aria-controls={open ? `${id}-card` : undefined}
	inert={away}
	onpointerenter={(event) => {
		prime();
		enter(event);
	}}
	onpointerleave={(event) => {
		release();
		leave(event);
	}}
	onpointerdown={press}
	onpointerup={release}
	onpointercancel={release}
	oncontextmenu={(event) => {
		event.preventDefault();
	}}
	onclick={shuffle}
	onfocus={focused}
	onblur={blurred}
	onkeydown={onKey}
>
	<span class="disc">
		{#key fired}
			<span class="glyph" class:fired={fired > 0}
				><Icon name="shuffle" size={26} pathLength={60} /></span
			>
		{/key}
	</span>
	{#key fired}
		{#if fired > 0}<span class="ripple" aria-hidden="true"></span>{/if}
	{/key}
	{#key filter}
		{#if words.tag !== ''}
			<span class="tag" aria-hidden="true">
				{#if filter === 'hearted'}
					<svg width="12" height="12" viewBox="0 0 24 24"><path d={heartPath}></path></svg>
				{:else}
					{words.tag}
				{/if}
			</span>
		{/if}
	{/key}
</button>

{#if open}
	<div
		bind:this={card}
		id={`${id}-card`}
		class="card"
		role="dialog"
		tabindex="-1"
		aria-label="Shuffle"
		onpointerenter={enter}
		onpointerleave={leave}
		onfocusout={blurred}
		onkeydown={onKey}
		out:fade={{ duration: 120, easing: cubicOut }}
	>
		<span class="key" aria-hidden="true">CLICK TO PLAY</span>
		<p class="label">SHUFFLE</p>
		<p class="what">{words.what}</p>
		<p class="how">{songsLine(count)}</p>
		<hr />
		<p class="line">
			{#each settingsLine(shuffleSettings.current).split(' · ') as part, index (part)}{index > 0
					? ' · '
					: ''}<span class="part">{part}</span>{/each}
		</p>
		<a class="go" href={settingsHref}>Shuffle settings →</a>
	</div>
{/if}

<style>
	/*
	 * Its size and place: on the island's line, right, beside the island; stacked, left, just above
	 * it, off the island's fox Pause. Not a floor: it never stands under the rail.
	 */
	.fab,
	.card {
		--fab: 64px;
		/* What the glyph and the tag scale by, with the disc. */
		--fab-scale: 1;
		--fab-bottom: calc(24px + (74px - var(--fab)) / 2 + env(safe-area-inset-bottom));
		--fab-edge: 24px;
	}

	@media (max-width: 1024px) {
		.fab,
		.card {
			--fab-bottom: calc(110px + env(safe-area-inset-bottom));
			--fab-edge: 16px;
		}
	}

	@media (max-width: 699px) {
		.fab,
		.card {
			--fab: 56px;
			--fab-scale: 0.875;
		}
	}

	@media (min-width: 2560px) {
		.fab,
		.card {
			--fab: 80px;
			--fab-scale: 1.25;
		}
	}

	.fab {
		position: fixed;
		z-index: 1;
		right: var(--fab-edge);
		bottom: var(--fab-bottom);
		width: var(--fab);
		height: var(--fab);
		padding: 0;
		border: 0;
		background: none;
		color: var(--fox-ink);
		cursor: pointer;
		-webkit-tap-highlight-color: transparent;
		-webkit-touch-callout: none;
		touch-action: manipulation;
		user-select: none;
	}

	@media (max-width: 1024px) {
		.fab {
			right: auto;
			left: var(--fab-edge);
		}
	}

	.fab:focus-visible {
		outline: none;
	}

	.fab:focus-visible .disc {
		outline: 2px solid var(--fox-line);
		outline-offset: 4px;
	}

	.disc {
		position: absolute;
		inset: 0;
		display: grid;
		place-items: center;
		overflow: hidden;
		border-radius: 50%;
		background: var(--fox);
		/* Quiet: no print, a tight edge and a fox glow. It rises to a hover and sinks to a press. */
		box-shadow: var(--fab-shadow);
		transition:
			translate var(--morph-fast) var(--curve-settle),
			box-shadow var(--morph-fast) var(--curve-settle);
	}

	/* Grain on the fill: the fox, printed rather than lit. */
	.disc::after {
		content: '';
		position: absolute;
		inset: 0;
		background-image: var(--grain);
		background-size: 160px;
		opacity: calc(var(--grain-alpha) * 3);
		mix-blend-mode: multiply;
		pointer-events: none;
	}

	.glyph {
		position: relative;
		z-index: 1;
		display: grid;
		scale: var(--fab-scale);
	}

	.glyph :global(path) {
		stroke-dasharray: 60;
		stroke-dashoffset: 0;
	}

	@media (hover: hover) {
		.fab:hover .disc {
			translate: 0 -2px;
			box-shadow: var(--fab-shadow-lift);
		}
	}

	/* Down: at once, no transition. The disc sinks onto its edge. */
	.fab.down .disc,
	.fab.down:hover .disc {
		translate: 0 1px;
		box-shadow: var(--fab-shadow-down);
		transition: none;
	}

	/* After the song has started: the glyph reprints along its strokes, and one ring goes out. */
	.glyph.fired :global(path) {
		animation: redraw var(--morph-fast) ease-out;
	}

	.ripple {
		position: absolute;
		inset: 0;
		border-radius: 50%;
		border: 2px solid var(--fox-line);
		opacity: 0;
		pointer-events: none;
		animation: ripple var(--morph) ease-out;
	}

	@keyframes redraw {
		from {
			stroke-dashoffset: 60;
		}
	}

	@keyframes ripple {
		from {
			opacity: 0.9;
			scale: 1;
		}

		to {
			opacity: 0;
			scale: 1.75;
		}
	}

	/* Arrival: stamped in, tilted, after the list's first rows. */
	.fab.arrive {
		animation: stamp var(--morph) var(--curve-settle) 120ms backwards;
	}

	@keyframes stamp {
		from {
			opacity: 0;
			transform: scale(1.35) rotate(-14deg);
		}

		45% {
			opacity: 1;
		}
	}

	/* Tucked: the selection bar's Shuffle is the one on screen. Ease-in, down into the corner. */
	.fab.away {
		pointer-events: none;
		animation: tuck var(--morph-drop) var(--curve-fall) forwards;
	}

	.fab.unseen {
		opacity: 0;
		pointer-events: none;
	}

	@keyframes tuck {
		to {
			opacity: 0;
			transform: translateY(24px) scale(0.8);
		}
	}

	/* The scope, when it isn't the whole library: the filter chip's own mark, stamped on a shoulder. */
	.tag {
		position: absolute;
		z-index: 2;
		top: -7px;
		left: -10px;
		height: 26px;
		min-width: 26px;
		padding: 0 7px;
		display: grid;
		place-items: center;
		border-radius: var(--radius-pill);
		background: var(--ground);
		border: 2px solid var(--fox-line);
		color: var(--fox-text);
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.02em;
		white-space: nowrap;
		rotate: -8deg;
		scale: var(--fab-scale);
		transform-origin: 100% 100%;
		animation: tag-stamp var(--morph-fast) var(--curve-settle);
	}

	.tag svg {
		fill: currentColor;
	}

	@keyframes tag-stamp {
		from {
			opacity: 0;
			scale: calc(var(--fab-scale) * 1.6);
			rotate: -22deg;
		}
	}

	@media (max-width: 1024px) {
		.tag {
			left: auto;
			right: -10px;
			rotate: 8deg;
			transform-origin: 0 100%;
		}
	}

	/* The card: what a press shuffles, by what, and the way to change it. */
	.card {
		position: fixed;
		z-index: 3;
		right: var(--fab-edge);
		bottom: calc(var(--fab-bottom) + var(--fab) + 14px);
		width: min(300px, calc(100vw - 32px));
		padding: 16px 18px 14px;
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid var(--edge-strong);
		box-shadow: var(--shadow-float);
		transform-origin: 100% 100%;
		animation: card-in var(--morph-room) var(--curve-settle);
	}

	@media (max-width: 1024px) {
		.card {
			right: auto;
			left: var(--fab-edge);
			transform-origin: 0 100%;
		}
	}

	@media (min-width: 2560px) {
		.card {
			scale: 1.25;
		}
	}

	@keyframes card-in {
		from {
			opacity: 0;
			transform: translateY(8px) scale(0.96);
		}
	}

	.card p {
		margin: 0;
	}

	.label,
	.key {
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 600;
		letter-spacing: 0.1em;
		color: var(--text-label);
	}

	.key {
		float: right;
		margin-top: 18px;
		font-weight: 400;
		letter-spacing: 0.08em;
	}

	@media (hover: none), (max-width: 699px) {
		.key {
			display: none;
		}
	}

	.what {
		margin-top: 2px;
		font-size: 22px;
		font-weight: 900;
		letter-spacing: -0.04em;
		line-height: 1.05;
	}

	.how {
		margin-top: 4px;
		font-family: var(--font-mono);
		font-size: 12px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	hr {
		margin: 12px 0 10px;
		border: 0;
		border-top: 1px solid var(--edge);
	}

	.line {
		font-family: var(--font-mono);
		font-size: 12px;
		line-height: 1.6;
		letter-spacing: 0.06em;
		color: var(--text-secondary);
	}

	/* The line breaks between settings, never inside one. */
	.part {
		white-space: nowrap;
	}

	.go {
		min-height: var(--target);
		margin-top: 8px;
		margin-left: -10px;
		padding: 0 10px;
		display: inline-flex;
		align-items: center;
		border-radius: var(--radius-pill);
		color: var(--fox-text);
		font-size: 14px;
		font-weight: 700;
		text-decoration: none;
	}

	.go:hover {
		background: var(--hover);
	}

	/* Reduced motion: opacity only, and a press is a change of state, not a move. */
	@media (prefers-reduced-motion: reduce) {
		.fab.arrive {
			animation: fade-in 120ms linear backwards;
		}

		.fab.away {
			animation: fade-out 120ms linear forwards;
		}

		.disc {
			transition: none;
		}

		.fab:hover .disc,
		.fab.down .disc {
			translate: 0 0;
		}

		.ripple {
			display: none;
		}

		.glyph.fired :global(path) {
			animation: none;
		}

		.tag,
		.card {
			animation: fade-in 120ms linear;
		}
	}

	@keyframes fade-in {
		from {
			opacity: 0;
		}
	}

	@keyframes fade-out {
		to {
			opacity: 0;
		}
	}
</style>
