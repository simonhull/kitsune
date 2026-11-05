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
	class="card"
	data-morph-part="card"
	data-morph-key={`card:${album.id}`}
	href={resolve('/album/[id]', { id: album.id })}
>
	<span class="art" use:morph={{ id: `cover:${album.id}`, slot: 'card', kind: 'cover' }}
		><Cover albumId={album.id} entry={album} place="card" lazy /></span
	>
	<span class="names">
		<span
			class="title"
			title={album.title}
			use:morph={{ id: `title:${album.id}`, slot: 'card', kind: 'title' }}>{album.title}</span
		>
		<span class="artist" use:morph={{ id: `artist:${album.id}`, slot: 'card', kind: 'title' }}
			>{album.artist}</span
		>
	</span>
</a>

<style>
	.card {
		display: flex;
		flex-direction: column;
		gap: 12px;
		min-width: 0;
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
		min-width: 0;
	}

	/* One line each, which also gives every card one height: the grid's boxes depend on it. */
	.title,
	.artist {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* As wide as the words: a flight measures the element, and an empty right side would stretch. */
	.title,
	.artist {
		width: fit-content;
		max-width: 100%;
	}

	.title {
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
