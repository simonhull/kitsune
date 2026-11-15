<!--
	A playlist's cover: its first four albums as a 2×2, or with fewer the first alone, filling it; an
	empty playlist shows a quiet ground with a dashed mark. Given a size it's that square; without one
	it fills its box. Callers give it their own shadow.
-->
<script lang="ts">
	import Cover from '#lib/cover/Cover.svelte';
	import type { PlaylistAlbum } from '#lib/library/source.ts';
	import { mosaicOf } from './playlistWords.ts';

	interface Props {
		albums: readonly PlaylistAlbum[];
		/** Its side in CSS pixels; undefined fills its container, square. */
		size?: number | undefined;
		radius?: string;
		label?: string;
	}

	let { albums, size, radius = 'var(--radius-12)', label }: Props = $props();

	const shown = $derived(mosaicOf(albums));
	const grid = $derived(shown.length === 4);
	const box = $derived(size === undefined ? undefined : grid ? size / 2 : size);
</script>

<span
	class="mosaic"
	class:grid
	class:empty={shown.length === 0}
	role={label === undefined ? undefined : 'img'}
	aria-label={label}
	aria-hidden={label === undefined ? 'true' : undefined}
	style:width={size === undefined ? '100%' : `${String(size)}px`}
	style:height={size === undefined ? undefined : `${String(size)}px`}
	style:border-radius={radius}
>
	{#each shown as album (album.id)}
		<span class="tile"><Cover albumId={album.id} entry={album} place="small" lazy {box} /></span>
	{:else}
		<span class="mark"></span>
	{/each}
</span>

<style>
	.mosaic {
		position: relative;
		display: grid;
		flex: none;
		aspect-ratio: 1;
		overflow: hidden;
		box-sizing: border-box;
	}

	.grid {
		grid-template-columns: repeat(2, minmax(0, 1fr));
		grid-template-rows: repeat(2, minmax(0, 1fr));
	}

	.tile {
		position: relative;
		display: block;
		min-width: 0;
		min-height: 0;
	}

	.empty {
		place-items: center;
		background: var(--card);
		box-shadow: inset 0 0 0 1px var(--edge);
	}

	.mark {
		width: 38%;
		aspect-ratio: 1;
		box-sizing: border-box;
		border: 2px dashed var(--edge-strong);
		border-radius: 30%;
	}
</style>
