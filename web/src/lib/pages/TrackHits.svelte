<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { clock } from '#lib/library/format.ts';
	import type { TrackHit } from '#lib/library/source.ts';

	interface Props {
		tracks: readonly TrackHit[];
	}

	let { tracks }: Props = $props();
</script>

<div class="hits">
	{#each tracks as track (track.id)}
		<a class="hit" href={resolve('/album/[id]', { id: track.albumId })}>
			<span class="art"><Cover albumId={track.albumId} entry={track} place="small" lazy /></span>
			<span class="title">{track.title}</span>
			<span class="from">{[track.album, ...track.artists].join(' · ')}</span>
			<span class="length">{clock(track.durationMs / 1000)}</span>
		</a>
	{/each}
</div>

<style>
	.hits {
		display: flex;
		flex-direction: column;
	}

	.hit {
		min-height: 52px;
		padding: 7px 0;
		border-bottom: 1px solid var(--edge-card);
		display: grid;
		grid-template-columns: 36px minmax(0, 1fr) minmax(0, 360px) auto;
		align-items: center;
		gap: 14px;
	}

	.art {
		position: relative;
		display: block;
		aspect-ratio: 1;
		border-radius: var(--radius-12);
		overflow: hidden;
	}

	.title,
	.from {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.title {
		font-size: var(--type-body);
		font-weight: 600;
	}

	.hit:hover .title {
		color: var(--fox);
	}

	.from {
		font-size: 14px;
		color: var(--text-meta);
	}

	.length {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		color: var(--text-meta);
	}

	/* The album moves under the title, and the row stays one target. */
	@media (max-width: 720px) {
		.hit {
			min-height: 60px;
			grid-template-columns: 44px minmax(0, 1fr) auto;
			gap: 2px 12px;
		}

		.art {
			grid-row: span 2;
		}

		.title {
			grid-column: 2;
		}

		.from {
			grid-column: 2;
			grid-row: 2;
			font-size: 13px;
		}

		.length {
			grid-column: 3;
			grid-row: 1 / span 2;
		}
	}
</style>
