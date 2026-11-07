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
	import { library } from '#lib/library/library.svelte.ts';
	import type { Album } from '#lib/library/source.ts';
	import HeartButton from '#lib/shell/HeartButton.svelte';
	import Icon from '#lib/shell/Icon.svelte';
	import StarButtons from '#lib/shell/StarButtons.svelte';

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
		/** Whether the album is being edited: the panel takes the editor's shape. */
		editing: boolean;
		/** One track is open to every tag: the stage has drawn up further. */
		one: boolean;
		/** Takes the rows' place in the panel, once the files have been read. */
		editor: Snippet<[Album]> | undefined;
	}

	let { album, cuedId, sounding, play, gone, unavailable, retry, editing, one, editor }: Props =
		$props();

	const multiDisc = $derived(isMultiDisc(album?.tracks ?? []));
</script>

<section aria-label="Tracks" class="panel" class:editing class:one data-morph-part="panel">
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
				<!-- The song is one button, and its marks others beside it: a button can't hold a button. -->
				<div
					class="row"
					class:cued
					class:discs={multiDisc}
					class:rated={library.marks.stars(track.id) !== null}
				>
					<button
						type="button"
						class="play"
						aria-label={`Play ${track.title}`}
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
					</button>
					<StarButtons trackId={track.id} title={track.title} />
					<HeartButton kind="tracks" id={track.id} name={track.title} />
					<span class="mono length">{clock(track.durationMs / 1000)}</span>
				</div>
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
		transition:
			margin-top var(--morph-slow) var(--curve-settle),
			width var(--morph-slow) var(--curve-settle),
			max-width var(--morph-slow) var(--curve-settle);
	}

	.editing {
		margin-top: -34px;
		width: calc(100% - 64px);
		max-width: 1376px;
		padding: 12px 24px;
		border-radius: var(--radius-28);
		background: var(--card);
		backdrop-filter: none;
		transition-duration: var(--morph);
		transition-timing-function: var(--curve-morph);
	}

	.one {
		margin-top: -84px;
		padding: 8px 24px;
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

	/* On the art, so the marks wear the accent. */
	.row {
		--heart-on: var(--accent);
		--heart-off: var(--text-secondary);

		display: grid;
		grid-template-columns: minmax(0, 1fr) auto 44px 80px;
		align-items: center;
		width: 100%;
		min-height: 60px;
		padding: 0 16px;
		border-radius: 18px;
	}

	.play {
		display: grid;
		grid-template-columns: 56px minmax(0, 1fr) 160px;
		gap: 16px;
		align-items: center;
		min-width: 0;
		min-height: 60px;
		padding: 0 16px 0 0;
		border: 0;
		background: none;
		color: inherit;
		font: inherit;
		text-align: left;
	}

	/* Shown once rated or hearted, on hover, and on the cued row. */
	.row :global(.stars),
	.row :global(.heart) {
		opacity: 0;
	}

	.row.rated :global(.stars),
	.row :global(.heart.on),
	.row :global(.stars:has(:focus-visible)),
	.row :global(.heart:focus-visible),
	.row.cued :global(.stars),
	.row.cued :global(.heart) {
		opacity: 1;
	}

	@media (hover: hover) {
		.row:not(.cued):hover {
			background: rgb(255 255 255 / 8%);
		}

		.row:hover :global(.stars),
		.row:hover :global(.heart) {
			opacity: 1;
		}
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

		.editing {
			margin-top: -20px;
			width: 100%;
			padding: 8px 16px;
			border: 0;
			border-radius: var(--radius-28) var(--radius-28) 0 0;
		}

		.row {
			grid-template-columns: minmax(0, 1fr) auto 44px 48px;
			padding: 8px 10px;
		}

		.play {
			grid-template-columns: 28px minmax(0, 1fr);
			gap: 10px;
		}

		.row.discs .play {
			grid-template-columns: 44px minmax(0, 1fr);
		}

		.gain {
			display: none;
		}
	}
</style>
