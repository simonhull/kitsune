<script lang="ts">
	import '@fontsource-variable/geist/wght.css';
	import '@fontsource-variable/geist-mono/wght.css';
	import '#lib/styles/tokens.css';
	import '#lib/styles/base.css';
	import { onMount, setContext } from 'svelte';
	import { page, updated } from '$app/state';
	import { beforeNavigate, goto, onNavigate, preloadCode } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { paletteFor } from '#lib/cover/palette.ts';
	import { theme } from '#lib/theme/theme.svelte.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { loginPath } from '#lib/library/session.ts';
	import { morphsKey, startMorphs } from '#lib/morph/engine.ts';
	import Hud from '#lib/morph/Hud.svelte';
	import { watchInput } from '#lib/morph/journey.ts';
	import AlbumMenu from '#lib/pages/AlbumMenu.svelte';
	import { albumsView } from '#lib/pages/albumsView.svelte.ts';
	import { homeGrid } from '#lib/pages/gridWindow.svelte.ts';
	import { columnsView } from '#lib/pages/columnsView.svelte.ts';
	import { libraryView } from '#lib/pages/libraryView.svelte.ts';
	import { onScreen, selection } from '#lib/pages/selection.svelte.ts';
	import { sorts } from '#lib/pages/sortsView.svelte.ts';
	import { bulkEdits } from '#lib/pages/bulkEdits.svelte.ts';
	import MatchSheet from '#lib/matching/MatchSheet.svelte';
	import { matchSheet } from '#lib/matching/sheet.svelte.ts';
	import { tagEdits } from '#lib/pages/tagEdits.svelte.ts';
	import { createCarrier } from '#lib/player/carry.ts';
	import { togglesOnSpace } from '#lib/player/keys.ts';
	import { followMediaSession } from '#lib/player/mediaSession.svelte.ts';
	import { playbackSettings } from '#lib/player/playback.svelte.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import { reloadsInto } from '#lib/shell/freshness.ts';
	import { shuffles } from '#lib/shuffle/session.svelte.ts';
	import { shuffleSettings } from '#lib/shuffle/settings.svelte.ts';
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
	const nav = $derived(navOf(routeId));
	/** At a door, signing in or making the first account: nothing of the app shows there. */
	const atDoor = $derived(nav.look === 'door');

	// The stage keeps its last album so it has something to fade away. It's derived, not an
	// effect: the stage must exist the moment the route changes, for the morph to pick it up.
	let lastStageAlbum: string | null = null;
	const stageAlbum = $derived.by(() => {
		const next = albumOn(page) ?? (onNowPlaying ? library.playing?.album.id : undefined);
		lastStageAlbum = next ?? lastStageAlbum;
		return lastStageAlbum;
	});

	const stageEntry = $derived(stageAlbum === null ? undefined : library.entry(stageAlbum));
	const palette = $derived(stageAlbum === null ? null : paletteFor(stageEntry, theme.current));

	const edit = $derived(onAlbum && stageAlbum !== null ? tagEdits.of(stageAlbum) : undefined);
	/** Many tracks of this album at once: the page draws up as for any edit. */
	const many = $derived(
		onAlbum &&
			stageAlbum !== null &&
			bulkEdits.current?.layout === 'album' &&
			bulkEdits.current.albumIds.includes(stageAlbum)
	);

	/** A mark the server didn't take has already gone back: this says why it moved. */
	const markTrouble = $derived.by(() => {
		const failure = library.marks.failure;
		if (failure === null) {
			return '';
		}
		return failure.kind === 'stars'
			? "Those stars couldn't be saved, so they're back as they were."
			: "That heart couldn't be saved, so it's back as it was.";
	});

	const morphs = startMorphs();
	setContext(morphsKey, morphs);
	onNavigate(morphs.navigate);
	// A newer build waits: the next navigation loads it whole, unless a song would be cut off.
	beforeNavigate((navigation) => {
		const to = navigation.to?.url;
		if (
			to !== undefined &&
			reloadsInto({
				updated: updated.current,
				sounding: player.sounding,
				started: player.started,
				type: navigation.type
			})
		) {
			navigation.cancel();
			location.href = to.href;
		}
	});

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
		if (togglesOnSpace(event) && library.playing !== null && !atDoor) {
			event.preventDefault();
			player.toggle();
		}
	}

	// Read as soon as someone is signed in, so the first song already plays at their level.
	$effect(() => {
		if (library.user !== null) {
			void playbackSettings.load();
		}
	});

	// A change to what hearts count as, here or heard from elsewhere, blends the stars a heart gives
	// in place (StarsInput's rule): the root says so for a moment, so no row holds an effect of its own.
	/** Long enough for the blend (160 ms) to run out before the root lets it go. */
	const starsBlendMs = 200;
	let heartStarsSeen: number | null = null;
	let blendTimer: ReturnType<typeof setTimeout> | undefined;
	$effect(() => {
		const heartStars = library.marks.heartStars;
		if (heartStarsSeen !== null && heartStars !== heartStarsSeen) {
			clearTimeout(blendTimer);
			document.documentElement.dataset['starsBlend'] = '';
			blendTimer = setTimeout(() => {
				delete document.documentElement.dataset['starsBlend'];
			}, starsBlendMs);
		}
		heartStarsSeen = heartStars;
	});

	onMount(() => {
		library.onExpired = async () => {
			if (location.pathname !== resolve('/login')) {
				// eslint-disable-next-line svelte/no-navigation-without-resolve -- Built from resolve('/login') and the current path, with its `next`.
				await goto(loginPath(location), { replaceState: true });
			}
			homeGrid.reset();
			albumsView.reset();
			libraryView.reset();
			columnsView.reset();
			sorts.reset();
			selection.clear();
			shuffles.reset();
			shuffleSettings.reset();
			playbackSettings.reset();
			tagEdits.reset();
			bulkEdits.reset();
			matchSheet.reset();
		};
		void Promise.all(routeIds.map((route) => preloadCode(route)));
		const leaveSession = followMediaSession(player);
		const stopWatching = watchInput();
		const stopFollowing = theme.follow();
		return () => {
			leaveSession();
			stopWatching();
			stopFollowing();
		};
	});
</script>

<svelte:window
	onkeydown={onKeyDown}
	ononline={() => {
		library.flushPlays();
	}}
	onpagehide={() => {
		player.leave();
	}}
/>
<svelte:document onvisibilitychange={onVisibilityChange} />

<div
	class="shell"
	class:editing={edit !== undefined ||
		many ||
		(bulkEdits.current !== null && routeId === '/library')}
	class:one={edit?.open != null}
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
	<Nav
		look={nav.look}
		current={nav.current}
		healthIssues={library.admin ? library.facts?.issues : undefined}
		you={routeId === '/you'}
		attention={library.lastfm?.state === 'reconnect'}
	/>
	<main>
		{#if library.down && library.facts === null && !atDoor}
			<p class="down" role="status">The server isn't answering. Trying again…</p>
		{:else}
			{@render children()}
		{/if}
	</main>
	<p class="visually-hidden" role="status">{markTrouble}</p>
	<div class="ghost-layer" aria-hidden="true"></div>
	{#if palette !== null}<Bloom {palette} />{/if}
	{#if library.playing !== null && !atDoor}
		<PlayerIsland
			playing={library.playing}
			expanded={onNowPlaying}
			away={onScreen.current?.any === true && bulkEdits.current === null}
		/>
	{/if}
	<div class="morph-layer" aria-hidden="true"></div>
	<AlbumMenu />
	{#if library.admin && !atDoor}<MatchSheet />{/if}
	{#if import.meta.env.DEV}<Hud />{/if}
</div>

<style>
	.shell {
		/* An album's stage takes the step with its cover and words. */
		--hero-height: calc(680px * var(--step));
		--accent: var(--fox);
		--on-accent: var(--fox-ink);
		--wash: var(--text-secondary);
		--stage: var(--ground);

		position: relative;
		min-height: 100dvh;
	}

	.shell.editing {
		--hero-height: 430px;
	}

	.shell.one {
		--hero-height: 260px;
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

		.shell.editing {
			--hero-height: 300px;
		}

		/* On a phone the editor has the screen: its board has no room for either. */
		.shell.editing :global(.nav),
		.shell.editing :global(.island) {
			opacity: 0;
			pointer-events: none;
		}
	}
</style>
