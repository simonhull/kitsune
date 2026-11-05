<script lang="ts">
	import type { Snippet } from 'svelte';
	import {
		addedOn,
		clock,
		credit,
		formatLine,
		gain,
		isMultiDisc,
		trackGain,
		trackLabel
	} from '#lib/library/format.ts';
	import type { Album } from '#lib/library/source.ts';
	import Icon from '#lib/shell/Icon.svelte';

	interface Props {
		album: Album | undefined;
		/** The ID of the track cued from this album, if any. */
		cuedId: string | undefined;
		/** Whether that track is the one sounding now. */
		sounding: boolean;
		/** Plays the album from this track. */
		play: (index: number) => void;
		/** The server no longer has it, though a list still did. */
		gone: boolean;
		/** It couldn't be fetched this time. */
		unavailable: boolean;
		retry: () => void;
		/** Takes the rows' place in the panel, while the album is being edited. */
		editor: Snippet<[Album]> | undefined;
	}

	let { album, cuedId, sounding, play, gone, unavailable, retry, editor }: Props = $props();

	const multiDisc = $derived(isMultiDisc(album?.tracks ?? []));
</script>

<section aria-label="Tracks" class="panel" data-morph-part="panel">
	{#if album !== undefined && editor !== undefined}
		{@render editor(album)}
	{:else if gone}
		<p class="note">This album isn't in the library any more.</p>
	{:else if unavailable}
		<p class="note">
			The tracks couldn't be loaded.
			<button type="button" onclick={retry}>Try again</button>
		</p>
	{:else if album !== undefined}
		<div class="rows">
			{#each album.tracks as track, index (track.id)}
				{@const others = credit(track, album.artist)}
				{@const rg = trackGain(track)}
				{@const cued = track.id === cuedId}
				<button
					type="button"
					class="row"
					class:cued
					class:discs={multiDisc}
					aria-current={cued ? 'true' : undefined}
					onclick={() => {
						play(index);
					}}
				>
					<span class="mono number">
						{#if cued && sounding}
							<Icon name="play" size={16} />
						{:else}
							{trackLabel(track, multiDisc)}
						{/if}
					</span>
					<span class="name">
						<span class="title">{track.title}</span>
						{#if others !== undefined}
							<span class="featuring">{others}</span>
						{/if}
					</span>
					{#if rg === null}
						<span class="mono gain missing">no RG</span>
					{:else}
						<span class="mono gain">RG {gain(rg)}</span>
					{/if}
					<span class="mono length">{clock(track.durationMs / 1000)}</span>
				</button>
			{/each}
			<div class="facts">
				<span>{formatLine(album.tracks)}</span>
				<span>added {addedOn(album.addedAt)}</span>
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

	.note {
		min-height: var(--target);
		padding: 16px;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0 8px;
		color: var(--text-secondary);
	}

	.note button {
		min-height: var(--target);
		padding: 0;
		border: 0;
		background: none;
		color: var(--fox);
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
		width: 100%;
		min-height: 60px;
		padding: 0 16px;
		border: 0;
		border-radius: 18px;
		background: none;
		text-align: left;
	}

	.row.cued {
		background: color-mix(in srgb, var(--accent) 10%, transparent);
	}

	.mono {
		font-family: var(--font-mono);
		font-variant-numeric: tabular-nums;
	}

	.number {
		display: flex;
		color: var(--text-label);
	}

	.cued .number {
		color: var(--accent);
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

	.cued .title {
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

		.row.discs {
			grid-template-columns: 44px minmax(0, 1fr) 48px;
		}

		.gain {
			display: none;
		}
	}
</style>
