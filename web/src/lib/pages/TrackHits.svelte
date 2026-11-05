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
			<span class="names">
				<span class="title">{track.title}</span>
				<span class="from">{[...track.artists, track.album].join(' · ')}</span>
			</span>
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
		min-height: 60px;
		padding: 8px 12px;
		margin: 0 -12px;
		border-radius: 18px;
		display: grid;
		grid-template-columns: 44px minmax(0, 1fr) auto;
		align-items: center;
		gap: 16px;
	}

	.hit:hover {
		background: rgb(255 255 255 / 5%);
	}

	.art {
		position: relative;
		display: block;
		aspect-ratio: 1;
		border-radius: var(--radius-12);
		overflow: hidden;
	}

	.names {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.title,
	.from {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.title {
		font-size: 16px;
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
		font-size: 13px;
		font-variant-numeric: tabular-nums;
		color: var(--text-meta);
	}
</style>
