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

<div class="artists">
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
		max-width: 1400px;
		margin: 0 auto;
		padding: 106px 32px 140px;
		display: flex;
		flex-direction: column;
		gap: 40px;
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
		font-size: 13px;
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
		color: var(--fox);
	}

	ul {
		margin: 0;
		padding: 0;
		list-style: none;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 0 24px;
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
		background: rgb(255 255 255 / 5%);
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
		color: var(--fox);
	}

	.count {
		font-family: var(--font-mono);
		font-size: 13px;
		font-variant-numeric: tabular-nums;
		color: var(--text-label);
	}

	@media (max-width: 720px) {
		.artists {
			padding: 98px 16px 128px;
			gap: 24px;
		}
	}
</style>
