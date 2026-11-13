<script module lang="ts">
	/** Counts presses across every cover, so a later press outranks a fetch still out for an earlier one. */
	let latest = 0;
</script>

<script lang="ts">
	import { library } from '#lib/library/library.svelte.ts';
	import type { AlbumEntry } from '#lib/library/source.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import { playsAlbum, type StartedFrom } from '#lib/player/queue.ts';
	import Icon from '#lib/shell/Icon.svelte';

	interface Props {
		/** What it plays and names: an album card's entry, or a Library album. */
		album: Pick<AlbumEntry, 'id' | 'title'>;
		/** Where it plays from: from a list, the album goes on into the list's next one. */
		from?: StartedFrom;
	}

	let { album, from = 'elsewhere' }: Props = $props();

	const sounding = $derived(playsAlbum(player.queue, album.id, from) && player.sounding);

	/** Asks for the songs before the press: a pointer on the way, or a finger going down. */
	function warm(): void {
		void library.loadAlbum(album.id);
	}

	/**
	 * Plays the album from its first song, as its page's Play does, or pauses it if it's the one
	 * sounding. Its songs are asked for as a pointer or focus comes near (see `warm`), so a press
	 * normally finds them here and starts at once; only a press with nothing warmed waits for them.
	 */
	async function press(): Promise<void> {
		latest += 1;
		const mine = latest;
		const loaded = library.album(album.id);
		if (loaded !== undefined) {
			player.playOrToggle(loaded, from);
			return;
		}
		await library.loadAlbum(album.id);
		const fetched = library.album(album.id);
		if (fetched !== undefined && mine === latest) {
			player.playOrToggle(fetched, from);
		}
	}
</script>

<!--
	Over the cover, the size of the cover, so the button sits on its corner whatever the card's width.
	Only the button takes the pointer: the rest of the cover still opens the album.
-->
<span class="over">
	<button
		type="button"
		class="cover-play"
		class:sounding
		aria-label={`${sounding ? 'Pause' : 'Play'} ${album.title}`}
		onpointerenter={warm}
		onpointerdown={warm}
		onfocus={warm}
		onclick={() => void press()}
	>
		<Icon name={sounding ? 'pause' : 'play'} size={18} />
	</button>
</span>

<style>
	.over {
		position: absolute;
		inset: 0 0 auto;
		aspect-ratio: 1;
		pointer-events: none;
	}

	/*
	 * The Library's disc play, filled with the fox, or with the album's accent where a page gives it
	 * one: it comes up on hover and focus, and moves nothing.
	 */
	.cover-play {
		--size: var(--cover-play-size, calc(48px * var(--step)));

		position: absolute;
		right: var(--cover-play-inset, calc(12px * var(--step)));
		bottom: var(--cover-play-inset, calc(12px * var(--step)));
		width: var(--size);
		height: var(--size);
		padding: 0;
		display: grid;
		place-items: center;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--cover-play-fill, var(--fox));
		color: var(--cover-play-ink, var(--fox-ink));
		box-shadow: var(--shadow-deck);
		pointer-events: auto;
		opacity: 0;
		scale: 0.85;
		transition:
			opacity var(--morph-drop) var(--curve-morph),
			scale var(--morph-drop) var(--curve-morph);
	}

	.cover-play :global(svg) {
		width: calc(var(--size) * 0.4);
		height: calc(var(--size) * 0.4);
	}

	:global(:hover) > .over .cover-play,
	:global(:has(> a:focus-visible)) > .over .cover-play,
	.cover-play:focus-visible,
	.cover-play.sounding {
		opacity: 1;
		scale: 1;
	}

	.cover-play:hover {
		background: color-mix(in srgb, var(--cover-play-fill, var(--fox)) 88%, var(--text));
	}

	.cover-play:active {
		scale: 0.94;
	}

	/* Without hover a play nobody can find isn't there: it shows, and a finger gets its 44. */
	@media (hover: none) {
		.cover-play {
			--size: var(--target);

			opacity: 1;
			scale: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.cover-play {
			transition-duration: 0ms;
		}
	}
</style>
