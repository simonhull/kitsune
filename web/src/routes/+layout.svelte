<script lang="ts">
	import '@fontsource-variable/geist/wght.css';
	import '@fontsource-variable/geist-mono/wght.css';
	import '#lib/styles/tokens.css';
	import '#lib/styles/base.css';
	import { onMount } from 'svelte';
	import { preloadCode } from '$app/navigation';
	import { page } from '$app/state';
	import favicon from '#lib/assets/favicon.svg';
	import { coverFor } from '#lib/cover/cover.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import Backdrop from '#lib/shell/Backdrop.svelte';
	import Nav from '#lib/shell/Nav.svelte';
	import PlayerIsland from '#lib/shell/PlayerIsland.svelte';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const routeId = $derived(page.route.id);
	const onNowPlaying = $derived(routeId === '/now-playing');
	const onAlbum = $derived(routeId === '/album/[id]');
	const albumParam = $derived(page.params['id']);

	// The stage keeps its last album so it has something to fade away.
	let stageAlbum = $state<string | null>(null);
	$effect(() => {
		const next = onAlbum ? albumParam : onNowPlaying ? library.playing?.album.id : undefined;
		if (next !== undefined) {
			stageAlbum = next;
		}
	});

	const palette = $derived(stageAlbum === null ? null : coverFor(stageAlbum).palette);

	onMount(() => {
		void library.loadShell();
		void Promise.all(['/', '/album/[id]', '/now-playing'] as const).then((routes) =>
			Promise.all(routes.map((route) => preloadCode(route)))
		);
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<div
	class="shell"
	style:--accent={palette?.accent}
	style:--on-accent={palette?.onAccent}
	style:--wash={palette?.wash}
	style:--stage={palette?.stage}
>
	<Backdrop albumId={stageAlbum} mode={onNowPlaying ? 'poster' : onAlbum ? 'album' : 'clear'} />
	{#if !onNowPlaying}
		<Nav route={onAlbum ? 'album' : 'home'} healthIssues={library.home?.health.issues} />
	{/if}
	<main>{@render children()}</main>
	{#if library.playing !== null}
		<PlayerIsland playing={library.playing} expanded={onNowPlaying} />
	{/if}
	<div class="morph-layer" aria-hidden="true"></div>
</div>

<style>
	.shell {
		--hero-height: 680px;
		--accent: var(--fox);
		--on-accent: var(--fox-ink);
		--wash: var(--text-secondary);
		--stage: var(--ground);

		position: relative;
		min-height: 100dvh;
	}

	main {
		position: relative;
		z-index: 1;
	}

	.morph-layer {
		position: fixed;
		inset: 0;
		z-index: 5;
		overflow: clip;
		pointer-events: none;
	}

	@media (max-width: 820px) {
		.shell {
			--hero-height: 640px;
		}
	}
</style>
