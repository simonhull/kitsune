<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import type { AlbumEntry } from '#lib/library/source.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { morph } from '#lib/morph/registry.ts';
	import { albumMenu } from './albumMenu.svelte.ts';
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

<!--
	Right-click opens the album's own menu; where nothing hovers, a ⋯ on the cover does. The cover's
	play sits beside the link, not in it: a button can't live inside a link.
-->
<div
	class="card"
	data-morph-part="card"
	data-morph-key={`card:${album.id}`}
	oncontextmenu={(event) => {
		event.preventDefault();
		albumMenu.open(album, event.clientX, event.clientY);
	}}
	role="group"
	aria-label={album.title}
>
	<a
		class="open"
		onpointerenter={warm}
		onfocus={warm}
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
	<CoverPlay {album} />
	<button
		type="button"
		class="more"
		aria-label={`More for ${album.title}`}
		aria-haspopup="menu"
		onclick={(event) => {
			const { right, bottom } = event.currentTarget.getBoundingClientRect();
			albumMenu.open(album, right - 300, bottom + 6);
		}}
		><svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"
			><circle cx="5" cy="12" r="1.8"></circle><circle cx="12" cy="12" r="1.8"></circle><circle
				cx="19"
				cy="12"
				r="1.8"
			></circle></svg
		></button
	>
</div>

<style>
	.card {
		position: relative;
		min-width: 0;
	}

	.open {
		display: flex;
		flex-direction: column;
		gap: calc(12px * var(--step));
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

	.open:hover .title {
		color: var(--fox-text);
	}

	.artist {
		font-size: calc(14px * var(--step));
		color: var(--text-meta);
	}

	/* Only where nothing hovers: a mouse has right-click. On the cover's corner, a finger's 44. */
	.more {
		display: none;
		position: absolute;
		top: 6px;
		right: 6px;
		width: var(--target);
		height: var(--target);
		place-items: center;
		padding: 0;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--glass-fill);
		color: var(--text);
	}

	.more svg {
		fill: currentColor;
	}

	@media (hover: none) {
		.more {
			display: grid;
		}
	}
</style>
