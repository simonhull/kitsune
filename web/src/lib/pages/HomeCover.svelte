<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { paletteFor } from '#lib/cover/palette.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { AlbumEntry } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';
	import { theme } from '#lib/theme/theme.svelte.ts';
	import CoverPlay from './CoverPlay.svelte';

	interface Props {
		album: AlbumEntry;
		/** Said in mono under the title in place of the artist, such as `122 plays`. */
		extra?: string | undefined;
	}

	let { album, extra }: Props = $props();

	/** The play stands on the art, so it wears the album's accent: the Two Owners Rule. */
	const palette = $derived(paletteFor(album, theme.current));

	/** Asks for the songs as a pointer or focus comes near, so the cover's play starts at once. */
	function warm(): void {
		void library.loadAlbum(album.id);
	}
</script>

<!-- The cover's play sits beside the link, not in it: a button can't live inside a link. -->
<div
	class="cover"
	style:--cover-play-fill={palette.accent}
	style:--cover-play-ink={palette.onAccent}
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
			{#if extra === undefined}
				<span class="artist" use:morph={{ id: `artist:${album.id}`, slot: 'card', kind: 'title' }}
					>{album.artist}</span
				>
			{:else}
				<span class="extra">{extra}</span>
			{/if}
		</span>
	</a>
	<CoverPlay {album} />
</div>

<style>
	.cover {
		--cover-play-size: calc(52px * var(--step));
		--cover-play-inset: calc(10px * var(--step));

		position: relative;
		min-width: 0;
		width: var(--home-cover);
	}

	.open {
		display: flex;
		flex-direction: column;
		gap: calc(10px * var(--step));
		min-width: 0;
	}

	.art {
		position: relative;
		display: block;
		aspect-ratio: 1;
		border-radius: var(--radius-20);
		overflow: hidden;
		box-shadow: var(--shadow-deck);
	}

	.names {
		display: flex;
		flex-direction: column;
		gap: calc(3px * var(--step));
		min-width: 0;
	}

	/* One line each, as wide as the words: a flight measures the element. */
	.title,
	.artist,
	.extra {
		overflow: hidden;
		width: fit-content;
		max-width: 100%;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.title {
		font-size: calc(15px * var(--step));
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

	.extra {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	/* A finger has no hover: a tap on the cover opens the album, and nothing else stands on it. */
	@media (hover: none) {
		.cover :global(.over) {
			display: none;
		}
	}

	@media (max-width: 720px) {
		.art {
			border-radius: 18px;
		}

		.open {
			gap: 8px;
		}

		.title {
			font-size: 14px;
		}

		.artist {
			font-size: 13px;
		}
	}
</style>
