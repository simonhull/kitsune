<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { artistFigure, orderArtists } from '#lib/library/artistOrder.ts';
	import { artistCount } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { artistOrders, wayOf } from '#lib/library/sorts.ts';
	import SortControl, { type SortGroup } from '#lib/pages/SortControl.svelte';
	import { sorts } from '#lib/pages/sortsView.svelte.ts';
	import { morph } from '#lib/morph/registry.ts';
	import ArtistThumb from '#lib/art/ArtistThumb.svelte';

	const load = $derived(library.artists);
	const sort = $derived(sorts.artists.value);
	const shown = $derived(load.kind === 'ready' ? orderArtists(load.value, sort) : []);
	const sortGroups: SortGroup[] = $derived([
		{
			label: 'SORT ARTISTS BY',
			options: artistOrders,
			current: sort,
			choose: (next) => {
				sorts.artists.set(next as typeof sort);
			}
		}
	]);

	onMount(() => {
		// Every visit asks again: a list on screen stays up, and a scan may have found more.
		void library.loadArtists();
		void sorts.artists.load();
	});
</script>

<svelte:head>
	<title>Artists · Kitsune</title>
</svelte:head>

<div class="page-frame artists">
	<div class="intro">
		<h1 data-morph-part="intro" data-morph-key="headline" data-morph-headline>Artists</h1>
		{#if load.kind === 'ready' && load.value.length > 0}
			<span class="summary" data-morph-part="intro" data-morph-key="summary"
				>{artistCount(load.value.length)}</span
			>
			<div class="sort" data-morph-part="intro" data-morph-key="sort">
				<SortControl
					groups={sortGroups}
					words={artistOrders.find(({ by }) => by === sort.by)?.label ?? ''}
					way={wayOf(sort.by, sort.dir)}
					look="pill"
					label="Sort the artists"
				/>
			</div>
		{/if}
	</div>

	{#if load.kind === 'unavailable'}
		<p class="note" role="status" data-morph-part="intro" data-morph-key="note">
			The artists couldn't be loaded.
			<button type="button" onclick={() => void library.loadArtists()}>Try again</button>
		</p>
	{:else if load.kind === 'ready' && load.value.length === 0}
		<p class="note" role="status" data-morph-part="intro" data-morph-key="note">
			No artists yet. They arrive with the albums.
		</p>
	{:else if load.kind === 'ready'}
		<ul>
			{#each shown as artist (artist.id)}
				<li data-morph-part="card" data-morph-key={`artist:${artist.id}`}>
					<a href={resolve('/artists/[id]', { id: artist.id })}>
						<span class="who">
							<ArtistThumb
								artistId={artist.id}
								name={artist.name}
								thumb={artist.thumb ?? null}
								size={40}
								slot="row"
								lazy
							/>
							<span
								class="name"
								title={artist.name}
								use:morph={{ id: `name:${artist.id}`, slot: 'row', kind: 'title' }}
								>{artist.name}</span
							>
						</span>
						<span class="count">{artistFigure(artist, sort)}</span>
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

	.sort {
		margin-left: auto;
		align-self: center;
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
		min-height: calc(56px * var(--step));
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

	.who {
		min-width: 0;
		display: flex;
		align-items: center;
		gap: calc(14px * var(--step));
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
