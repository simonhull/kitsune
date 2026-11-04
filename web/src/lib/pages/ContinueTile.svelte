<script lang="ts">
	import { resolve } from '$app/paths';
	import { coverFor } from '#lib/cover/cover.ts';
	import GeneratedCover from '#lib/cover/GeneratedCover.svelte';
	import { clock } from '#lib/library/format.ts';
	import type { Home } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import Wash from '#lib/shell/Wash.svelte';

	interface Props {
		listening: Home['continueListening'];
	}

	let { listening }: Props = $props();

	const album = $derived(listening.album);
	const palette = $derived(coverFor(album.id).palette);
</script>

<a
	class="tile"
	href={resolve('/album/[id]', { id: album.id })}
	style:background={palette.stage}
	style:--tile-accent={palette.accent}
	style:--tile-on-accent={palette.onAccent}
	style:--tile-wash={palette.wash}
>
	<Wash albumId={album.id} variant="tile" />
	<span class="art" use:morph={{ id: `cover:${album.id}`, slot: 'card', kind: 'cover' }}
		><GeneratedCover albumId={album.id} /></span
	>
	<span class="words">
		<span class="label">Continue listening</span>
		<span class="title" use:morph={{ id: `title:${album.id}`, slot: 'card', kind: 'title' }}
			>{album.title}</span
		>
		<span class="detail"
			>{album.artist} · track {listening.trackNumber} of {listening.trackCount} · {clock(
				listening.positionSeconds
			)} in</span
		>
		<span class="resume"><Icon name="play" />Resume</span>
	</span>
</a>

<style>
	.tile {
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
	}

	.label {
		font-size: 13px;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--tile-wash);
	}

	.title {
		width: fit-content;
		font-size: 64px;
		font-weight: 800;
		letter-spacing: -0.05em;
		line-height: 0.9;
	}

	.detail {
		font-size: var(--type-title);
		color: var(--tile-wash);
	}

	.resume {
		align-self: flex-start;
		margin-top: 8px;
		height: 52px;
		padding: 0 26px 0 20px;
		border-radius: var(--radius-pill);
		background: var(--tile-accent);
		color: var(--tile-on-accent);
		font-weight: 700;
		font-size: 16px;
		display: flex;
		align-items: center;
		gap: 10px;
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
			font-size: 52px;
		}

		.detail {
			font-size: var(--type-body);
		}
	}
</style>
