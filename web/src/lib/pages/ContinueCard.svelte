<script lang="ts">
	import { addedLine } from '#lib/library/home.ts';
	import { clock } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { AlbumEntry } from '#lib/library/source.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import { playsAlbum } from '#lib/player/queue.ts';
	import FeatureTile from './FeatureTile.svelte';

	interface Props {
		/** The newest album: the card's while the player holds nothing. */
		feature: AlbumEntry | undefined;
	}

	let { feature }: Props = $props();

	const held = $derived(library.playing);
	// The clock is the sound's, as the island's is.
	const seconds = $derived(held === null ? 0 : (player.current ?? held.track).durationMs / 1000);
	const position = $derived(player.current === null ? 0 : player.position);
	const percent = $derived(`${String(seconds > 0 ? (position / seconds) * 100 : 0)}%`);
	const featureSounding = $derived(
		feature !== undefined && playsAlbum(player.queue, feature.id, 'elsewhere') && player.sounding
	);

	/** Counts presses, so a later press outranks a fetch still out for an earlier one. */
	let presses = 0;

	function warmFeature(): void {
		if (feature !== undefined) {
			void library.loadAlbum(feature.id);
		}
	}

	/** Plays the newest album from its first song, as its page's Play does. */
	async function playFeature(album: AlbumEntry): Promise<void> {
		presses += 1;
		const mine = presses;
		const loaded = library.album(album.id);
		if (loaded !== undefined) {
			player.playOrToggle(loaded);
			return;
		}
		await library.loadAlbum(album.id);
		const fetched = library.album(album.id);
		if (fetched !== undefined && mine === presses) {
			player.playOrToggle(fetched);
		}
	}
</script>

{#if held !== null}
	<FeatureTile
		album={held.album}
		label={player.sounding ? 'Playing now' : 'Pick up where you left off'}
		title={held.track.title}
		detail={`${held.track.artists.join(', ') || held.album.artist} · ${held.album.title}`}
		albumWords={false}
		sounding={player.sounding}
		playName={held.track.title}
		onplay={() => {
			player.toggle();
		}}
	>
		{#snippet below()}
			<span class="progress">
				<span class="track" aria-hidden="true"
					><span class="fill" style:width={percent}></span></span
				>
				<span class="time"
					>{clock(position)}<span class="total">{` / ${clock(seconds)}`}</span></span
				>
			</span>
		{/snippet}
	</FeatureTile>
{:else if feature !== undefined}
	<FeatureTile
		album={feature}
		label="Just added"
		title={feature.title}
		detail={feature.artist}
		albumWords={true}
		sounding={featureSounding}
		playName={feature.title}
		onwarm={warmFeature}
		onplay={() => void playFeature(feature)}
	>
		{#snippet below()}
			{@const line = addedLine(feature)}
			{#if line !== ''}<span class="added">{line}</span>{/if}
		{/snippet}
	</FeatureTile>
{/if}

<style>
	.progress {
		display: flex;
		align-items: center;
		gap: calc(14px * var(--step));
		margin-top: calc(6px * var(--step));
	}

	.track {
		position: relative;
		width: calc(320px * var(--step));
		max-width: 60%;
		height: calc(4px * var(--step));
		border-radius: var(--radius-pill);
		background: color-mix(in srgb, var(--tile-wash) 22%, transparent);
	}

	.fill {
		position: absolute;
		inset: 0 auto 0 0;
		border-radius: var(--radius-pill);
		background: var(--tile-accent);
	}

	.time,
	.added {
		font-family: var(--font-mono);
		font-size: calc(13px * var(--step));
		color: var(--tile-wash);
		font-variant-numeric: tabular-nums;
	}

	@media (max-width: 720px) {
		.progress {
			gap: 12px;
			margin-top: 4px;
			/* Clear of the play in the corner. */
			padding-right: 76px;
		}

		.track {
			flex: 1;
			width: auto;
			max-width: none;
		}

		.time,
		.added {
			font-size: 12px;
		}

		.total {
			display: none;
		}
	}
</style>
