<script lang="ts">
	import { resolve } from '$app/paths';
	import GeneratedCover from '#lib/cover/GeneratedCover.svelte';
	import { clock } from '#lib/library/format.ts';
	import type { NowPlaying } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';
	import Icon from './Icon.svelte';
	import { islandSlot } from './slot.svelte.ts';

	interface Props {
		playing: NowPlaying;
		/** On Now playing the island becomes the page's pill controls, in place. */
		expanded: boolean;
	}

	let { playing, expanded }: Props = $props();

	const progress = $derived(playing.positionSeconds / playing.track.seconds);
	const position = $derived(clock(playing.positionSeconds));
	const duration = $derived(clock(playing.track.seconds));
	const slot = $derived(expanded ? islandSlot.rect : null);
</script>

<aside
	aria-label="Player"
	class="island"
	class:expanded
	style:left={slot === null ? undefined : `${String(slot.left)}px`}
	style:top={slot === null ? undefined : `${String(slot.top)}px`}
	style:width={slot === null ? undefined : `${String(slot.width)}px`}
>
	{#if expanded}
		<div class="transport">
			<button type="button" class="skip" aria-label="Previous"
				><Icon name="previous" size={22} /></button
			>
			<button type="button" class="pause large" aria-label="Pause"
				><Icon name="pause" size={22} /></button
			>
			<button type="button" class="skip" aria-label="Next"><Icon name="next" size={22} /></button>
		</div>
		<div class="scrub">
			<span class="time">{position}</span>
			<div class="track">
				<div class="fill" style:width={`${String(progress * 100)}%`}></div>
			</div>
			<span class="time">{duration}</span>
		</div>
		<button type="button" class="skip favorite" aria-label="Favorite" aria-pressed="true"
			><Icon name="heart" size={20} /></button
		>
	{:else}
		<a class="now" href={resolve('/now-playing')}>
			<span
				class="thumb"
				use:morph={{ id: `cover:${playing.album.id}`, slot: 'island', kind: 'cover' }}
				><GeneratedCover albumId={playing.album.id} /></span
			>
			<span class="names">
				<span class="song">{playing.track.title}</span>
				<span class="artist">{playing.album.artist}</span>
			</span>
		</a>
		<span class="time compact">{position} / {duration}</span>
		<button type="button" class="skip" aria-label="Previous"><Icon name="previous" /></button>
		<button type="button" class="pause" aria-label="Pause"><Icon name="pause" size={20} /></button>
		<button type="button" class="skip" aria-label="Next"><Icon name="next" /></button>
		<span class="progress" style:width={`${String(progress * 100)}%`} aria-hidden="true"></span>
	{/if}
</aside>

<style>
	.island {
		position: fixed;
		left: 50%;
		bottom: calc(24px + env(safe-area-inset-bottom));
		transform: translateX(-50%);
		z-index: 4;
		width: min(720px, calc(100% - 32px));
		display: flex;
		align-items: center;
		gap: 16px;
		padding: 10px 12px 10px 10px;
		border-radius: var(--radius-28);
		background: var(--glass-fill-player);
		border: 1px solid var(--edge);
		backdrop-filter: var(--glass-blur-player);
		box-shadow: var(--glass-shadow-player);
		overflow: hidden;
	}

	.island.expanded {
		bottom: auto;
		transform: none;
		gap: 14px;
		padding: 16px 20px;
		border-radius: var(--radius-pill);
		background: rgb(10 9 8 / 40%);
		border-color: rgb(255 255 255 / 10%);
		box-shadow: none;
	}

	.now {
		display: flex;
		align-items: center;
		gap: 12px;
		flex: 1;
		min-width: 0;
		min-height: var(--target);
	}

	.thumb {
		position: relative;
		flex: none;
		width: 52px;
		height: 52px;
		border-radius: var(--radius-16);
		overflow: hidden;
	}

	.names {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.song {
		font-size: var(--type-body);
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.artist {
		font-size: 13px;
		color: var(--text-meta);
	}

	.time {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		color: var(--text-meta);
		white-space: nowrap;
	}

	.expanded .time {
		color: var(--wash);
	}

	button {
		flex: none;
		border: 0;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.skip {
		width: var(--target);
		height: var(--target);
		background: transparent;
		color: var(--text);
	}

	.pause {
		width: 52px;
		height: 52px;
		background: var(--fox);
		color: var(--fox-ink);
	}

	.pause.large {
		width: 56px;
		height: 56px;
	}

	.favorite {
		color: var(--accent);
	}

	.progress {
		position: absolute;
		left: 0;
		bottom: 0;
		height: 3px;
		background: var(--fox);
	}

	.transport,
	.scrub {
		display: flex;
		align-items: center;
		gap: 14px;
	}

	.scrub {
		flex: 1;
		min-width: 0;
	}

	.track {
		position: relative;
		flex: 1;
		height: 8px;
		border-radius: 4px;
		background: rgb(255 255 255 / 16%);
	}

	.fill {
		position: absolute;
		inset: 0 auto 0 0;
		border-radius: 4px;
		background: var(--fox);
	}

	@media (max-width: 640px) {
		.island {
			gap: 4px;
		}

		.compact {
			display: none;
		}

		.island.expanded {
			flex-wrap: wrap;
			justify-content: space-between;
			gap: 6px 8px;
			padding: 12px 16px;
			border-radius: var(--radius-32);
		}

		.scrub {
			order: 3;
			flex-basis: 100%;
		}
	}
</style>
