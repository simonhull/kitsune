<!--
	The one checkbox of a song list: a row's, a list's header's, a disc heading's. A real checkbox,
	clear and as big as a finger needs, over a box drawn to the boards: empty with an edge that holds
	3:1 against its row, the fox with a check when picked, and with a dash when part of a group is.

	Whether it shows is the list's to say, with no state here: at rest it's hidden; it shows while the
	pointer is in its gutter (`.pick-gutter`), on focus, when it's checked, and on every box while the
	list is picking (`data-selecting` on the list's container). A row also shows its own on focus.
-->
<script lang="ts">
	import type { BoxState } from './selection.svelte.ts';

	interface Props {
		state: BoxState;
		/** What a screen reader says it does, such as "Select Breakwater". */
		label: string;
		/**
		 * Takes the group, or lets it go when it's all picked. A row's has none: its list takes every
		 * row's clicks at once.
		 */
		ontoggle?: (() => void) | undefined;
	}

	let { state, label, ontoggle }: Props = $props();
</script>

<span class="pick" class:on={state !== 'none'}>
	<input
		type="checkbox"
		aria-label={label}
		checked={state === 'all'}
		indeterminate={state === 'some'}
		onclick={ontoggle === undefined
			? undefined
			: (event) => {
					ontoggle();
					// It says what the list did, whatever the click did to it.
					event.currentTarget.checked = state === 'all';
					event.currentTarget.indeterminate = state === 'some';
				}}
	/>
	<svg viewBox="0 0 24 24" aria-hidden="true">
		{#if state === 'some'}
			<path d="M6 12h12"></path>
		{:else}
			<path class="tick" d="M5 12.5l4.5 4.5L19 7.5"></path>
		{/if}
	</svg>
</span>

<style>
	.pick {
		position: relative;
		flex: none;
		width: var(--box-size, 16px);
		height: var(--box-size, 16px);
		box-sizing: border-box;
		display: grid;
		place-items: center;
		border: 1.5px solid var(--check-edge);
		border-radius: calc(var(--box-size, 16px) / 4);
		background: transparent;
		opacity: 0;
		/*
		 * Only the showing fades. A border that eased too would ease on every hidden box whose row starts
		 * or stops playing, a run of frames on the way to the sound for nothing anyone sees.
		 */
		transition: opacity 120ms var(--curve-morph);
	}

	/* The real box, unseen, over the drawn one and past it: 28 for a pointer, 44 for a finger. */
	input {
		position: absolute;
		inset: -7px;
		margin: 0;
		opacity: 0;
		cursor: pointer;
	}

	@media (pointer: coarse) {
		input {
			inset: -15px;
		}
	}

	svg {
		width: 75%;
		height: 75%;
		fill: none;
		stroke: var(--fox-ink);
		stroke-width: 3.4;
		stroke-linecap: round;
		stroke-linejoin: round;
		pointer-events: none;
	}

	.tick {
		opacity: 0;
	}

	.on {
		border-color: var(--fox);
		background: var(--fox);
		opacity: 1;
	}

	.on .tick {
		opacity: 1;
	}

	@media (hover: hover) {
		.pick:not(.on):has(input:hover) {
			border-color: var(--fox-line);
		}

		:global(.pick-gutter:hover) .pick {
			opacity: 1;
		}
	}

	.pick:has(input:focus-visible) {
		opacity: 1;
		outline: 2px solid var(--fox-line);
		outline-offset: 2px;
	}

	:global([data-selecting]) .pick {
		opacity: 1;
	}

	/* On the playing row, which is the fox: its ink draws the box, and fills it when picked. */
	:global(.cued) .pick {
		--check-edge: color-mix(in srgb, var(--fox-ink) 55%, transparent);
	}

	:global(.cued) .pick.on {
		border-color: var(--fox-ink);
		background: var(--fox-ink);
	}

	:global(.cued) svg {
		stroke: var(--fox);
	}

	@media (prefers-reduced-motion: reduce) {
		.pick {
			transition: none;
		}
	}
</style>
