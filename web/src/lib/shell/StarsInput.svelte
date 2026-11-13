<!--
	A song's rating: one slider, five stars, each full, half or empty, and one tab stop. The pointer's
	x picks a half and a click sets it (the value it has clears it); the keys move a half at a time;
	on touch a tap cycles a star (k, k − ½, none) and a drag scrubs in halves, set on release.

	With `onchange` it handles its own events. Without it, as in a song row, it is markup only, and
	the list's one set of handlers (songRow.ts's rowEvents) does the same through the same helpers,
	so the Library's rows mount no handler each.
-->
<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import {
		fillOf,
		keyed,
		pressed,
		starAt,
		starsWords,
		tapped,
		type StarsBy
	} from '#lib/library/stars.ts';
	import { dropPreview, fractionAcross, halfUnder, showPreview } from './starsPreview.ts';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'onchange'> {
		/** ½ to 5 in halves, or null for none. */
		value: number | null;
		/** What it rates, for a screen reader: "Rating of Breakwater". */
		label: string;
		/**
		 * Where it stands: a song row's, a menu's, Now playing's, the selection bar's, the player
		 * island's, a setting's, or the You page card's.
		 */
		size?: 'row' | 'menu' | 'stage' | 'bar' | 'island' | 'setting' | 'card';
		/** Whether ↑ and ↓ move it too. A menu keeps them for going between its items. */
		vertical?: boolean;
		onchange?: ((stars: number | null, by: StarsBy) => void) | undefined;
	}

	let {
		value,
		label,
		size = 'row',
		vertical = true,
		onchange,
		class: className,
		...rest
	}: Props = $props();

	const places = [1, 2, 3, 4, 5];
	/** A finger down on it: where it went down, and whether it has moved far enough to scrub. */
	let finger: { x: number; scrubbing: boolean } | null = null;
	/** A finger's tap was handled on its release, so the click after it is not. */
	let tapDone = false;

	/** How far a finger goes before a tap becomes a scrub, in px. */
	const scrubAfter = 6;

	function set(stars: number | null, by: StarsBy): void {
		onchange?.(stars, by);
	}

	function onpointerdown(event: PointerEvent & { currentTarget: HTMLSpanElement }): void {
		if (event.pointerType !== 'touch') {
			return;
		}
		finger = { x: event.clientX, scrubbing: false };
		event.currentTarget.setPointerCapture(event.pointerId);
	}

	function onpointermove(event: PointerEvent & { currentTarget: HTMLSpanElement }): void {
		const el = event.currentTarget;
		if (event.pointerType !== 'touch') {
			showPreview(el, halfUnder(el, event.clientX));
			return;
		}
		if (finger === null) {
			return;
		}
		finger.scrubbing ||= Math.abs(event.clientX - finger.x) > scrubAfter;
		if (finger.scrubbing) {
			showPreview(el, halfUnder(el, event.clientX));
		}
	}

	function onpointerup(event: PointerEvent & { currentTarget: HTMLSpanElement }): void {
		if (event.pointerType !== 'touch' || finger === null) {
			return;
		}
		const el = event.currentTarget;
		const { scrubbing } = finger;
		finger = null;
		tapDone = true;
		dropPreview(el);
		set(
			scrubbing
				? halfUnder(el, event.clientX)
				: tapped(value, starAt(fractionAcross(el, event.clientX))),
			'pointer'
		);
	}

	function onpointercancel(event: PointerEvent & { currentTarget: HTMLSpanElement }): void {
		finger = null;
		dropPreview(event.currentTarget);
	}

	function onclick(event: MouseEvent & { currentTarget: HTMLSpanElement }): void {
		if (tapDone) {
			tapDone = false;
			return;
		}
		set(pressed(value, halfUnder(event.currentTarget, event.clientX)), 'pointer');
	}

	function onkeydown(event: KeyboardEvent): void {
		if (!vertical && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) {
			return;
		}
		const next = keyed(value, event.key);
		if (next === undefined || event.altKey || event.ctrlKey || event.metaKey) {
			return;
		}
		event.preventDefault();
		set(next, 'key');
	}

	const handlers = $derived(
		onchange === undefined
			? {}
			: {
					onpointerdown,
					onpointermove,
					onpointerup,
					onpointercancel,
					onpointerleave: (event: PointerEvent & { currentTarget: HTMLSpanElement }) => {
						dropPreview(event.currentTarget);
					},
					onclick,
					onkeydown
				}
	);
</script>

<span
	{...rest}
	class={['stars-input', size, { live: onchange !== undefined }, className]}
	role="slider"
	tabindex="0"
	aria-label={label}
	aria-valuemin={0}
	aria-valuemax={5}
	aria-valuenow={value ?? 0}
	aria-valuetext={starsWords(value)}
	{...handlers}
>
	{#each places as place (place)}
		<span class={['st', fillOf(value, place)]} aria-hidden="true">★</span>
	{/each}
</span>

<style>
	/*
	 * Where it stands can set --stars-on, the filled ink, --stars-off, the empty one, and
	 * --stars-preview, what a hover would set: the playing row sets all three in its own ink.
	 */
	.stars-input {
		--on: var(--stars-on, var(--star));
		--off: var(--stars-off, var(--star-off));
		--pv: var(--stars-preview, color-mix(in srgb, var(--star) 70%, transparent));
		--glyph: var(--star-type, 13px);
		--cell: calc(var(--glyph) + 2px);
		--cell-height: 28px;

		flex: none;
		display: inline-flex;
		border-radius: 6px;
		font-size: var(--glyph);
		letter-spacing: 0;
		cursor: pointer;
		user-select: none;
		-webkit-user-select: none;
	}

	.stars-input:focus-visible {
		outline: 2px solid var(--fox-line);
		outline-offset: 2px;
	}

	.menu {
		--glyph: 16px;
		--cell: 24px;
		--cell-height: 36px;
	}

	.stage {
		--glyph: 15px;
		--cell: 24px;
		--cell-height: var(--target);
	}

	.bar {
		--glyph: 20px;
		--cell: 40px;
		--cell-height: 44px;
	}

	/* The player island's: 16 px stars in 22 × 44 targets, where the heart was. */
	.island {
		--glyph: 16px;
		--cell: 22px;
		--cell-height: 44px;
	}

	/* A setting's line, as the Last.fm love line: 20 px stars in 32 × 44 targets. */
	.setting {
		--glyph: 20px;
		--cell: 32px;
		--cell-height: 44px;
	}

	/* The You page card's choice: 28 px stars in 44 px targets. */
	.card {
		--glyph: 28px;
		--cell: 44px;
		--cell-height: 44px;
	}

	/* A finger gets a whole target a star, and a drag across them scrubs instead of scrolling. */
	@media (pointer: coarse) {
		.live {
			touch-action: none;
		}

		.menu,
		.stage,
		.bar,
		.setting,
		.card {
			--cell: var(--target);
			--cell-height: var(--target);
		}

		.menu {
			--glyph: 20px;
		}

		.stage {
			--glyph: 22px;
		}
	}

	.st {
		--c: var(--on);

		width: var(--cell);
		height: var(--cell-height);
		display: grid;
		place-items: center;
		line-height: 1;
		color: var(--off);
	}

	.full {
		color: var(--c);
	}

	/* A half: the glyph's left half in the fill, its right half empty. */
	.half {
		background: linear-gradient(90deg, var(--c) 50%, var(--off) 50%);
		background-clip: text;
		-webkit-background-clip: text;
		color: transparent;
	}

	/*
	 * A preview, in halves (starsPreview.ts writes it): the stars fill to exactly what a click would
	 * set, in the preview's ink, and the rest go empty, so it shows nothing else.
	 */
	.stars-input:global([data-preview]) .st {
		--c: var(--pv);

		background: none;
		color: var(--off);
	}

	.stars-input:global([data-preview='2']) .st:nth-child(-n + 1),
	.stars-input:global([data-preview='3']) .st:nth-child(-n + 1),
	.stars-input:global([data-preview='4']) .st:nth-child(-n + 2),
	.stars-input:global([data-preview='5']) .st:nth-child(-n + 2),
	.stars-input:global([data-preview='6']) .st:nth-child(-n + 3),
	.stars-input:global([data-preview='7']) .st:nth-child(-n + 3),
	.stars-input:global([data-preview='8']) .st:nth-child(-n + 4),
	.stars-input:global([data-preview='9']) .st:nth-child(-n + 4),
	.stars-input:global([data-preview='10']) .st {
		color: var(--c);
	}

	.stars-input:global([data-preview='1']) .st:nth-child(1),
	.stars-input:global([data-preview='3']) .st:nth-child(2),
	.stars-input:global([data-preview='5']) .st:nth-child(3),
	.stars-input:global([data-preview='7']) .st:nth-child(4),
	.stars-input:global([data-preview='9']) .st:nth-child(5) {
		background: linear-gradient(90deg, var(--c) 50%, var(--off) 50%);
		background-clip: text;
		-webkit-background-clip: text;
		color: transparent;
	}
</style>
