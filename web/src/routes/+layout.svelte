<script lang="ts">
	import '@fontsource-variable/geist/wght.css';
	import '@fontsource-variable/geist-mono/wght.css';
	import '#lib/styles/tokens.css';
	import '#lib/styles/base.css';
	import { onMount, setContext } from 'svelte';
	import { page } from '$app/state';
	import { goto, onNavigate, preloadCode } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { paletteFor } from '#lib/cover/palette.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { loginPath } from '#lib/library/session.ts';
	import { morphsKey, startMorphs } from '#lib/morph/engine.ts';
	import Hud from '#lib/morph/Hud.svelte';
	import { watchInput } from '#lib/morph/journey.ts';
	import { albumsView } from '#lib/pages/albumsView.svelte.ts';
	import { homeGrid } from '#lib/pages/gridWindow.svelte.ts';
	import { createCarrier } from '#lib/player/carry.ts';
	import { togglesOnSpace } from '#lib/player/keys.ts';
	import { followMediaSession } from '#lib/player/mediaSession.svelte.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import Backdrop from '#lib/shell/Backdrop.svelte';
	import Bloom from '#lib/shell/Bloom.svelte';
	import Nav from '#lib/shell/Nav.svelte';
	import PlayerIsland from '#lib/shell/PlayerIsland.svelte';
	import { albumOn, navOf, routeIds } from '#lib/shell/routes.ts';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const routeId = $derived(page.route.id);
	const onNowPlaying = $derived(routeId === '/now-playing');
	const onAlbum = $derived(routeId === '/album/[id]');
	const onLogin = $derived(routeId === '/login');
	const nav = $derived(navOf(routeId));

	// The stage keeps its last album so it has something to fade away. It's derived, not an
	// effect: the stage must exist the moment the route changes, for the morph to pick it up.
	let lastStageAlbum: string | null = null;
	const stageAlbum = $derived.by(() => {
		const next = albumOn(page) ?? (onNowPlaying ? library.playing?.album.id : undefined);
		lastStageAlbum = next ?? lastStageAlbum;
		return lastStageAlbum;
	});

	const stageEntry = $derived(stageAlbum === null ? undefined : library.entry(stageAlbum));
	const palette = $derived(stageAlbum === null ? null : paletteFor(stageEntry));

	const morphs = startMorphs();
	setContext(morphsKey, morphs);
	onNavigate(morphs.navigate);

	const carrier = createCarrier({
		hidden: () => document.hidden,
		onNowPlaying: () => onNowPlaying,
		blend: morphs.blend
	});
	player.carry = carrier.carry;

	function onVisibilityChange(): void {
		if (document.hidden) {
			carrier.flush();
		}
	}

	function onKeyDown(event: KeyboardEvent): void {
		if (togglesOnSpace(event) && library.playing !== null && !onLogin) {
			event.preventDefault();
			player.toggle();
		}
	}

	onMount(() => {
		library.onExpired = async () => {
			if (location.pathname !== resolve('/login')) {
				// eslint-disable-next-line svelte/no-navigation-without-resolve -- Built from resolve('/login') and the current path, with its `next`.
				await goto(loginPath(location), { replaceState: true });
			}
			homeGrid.reset();
			albumsView.reset();
		};
		void Promise.all(routeIds.map((route) => preloadCode(route)));
		const leaveSession = followMediaSession(player);
		const stopWatching = watchInput();
		return () => {
			leaveSession();
			stopWatching();
		};
	});
</script>

<svelte:window
	onkeydown={onKeyDown}
	ononline={() => {
		library.flushPlays();
	}}
/>
<svelte:document onvisibilitychange={onVisibilityChange} />

<div
	class="shell"
	style:--accent={palette?.accent}
	style:--on-accent={palette?.onAccent}
	style:--wash={palette?.wash}
	style:--stage={palette?.stage}
>
	<Backdrop
		albumId={stageAlbum}
		{palette}
		known={stageEntry !== undefined}
		mode={onNowPlaying ? 'poster' : onAlbum ? 'album' : 'clear'}
	/>
	<Nav look={nav.look} current={nav.current} healthIssues={library.facts?.issues} />
	<main>
		{#if library.down && library.facts === null && !onLogin}
			<p class="down" role="status">The server isn't answering. Trying again…</p>
		{:else}
			{@render children()}
		{/if}
	</main>
	<div class="ghost-layer" aria-hidden="true"></div>
	{#if palette !== null}<Bloom {palette} />{/if}
	{#if library.playing !== null && !onLogin}
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

	.down {
		min-height: 100dvh;
		padding: 16px;
		display: flex;
		align-items: center;
		justify-content: center;
		text-align: center;
		color: var(--text-secondary);
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
