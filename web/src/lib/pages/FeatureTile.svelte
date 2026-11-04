<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { paletteFor } from '#lib/cover/palette.ts';
	import type { AlbumEntry } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';
	import Wash from '#lib/shell/Wash.svelte';

	interface Props {
		album: AlbumEntry;
	}

	let { album }: Props = $props();

	const palette = $derived(paletteFor(album));
</script>

<a
	class="tile"
	data-morph-part="card"
	data-morph-key="feature"
	href={resolve('/album/[id]', { id: album.id })}
	style:background={palette.stage}
	style:--tile-wash={palette.wash}
>
	<Wash {palette} variant="tile" />
	<span class="art" use:morph={{ id: `cover:${album.id}`, slot: 'card', kind: 'cover' }}
		><Cover albumId={album.id} entry={album} place="card" /></span
	>
	<span class="words">
		<span class="label">Just added</span>
		<span
			class="title"
			title={album.title}
			style:--chars={album.title.length}
			use:morph={{ id: `title:${album.id}`, slot: 'card', kind: 'title' }}>{album.title}</span
		>
		<span class="detail">{album.artist}</span>
	</span>
</a>

<style>
	.tile {
		container-type: inline-size;
		grid-column: span 2;
		grid-row: span 2;
		position: relative;
		border-radius: var(--radius-28);
		overflow: hidden;
		display: flex;
		flex-direction: column;
		justify-content: flex-end;
		padding: 32px;
	}

	.art {
		position: absolute;
		right: 32px;
		top: 32px;
		width: 220px;
		height: 220px;
		border-radius: 18px;
		overflow: hidden;
		box-shadow: 0 30px 60px rgb(0 0 0 / 45%);
		transform: rotate(4deg);
	}

	.words {
		position: relative;
		display: flex;
		flex-direction: column;
		gap: 12px;
		min-width: 0;
	}

	.label {
		font-size: 13px;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--tile-wash);
	}

	/* Fit to one line if it can, then two, and a long title is cut rather than left to spill. */
	.title {
		--fit: calc((100cqw - 64px) / (var(--chars) * 0.5));

		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		overflow: hidden;
		width: fit-content;
		max-width: 100%;
		padding-bottom: 0.09em;
		margin-bottom: -0.09em;
		font-size: clamp(30px, var(--fit), 64px);
		font-weight: 800;
		letter-spacing: -0.05em;
		line-height: 0.96;
		text-wrap: balance;
	}

	.detail {
		overflow: hidden;
		font-size: var(--type-title);
		color: var(--tile-wash);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	@media (max-width: 720px) {
		.tile {
			grid-column: auto;
			min-height: 400px;
			padding: 24px;
		}

		.art {
			right: 24px;
			top: 24px;
			width: 150px;
			height: 150px;
		}

		.title {
			--fit: calc((100cqw - 48px) / (var(--chars) * 0.5));

			font-size: clamp(28px, var(--fit), 52px);
		}

		.detail {
			font-size: var(--type-body);
		}
	}
</style>
