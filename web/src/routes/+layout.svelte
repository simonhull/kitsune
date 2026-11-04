<script lang="ts">
	import '@fontsource-variable/geist/wght.css';
	import '@fontsource-variable/geist-mono/wght.css';
	import '#lib/styles/tokens.css';
	import '#lib/styles/base.css';
	import { onMount } from 'svelte';
	import { onNavigate, preloadCode } from '$app/navigation';
	import { page } from '$app/state';
	import { coverFor } from '#lib/cover/cover.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { startMorphs } from '#lib/morph/engine.ts';
	import Hud from '#lib/morph/Hud.svelte';
	import { watchInput } from '#lib/morph/journey.ts';
	import Backdrop from '#lib/shell/Backdrop.svelte';
	import Bloom from '#lib/shell/Bloom.svelte';
	import Nav from '#lib/shell/Nav.svelte';
	import PlayerIsland from '#lib/shell/PlayerIsland.svelte';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const routeId = $derived(page.route.id);
	const onNowPlaying = $derived(routeId === '/now-playing');
	const onAlbum = $derived(routeId === '/album/[id]');
	const albumParam = $derived(page.params['id']);

	// The stage keeps its last album so it has something to fade away. It's derived, not an
	// effect: the stage must exist the moment the route changes, for the morph to pick it up.
	let lastStageAlbum: string | null = null;
	const stageAlbum = $derived.by(() => {
		const next = onAlbum ? albumParam : onNowPlaying ? library.playing?.album.id : undefined;
		lastStageAlbum = next ?? lastStageAlbum;
		return lastStageAlbum;
	});

	const palette = $derived(stageAlbum === null ? null : coverFor(stageAlbum).palette);

	onNavigate(startMorphs());

	onMount(() => {
		void library.loadShell();
		void Promise.all(['/', '/album/[id]', '/now-playing'] as const).then((routes) =>
			Promise.all(routes.map((route) => preloadCode(route)))
		);
		return watchInput();
	});
</script>

<div
	class="shell"
	style:--accent={palette?.accent}
	style:--on-accent={palette?.onAccent}
	style:--wash={palette?.wash}
	style:--stage={palette?.stage}
>
	<Backdrop albumId={stageAlbum} mode={onNowPlaying ? 'poster' : onAlbum ? 'album' : 'clear'} />
	<Nav
		route={onNowPlaying ? 'gone' : onAlbum ? 'album' : 'home'}
		healthIssues={library.home?.health.issues}
	/>
	<main>{@render children()}</main>
	<div class="ghost-layer" aria-hidden="true"></div>
	{#if stageAlbum !== null}<Bloom albumId={stageAlbum} />{/if}
	{#if library.playing !== null}
		<PlayerIsland playing={library.playing} expanded={onNowPlaying} />
	{/if}
	<div class="morph-layer" aria-hidden="true"></div>
	{#if import.meta.env.DEV}<Hud />{/if}
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

	/* Under the shell: what's left of the outgoing page. */
	.ghost-layer {
		position: fixed;
		inset: 0;
		z-index: 2;
		overflow: clip;
		pointer-events: none;
	}

	/* Over everything: what's in the air. */
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
