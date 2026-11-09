<script lang="ts">
	import type { Snippet } from 'svelte';
	import {
		addedOn,
		clock,
		credit,
		discFacts,
		discHeads,
		formatLine,
		gain,
		gainMeasured,
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
		/** Plays one disc of the album. */
		playDisc: (disc: number) => void;
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

	let {
		album,
		cuedId,
		sounding,
		play,
		playDisc,
		gone,
		unavailable,
		retry,
		editing,
		one,
		editor
	}: Props = $props();

	const multiDisc = $derived(isMultiDisc(album?.tracks ?? []));
	/** Each disc's heading, by its first track. Under one, a track's number starts again. */
	const heads = $derived(discHeads(album ?? { tracks: [], discs: [] }));
	/** Numbers say the disc only where no heading does. */
	const discLabels = $derived(multiDisc && heads.size === 0);
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
				{@const head = heads.get(index)}
				{#if head !== undefined}
					{@const label = `Disc ${String(head.no)}`}
					{@const facts = discFacts(head)}
					<div class="disc" class:first={index === 0}>
						<h2 class="disc-no">{label}</h2>
						<p class="disc-about">
							{#if head.name !== undefined}
								<span class="disc-name">{head.name}</span>
							{/if}
							<span class="disc-facts"
								><span class="disc-songs">{`${facts.songs} · `}</span>{facts.length}</span
							>
						</p>
						<button
							type="button"
							class="disc-play"
							aria-label={head.name === undefined ? `Play ${label}` : `Play ${label}, ${head.name}`}
							onclick={() => {
								playDisc(head.no);
							}}
						>
							<Icon name="play" size={12} />
							<span class="disc-play-words">Play disc</span>
						</button>
					</div>
				{/if}
				<!-- The song is one button, and its marks others beside it: a button can't hold a button. -->
				<div
					class="row"
					class:cued
					class:discs={discLabels}
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
								{trackLabel(track, discLabels)}
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
							<span class="mono gain"
								>RG {gain(rg)}{#if gainMeasured(track)}<span class="measured">
										measured</span
									>{/if}</span
							>
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
		background: color-mix(in srgb, var(--card) 82%, transparent);
		border: 1px solid var(--edge);
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
		color: var(--fox-text);
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

	/* A disc's heading: the album's own section, with its name, its length and its own play. */
	.disc {
		display: flex;
		align-items: flex-end;
		gap: 20px;
		padding: 40px 16px 14px;
		margin-bottom: 4px;
		border-bottom: 1px solid var(--edge-strong);
	}

	.disc.first {
		padding-top: 12px;
	}

	.disc-no {
		flex: none;
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
		line-height: 1;
	}

	.disc-about {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: baseline;
		gap: 20px;
	}

	.disc-name {
		min-width: 0;
		overflow: hidden;
		padding-bottom: 2px;
		font-size: 20px;
		font-weight: 600;
		color: var(--text-secondary);
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.disc-facts {
		flex: none;
		margin-left: auto;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.disc-play {
		flex: none;
		height: var(--target);
		padding: 0 18px;
		display: flex;
		align-items: center;
		gap: 8px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: none;
		color: var(--text);
		font-size: 14px;
		font-weight: 600;
	}

	@media (hover: hover) {
		.disc-play:hover {
			border-color: var(--edge-hover);
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
			background: var(--hover);
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
		color: var(--fox-text);
	}

	/* Kitsune's own measure, not the file's tags: said, quieter. */
	.measured {
		color: var(--text-meta);
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

		/* The phone's heading: the number over its name and length, and a round play to thumb. */
		.disc {
			display: grid;
			grid-template-columns: minmax(0, 1fr) var(--target);
			grid-template-areas: 'no play' 'about play';
			align-content: center;
			align-items: center;
			gap: 2px 10px;
			min-height: 64px;
			padding: 18px 8px 8px 10px;
			margin-bottom: 0;
		}

		.disc.first {
			padding-top: 0;
		}

		.disc-no {
			grid-area: no;
			font-size: 22px;
			letter-spacing: -0.03em;
		}

		.disc-about {
			grid-area: about;
			display: block;
			overflow: hidden;
			font-size: 13px;
			color: var(--text-secondary);
			white-space: nowrap;
			text-overflow: ellipsis;
		}

		.disc-name {
			padding: 0;
			font-size: inherit;
			font-weight: 400;
		}

		.disc-name::after {
			content: ' · ';
		}

		.disc-facts {
			margin: 0;
			font-family: inherit;
			font-size: inherit;
			letter-spacing: 0;
			color: inherit;
			text-transform: lowercase;
		}

		.disc-songs {
			display: none;
		}

		.disc-play {
			grid-area: play;
			width: var(--target);
			padding: 0;
			justify-content: center;
		}

		.disc-play-words {
			display: none;
		}
	}
</style>
