<script lang="ts">
	import type { Snippet } from 'svelte';
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { paletteFor } from '#lib/cover/palette.ts';
	import { theme } from '#lib/theme/theme.svelte.ts';
	import { emWidth } from '#lib/library/fit.ts';
	import type { AlbumEntry } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import Wash from '#lib/shell/Wash.svelte';

	interface Props {
		/** Whose stage, cover and link it is. */
		album: AlbumEntry;
		/** Over the title, in small capitals, such as `Just added`. */
		label: string;
		/** In large type: the album's title, or the song's. */
		title: string;
		/** Under it, in the wash. */
		detail: string;
		/** Whether the words are the album's own, so they fly to its page with the cover. */
		albumWords: boolean;
		/** Whether the play is sounding: its button then pauses. */
		sounding: boolean;
		/** What the button says, past Play or Pause. */
		playName: string;
		onplay: () => void;
		/** A pointer or focus on its way to the play: what it plays can be asked for now. */
		onwarm?: () => void;
		/** Under the detail: how far in, or when the album came. */
		below?: Snippet;
	}

	let {
		album,
		label,
		title,
		detail,
		albumWords,
		sounding,
		playName,
		onplay,
		onwarm = () => undefined,
		below
	}: Props = $props();

	const palette = $derived(paletteFor(album, theme.current));
</script>

<!--
	The Continue card: the album's stage, its cover tilted at the corner and its play in the album's
	accent (the Two Owners Rule). The play sits beside the link, not in it: a button can't live inside
	a link, and anywhere else on the card opens the album.
-->
<div
	class="tile"
	data-morph-part="card"
	data-morph-key="feature"
	style:background={palette.stage}
	style:--tile-wash={palette.wash}
	style:--tile-accent={palette.accent}
	style:--tile-on-accent={palette.onAccent}
>
	<Wash {palette} variant="tile" />
	<a
		class="open"
		href={resolve('/album/[id]', { id: album.id })}
		aria-label={`${title}, ${detail}`}
		onpointerenter={onwarm}
		onfocus={onwarm}
	>
		<span class="art" use:morph={{ id: `cover:${album.id}`, slot: 'card', kind: 'cover' }}
			><Cover albumId={album.id} entry={album} place="card" /></span
		>
		<span class="words">
			<span class="label">{label}</span>
			{#if albumWords}
				<span
					class="title"
					{title}
					style:--width={emWidth(title, 800, -0.05)}
					use:morph={{ id: `title:${album.id}`, slot: 'card', kind: 'title' }}>{title}</span
				>
				<span class="detail" use:morph={{ id: `artist:${album.id}`, slot: 'card', kind: 'title' }}
					>{detail}</span
				>
			{:else}
				<span class="title" {title} style:--width={emWidth(title, 800, -0.05)}>{title}</span>
				<span class="detail">{detail}</span>
			{/if}
			{@render below?.()}
		</span>
	</a>
	<button
		type="button"
		class="play"
		aria-label={`${sounding ? 'Pause' : 'Play'} ${playName}`}
		onpointerenter={onwarm}
		onpointerdown={onwarm}
		onfocus={onwarm}
		onclick={onplay}><Icon name={sounding ? 'pause' : 'play'} size={30} /></button
	>
</div>

<style>
	.tile {
		container-type: inline-size;
		grid-column: span 2;
		grid-row: span 2;
		position: relative;
		border-radius: var(--radius-28);
		overflow: hidden;
	}

	.open {
		position: absolute;
		inset: 0;
		display: flex;
		flex-direction: column;
		justify-content: flex-end;
		padding: calc(40px * var(--step));
		color: var(--text);
	}

	.art {
		position: absolute;
		right: calc(48px * var(--step));
		top: calc(40px * var(--step));
		width: calc(240px * var(--step));
		height: calc(240px * var(--step));
		border-radius: calc(18px * var(--step));
		overflow: hidden;
		box-shadow: var(--shadow-tile);
		transform: rotate(4deg);
	}

	.words {
		position: relative;
		display: flex;
		flex-direction: column;
		gap: calc(14px * var(--step));
		min-width: 0;
		/* Clear of the play in the corner. */
		padding-right: calc(96px * var(--step));
	}

	.label {
		font-size: calc(13px * var(--step));
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--tile-wash);
	}

	/* Fit to one line if it can, then two, and a long title is cut rather than left to spill. */
	.title {
		--fit: calc(min(100cqw - 176px * var(--step), 480px * var(--step)) / max(var(--width), 1));

		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		overflow: hidden;
		width: fit-content;
		max-width: calc(480px * var(--step));
		padding-bottom: 0.09em;
		margin-bottom: -0.09em;
		font-size: clamp(30px, var(--fit), calc(64px * var(--step)));
		font-weight: 800;
		letter-spacing: -0.05em;
		line-height: 0.96;
		text-wrap: balance;
	}

	.detail {
		overflow: hidden;
		width: fit-content;
		max-width: 100%;
		font-size: var(--type-title);
		color: var(--tile-wash);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.play {
		position: absolute;
		right: calc(48px * var(--step));
		bottom: calc(40px * var(--step));
		width: calc(72px * var(--step));
		height: calc(72px * var(--step));
		padding: 0;
		display: grid;
		place-items: center;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--tile-accent);
		color: var(--tile-on-accent);
		box-shadow: 0 16px 40px color-mix(in srgb, var(--tile-accent) 30%, transparent);
		transition: scale var(--morph-drop) var(--curve-morph);
	}

	.play :global(svg) {
		width: calc(30px * var(--step));
		height: calc(30px * var(--step));
	}

	.play:hover {
		background: color-mix(in srgb, var(--tile-accent) 88%, var(--text));
	}

	.play:active {
		scale: 0.94;
	}

	/* From 2560 the card spans the column over the tiles: it has a height of its own. */
	@media (min-width: 2560px) {
		.tile {
			grid-row: auto;
			height: calc(504px * var(--step));
		}
	}

	@media (max-width: 720px) {
		.tile {
			grid-column: span 2;
			grid-row: auto;
			height: 360px;
			border-radius: var(--radius-28);
		}

		.open {
			padding: 24px;
		}

		.art {
			right: 24px;
			top: 24px;
			width: 150px;
			height: 150px;
		}

		.words {
			gap: 10px;
			padding-right: 0;
		}

		.label {
			font-size: 12px;
		}

		.title {
			--fit: calc((100cqw - 48px) / max(var(--width), 1));

			font-size: clamp(28px, var(--fit), 44px);
		}

		.detail {
			font-size: var(--type-body);
		}

		.play {
			right: 24px;
			bottom: 22px;
			width: 60px;
			height: 60px;
		}

		.play :global(svg) {
			width: 26px;
			height: 26px;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.play {
			transition-duration: 0ms;
		}
	}
</style>
