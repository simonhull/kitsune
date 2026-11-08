<!--
	The box that picks a group of songs at once: a list's header, or a disc's heading. It shows
	mixed when part of the group is picked, and a press takes the whole group or lets it go.
-->
<script lang="ts">
	import type { BoxState } from './selection.svelte.ts';

	interface Props {
		state: BoxState;
		/** What it picks, such as "every song on Disc 2". */
		what: string;
		/** Takes the group, or lets it go when it's all picked. */
		ontoggle: () => void;
	}

	let { state, what, ontoggle }: Props = $props();
</script>

<input
	type="checkbox"
	class="pick-box"
	aria-label={state === 'all' ? `Clear ${what}` : `Select ${what}`}
	checked={state === 'all'}
	indeterminate={state === 'some'}
	onclick={(event) => {
		ontoggle();
		// It says what the list did, whatever the click did to it.
		event.currentTarget.checked = state === 'all';
		event.currentTarget.indeterminate = state === 'some';
	}}
/>

<style>
	/*
	 * Hidden at rest, as a row's box is: it shows while the pointer is over its place in the gutter,
	 * on focus, and while the list is picking (`data-selecting` on the list's container).
	 */
	.pick-box {
		flex: none;
		width: var(--box-size, 16px);
		height: var(--box-size, 16px);
		margin: 0;
		accent-color: var(--fox);
		opacity: 0;
		transition: opacity 120ms var(--curve-morph);
	}

	@media (hover: hover) {
		:global(.pick-gutter:hover) .pick-box {
			opacity: 1;
		}
	}

	.pick-box:focus-visible,
	:global([data-selecting]) .pick-box {
		opacity: 1;
	}

	@media (prefers-reduced-motion: reduce) {
		.pick-box {
			transition: none;
		}
	}
</style>
