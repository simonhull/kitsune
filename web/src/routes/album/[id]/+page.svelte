<script lang="ts">
	import { untrack } from 'svelte';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import { whenSettled } from '#lib/morph/settled.ts';
	import AlbumHero from '#lib/pages/AlbumHero.svelte';
	import TrackList from '#lib/pages/TrackList.svelte';
	import type { PageProps } from './$types';

	let { params }: PageProps = $props();

	const id = $derived(params.id);
	const entry = $derived(library.entry(id));
	const album = $derived(library.album(id));
	const cuedId = $derived(library.playing?.album.id === id ? library.playing.track.id : undefined);

	// Never awaited: the first frame is built from the list entry, and tracks stream in.
	$effect(() => {
		const wanted = id;
		// Untracked: a failure must wait for Try again, not re-run this and ask forever.
		untrack(() => void library.loadAlbum(wanted));
	});

	// The album last opened is the one cued, but the island mustn't change under a flight.
	$effect(() => {
		const loaded = album;
		if (loaded !== undefined) {
			return whenSettled(() => {
				library.cueAlbum(loaded);
			});
		}
	});
</script>

<svelte:head>
	<title>{entry?.title ?? 'Album'} · Kitsune</title>
</svelte:head>

{#if library.failed(id) && entry === undefined}
	<div class="missing">
		<h1>That album isn't in the library</h1>
		<a href={resolve('/')} data-sveltekit-reset="false">Back to Home</a>
	</div>
{:else}
	<div class="album">
		<AlbumHero albumId={id} {entry} {album} />
		<TrackList
			{album}
			{cuedId}
			gone={library.failed(id)}
			unavailable={library.unavailable(id)}
			retry={() => void library.loadAlbum(id)}
		/>
	</div>
{/if}

<style>
	.album {
		padding-bottom: 140px;
	}

	.missing {
		min-height: 100dvh;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 16px;
		padding: 16px;
		text-align: center;
	}

	.missing h1 {
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.missing a {
		min-height: var(--target);
		display: flex;
		align-items: center;
		color: var(--fox);
	}
</style>
