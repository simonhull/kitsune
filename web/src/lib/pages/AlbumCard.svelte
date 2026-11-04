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

<a class="card" data-morph-part="card" href={resolve('/album/[id]', { id: album.id })}>
	<span class="art" use:morph={{ id: `cover:${album.id}`, slot: 'card', kind: 'cover' }}
		><GeneratedCover albumId={album.id} /></span
	>
	<span class="names">
		<span class="title" use:morph={{ id: `title:${album.id}`, slot: 'card', kind: 'title' }}
			>{album.title}</span
		>
		<span class="artist">{album.artist}</span>
	</span>
</a>

<style>
	.card {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.art {
		position: relative;
		display: block;
		aspect-ratio: 1;
		border-radius: var(--radius-20);
		overflow: hidden;
	}

	.names {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.title {
		width: fit-content;
		font-size: var(--type-title);
		font-weight: 700;
		letter-spacing: var(--tracking-title);
	}

	.card:hover .title {
		color: var(--fox);
	}

	.artist {
		font-size: 14px;
		color: var(--text-meta);
	}
</style>
