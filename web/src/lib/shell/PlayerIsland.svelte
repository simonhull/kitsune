<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { clock } from '#lib/library/format.ts';
	import type { NowPlaying } from '#lib/library/cue.ts';
	import { morph, type MorphSlot } from '#lib/morph/registry.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import { leftSpoken, tagWords, type Left } from '#lib/player/remember.ts';
	import { repeatLabel } from '#lib/player/repeat.ts';
	import type { Arrival } from '#lib/player/resume.svelte.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { shuffles } from '#lib/shuffle/session.svelte.ts';
	import Icon from './Icon.svelte';
	import { floor } from './floor.svelte.ts';
	import { roving } from './roving.ts';
	import Scrubber from './Scrubber.svelte';
	import StarsInput from './StarsInput.svelte';
	import { islandSlot } from './slot.svelte.ts';

	interface Props {
		playing: NowPlaying;
		/** On Now playing the island becomes the page's pill controls, in place. */
		expanded: boolean;
		/** Songs are picked on the Library list: their bar has the island's place until they aren't. */
		away?: boolean;
		/** How it arrives when it mounts: restored, or by a first play (the default). */
		arrival?: Arrival | null;
		/** Where a remembered record was left, when another browser left it or its song is gone. */
		left?: Left | null;
		/** Told once the arrival has run. */
		onArrived?: () => void;
	}

	let {
		playing,
		expanded,
		away = false,
		arrival = null,
		left = null,
		onArrived = () => undefined
	}: Props = $props();

	/** How long an arrival takes, the wind and the print included. */
	const arrivalMs = 1000;
	/** The wind starts once the island has mostly risen, and counts for this long. */
	const windDelayMs = 260;
	const windMs = 600;
	/** The tag fades on its own this long after it stamps in. */
	const tagLastsMs = 10_000;

	const reduced = (): boolean =>
		typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;

	// The arrival is how it mounted: nothing arrives on Now playing, where it's the page's controls.
	let arriving = $state<Arrival | null>(untrack(() => (expanded ? null : (arrival ?? 'play'))));
	/** The clock's seconds while the line winds out to where it was left. */
	let wound = $state<number | null>(null);

	// The clock is the sound's, not the cue's: a held change must not put them out of step.
	const seconds = $derived((player.current ?? playing.track).durationMs / 1000);
	const position = $derived(player.current === null ? 0 : player.position);
	const percent = $derived(`${String(seconds > 0 ? (position / seconds) * 100 : 0)}%`);
	const slot = $derived(expanded ? islandSlot.rect : null);
	const spoken = $derived(left === null ? undefined : leftSpoken(left, Date.now()));
	/** Where the words a screen reader hears of the record are, under the island. */
	const leftId = $props.id();

	onMount(() => {
		const kind = arriving;
		if (kind === null) {
			return;
		}
		let frame = 0;
		if (kind === 'restore' && !reduced() && position > 0) {
			// The clock counts up with the line, 0:00 to where it was left: ease-out, like the wind.
			const to = position;
			const start = performance.now();
			wound = 0;
			const count = (now: number): void => {
				const p = Math.min(1, Math.max(0, (now - start - windDelayMs) / windMs));
				wound = to * (1 - (1 - p) ** 3);
				frame = p < 1 ? requestAnimationFrame(count) : 0;
				if (p >= 1) {
					wound = null;
				}
			};
			frame = requestAnimationFrame(count);
		}
		const done = setTimeout(() => {
			// A tab that stops painting mid-wind still ends on the right time.
			wound = null;
			arriving = null;
			onArrived();
		}, arrivalMs);
		return () => {
			cancelAnimationFrame(frame);
			clearTimeout(done);
		};
	});

	/** The stamped tag: it stamps in, and goes when the sound starts, or on its own a while later. */
	let tag = $state<{ words: string; phase: 'stamp' | 'drop' | 'fade' } | null>(null);
	let tagTimer: ReturnType<typeof setTimeout> | undefined;

	$effect(() => {
		const now = left;
		untrack(() => {
			clearTimeout(tagTimer);
			if (now === null) {
				return;
			}
			tag = { words: tagWords(now, Date.now()), phase: 'stamp' };
			tagTimer = setTimeout(() => {
				if (tag?.phase === 'stamp') {
					tag = reduced() ? null : { ...tag, phase: 'fade' };
				}
			}, tagLastsMs);
		});
		return () => {
			clearTimeout(tagTimer);
		};
	});

	// Play pressed: the tag drops away at once, into the island.
	$effect(() => {
		if (player.sounding) {
			untrack(() => {
				if (tag?.phase === 'stamp') {
					tag = { ...tag, phase: 'drop' };
				}
			});
		}
	});

	function tagGone(event: AnimationEvent): void {
		if (event.target === event.currentTarget && tag !== null && tag.phase !== 'stamp') {
			tag = null;
		}
	}

	const repeat = $derived(player.repeating);
	const repeatName = $derived(repeatLabel(player.repeat, player.endless));

	/**
	 * One-shot motions, by a press or a key alike: Repeat turns once per change, and Shuffle
	 * scrambles once as it goes on. Each alternates between twin keyframes, so a quick second press
	 * starts it again, and each clears when it ends, so a button mounting later doesn't play it.
	 */
	let turn = $state<'a' | 'b' | null>(null);
	let scramble = $state<'a' | 'b' | null>(null);
	let lastRepeat = untrack(() => player.repeat);
	let lastShuffling = untrack(() => player.shuffling);

	$effect(() => {
		const now = player.repeat;
		untrack(() => {
			if (now !== lastRepeat) {
				turn = turn === 'a' ? 'b' : 'a';
			}
			lastRepeat = now;
		});
	});

	$effect(() => {
		const now = player.shuffling;
		untrack(() => {
			if (now && !lastShuffling) {
				scramble = scramble === 'a' ? 'b' : 'a';
			}
			lastShuffling = now;
		});
	});
</script>

{#snippet pair(glyph: number)}
	<span class="pair">
		<button
			type="button"
			class="ctl shuffle"
			class:on={player.shuffling}
			class:scramble-a={scramble === 'a'}
			class:scramble-b={scramble === 'b'}
			aria-label="Shuffle"
			aria-pressed={player.shuffling}
			data-roam
			onclick={() => {
				shuffles.toggle();
			}}
			onanimationend={(event) => {
				if (event.animationName.includes('scramble')) {
					scramble = null;
				}
			}}
			><Icon name="shuffle" size={glyph} />{#if player.shuffling}<span
					class="lit"
					aria-hidden="true"
				></span>{/if}</button
		>
		<button
			type="button"
			class="ctl repeat"
			class:on={repeat !== 'off'}
			class:turn-a={turn === 'a'}
			class:turn-b={turn === 'b'}
			aria-label={repeatName}
			data-roam
			onclick={() => {
				player.cycleRepeat();
			}}
			onanimationend={(event) => {
				if (event.animationName.includes('turn')) {
					turn = null;
				}
			}}
			><Icon name="repeat" size={glyph} />{#if repeat === 'one'}<span class="one" aria-hidden="true"
					>1</span
				>{/if}{#if repeat !== 'off'}<span class="lit" aria-hidden="true"></span>{/if}</button
		>
	</span>
{/snippet}

{#snippet transport(place: MorphSlot, skip: number | undefined, pause: number)}
	<button
		type="button"
		class="ctl skip previous"
		aria-label="Previous"
		data-roam
		onclick={() => {
			player.previous();
		}}
		use:morph={{ id: 'control:previous', slot: place, kind: 'control' }}
		><Icon name="previous" size={skip} /></button
	>
	<button
		type="button"
		class="pause"
		class:large={expanded}
		aria-label={player.sounding ? 'Pause' : 'Play'}
		data-roam
		onclick={() => {
			player.toggle();
		}}
		use:morph={{ id: 'control:pause', slot: place, kind: 'control' }}
		><Icon name={player.sounding ? 'pause' : 'play'} size={pause} /></button
	>
	<button
		type="button"
		class="ctl skip next"
		aria-label="Next"
		data-roam
		onclick={() => {
			player.next();
		}}
		use:morph={{ id: 'control:next', slot: place, kind: 'control' }}
		><Icon name="next" size={skip} /></button
	>
{/snippet}

<aside
	aria-label="Player"
	aria-describedby={spoken === undefined ? undefined : leftId}
	class="island"
	class:expanded
	class:away
	class:arrive={arriving !== null}
	class:restore={arriving === 'restore'}
	inert={away}
	use:floor={!away && !expanded}
	style:--slot-left={slot === null ? undefined : `${String(slot.left)}px`}
	style:--slot-top={slot === null ? undefined : `${String(slot.top)}px`}
	style:--slot-width={slot === null ? undefined : `${String(slot.width)}px`}
>
	<!-- A hard print, off register, flashes once under the glass as it lands. -->
	{#if arriving === 'play'}<span class="print" aria-hidden="true"></span>{/if}
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
	<!-- One Tab stop: ← and → go along it (roving.ts). -->
	<div class="content" data-island-content role="toolbar" aria-label="Player controls" use:roving>
		{#if expanded}
			<div class="transport">{@render pair(20)}{@render transport('poster', 22, 22)}</div>
			<Scrubber
				{position}
				duration={seconds}
				live={player.started}
				seek={(to) => {
					player.seek(to);
				}}
			/>
		{:else}
			<a class="now" href={resolve('/now-playing')} data-roam>
				<!-- Keyed apart: a change of track mustn't remount the cover it shares. -->
				{#key playing.album.id}
					<span class="cover">
						<span
							class="thumb"
							use:morph={{ id: `cover:${playing.album.id}`, slot: 'island', kind: 'cover' }}
							><Cover albumId={playing.album.id} entry={playing.album} place="small" /><span
								class="up"
								aria-hidden="true"><Icon name="chevron-up" size={18} /></span
							></span
						>
					</span>
				{/key}
				{#key playing.track.id}
					<span class="names">
						<span class="song" use:morph={{ id: 'song', slot: 'island', kind: 'title' }}
							>{playing.track.title}</span
						>
						<span class="artist">{playing.album.artist}</span>
					</span>
				{/key}
			</a>
			<span class="time compact">{clock(wound ?? position)} / {clock(seconds)}</span>
			<!-- Where the heart was: the song's stars, the same control Now playing has. -->
			<StarsInput
				class="rating"
				size="island"
				data-roam
				value={library.marks.stars(playing.track.id)}
				label={`Rating of ${playing.track.title}`}
				onchange={(stars) => {
					library.marks.rate(playing.track.id, stars);
				}}
			/>
			<div class="controls">{@render pair(19)}{@render transport('island', 20, 20)}</div>
		{/if}
	</div>
	{#if spoken !== undefined}<span id={leftId} class="visually-hidden">{spoken}</span>{/if}
	{#if tag !== null && !expanded}
		<span
			class="tag"
			class:stamp={tag.phase === 'stamp'}
			class:drop={tag.phase === 'drop'}
			class:fade={tag.phase === 'fade'}
			aria-hidden="true"
			onanimationend={tagGone}>{tag.words}</span
		>
	{/if}
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
		width: min(820px, calc(100% - 32px));
	}

	/* The selection bar has its place: it sinks out of the way, and comes back the same way. */
	.island.away {
		opacity: 0;
		transform: translate(-50%, 24px);
		pointer-events: none;
		transition:
			opacity var(--morph-fast) var(--curve-morph),
			transform var(--morph-fast) var(--curve-morph);
	}

	.island:not(.away, .expanded) {
		transition:
			opacity var(--morph-fast) var(--curve-morph),
			transform var(--morph-fast) var(--curve-settle);
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
		background: color-mix(in srgb, var(--ground) 40%, transparent);
		border-color: var(--edge-raised);
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

	/* A new album arrives at rest, in place: no morph carries this one. */
	.thumb,
	.compact {
		animation: cue-in var(--morph-fast) var(--curve-morph);
	}

	@keyframes cue-in {
		from {
			opacity: 0;
		}
	}

	/* A new track is a needle drop: it lands in three hard steps, and never over the art. */
	.names {
		animation: needle-drop 120ms steps(3, jump-end);
	}

	@keyframes needle-drop {
		from {
			opacity: 0;
			translate: 0 -6px;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.thumb,
		.names,
		.compact {
			animation: cue-in 120ms linear;
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

	/* The cover's own box: it lifts on hover and wears the focus print, outside the art's clip. */
	.cover {
		position: relative;
		flex: none;
		border-radius: var(--radius-16);
		transition:
			translate 200ms var(--curve-settle),
			rotate 200ms var(--curve-settle);
	}

	.thumb {
		position: relative;
		display: block;
		width: 52px;
		height: 52px;
		border-radius: var(--radius-16);
		overflow: hidden;
	}

	/* Where the cover goes: a veil of ground, and an arrow up into Now playing. */
	.up {
		position: absolute;
		inset: 0;
		display: grid;
		place-items: center;
		background: color-mix(in srgb, var(--ground) 45%, transparent);
		color: var(--text);
		opacity: 0;
		transition: opacity 120ms ease-out;
	}

	.now:focus-visible {
		outline: none;
	}

	@media (hover: hover) {
		.now:hover .cover {
			translate: 0 -2px;
			rotate: -2deg;
		}

		.now:hover .up {
			opacity: 1;
		}

		.now:hover .up :global(svg) {
			animation: nudge 600ms ease-in-out infinite alternate;
		}
	}

	@keyframes nudge {
		to {
			translate: 0 -2px;
		}
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

	button {
		flex: none;
		border: 0;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	/* The compact island's controls: the pair, 10 px, then the transport, as the board has them. */
	.controls,
	.pair {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.pair {
		margin-right: 10px;
	}

	/*
	 * The round controls: Shuffle and Repeat, Previous and Next. Off is the secondary ink, on the
	 * fox's words with a fox dot under the glyph.
	 */
	.ctl {
		position: relative;
		isolation: isolate;
		width: var(--target);
		height: var(--target);
		padding: 0;
		background: transparent;
		color: var(--text-secondary);
		transition: color 120ms ease-out;
	}

	.skip {
		color: var(--text);
	}

	.ctl.on {
		color: var(--fox-text);
	}

	/* A wash disc behind the glyph: it comes up on hover, stronger and smaller pressed. */
	.ctl::before {
		content: '';
		position: absolute;
		inset: 2px;
		z-index: -1;
		border-radius: 50%;
		background: var(--control-wash);
		opacity: 0;
		scale: 0.6;
		transition:
			scale var(--morph-room) var(--curve-room),
			opacity 120ms ease-out;
	}

	.ctl :global(svg) {
		transition:
			translate var(--morph-room) var(--curve-settle),
			rotate var(--morph-room) var(--curve-settle),
			scale var(--morph-room) var(--curve-settle);
	}

	.lit {
		position: absolute;
		bottom: 4px;
		left: 50%;
		width: 4px;
		height: 4px;
		margin-left: -2px;
		border-radius: 50%;
		background: var(--fox);
	}

	/* Repeat this song: a small fox badge with a 1, at the glyph's shoulder. */
	.one {
		position: absolute;
		top: 6px;
		right: 5px;
		width: 13px;
		height: 13px;
		display: grid;
		place-items: center;
		border-radius: 50%;
		background: var(--fox);
		color: var(--fox-ink);
		font-family: var(--font-mono);
		font-size: 9px;
		font-weight: 700;
		line-height: 1;
	}

	.pause {
		position: relative;
		width: 52px;
		height: 52px;
		background: var(--fox);
		color: var(--fox-ink);
		transition:
			translate 200ms var(--curve-settle),
			scale 200ms var(--curve-settle),
			box-shadow 200ms ease-out;
	}

	.pause.large {
		width: 56px;
		height: 56px;
	}

	/* Hover only where something hovers: a tap on a phone mustn't leave a wash behind. */
	@media (hover: hover) {
		.ctl:hover {
			color: var(--text);
		}

		.ctl.on:hover {
			color: var(--fox-text);
		}

		.ctl:hover::before {
			opacity: 1;
			scale: 1;
		}

		/* Each glyph says what it does: the skips lean their way, Repeat turns, Shuffle widens. */
		.previous:hover :global(svg) {
			translate: -2px 0;
		}

		.next:hover :global(svg) {
			translate: 2px 0;
		}

		.repeat:hover :global(svg) {
			rotate: -24deg;
		}

		.shuffle:hover :global(svg) {
			scale: 1.12 1;
		}

		/* Play lifts, and a fox halo comes up round it. */
		.pause:hover {
			translate: 0 -1px;
			scale: 1.05;
			box-shadow:
				0 0 0 5px color-mix(in srgb, var(--fox) 22%, transparent),
				0 8px 22px -6px color-mix(in srgb, var(--fox) 70%, transparent);
		}
	}

	/* Pressed: a short squash, 60 ms in, released on the settle curve. */
	.ctl:active::before {
		opacity: 1;
		scale: 0.92;
		background: var(--control-wash-strong);
		transition-duration: 60ms;
	}

	.ctl:active :global(svg) {
		scale: 0.86;
		transition-duration: 60ms;
	}

	.pause:active {
		translate: 0 0;
		scale: 0.92;
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--fox) 30%, transparent);
		transition-duration: 60ms;
	}

	/* Repeat turns once per change; Shuffle going on scrambles once, in four hard steps. */
	.repeat.turn-a :global(svg) {
		animation: turn-a 360ms var(--curve-settle);
	}

	.repeat.turn-b :global(svg) {
		animation: turn-b 360ms var(--curve-settle);
	}

	.shuffle.scramble-a :global(svg) {
		animation: scramble-a 280ms steps(4, jump-end);
	}

	.shuffle.scramble-b :global(svg) {
		animation: scramble-b 280ms steps(4, jump-end);
	}

	@keyframes turn-a {
		from {
			rotate: -360deg;
		}
	}

	@keyframes turn-b {
		from {
			rotate: -360deg;
		}
	}

	@keyframes scramble-a {
		25% {
			translate: -2px 1px;
			scale: -1 1;
		}

		50% {
			translate: 2px -1px;
		}

		75% {
			translate: -1px 0;
			scale: -1 1;
		}
	}

	@keyframes scramble-b {
		25% {
			translate: -2px 1px;
			scale: -1 1;
		}

		50% {
			translate: 2px -1px;
		}

		75% {
			translate: -1px 0;
			scale: -1 1;
		}
	}

	/*
	 * Keyboard focus is printed off register, the island's arrival print in small: a fox ring 3 px
	 * out, and a teal copy a pixel and a half down and right. It lands in three hard steps, and
	 * never fades. A click never shows it.
	 */
	.ctl:focus-visible,
	.pause:focus-visible {
		outline: none;
	}

	.ctl:focus-visible {
		color: var(--text);
	}

	.ctl.on:focus-visible {
		color: var(--fox-text);
	}

	.ctl:focus-visible::after,
	.pause:focus-visible::after,
	.now:focus-visible .cover::after {
		content: '';
		position: absolute;
		inset: -3px;
		border-radius: inherit;
		box-shadow:
			0 0 0 2px var(--fox-line),
			1.5px 1.5px 0 2px color-mix(in srgb, var(--misprint) 55%, transparent);
		pointer-events: none;
		animation: print-land 180ms steps(3, jump-end);
	}

	.ctl:focus-visible::after,
	.pause:focus-visible::after {
		border-radius: 50%;
	}

	.now:focus-visible .cover::after {
		border-radius: calc(var(--radius-16) + 3px);
	}

	@keyframes print-land {
		from {
			opacity: 0;
			scale: 1.25;
		}
	}

	/* Reduced motion: no glyph moves and nothing plays; the wash, the colors, the halo and the ring stay. */
	@media (prefers-reduced-motion: reduce) {
		.ctl :global(svg),
		.ctl:hover :global(svg),
		.ctl:active :global(svg),
		.pause,
		.pause:hover,
		.pause:active,
		.cover,
		.now:hover .cover,
		.ctl::before,
		.ctl:hover::before,
		.ctl:active::before {
			translate: none;
			rotate: none;
			scale: none;
		}

		.repeat.turn-a :global(svg),
		.repeat.turn-b :global(svg),
		.shuffle.scramble-a :global(svg),
		.shuffle.scramble-b :global(svg),
		.now:hover .up :global(svg),
		.ctl:focus-visible::after,
		.pause:focus-visible::after,
		.now:focus-visible .cover::after {
			animation: none;
		}
	}

	.progress {
		position: absolute;
		left: 0;
		bottom: 0;
		height: 3px;
		background: var(--fox);
	}

	/* First play: it rises off the floor and lands; a hard print flashes once under the glass. */
	.island.arrive {
		animation: island-rise var(--morph) var(--curve-settle) backwards;
	}

	.island.arrive .content {
		animation: content-land var(--morph-fast) var(--curve-morph) 80ms backwards;
	}

	.island.arrive .names {
		animation: needle-drop 120ms steps(3, jump-end) 120ms backwards;
	}

	.print {
		position: absolute;
		inset: 0;
		border-radius: var(--island-radius);
		border: 2px solid var(--lift-offset);
		translate: 4px 4px;
		opacity: 0;
		pointer-events: none;
		animation: print-flash 520ms var(--curve-morph) 200ms;
	}

	@keyframes island-rise {
		from {
			opacity: 0;
			translate: 0 40px;
			scale: 0.94;
		}

		40% {
			opacity: 1;
		}
	}

	@keyframes content-land {
		from {
			opacity: 0;
		}
	}

	@keyframes print-flash {
		0% {
			opacity: 0;
		}

		35% {
			opacity: 0.9;
		}

		100% {
			opacity: 0;
			translate: 0 0;
		}
	}

	/* Restored: the same rise, then the line winds out to where it was left as the clock counts. */
	.island.restore .progress {
		animation: wind var(--morph-slow) var(--curve-room) 260ms backwards;
	}

	@keyframes wind {
		from {
			width: 0;
		}
	}

	/* Where it was left: a stamped tag on the island's shoulder. */
	.tag {
		position: absolute;
		left: 22px;
		bottom: calc(100% - 12px);
		z-index: 5;
		height: 26px;
		padding: 0 10px;
		display: flex;
		align-items: center;
		border-radius: var(--radius-pill);
		background: var(--ground);
		border: 2px solid var(--fox-line);
		color: var(--fox-text);
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.08em;
		white-space: nowrap;
		rotate: -2deg;
		transform-origin: 0 100%;
		pointer-events: none;
	}

	.tag::after {
		content: '';
		position: absolute;
		inset: 0;
		border-radius: inherit;
		background-image: var(--grain);
		background-size: 160px;
		opacity: calc(var(--grain-alpha) * 2);
		pointer-events: none;
	}

	.tag.stamp {
		animation: tag-stamp var(--morph-fast) var(--curve-settle) 520ms backwards;
	}

	.tag.drop {
		animation: tag-drop var(--morph-drop) var(--curve-fall) forwards;
	}

	.tag.fade {
		animation: tag-fade var(--morph-slow) var(--curve-morph) forwards;
	}

	@keyframes tag-stamp {
		from {
			opacity: 0;
			scale: 1.5;
			rotate: -12deg;
		}
	}

	@keyframes tag-drop {
		to {
			opacity: 0;
			translate: 0 14px;
			rotate: 3deg;
		}
	}

	@keyframes tag-fade {
		to {
			opacity: 0;
		}
	}

	/* Reduced motion: opacity only, with the line and the clock already in place. */
	@media (prefers-reduced-motion: reduce) {
		.island.arrive {
			animation: content-land 120ms linear backwards;
		}

		.island.arrive .content,
		.island.arrive .names,
		.island.restore .progress,
		.print {
			animation: none;
		}

		.tag.stamp {
			animation: content-land 120ms linear 120ms backwards;
		}

		.tag.drop {
			animation: tag-fade 120ms linear forwards;
		}
	}

	.transport {
		display: flex;
		align-items: center;
		gap: 14px;
	}

	/* Now playing's pill starts with the pair, then 12 px, then the transport. */
	.transport .pair {
		gap: 14px;
		margin-right: 12px;
	}

	/* The compact island keeps the pair down to 900 wide; below that, Now playing has it. */
	@media (max-width: 900px) {
		.controls .pair {
			display: none;
		}
	}

	@media (max-width: 640px) {
		/* The scrubber across the top, then the controls centered: the pair, 14 px, the transport. */
		.expanded .transport {
			flex-basis: 100%;
			justify-content: center;
			gap: 6px;
		}

		.expanded .transport .pair {
			gap: 6px;
			margin-right: 14px;
		}

		.expanded .content :global(.scrub) {
			order: -1;
		}
		.content {
			gap: 4px;
		}

		.compact {
			display: none;
		}

		/* A phone's island has no room to rate in: Now playing does it, at a finger's size. */
		.content :global(.rating) {
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
	}
</style>
