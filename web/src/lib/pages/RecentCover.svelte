<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
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
		<Cover albumId={album.id} entry={album} place="small" />
	</span>
</a>

<style>
	.recent {
		position: relative;
		display: block;
		flex: none;
		width: var(--recent-size, 112px);
		height: var(--recent-size, 112px);
	}

	.art {
		position: absolute;
		inset: 0;
		border-radius: calc(14px * var(--step));
		overflow: hidden;
	}
</style>
