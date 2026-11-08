<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { paletteFor } from '#lib/cover/palette.ts';
	import { theme } from '#lib/theme/theme.svelte.ts';
	import { emWidth } from '#lib/library/fit.ts';
	import { balancedLines } from '#lib/library/format.ts';
	import { artistLine, colorSource, type TopHit } from '#lib/library/search.svelte.ts';
	import type { SearchResults } from '#lib/library/source.ts';

	interface Props {
		top: TopHit;
		/** The whole answer, which is where an artist finds its color. */
		results: SearchResults;
	}

	let { top, results }: Props = $props();

	const shown = $derived.by(() => {
		switch (top.kind) {
			case 'artist':
				return {
					name: top.artist.name,
					detail: artistLine(top.artist, results),
					href: resolve('/artists/[id]', { id: top.artist.id }),
					cover: null
				};
			case 'album':
				return {
					name: top.album.title,
					detail: top.album.artist,
					href: resolve('/album/[id]', { id: top.album.id }),
					cover: { albumId: top.album.id, entry: top.album }
				};
			case 'track':
				return {
					name: top.track.title,
					detail: [...top.track.artists, top.track.album].join(' · '),
					href: resolve('/album/[id]', { id: top.track.albumId }),
					cover: { albumId: top.track.albumId, entry: top.track }
				};
		}
	});

	const palette = $derived(paletteFor(colorSource(top, results), theme.current));
	/** It sets on two lines, so the longer of them is what has to fit. */
	const longest = $derived(
		Math.max(...balancedLines(shown.name).map((line) => emWidth(line, 900, -0.05)))
	);
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- `shown.href` comes from resolve() above. -->
<a
	class="top"
	href={shown.href}
	style:background={palette.field}
	style:--top-accent={palette.accent}
	style:--top-wash={palette.wash}
>
	<!-- eslint-enable svelte/no-navigation-without-resolve -->
	<span class="disc" aria-hidden="true"></span>
	{#if shown.cover !== null}
		<span class="art"
			><Cover albumId={shown.cover.albumId} entry={shown.cover.entry} place="card" /></span
		>
	{/if}
	<span class="label">Top result · {top.kind}</span>
	<span class="words">
		<!-- The fit divides by the words' width, known before layout: the first frame is final. -->
		<span class="name" title={shown.name} style:--width={longest}>{shown.name}</span>
		{#if shown.detail !== ''}
			<span class="detail">{shown.detail}</span>
		{/if}
	</span>
</a>

<style>
	.top {
		/* What the disc and the cover take from the words' width. */
		--beside: 220px;

		container-type: inline-size;
		position: relative;
		min-height: 300px;
		padding: 28px;
		border-radius: var(--radius-28);
		overflow: hidden;
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		gap: 24px;
		color: var(--text);
	}

	.disc {
		position: absolute;
		right: -40px;
		top: -40px;
		width: 280px;
		height: 280px;
		border-radius: 50%;
		background: var(--top-accent);
	}

	.art {
		position: absolute;
		right: 28px;
		top: 28px;
		width: 176px;
		height: 176px;
		border-radius: 18px;
		overflow: hidden;
		box-shadow: var(--shadow-tile);
	}

	.label {
		position: relative;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--top-wash);
	}

	.words {
		position: relative;
		display: flex;
		flex-direction: column;
		gap: 10px;
		min-width: 0;
	}

	.name {
		--room: calc(100cqw - var(--beside));
		--fit: calc(var(--room) / max(var(--width), 1));

		display: block;
		max-width: var(--room);
		font-size: clamp(30px, var(--fit), 64px);
		font-weight: 900;
		letter-spacing: -0.05em;
		line-height: 0.95;
		overflow-wrap: anywhere;
		text-wrap: balance;
	}

	.top:hover .name {
		color: var(--top-accent);
	}

	.detail {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 14px;
		font-weight: 600;
	}

	/* The tile is as wide as the page here, and the words go under the art. */
	@media (max-width: 720px) {
		.top {
			--beside: 0px;
		}

		.disc {
			width: 156px;
			height: 156px;
		}

		.art {
			width: 104px;
			height: 104px;
		}

		/* However many lines a long name takes, they start below the art. */
		.words {
			margin-top: 84px;
		}

		.name {
			font-size: clamp(30px, var(--fit), 52px);
		}
	}
</style>
