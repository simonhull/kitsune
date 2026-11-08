<!--
	The one song row, in every list of songs in the app: props in, events out. It has no effects or
	stores of its own, so the Library's 50,000 songs cost what they did. A list turns its parts on or
	off (the handle, the cover, the number, the columns, a second line) and says what each event does;
	the row's own logic is in songRow.ts, beside it.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import Cover from '#lib/cover/Cover.svelte';
	import { trackLabel } from '#lib/library/format.ts';
	import type { ColumnId, LibraryTrack } from '#lib/library/source.ts';
	import HeartButton from '#lib/shell/HeartButton.svelte';
	import Icon from '#lib/shell/Icon.svelte';
	import { cellOf, fitLine, phoneLine, timeOf, type RowAlbum, type RowParts } from './columns.ts';
	import { longPress, tapOf, type PickHow } from './songRow.ts';

	interface Props {
		track: LibraryTrack;
		/** The album it's on, for its cover and the cells that name it. */
		album: RowAlbum;
		/** Under 700: two lines a song, and no columns. */
		phone: boolean;
		parts: RowParts;
		/** The columns shown. Their grid is the list's `--song-grid`, which the header shares. */
		shown: readonly ColumnId[];
		/** The person's columns, which a phone's second line is made from. */
		order: readonly ColumnId[];
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
		onplay: (trackId: string) => void;
		onpick: (trackId: string, how: PickHow) => void;
		/** Opens the ⋯ menu for it, at a place. */
		onmenu: (trackId: string, x: number, y: number) => void;
		/** Space, the arrows and Shift with them, on the row. */
		onkey: (event: KeyboardEvent, trackId: string) => void;
		onrate: (trackId: string, stars: number) => void;
		/** The handle pressed to drag it, or Alt with an arrow on it. */
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
		onplay,
		onpick,
		onmenu,
		onkey,
		onrate,
		ondrag,
		onmove
	}: Props = $props();

	const context = $derived({ track, album, stars, hearted, artistWhenDifferent, now });
	const mark = $derived(track.shuffle);
	/** The play target spans the cover, the number and the title, after the handle and the box. */
	const playColumns = $derived.by(() => {
		const from = (parts.handle ? 2 : 1) + 1;
		return `${String(from)} / ${String(from + (parts.cover ? 1 : 0) + (parts.number ? 1 : 0) + 1)}`;
	});

	function tap(event: MouseEvent): void {
		if (longPress.swallow()) {
			event.preventDefault();
			return;
		}
		// The list says it's picking on its container, so starting a selection touches no row.
		const picking =
			event.currentTarget instanceof Element &&
			event.currentTarget.closest('[data-selecting]') !== null;
		const how = tapOf(event, longPress.touch, picking);
		if (how === 'play') {
			onplay(track.id);
		} else {
			onpick(track.id, how);
		}
	}
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
	style:top={top === undefined ? undefined : `${String(top)}px`}
	style:height={height === undefined ? undefined : `${String(height)}px`}
	oncontextmenu={(event) => {
		event.preventDefault();
		onmenu(track.id, event.clientX, event.clientY);
	}}
	onpointerdown={(event) => {
		longPress.start(event.pointerType, () => {
			navigator.vibrate?.(10);
			onpick(track.id, 'toggle');
		});
	}}
	onpointerup={() => {
		longPress.end();
	}}
	onpointercancel={() => {
		longPress.end();
	}}
	onpointerleave={() => {
		longPress.end();
	}}
>
	{#if joins !== undefined && mark?.run !== undefined}
		<span class="bracket" class:start={!joins.up} class:end={!joins.down} aria-hidden="true"></span>
	{/if}
	{#if parts.handle}
		<button
			type="button"
			class="handle"
			aria-label={`Move ${track.title}. Alt with an arrow moves it.`}
			onpointerdown={(event) => {
				event.stopPropagation();
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
	<span class="box">
		<input
			type="checkbox"
			aria-label={`Select ${track.title}`}
			checked={picked}
			onclick={(event) => {
				onpick(track.id, event.shiftKey ? 'range' : 'toggle');
				// The box says what the list did with the click, whatever the click did to it.
				event.currentTarget.checked = picked;
			}}
		/>
	</span>
	<button
		type="button"
		class="play"
		aria-label={`Play ${track.title}`}
		aria-current={cued ? 'true' : undefined}
		style:grid-column={phone ? undefined : playColumns}
		onclick={tap}
		onkeydown={(event) => {
			onkey(event, track.id);
		}}
	>
		{#if parts.cover}
			<span class="cover"><Cover albumId={album.id} entry={album} place="small" lazy /></span>
		{/if}
		{#if parts.number}
			<span class="mono number">
				{#if cued && sounding}
					<Icon name="play" size={12} />
				{:else}
					{trackLabel(track, false)}
				{/if}
			</span>
		{/if}
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
	{#if second !== undefined}
		<span class="second" style:grid-column={phone ? undefined : playColumns}
			>{@render second()}</span
		>
	{/if}
	{#each phone ? [] : shown as id (id)}
		{@const cell = cellOf(id, context)}
		{#if cell.look === 'stars'}
			<span class="rate" role="group" aria-label={`Rate ${track.title}`}>
				{#each [1, 2, 3, 4, 5] as star (star)}
					<button
						type="button"
						class:on={star <= cell.stars}
						aria-label={`Rate ${track.title} ${String(star)} of 5`}
						aria-pressed={star <= cell.stars}
						onclick={() => {
							onrate(track.id, star);
						}}>★</button
					>
				{/each}
			</span>
		{:else if cell.look === 'heart'}
			<span class="heart-cell"
				><HeartButton kind="tracks" id={track.id} name={track.title} size={16} /></span
			>
		{:else}
			<span
				class={['cell', cell.look]}
				class:end={id === 'plays' || id === 'bitrate' || id === 'gain'}
				class:warn={cell.warn === true}>{cell.text}</span
			>
		{/if}
	{/each}
	{#if !phone || second !== undefined}<span class="mono time">{timeOf(track)}</span>{/if}
	<button
		type="button"
		class="more"
		aria-label={`More for ${track.title}`}
		aria-haspopup="menu"
		onclick={(event) => {
			const { right, bottom } = event.currentTarget.getBoundingClientRect();
			onmenu(track.id, right - 300, bottom + 4);
		}}
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
		grid-template-columns: var(--song-grid);
		align-items: center;
		column-gap: var(--col-gap, 12px);
		padding: 0 var(--row-pad, 14px);
		border-bottom: 1px solid var(--edge-card);
		font-size: var(--type-body);
	}

	/* In a list that places every row itself, as the Library's does. */
	.row.placed {
		position: absolute;
		left: 0;
		right: 0;
	}

	/* The cover, the number and the title are one target: tapping any of them plays the song. */
	.play {
		min-width: 0;
		height: 100%;
		display: grid;
		grid-template-columns: subgrid;
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

	.credit {
		flex: 0 1 auto;
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
	 * The box is hidden at rest. It shows while the pointer is in the gutter (the whole column, edge
	 * to edge and top to bottom, so passing down it shows each box in turn), on a row with the
	 * focus, on a picked row, and on every row while the list is picking: the list says so with
	 * `data-selecting` on its container, so starting a selection changes no row. songRow.ts's
	 * `boxShows` says the same in words, and its tests hold them.
	 */
	.box {
		align-self: stretch;
		display: flex;
		align-items: center;
		margin-left: calc(-1 * var(--row-pad, 14px));
		padding-left: var(--row-pad, 14px);
	}

	.box input {
		width: var(--box-size, 16px);
		height: var(--box-size, 16px);
		margin: 0;
		accent-color: var(--fox);
		opacity: 0;
		transition: opacity 120ms var(--curve-morph);
	}

	@media (hover: hover) {
		.box:hover input {
			opacity: 1;
		}
	}

	.row:focus-within .box input,
	.row.picked .box input,
	:global([data-selecting]) .box input {
		opacity: 1;
	}

	@media (prefers-reduced-motion: reduce) {
		.box input {
			transition: none;
		}
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

	.cued .box input {
		accent-color: var(--fox-ink);
	}

	/*
	 * A line under the title, laid over the play target in the title's column, so a press anywhere
	 * else on the row still plays: the title rises above the middle, the line sits below it.
	 */
	.row.lined {
		height: var(--song-row-lined, 64px);
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
