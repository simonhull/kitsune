<script lang="ts">
	import { resolve } from '$app/paths';
	import GeneratedCover from '#lib/cover/GeneratedCover.svelte';
	import type { AlbumEntry } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';

	interface Props {
		album: AlbumEntry;
	}

	let { album }: Props = $props();
</script>

<a
	class="recent"
	href={resolve('/album/[id]', { id: album.id })}
	aria-label={`${album.title} by ${album.artist}`}
>
	<span class="art" use:morph={{ id: `cover:${album.id}`, slot: 'card', kind: 'cover' }}>
		<GeneratedCover albumId={album.id} />
	</span>
</a>

<style>
	.recent {
		position: relative;
		display: block;
		flex: none;
		width: 112px;
		height: 112px;
	}

	.art {
		position: absolute;
		inset: 0;
		border-radius: 14px;
		overflow: hidden;
	}
</style>
