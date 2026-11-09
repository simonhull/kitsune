<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { library } from '#lib/library/library.svelte.ts';
	import type { AlbumEntry } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';
	import CoverPlay from './CoverPlay.svelte';

	interface Props {
		album: AlbumEntry;
	}

	let { album }: Props = $props();

	/** Asks for the songs as a pointer or focus comes near, so the cover's play starts at once. */
	function warm(): void {
		void library.loadAlbum(album.id);
	}
</script>

<div class="recent">
	<a
		class="open"
		onpointerenter={warm}
		onfocus={warm}
		href={resolve('/album/[id]', { id: album.id })}
		aria-label={`${album.title} by ${album.artist}`}
	>
		<span class="art" use:morph={{ id: `cover:${album.id}`, slot: 'card', kind: 'cover' }}>
			<Cover albumId={album.id} entry={album} place="small" />
		</span>
	</a>
	<CoverPlay {album} />
</div>

<style>
	.recent {
		--cover-play-size: calc(36px * var(--step));
		--cover-play-inset: calc(8px * var(--step));

		position: relative;
		display: block;
		flex: none;
		width: var(--recent-size, 112px);
		height: var(--recent-size, 112px);
	}

	.open {
		position: absolute;
		inset: 0;
	}

	.art {
		position: absolute;
		inset: 0;
		border-radius: calc(14px * var(--step));
		overflow: hidden;
	}
</style>
