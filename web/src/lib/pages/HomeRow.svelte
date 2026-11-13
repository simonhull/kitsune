<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';

	interface Props {
		/** The heading, which also names the region. */
		label: string;
		/** Said in mono beside the heading, such as `the last 90 days`. */
		note?: string | undefined;
		/** Names the row for the page's arrivals. */
		morphKey: string;
		items: readonly T[];
		key: (item: T) => string;
		item: Snippet<[T]>;
		/** What closes the row, after its covers: it always has a place. */
		end?: Snippet;
		/** At the heading's far end. */
		aside?: Snippet;
	}

	let { label, note, morphKey, items, key, item, end, aside }: Props = $props();

	let row: HTMLElement | undefined = $state();
	/** How many covers the row has room for, once it has been measured. */
	let room = $state<number | null>(null);

	/**
	 * One row of whole covers that never wraps: the grid lays out as many as fit, spread to both
	 * edges, and the rest wrap out of sight. Measuring then drops those, so a cover nobody can see
	 * takes no focus. A row that scrolls, on a phone, keeps them all.
	 */
	$effect(() => {
		const element = row;
		if (element === undefined) {
			return;
		}
		const measure = (): void => {
			const style = getComputedStyle(element);
			const first = element.firstElementChild;
			if (style.overflowX === 'auto' || !(first instanceof HTMLElement)) {
				room = null;
				return;
			}
			const gap = Number.parseFloat(style.columnGap) || 0;
			const width = first.offsetWidth;
			room =
				width > 0 ? Math.max(1, Math.floor((element.clientWidth + gap) / (width + gap))) : null;
		};
		const observer = new ResizeObserver(measure);
		observer.observe(element);
		return () => {
			observer.disconnect();
		};
	});

	const shown = $derived(
		room === null ? items : items.slice(0, Math.max(0, room - (end === undefined ? 0 : 1)))
	);
</script>

<section aria-label={label} class="home-row" data-morph-part="card" data-morph-key={morphKey}>
	<div class="head">
		<h2>{label}</h2>
		{#if note !== undefined}<span class="note">{note}</span>{/if}
		{#if aside !== undefined}<span class="aside">{@render aside()}</span>{/if}
	</div>
	<div class="covers" bind:this={row}>
		{#each shown as each (key(each))}
			{@render item(each)}
		{/each}
		{@render end?.()}
	</div>
</section>

<style>
	.home-row {
		--home-cover: calc(168px * var(--step));

		display: flex;
		flex-direction: column;
		gap: calc(20px * var(--step));
		min-width: 0;
	}

	.head {
		display: flex;
		align-items: baseline;
		gap: calc(16px * var(--step));
	}

	h2 {
		font-size: calc(32px * var(--step));
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.note {
		font-family: var(--font-mono);
		font-size: calc(13px * var(--step));
		color: var(--text-meta);
	}

	.aside {
		margin-left: auto;
	}

	/*
	 * Whole covers spread edge to edge, at least the gap apart; what doesn't fit wraps onto a second
	 * line the row's height cuts off, until measuring drops it.
	 */
	.covers {
		display: grid;
		grid-template-columns: repeat(auto-fill, var(--home-cover));
		grid-auto-rows: auto;
		justify-content: space-between;
		column-gap: calc(24px * var(--step));
		row-gap: 100vh;
		max-height: calc(var(--home-cover) + 64px * var(--step));
		/* Clipped, not scrolled: focus can't pull a hidden line into view. The shadows stay whole. */
		overflow: clip;
		overflow-clip-margin: calc(32px * var(--step));
	}

	@media (max-width: 720px) {
		.home-row {
			--home-cover: 136px;

			gap: 14px;
		}

		.head {
			gap: 12px;
		}

		h2 {
			font-size: 24px;
			letter-spacing: -0.035em;
		}

		.note {
			font-size: 12px;
		}

		/* A row a thumb moves along, to the screen's edge: the person's own movement, not the page's. */
		.covers {
			display: flex;
			gap: 14px;
			max-height: none;
			margin-right: calc(-1 * var(--gutter));
			padding-right: var(--gutter);
			overflow-x: auto;
			overscroll-behavior-x: contain;
			scrollbar-width: none;
		}

		.covers > :global(*) {
			flex: none;
		}
	}
</style>
