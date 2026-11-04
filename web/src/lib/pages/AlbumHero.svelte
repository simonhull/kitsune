<script lang="ts">
	import Cover from '#lib/cover/Cover.svelte';
	import { emWidth } from '#lib/library/fit.ts';
	import { albumFacts } from '#lib/library/format.ts';
	import type { Album, AlbumEntry } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';
	import Icon from '#lib/shell/Icon.svelte';

	interface Props {
		albumId: string;
		/** The list entry: present on the first frame when we came from a list. */
		entry: AlbumEntry | undefined;
		/** The full album, once it has streamed in. */
		album: Album | undefined;
	}

	let { albumId, entry, album }: Props = $props();

	const meta = $derived(album === undefined ? '' : albumFacts(album, album));
</script>

<section aria-label="Album" class="hero">
	<div
		class="art"
		data-morph-part="art"
		use:morph={{ id: `cover:${albumId}`, slot: 'hero', kind: 'cover' }}
	>
		<Cover {albumId} {entry} place="stage" />
	</div>
	<div class="words" data-morph-part="words">
		<span class="meta" class:ready={album !== undefined}>{meta}&nbsp;</span>
		<!-- The fit divides by the words' width, known before layout: the first frame is final. -->
		<h1
			title={entry?.title}
			style:--width={emWidth(entry?.title ?? '', 900, -0.07)}
			use:morph={{ id: `title:${albumId}`, slot: 'hero', kind: 'title' }}
		>
			{entry?.title ?? ''}
		</h1>
		<div class="actions">
			<button type="button" class="play" aria-label={`Play ${entry?.title ?? 'album'}`} disabled>
				<Icon name="play" size={28} />
			</button>
			<span class="artist">
				<span class="name" use:morph={{ id: `artist:${albumId}`, slot: 'hero', kind: 'title' }}
					>{entry?.artist ?? ''}</span
				>
			</span>
			<button type="button" class="ghost" disabled>Shuffle</button>
			<button type="button" class="ghost round" aria-label="Favorite" disabled
				><Icon name="heart-outline" /></button
			>
			<span class="ghost" aria-disabled="true">Edit tags</span>
		</div>
	</div>
</section>

<style>
	.hero {
		--hero-pad: 64px;
		--hero-cover: 380px;

		container-type: inline-size;
		position: relative;
		height: var(--hero-height);
	}

	.art {
		position: absolute;
		right: var(--hero-pad);
		top: 120px;
		width: var(--hero-cover);
		height: var(--hero-cover);
		border-radius: var(--radius-24);
		overflow: hidden;
		box-shadow: 0 40px 90px rgb(0 0 0 / 50%);
	}

	.words {
		position: absolute;
		left: var(--hero-pad);
		right: var(--hero-pad);
		bottom: 120px;
		display: flex;
		flex-direction: column;
		gap: 18px;
	}

	.meta {
		font-family: var(--font-mono);
		font-size: 13px;
		color: var(--wash);
		opacity: 0;
		transition: opacity var(--morph-fast);
	}

	.meta.ready {
		opacity: 1;
	}

	/*
	 * One line, fit by length, while it can stay headline-sized; past that two lines at a floor.
	 * The padding keeps descenders out of the clip: this line height is tighter than the glyphs.
	 */
	h1 {
		--room: calc(100cqw - 2 * var(--hero-pad) - var(--hero-cover) - 32px);
		--fit: calc(var(--room) / max(var(--width), 1));

		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		overflow: hidden;
		width: fit-content;
		max-width: var(--room);
		padding-bottom: 0.09em;
		margin-bottom: -0.09em;
		font-size: max(min(var(--type-hero), var(--fit)), clamp(32px, var(--fit) * 2, 44px));
		font-weight: 900;
		letter-spacing: var(--tracking-hero);
		line-height: 0.92;
		text-wrap: balance;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 14px;
	}

	.play {
		width: 72px;
		height: 72px;
		border: 0;
		border-radius: 50%;
		background: var(--accent);
		color: var(--on-accent);
		display: flex;
		align-items: center;
		justify-content: center;
		box-shadow: 0 16px 40px color-mix(in srgb, var(--accent) 30%, transparent);
	}

	/* As wide as the words, whatever room the row gives the artist: a flight measures this. */
	.name {
		display: inline-block;
		max-width: 100%;
	}

	/* Nothing can sound until streaming lands: a hollow button keeps its place and promises nothing. */
	.play:disabled {
		background: transparent;
		box-shadow: inset 0 0 0 2px var(--accent);
		color: var(--accent);
		cursor: default;
	}

	.artist {
		min-width: 0;
		font-size: 22px;
		font-weight: 700;
		letter-spacing: var(--tracking-title);
		margin-right: 12px;
		overflow-wrap: anywhere;
	}

	.ghost {
		height: var(--target);
		padding: 0 18px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: rgb(10 9 8 / 30%);
		color: var(--text);
		font-size: 14px;
		font-weight: 600;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.ghost:disabled,
	.ghost[aria-disabled] {
		color: var(--text-label);
		cursor: default;
	}

	.round {
		width: var(--target);
		padding: 0;
		border-radius: 50%;
	}

	@media (max-width: 820px) {
		.hero {
			--hero-pad: 16px;

			height: auto;
			padding: 96px var(--hero-pad) 96px;
			display: flex;
			flex-direction: column;
			gap: 24px;
		}

		.art {
			position: relative;
			inset: auto;
			align-self: center;
			width: min(280px, 72cqw);
			height: auto;
			aspect-ratio: 1;
		}

		.words {
			position: static;
			gap: 14px;
		}

		h1 {
			--room: calc(100cqw - 2 * var(--hero-pad));

			font-size: max(min(96px, var(--fit)), clamp(22px, var(--fit) * 2, 30px));
		}

		.play {
			width: 56px;
			height: 56px;
		}

		.artist {
			font-size: 18px;
			flex-basis: calc(100% - 70px);
			margin-right: 0;
		}
	}
</style>
