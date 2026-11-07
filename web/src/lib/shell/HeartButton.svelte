<script lang="ts">
	import { library } from '#lib/library/library.svelte.ts';
	import type { MarkKind } from '#lib/library/source.ts';
	import { heartPath } from './Icon.svelte';

	interface Props {
		kind: MarkKind;
		id: string;
		/** What it hearts, for the label: "Heart Breakwater". */
		name: string;
		size?: number;
	}

	let { kind, id, name, size = 16 }: Props = $props();

	const hearted = $derived(library.marks.hearted(kind, id));
	/** Set by a press that fills it, and only then: a heart that scrolls into view has nothing to say. */
	let popping = $state(false);

	function press(): void {
		popping = !hearted;
		library.marks.toggle(kind, id);
	}
</script>

<button
	type="button"
	class={['heart', { on: hearted, popping }]}
	aria-label={`Heart ${name}`}
	aria-pressed={hearted}
	onclick={press}
	onanimationend={() => {
		popping = false;
	}}
>
	<svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
		<path d={heartPath}></path>
	</svg>
</button>

<style>
	/*
	 * Where it stands sets --heart-on, the fill, and --heart-off, the outline of an empty heart, and
	 * reaches it as :global(.heart) for anything else, such as when it shows.
	 */
	.heart {
		flex: none;
		width: var(--target);
		height: var(--target);
		padding: 0;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		display: grid;
		place-items: center;
	}

	path {
		fill: var(--heart-on, var(--text));
		fill-opacity: 0;
		stroke: var(--heart-off, var(--text-secondary));
		stroke-width: 2;
		/* Emptying is its own fade: it isn't the pop played backward. */
		transition:
			fill-opacity 160ms ease-out,
			stroke-opacity 160ms ease-out;
	}

	.on path {
		fill-opacity: 1;
		stroke-opacity: 0;
		transition: none;
	}

	.popping svg {
		animation: pop 220ms var(--curve-settle);
	}

	@keyframes pop {
		0% {
			transform: scale(0.7);
		}

		55% {
			transform: scale(1.22);
		}

		100% {
			transform: scale(1);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.popping svg {
			animation: none;
		}

		path {
			transition: none;
		}
	}
</style>
