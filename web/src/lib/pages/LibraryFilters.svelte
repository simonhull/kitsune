<script lang="ts">
	import { starsText } from '#lib/library/stars.ts';
	import { heartPath } from '#lib/shell/Icon.svelte';
	import { goodStars, type LibraryFilter } from './libraryRows.ts';

	interface Props {
		filter: LibraryFilter;
		choose: (filter: LibraryFilter) => void;
		/** What an unrated heart counts as: at 4 or more, 4 and up takes hearted songs too. */
		heartStars?: number;
	}

	let { filter, choose, heartStars = 0 }: Props = $props();
	const withHearts = $derived(heartStars >= goodStars);

	/** `short` is what a phone shows, to leave room for Shuffle and Sort beside them. */
	const chips: readonly { filter: LibraryFilter; label: string; short?: string }[] = [
		{ filter: 'everything', label: 'Everything', short: 'All' },
		{ filter: 'hearted', label: 'Hearted' },
		{ filter: 'rated', label: '★ 4 and up' }
	];
</script>

<!-- The shell's, so amber. They narrow the one list; nothing animates out. -->
<div class="filters" role="group" aria-label="Show">
	{#each chips as chip (chip.filter)}
		<button
			type="button"
			class:on={chip.filter === filter}
			aria-pressed={chip.filter === filter}
			aria-label={chip.filter === 'rated' && withHearts
				? '4 stars and up, hearts included'
				: undefined}
			onclick={() => {
				choose(chip.filter);
			}}
		>
			{#if chip.filter === 'hearted'}
				<svg width="14" height="14" viewBox="0 0 24 24" aria-hidden="true"
					><path d={heartPath}></path></svg
				>
			{/if}
			{#if chip.short === undefined}
				{chip.label}
			{:else}
				<span class="long">{chip.label}</span><span class="short">{chip.short}</span>
			{/if}
			{#if chip.filter === 'rated' && withHearts}
				<span class="plus" title={`Hearted songs with no stars count as ${starsText(heartStars)}★`}
					>+<svg width="11" height="11" viewBox="0 0 24 24" aria-hidden="true"
						><path d={heartPath}></path></svg
					></span
				>
			{/if}
		</button>
	{/each}
</div>

<style>
	.filters {
		display: flex;
		gap: 6px;
		width: fit-content;
		padding: 4px;
		border-radius: var(--radius-pill);
		background: var(--card);
		border: 1px solid var(--edge);
	}

	button {
		height: 36px;
		padding: 0 14px;
		display: flex;
		align-items: center;
		gap: 6px;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text-secondary);
		font-size: 13px;
		font-weight: 600;
		white-space: nowrap;
	}

	button:hover {
		color: var(--text);
	}

	.on,
	.on:hover {
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	path {
		fill: none;
		stroke: currentcolor;
		stroke-width: 2;
	}

	.short {
		display: none;
	}

	/* 4 and up takes unrated hearts too: a small heart says so. */
	.plus {
		display: inline-flex;
		align-items: center;
		gap: 1px;
		margin-left: -2px;
		font-size: 11px;
		opacity: 0.9;
	}

	.plus path {
		fill: currentcolor;
		stroke: none;
	}

	/* A phone keeps the three, and Shuffle and Sort beside them, on one line. */
	@media (max-width: 600px) {
		.long {
			display: none;
		}

		.short {
			display: inline;
		}
	}

	@media (max-width: 400px) {
		button {
			padding: 0 12px;
		}
	}
</style>
