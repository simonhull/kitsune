<script lang="ts">
	import '@fontsource-variable/geist/wght.css';
	import '@fontsource-variable/geist-mono/wght.css';
	import '#lib/styles/tokens.css';
	import '#lib/styles/base.css';
	import { onMount, setContext, untrack } from 'svelte';
	import { page, updated } from '$app/state';
	import { beforeNavigate, goto, onNavigate, preloadCode } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { paletteFor } from '#lib/cover/palette.ts';
	import { theme } from '#lib/theme/theme.svelte.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { loginPath } from '#lib/library/session.ts';
	import { morphsKey, startMorphs } from '#lib/morph/engine.ts';
	import Hud from '#lib/morph/Hud.svelte';
	import { hasInAppHistory, watchInput } from '#lib/morph/journey.ts';
	import AlbumMenu from '#lib/pages/AlbumMenu.svelte';
	import { albumsView } from '#lib/pages/albumsView.svelte.ts';
	import { columnsView } from '#lib/pages/columnsView.svelte.ts';
	import { libraryView } from '#lib/pages/libraryView.svelte.ts';
	import { onScreen, selection } from '#lib/pages/selection.svelte.ts';
	import { sorts } from '#lib/pages/sortsView.svelte.ts';
	import MatchSheet from '#lib/matching/MatchSheet.svelte';
	import { matchSheet } from '#lib/matching/sheet.svelte.ts';
	import EditorHost from '#lib/pages/EditorHost.svelte';
	import { tagEditor } from '#lib/pages/tagEditor.svelte.ts';
	import { createCarrier } from '#lib/player/carry.ts';
	import { playerKey } from '#lib/player/keys.ts';
	import { followMediaSession } from '#lib/player/mediaSession.svelte.ts';
	import { playbackSettings } from '#lib/player/playback.svelte.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import { resume } from '#lib/player/resume.svelte.ts';
	import { reloadsInto } from '#lib/shell/freshness.ts';
	import { shuffles } from '#lib/shuffle/session.svelte.ts';
	import { shuffleSettings } from '#lib/shuffle/settings.svelte.ts';
	import Backdrop from '#lib/shell/Backdrop.svelte';
	import Bloom from '#lib/shell/Bloom.svelte';
	import Nav from '#lib/shell/Nav.svelte';
	import PlayerIsland from '#lib/shell/PlayerIsland.svelte';
	import TrashToast from '#lib/shell/TrashToast.svelte';
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

	/** The tag editor is open over the page. */
	const editing = $derived(tagEditor.current !== null);

	/** Stars the server didn't take have already gone back: this says why they moved. */
	const markTrouble = $derived(
		library.marks.failure === null
			? ''
			: "Those stars couldn't be saved, so they're back as they were."
	);

	const morphs = startMorphs();
	setContext(morphsKey, morphs);
	onNavigate(morphs.navigate);
	// A newer build waits: the next navigation loads it whole, unless a song would be cut off or the
	// server doesn't hold where the player is yet.
	beforeNavigate((navigation) => {
		const to = navigation.to?.url;
		if (
			to !== undefined &&
			reloadsInto({
				updated: updated.current,
				sounding: player.sounding,
				unsaved: resume.unsaved,
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
			// Mobile browsers often never fire pagehide: hidden may be the last word this page gets.
			void resume.flush({ keepalive: true });
		} else {
			void resume.catchUp();
		}
	}

	/** The player's shortcuts, anywhere in the app once something is in the island. */
	function onKeyDown(event: KeyboardEvent): void {
		const playing = library.playing;
		const key = playing === null || atDoor ? null : playerKey(event);
		if (key === null || playing === null) {
			return;
		}
		event.preventDefault();
		switch (key.kind) {
			case 'toggle':
				player.toggle();
				break;
			case 'seek':
				player.seek(player.position + key.by);
				break;
			case 'previous':
				player.previous();
				break;
			case 'next':
				player.next();
				break;
			case 'shuffle':
				shuffles.toggle();
				break;
			case 'repeat':
				player.cycleRepeat();
				break;
			case 'rate':
				library.marks.rate(playing.track.id, key.stars);
				break;
			case 'now-playing':
				toggleNowPlaying(playing.album.id);
				break;
		}
	}

	/** N: into Now playing, or out again the way its Close goes. */
	function toggleNowPlaying(albumId: string): void {
		if (!onNowPlaying) {
			void goto(resolve('/now-playing'));
		} else if (hasInAppHistory()) {
			history.back();
		} else {
			void goto(resolve('/album/[id]', { id: albumId }));
		}
	}

	// Read as soon as someone is signed in, so the first song already plays at their level. Where
	// they left off is asked for beside the shell's own reads, and never holds them up.
	$effect(() => {
		if (library.user !== null) {
			void playbackSettings.load();
			untrack(() => void resume.restore());
		}
	});

	onMount(() => {
		library.onExpired = async () => {
			if (location.pathname !== resolve('/login')) {
				// eslint-disable-next-line svelte/no-navigation-without-resolve -- Built from resolve('/login') and the current path, with its `next`.
				await goto(loginPath(location), { replaceState: true });
			}
			albumsView.reset();
			libraryView.reset();
			columnsView.reset();
			sorts.reset();
			selection.clear();
			shuffles.reset();
			resume.reset();
			shuffleSettings.reset();
			playbackSettings.reset();
			tagEditor.reset();
			matchSheet.reset();
		};
		// Signing out saves where the player is first, so the next sign-in, anywhere, comes back to it.
		library.beforeSignOut = () => resume.flush();
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
		void resume.flush({ keepalive: true });
	}}
/>
<svelte:document onvisibilitychange={onVisibilityChange} />

<div
	class="shell"
	class:editing
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
	{#if !atDoor}<EditorHost />{/if}
	<div class="ghost-layer" aria-hidden="true"></div>
	{#if palette !== null}<Bloom {palette} />{/if}
	{#if library.playing !== null && !atDoor}
		<PlayerIsland
			playing={library.playing}
			expanded={onNowPlaying}
			away={onScreen.current?.any === true && !editing}
			arrival={resume.arrival}
			left={resume.left}
			onArrived={() => {
				resume.arrival = null;
			}}
		/>
	{/if}
	<div class="morph-layer" aria-hidden="true"></div>
	<AlbumMenu />
	{#if !atDoor}<TrashToast />{/if}
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

	@media (max-width: 640px) {
		/* On a phone the editor has the screen: its board has no room for either. */
		.shell.editing :global(.nav),
		.shell.editing :global(.island) {
			opacity: 0;
			pointer-events: none;
		}
	}
</style>
