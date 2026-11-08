<script lang="ts">
	import { heartPath } from '#lib/shell/Icon.svelte';
	import type { LibraryFilter } from './libraryRows.ts';

	interface Props {
		filter: LibraryFilter;
		choose: (filter: LibraryFilter) => void;
	}

	let { filter, choose }: Props = $props();

	const chips: readonly { filter: LibraryFilter; label: string }[] = [
		{ filter: 'everything', label: 'Everything' },
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
			onclick={() => {
				choose(chip.filter);
			}}
		>
			{#if chip.filter === 'hearted'}
				<svg width="14" height="14" viewBox="0 0 24 24" aria-hidden="true"
					><path d={heartPath}></path></svg
				>
			{/if}
			{chip.label}
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

	/* The narrowest phones keep the three, and Shuffle beside them, on one line. */
	@media (max-width: 400px) {
		button {
			padding: 0 12px;
		}
	}
</style>
