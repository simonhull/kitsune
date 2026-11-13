<script lang="ts">
	import type { LibraryFilter } from './libraryRows.ts';

	interface Props {
		filter: LibraryFilter;
		choose: (filter: LibraryFilter) => void;
	}

	let { filter, choose }: Props = $props();

	/** `short` is what a phone shows, to leave room for Sort beside them. */
	const chips: readonly { filter: LibraryFilter; label: string; short?: string }[] = [
		{ filter: 'everything', label: 'Everything', short: 'All' },
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
			{#if chip.short === undefined}
				{chip.label}
			{:else}
				<span class="long">{chip.label}</span><span class="short">{chip.short}</span>
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

	.short {
		display: none;
	}

	/* A phone keeps the two, and Shuffle and Sort beside them, on one line. */
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
