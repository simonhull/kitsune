<!--
	The one song row, in every list of songs in the app: props in, events out. It has no effects or
	stores of its own, so the Library's 50,000 songs cost what they did. A list turns its parts on or
	off (the handle, the cover, the number, the columns, a second line) and says what each event does;
	the row's own logic is in songRow.ts, beside it.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { trackLabel } from '#lib/library/format.ts';
	import type { ColumnId, LibraryTrack, MovableId } from '#lib/library/source.ts';
	import HeartButton from '#lib/shell/HeartButton.svelte';
	import Icon from '#lib/shell/Icon.svelte';
	import {
		cellOf,
		fitLine,
		leadingCount,
		lineupOf,
		phoneLine,
		timeOf,
		type RowAlbum,
		type RowParts
	} from './columns.ts';
	import PickBox from './PickBox.svelte';

	interface Props {
		track: LibraryTrack;
		/** The album it's on, for its cover and the cells that name it. */
		album: RowAlbum;
		/** Under 700: two lines a song, and no columns. */
		phone: boolean;
		parts: RowParts;
		/**
		 * What the row places, in order: the number, the title and the columns shown. Their grid is
		 * the list's (songGrid.ts), which the header shares.
		 */
		shown: readonly MovableId[];
		/** The person's columns, which a phone's second line is made from. */
		order: readonly MovableId[];
		artistWhenDifferent: boolean;
		/** How many characters a phone's second line has room for. */
		lineChars: number;
		/** Whether a column says the same for every song of the album: a phone's line leaves it out. */
		sameOnAlbum?: (id: ColumnId) => boolean;
		/** Who else is on it, after the title, as Up next says it. */
		credit?: string | undefined;
		/** A line under the title, in place of a phone's columns: Up next's why. */
		second?: Snippet | undefined;
		/** Where it stands in a list that places its rows, and how tall it is. */
		top?: number | undefined;
		height?: number | undefined;
		/** When "Yesterday" is, for Last played. */
		now: number;
		stars: number | null;
		hearted: boolean;
		/** It's the song cued, and whether it's sounding now. */
		cued: boolean;
		sounding: boolean;
		picked: boolean;
		/** Its combined run goes on above it and below it, joined by one bracket. */
		joins?: { up: boolean; down: boolean } | undefined;
		/** The handle pressed to drag it, or Alt with an arrow on it: a queue's. */
		ondrag?: ((trackId: string, event: PointerEvent) => void) | undefined;
		onmove?: ((trackId: string, by: -1 | 1) => void) | undefined;
	}

	let {
		track,
		album,
		phone,
		parts,
		shown,
		order,
		artistWhenDifferent,
		lineChars,
		sameOnAlbum,
		credit,
		second,
		top,
		height,
		now,
		stars,
		hearted,
		cued,
		sounding,
		picked,
		joins,
		ondrag,
		onmove
	}: Props = $props();

	const context = $derived({ track, album, stars, hearted, artistWhenDifferent, now });
	const mark = $derived(track.shuffle);
	const lineup = $derived(lineupOf(shown, parts));
	/** The title's own column, which a second line sits in. */
	const titleColumn = $derived(leadingCount(parts) + lineup.indexOf('title') + 1);
</script>

{#snippet marks()}
	{#if mark?.run !== undefined}
		<span class="mark combined">COMBINED · {mark.position} OF {mark.length}</span>
	{/if}
	{#if mark?.excluded === true}
		<span class="mark excluded"
			><svg width="11" height="11" viewBox="0 0 24 24" aria-hidden="true"
				><path d="M4 4l16 16M3 7h3.5c2 0 3.2 1 4.3 2.6M21 17h-3.5c-1.4 0-2.4-.4-3.2-1.2"
				></path></svg
			>EXCLUDED</span
		>
	{/if}
{/snippet}

{#snippet titled()}
	<span class="titled"
		><span class="name">{track.title}</span>{#if credit !== undefined}<span class="credit"
				>· {credit}</span
			>{/if}{@render marks()}</span
	>
{/snippet}

{#snippet number()}
	<span class="mono number" data-col="number">
		{#if cued && sounding}
			<Icon name="play" size={12} />
		{:else}
			{trackLabel(track, false)}
		{/if}
	</span>
{/snippet}

<!-- The title is the row's one control for a screen reader and the keyboard. -->
{#snippet play()}
	<button
		type="button"
		class="play"
		data-col="title"
		aria-label={`Play ${track.title}`}
		aria-current={cued ? 'true' : undefined}
	>
		{#if phone && second === undefined}
			<span class="lines">
				{@render titled()}
				<span class="line"
					><span class="parts"
						>{fitLine(
							phoneLine(order, context, sameOnAlbum ?? (() => false)),
							timeOf(track),
							lineChars
						).join(' · ')}</span
					><span class="line-time">{timeOf(track)}</span></span
				>
			</span>
		{:else}
			{@render titled()}
		{/if}
	</button>
{/snippet}

{#snippet column(id: ColumnId)}
	{@const cell = cellOf(id, context)}
	{#if cell.look === 'stars'}
		<span class="rate" data-col={id} role="group" aria-label={`Rate ${track.title}`}>
			{#each [1, 2, 3, 4, 5] as star (star)}
				<button
					type="button"
					class:on={star <= cell.stars}
					data-star={star}
					aria-label={`Rate ${track.title} ${String(star)} of 5`}
					aria-pressed={star <= cell.stars}>★</button
				>
			{/each}
		</span>
	{:else if cell.look === 'heart'}
		<span class="heart-cell" data-col={id}
			><HeartButton kind="tracks" id={track.id} name={track.title} size={16} /></span
		>
	{:else if id === 'album' && cell.text !== ''}
		<span class="cell name" data-col={id}
			><a class="link" href={resolve('/album/[id]', { id: album.id })}>{cell.text}</a></span
		>
	{:else if cell.measured === true}
		<span data-col={id} class="cell fact end" title="Measured by Kitsune"
			><span class="approx" aria-hidden="true">≈</span><span class="visually-hidden"
				>measured
			</span>{cell.text}</span
		>
	{:else}
		<span
			data-col={id}
			class={['cell', cell.look]}
			class:end={id === 'plays' || id === 'bitrate' || id === 'gain'}
			class:warn={cell.warn === true}>{cell.text}</span
		>
	{/if}
{/snippet}

<!--
	A click anywhere on the row plays it, through its list's one handler (songRow.ts's rowEvents).
	The title's button is the row's one control for a screen reader and the keyboard; the rest of
	the row is a pointer's convenience.
-->
<div
	class="row"
	class:two={phone}
	class:placed={top !== undefined}
	class:lined={second !== undefined}
	class:cued
	class:picked
	role="group"
	aria-label={track.title}
	data-track={track.id}
	data-album={album.id}
	style:top={top === undefined ? undefined : `${String(top)}px`}
	style:height={height === undefined ? undefined : `${String(height)}px`}
>
	{#if joins !== undefined && mark?.run !== undefined}
		<span class="bracket" class:start={!joins.up} class:end={!joins.down} aria-hidden="true"></span>
	{/if}
	{#if parts.handle}
		<button
			type="button"
			class="handle"
			data-own-click
			aria-label={`Move ${track.title}. Alt with an arrow moves it.`}
			onpointerdown={(event) => {
				ondrag?.(track.id, event);
			}}
			onkeydown={(event) => {
				if (event.altKey && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) {
					event.preventDefault();
					onmove?.(track.id, event.key === 'ArrowUp' ? -1 : 1);
				}
			}}
			><svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true"
				><circle cx="9" cy="6" r="1.6"></circle><circle cx="15" cy="6" r="1.6"></circle><circle
					cx="9"
					cy="12"
					r="1.6"
				></circle><circle cx="15" cy="12" r="1.6"></circle><circle cx="9" cy="18" r="1.6"
				></circle><circle cx="15" cy="18" r="1.6"></circle></svg
			></button
		>
	{/if}
	<span class="box pick-gutter" data-own-click
		><PickBox state={picked ? 'all' : 'none'} label={`Select ${track.title}`} /></span
	>
	{#if parts.cover}
		<span class="cover"><Cover albumId={album.id} entry={album} place="small" lazy /></span>
	{/if}
	{#if phone}
		{#if parts.number}{@render number()}{/if}
		{@render play()}
	{:else}
		<!-- In the person's order: the number and the title move like any column. -->
		{#each lineup as id (id)}
			{#if id === 'number'}
				{@render number()}
			{:else if id === 'title'}
				{@render play()}
			{:else}
				{@render column(id)}
			{/if}
		{/each}
	{/if}
	{#if second !== undefined}
		<span class="second" style:grid-column={phone ? undefined : String(titleColumn)}
			>{@render second()}</span
		>
	{/if}
	{#if !phone || second !== undefined}<span class="mono time">{timeOf(track)}</span>{/if}
	<button type="button" class="more" aria-label={`More for ${track.title}`} aria-haspopup="menu"
		><svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"
			><circle cx="5" cy="12" r="1.8"></circle><circle cx="12" cy="12" r="1.8"></circle><circle
				cx="19"
				cy="12"
				r="1.8"
			></circle></svg
		></button
	>
</div>

<style>
	/* The grid is the header's: every cell sits under its label. */
	.row {
		--heart-on: var(--star);
		--heart-off: var(--star-off);

		position: relative;
		box-sizing: border-box;
		min-width: 0;
		height: var(--song-row, 44px);
		display: grid;
		/* Its columns are the list's grid, a rule of songGrid.ts's. */
		align-items: center;
		column-gap: var(--col-gap, 12px);
		padding: 0 var(--row-pad, 14px);
		border-bottom: 1px solid var(--edge-card);
		font-size: var(--type-body);
		cursor: pointer;
		/* A Shift-click takes songs, not words. */
		user-select: none;
	}

	/* In a list that places every row itself, as the Library's does. */
	.row.placed {
		position: absolute;
		left: 0;
		right: 0;
	}

	/*
	 * The title's cell: the row's one button, which the keyboard and a screen reader use. A press
	 * anywhere else on the row plays the song too, through the list's handler.
	 */
	.play {
		min-width: 0;
		height: 100%;
		display: flex;
		align-items: center;
		padding: 0;
		border: 0;
		background: transparent;
		color: inherit;
		font: inherit;
		text-align: left;
	}

	.cover {
		position: relative;
		display: block;
		width: var(--song-cover, 40px);
		height: var(--song-cover, 40px);
		border-radius: 8px;
		overflow: hidden;
		box-shadow: 0 0 0 1px var(--cover-edge);
	}

	.titled {
		display: flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
	}

	.titled .name {
		flex: 0 1 auto;
		min-width: min(9ch, 100%);
	}

	.titled:has(.credit) .name {
		min-width: 0;
	}

	.credit {
		flex: 0 100 auto;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		margin-left: -4px;
		font-weight: 400;
		color: var(--text-secondary);
	}

	/* A narrow title column squeezes the marks before the title. */
	.mark {
		flex: 0 100 auto;
		min-width: 0;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		display: flex;
		align-items: center;
		gap: 5px;
		padding: 3px 7px;
		border-radius: var(--radius-pill);
		font-family: var(--font-mono);
		font-size: 10px;
		font-weight: 400;
		letter-spacing: 0.08em;
		line-height: 1;
	}

	.mark.combined {
		background: color-mix(in srgb, var(--fox) 16%, transparent);
		color: var(--fox-text);
	}

	.mark.excluded {
		border: 1px solid var(--edge-strong);
		color: var(--text-meta);
	}

	.mark svg {
		fill: none;
		stroke: currentColor;
		stroke-width: 2.4;
		stroke-linecap: round;
	}

	/* Combined rows next to each other, in their run's order, are joined by one bracket. */
	.bracket {
		position: absolute;
		left: calc(var(--row-pad, 14px) + var(--box-size, 16px) + 2px);
		top: 0;
		bottom: -1px;
		width: 7px;
		border-left: 2px solid var(--fox-line);
		pointer-events: none;
	}

	.bracket.start {
		top: 30%;
		border-top: 2px solid var(--fox-line);
		border-top-left-radius: 8px;
	}

	.bracket.end {
		bottom: 30%;
		border-bottom: 2px solid var(--fox-line);
		border-bottom-left-radius: 8px;
	}

	.name {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-weight: 600;
	}

	.cell {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.cell.name {
		font-size: var(--cell-type, 14px);
		font-weight: 400;
		color: var(--text-secondary);
	}

	.mono,
	.cell.fact {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		font-variant-numeric: tabular-nums;
		color: var(--text-meta);
	}

	.cell.warn {
		color: var(--fox-text);
	}

	/* A gain Kitsune measured: one quiet mark before the value, which reads as a tagged one. */
	.approx {
		color: var(--text-label);
	}

	.end,
	.time {
		text-align: right;
	}

	.number {
		display: flex;
		align-items: center;
	}

	.rate {
		display: flex;
		font-size: var(--star-type, 13px);
		letter-spacing: 1px;
	}

	.rate button {
		width: calc(var(--star-type, 13px) + 2px);
		height: 28px;
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--star-off);
		font: inherit;
		line-height: 1;
	}

	.rate button.on {
		color: var(--star);
	}

	.rate button:hover,
	.rate:hover button:has(~ button:hover) {
		color: color-mix(in srgb, var(--star) 70%, transparent);
	}

	.heart-cell {
		display: flex;
		justify-content: center;
	}

	.heart-cell :global(.heart) {
		width: 28px;
		height: 28px;
	}

	/* A queue's handle: half there at rest, whole on hover, gone on the song playing. */
	.handle {
		width: var(--box-size, 16px);
		height: 100%;
		display: grid;
		place-items: center;
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--text-meta);
		opacity: 0.5;
		cursor: grab;
		touch-action: none;
	}

	.handle circle {
		fill: currentColor;
	}

	.row.cued .handle {
		visibility: hidden;
	}

	/*
	 * The gutter, where the box is: the whole column, from the row's edge and top to bottom, so
	 * passing down it shows each box in turn. PickBox says when its box shows; a row shows its own on
	 * focus too.
	 */
	.box {
		align-self: stretch;
		display: flex;
		align-items: center;
		margin-left: calc(-1 * var(--row-pad, 14px));
		padding-left: var(--row-pad, 14px);
		cursor: default;
	}

	.row:focus-within .box :global(.pick) {
		opacity: 1;
	}

	.link:hover {
		color: var(--fox-text);
		text-decoration: underline;
	}

	.more {
		width: var(--more-size, 36px);
		height: var(--more-size, 36px);
		display: grid;
		place-items: center;
		padding: 0;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		color: inherit;
		opacity: 0;
	}

	.more svg {
		fill: currentColor;
	}

	.more:hover {
		background: var(--pressed);
	}

	.row:focus-within .more,
	.row:focus-within .handle,
	.row.cued .more {
		opacity: 1;
	}

	.row:focus-within {
		border-radius: var(--radius-12);
	}

	.row.picked {
		border-radius: var(--radius-12);
		background: color-mix(in srgb, var(--fox) 16%, transparent);
	}

	@media (hover: hover) {
		.row:hover {
			border-radius: var(--radius-12);
			background: var(--hover);
		}

		.row:hover .more,
		.row:hover .handle {
			opacity: 1;
		}

		.row.picked:hover {
			background: color-mix(in srgb, var(--fox) 22%, transparent);
		}
	}

	/* Nothing hovers on touch: ⋯ is always there. */
	@media (hover: none) {
		.more {
			opacity: 1;
		}
	}

	/* Playing is the frame's amber, in every list: amber means playback. */
	.row.cued,
	.row.cued:hover {
		--heart-on: var(--fox-ink);
		--heart-off: color-mix(in srgb, var(--fox-ink) 40%, transparent);

		border-radius: var(--radius-12);
		background: var(--fox);
		color: var(--fox-ink);
	}

	.cued .mono,
	.cued .cell,
	.cued .cell.name,
	.cued .credit,
	.cued .rate button.on {
		color: var(--fox-ink);
	}

	.cued .rate button {
		color: color-mix(in srgb, var(--fox-ink) 30%, transparent);
	}

	/*
	 * A line under the title, laid over the play target in the title's column, so a press anywhere
	 * else on the row still plays: the title rises above the middle, the line sits below it.
	 */
	.row.lined {
		height: var(--song-row-lined, 64px);
	}

	.lined > :global(*) {
		grid-row: 1;
	}

	.lined .play > .titled {
		translate: 0 -10px;
	}

	.second {
		grid-row: 1;
		min-width: 0;
		display: flex;
		translate: 0 11px;
		pointer-events: none;
	}

	.second > :global(*) {
		pointer-events: auto;
	}

	/* Two lines a song: the title, then a line of the person's columns, and its length. */
	.row.two {
		--song-row-two: 58px;

		height: var(--song-row-two);
		display: flex;
		gap: 10px;
		padding: 0 0 0 10px;
	}

	.two .box {
		display: none;
	}

	:global([data-selecting]) .two .box {
		display: flex;
	}

	.two .play {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.two .number {
		flex: none;
		width: 22px;
	}

	.lines {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 3px;
	}

	.line {
		display: flex;
		gap: 0.6em;
		min-width: 0;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		color: var(--text-meta);
	}

	.parts {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.parts:empty {
		display: none;
	}

	.parts:not(:empty)::after {
		content: ' ·';
	}

	.line-time {
		flex: none;
	}

	.two .more {
		flex: none;
		width: var(--target);
		height: var(--target);
	}

	.two .time {
		flex: none;
	}

	.cued .line {
		color: var(--fox-ink);
	}
</style>
