<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { clock } from '#lib/library/format.ts';
	import type { NowPlaying } from '#lib/library/cue.ts';
	import { morph } from '#lib/morph/registry.ts';
	import Icon from './Icon.svelte';
	import { islandSlot } from './slot.svelte.ts';

	interface Props {
		playing: NowPlaying;
		/** On Now playing the island becomes the page's pill controls, in place. */
		expanded: boolean;
	}

	let { playing, expanded }: Props = $props();

	// Cued, not playing: until plan 018 there's no sound, so the clock stands at the start.
	const position = clock(0);
	const duration = $derived(clock(playing.track.durationMs / 1000));
	const slot = $derived(expanded ? islandSlot.rect : null);
	const percent = '0%';
</script>

<aside
	aria-label="Player"
	class="island"
	class:expanded
	style:--slot-left={slot === null ? undefined : `${String(slot.left)}px`}
	style:--slot-top={slot === null ? undefined : `${String(slot.top)}px`}
	style:--slot-width={slot === null ? undefined : `${String(slot.width)}px`}
>
	<!-- The glass is its own layer: in flight it changes shape while the island's box is final. -->
	<div class="glass" data-island-glass>
		{#if !expanded}
			<span
				class="progress"
				style:width={percent}
				aria-hidden="true"
				use:morph={{ id: 'progress', slot: 'island', kind: 'bar' }}
			></span>
		{/if}
	</div>
	<div class="content" data-island-content>
		{#if expanded}
			<div class="transport">
				<button
					type="button"
					class="skip"
					aria-label="Previous"
					disabled
					use:morph={{ id: 'control:previous', slot: 'poster', kind: 'control' }}
					><Icon name="previous" size={22} /></button
				>
				<button
					type="button"
					class="pause large"
					aria-label="Play"
					disabled
					use:morph={{ id: 'control:pause', slot: 'poster', kind: 'control' }}
					><Icon name="play" size={22} /></button
				>
				<button
					type="button"
					class="skip"
					aria-label="Next"
					disabled
					use:morph={{ id: 'control:next', slot: 'poster', kind: 'control' }}
					><Icon name="next" size={22} /></button
				>
			</div>
			<div class="scrub">
				<span class="time">{position}</span>
				<div class="track">
					<div
						class="fill"
						style:width={percent}
						use:morph={{ id: 'progress', slot: 'poster', kind: 'bar' }}
					></div>
				</div>
				<span class="time">{duration}</span>
			</div>
			<button
				type="button"
				class="skip favorite"
				aria-label="Favorite"
				aria-pressed="false"
				disabled><Icon name="heart-outline" size={20} /></button
			>
		{:else}
			{#key playing.track.id}
				<a class="now" href={resolve('/now-playing')}>
					<span
						class="thumb"
						use:morph={{ id: `cover:${playing.album.id}`, slot: 'island', kind: 'cover' }}
						><Cover albumId={playing.album.id} entry={playing.album} place="small" /></span
					>
					<span class="names">
						<span class="song" use:morph={{ id: 'song', slot: 'island', kind: 'title' }}
							>{playing.track.title}</span
						>
						<span class="artist">{playing.album.artist}</span>
					</span>
				</a>
				<span class="time compact">{position} / {duration}</span>
			{/key}
			<button
				type="button"
				class="skip"
				aria-label="Previous"
				disabled
				use:morph={{ id: 'control:previous', slot: 'island', kind: 'control' }}
				><Icon name="previous" /></button
			>
			<button
				type="button"
				class="pause"
				aria-label="Play"
				disabled
				use:morph={{ id: 'control:pause', slot: 'island', kind: 'control' }}
				><Icon name="play" size={20} /></button
			>
			<button
				type="button"
				class="skip"
				aria-label="Next"
				disabled
				use:morph={{ id: 'control:next', slot: 'island', kind: 'control' }}
				><Icon name="next" /></button
			>
		{/if}
	</div>
</aside>

<style>
	.island {
		--island-radius: var(--radius-28);
		--island-radius-px: 28;

		position: fixed;
		left: 50%;
		bottom: calc(24px + env(safe-area-inset-bottom));
		transform: translateX(-50%);
		z-index: 4;
		width: min(720px, calc(100% - 32px));
	}

	.island.expanded {
		--island-radius: var(--radius-pill);
		--island-radius-px: 999;

		left: var(--slot-left);
		top: var(--slot-top);
		bottom: auto;
		width: var(--slot-width);
		transform: none;
	}

	.glass {
		position: absolute;
		inset: 0;
		overflow: hidden;
		border-radius: var(--island-radius);
		background: var(--glass-fill-player);
		border: 1px solid var(--edge);
		backdrop-filter: var(--glass-blur-player);
		box-shadow: var(--glass-shadow-player);
		transition:
			background var(--morph) var(--curve-morph),
			border-color var(--morph) var(--curve-morph),
			box-shadow var(--morph) var(--curve-morph);
	}

	.expanded .glass {
		background: rgb(10 9 8 / 40%);
		border-color: rgb(255 255 255 / 10%);
		box-shadow: none;
	}

	.content {
		position: relative;
		display: flex;
		align-items: center;
		gap: 16px;
		padding: 11px 13px 11px 11px;
	}

	.expanded .content {
		gap: 14px;
		padding: 17px 21px;
	}

	/* A new cue arrives at rest, in place: this is the one change here no morph carries. */
	.now,
	.compact {
		animation: cue-in var(--morph-fast) var(--curve-morph);
	}

	@keyframes cue-in {
		from {
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.now,
		.compact {
			animation-duration: 120ms;
		}
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
		overflow: hidden;
		font-size: 13px;
		color: var(--text-meta);
		text-overflow: ellipsis;
		white-space: nowrap;
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

	/* Nothing can sound until plan 018: the controls say so without leaving their places. */
	button:disabled {
		cursor: default;
	}

	.skip:disabled {
		color: var(--text-label);
	}

	.pause:disabled {
		background: transparent;
		box-shadow: inset 0 0 0 2px var(--fox);
		color: var(--fox);
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
		.content {
			gap: 4px;
		}

		.compact {
			display: none;
		}

		.island.expanded {
			--island-radius: var(--radius-32);
			--island-radius-px: 32;
		}

		.expanded .content {
			flex-wrap: wrap;
			justify-content: space-between;
			gap: 6px 8px;
			padding: 13px 17px;
		}

		.scrub {
			order: 3;
			flex-basis: 100%;
		}
	}
</style>
