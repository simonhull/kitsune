<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { artistCount } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';

	const load = $derived(library.artists);

	onMount(() => {
		// Every visit asks again: a list on screen stays up, and a scan may have found more.
		void library.loadArtists();
	});
</script>

<svelte:head>
	<title>Artists · Kitsune</title>
</svelte:head>

<div class="page-frame artists">
	<div class="intro">
		<h1>Artists</h1>
		{#if load.kind === 'ready' && load.value.length > 0}
			<span class="summary">{artistCount(load.value.length)}</span>
		{/if}
	</div>

	{#if load.kind === 'unavailable'}
		<p class="note" role="status">
			The artists couldn't be loaded.
			<button type="button" onclick={() => void library.loadArtists()}>Try again</button>
		</p>
	{:else if load.kind === 'ready' && load.value.length === 0}
		<p class="note" role="status">No artists yet. They arrive with the albums.</p>
	{:else if load.kind === 'ready'}
		<ul>
			{#each load.value as artist (artist.id)}
				<li>
					<a href={resolve('/artists/[id]', { id: artist.id })}>
						<span class="name" title={artist.name}>{artist.name}</span>
						<span class="count">{artist.albumCount.toLocaleString('en-US')}</span>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.artists {
		display: flex;
		flex-direction: column;
		gap: calc(40px * var(--step));
	}

	.intro {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 8px 24px;
	}

	h1 {
		font-size: clamp(44px, 7vw, var(--type-display));
		font-weight: 800;
		letter-spacing: var(--tracking-display);
		line-height: 0.9;
	}

	.summary {
		font-family: var(--font-mono);
		font-size: calc(13px * var(--step));
		color: var(--text-meta);
	}

	.note {
		min-height: var(--target);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0 8px;
		color: var(--text-secondary);
	}

	.note button {
		min-height: var(--target);
		padding: 0;
		border: 0;
		background: none;
		color: var(--fox-text);
	}

	ul {
		margin: 0;
		padding: 0;
		list-style: none;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(calc(280px * var(--step)), 1fr));
		gap: 0 calc(24px * var(--step));
	}

	a {
		min-height: var(--target);
		padding: 0 12px;
		margin: 0 -12px;
		border-radius: var(--radius-16);
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
	}

	a:hover {
		background: var(--hover);
	}

	.name {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: var(--type-body);
		font-weight: 500;
	}

	a:hover .name {
		color: var(--fox-text);
	}

	.count {
		font-family: var(--font-mono);
		font-size: calc(13px * var(--step));
		font-variant-numeric: tabular-nums;
		color: var(--text-label);
	}

	@media (max-width: 720px) {
		.artists {
			gap: 24px;
		}
	}
</style>
