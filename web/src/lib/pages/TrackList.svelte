<script lang="ts">
	import { clock, gain } from '#lib/library/format.ts';
	import type { Album } from '#lib/library/source.ts';

	interface Props {
		album: Album | undefined;
		/** The number of the track playing from this album, if any. */
		playingNumber: number | undefined;
	}

	let { album, playingNumber }: Props = $props();
</script>

<section aria-label="Tracks" class="panel" data-morph-part="panel">
	{#if album !== undefined}
		<div class="rows">
			{#each album.tracks as track (track.number)}
				{@const playing = track.number === playingNumber}
				<div class="row" class:playing>
					{#if playing}
						<span class="bars" role="img" aria-label="Playing">
							<span style:height="8px"></span>
							<span style:height="14px"></span>
							<span style:height="5px"></span>
						</span>
					{:else}
						<span class="mono number">{String(track.number).padStart(2, '0')}</span>
					{/if}
					<span class="name">
						<span class="title">{track.title}</span>
						{#if track.featuring !== undefined}
							<span class="featuring">feat. {track.featuring}</span>
						{/if}
					</span>
					{#if track.replayGain === null}
						<span class="mono gain missing">no RG</span>
					{:else}
						<span class="mono gain">RG {gain(track.replayGain)}</span>
					{/if}
					<span class="mono length">{clock(track.seconds)}</span>
				</div>
			{/each}
			<div class="facts">
				<span>{album.format}</span>
				<span>{album.path}</span>
				<span>added {album.added}</span>
			</div>
		</div>
	{/if}
</section>

<style>
	.panel {
		position: relative;
		margin: -72px auto 0;
		width: calc(100% - 128px);
		max-width: 1312px;
		min-height: 560px;
		padding: 16px;
		border-radius: var(--radius-32);
		background: rgb(21 19 17 / 82%);
		border: 1px solid rgb(255 255 255 / 7%);
		backdrop-filter: blur(24px);
	}

	.rows {
		display: flex;
		flex-direction: column;
		animation: arrive var(--morph-fast) both;
	}

	@keyframes arrive {
		from {
			opacity: 0;
		}
	}

	.row {
		display: grid;
		grid-template-columns: 56px minmax(0, 1fr) 160px 80px;
		gap: 16px;
		align-items: center;
		min-height: 60px;
		padding: 0 16px;
		border-radius: 18px;
	}

	.row.playing {
		background: color-mix(in srgb, var(--accent) 10%, transparent);
	}

	.mono {
		font-family: var(--font-mono);
		font-variant-numeric: tabular-nums;
	}

	.number {
		color: var(--text-label);
	}

	.name {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0 10px;
		min-width: 0;
	}

	.title {
		font-size: 16px;
		font-weight: 500;
	}

	.playing .title {
		font-weight: 700;
		color: var(--accent);
	}

	.featuring {
		font-size: 14px;
		color: var(--text-meta);
	}

	.gain {
		font-size: var(--type-mono);
		color: var(--text-label);
	}

	.gain.missing {
		color: var(--fox);
	}

	.length {
		color: var(--text-meta);
		text-align: right;
	}

	.bars {
		display: flex;
		align-items: flex-end;
		gap: 2px;
		height: 14px;
	}

	.bars span {
		width: 3px;
		background: var(--accent);
		border-radius: 1px;
	}

	.facts {
		display: flex;
		flex-wrap: wrap;
		gap: 8px 24px;
		margin-top: 8px;
		padding: 16px 16px 6px;
		border-top: 1px solid var(--edge-card);
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-label);
	}

	@media (max-width: 820px) {
		.panel {
			margin-top: -48px;
			width: calc(100% - 16px);
			padding: 8px;
			border-radius: var(--radius-28);
		}

		.row {
			grid-template-columns: 28px minmax(0, 1fr) 48px;
			gap: 10px;
			padding: 8px 10px;
		}

		.gain {
			display: none;
		}
	}
</style>
