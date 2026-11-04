<script lang="ts">
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import AlbumHero from '#lib/pages/AlbumHero.svelte';
	import TrackList from '#lib/pages/TrackList.svelte';
	import type { PageProps } from './$types';

	let { params }: PageProps = $props();

	const id = $derived(params.id);
	const entry = $derived(library.entry(id));
	const album = $derived(library.album(id));
	const playingNumber = $derived(
		library.playing?.album.id === id ? library.playing.track.number : undefined
	);

	// Never awaited: the first frame is built from the list entry, and tracks stream in.
	$effect(() => {
		void library.loadAlbum(id);
	});
</script>

<svelte:head>
	<title>{entry?.title ?? 'Album'} · Kitsune</title>
</svelte:head>

{#if library.failed(id)}
	<div class="missing">
		<h1>That album isn't in the library</h1>
		<a href={resolve('/')}>Back to Home</a>
	</div>
{:else}
	<div class="album">
		<AlbumHero albumId={id} {entry} {album} />
		<TrackList {album} {playingNumber} />
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
