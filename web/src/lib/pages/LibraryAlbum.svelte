<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { paletteFor } from '#lib/cover/palette.ts';
	import { albumStars } from '#lib/library/albumStars.ts';
	import { albumHoldings, trackLabel } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { ColumnId, ColumnsPref } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';
	import HeartButton from '#lib/shell/HeartButton.svelte';
	import Icon from '#lib/shell/Icon.svelte';
	import StarsShown from '#lib/shell/StarsShown.svelte';
	import { cellOf, fitLine, phoneLine, timeOf } from './columns.ts';
	import type { AlbumBlock, Row } from './libraryRows.ts';
	import { selection } from './selection.svelte.ts';

	/** How long a finger holds a row to start picking songs. */
	const longPressMs = 450;

	interface Props {
		block: AlbumBlock;
		/** Where it starts in its artist. */
		top: number;
		/** The rows of it that are mounted. */
		rows: readonly Row[];
		/** How far down the page's rows start, past what stands above them. */
		head: number;
		/** The cover here is a small one, above the songs. */
		stacked: boolean;
		/** Under 700: two lines a song, and no columns. */
		phone: boolean;
		/** How big the cover is drawn, in pixels. */
		cover: number;
		/** How many characters a phone's second line has room for. */
		lineChars: number;
		/** The columns that fit, and the grid they make. */
		shown: readonly ColumnId[];
		grid: string;
		pref: ColumnsPref;
		/** When "Yesterday" is. */
		now: number;
		/** The song cued, if any. */
		cuedId: string | undefined;
		/** Whether that song is sounding now. */
		sounding: boolean;
		/** A tap on this album couldn't be played. */
		trouble: boolean;
		play: (trackId: string) => void;
		/** A box clicked, or a row with Shift, Ctrl or Cmd. */
		pick: (trackId: string, how: 'toggle' | 'range') => void;
		/** Opens the ⋯ menu for a song, at a place. */
		openMenu: (trackId: string, x: number, y: number) => void;
		/** Space, the arrows and Shift with them, on a row. */
		rowKey: (event: KeyboardEvent, trackId: string) => void;
	}

	let {
		block,
		top,
		rows,
		head,
		stacked,
		phone,
		cover,
		lineChars,
		shown,
		grid,
		pref,
		now,
		cuedId,
		sounding,
		trouble,
		play,
		pick,
		openMenu,
		rowKey
	}: Props = $props();

	/** The long press under way: it picks the song if the finger stays, and the tap that ends it doesn't play. */
	let pressing: ReturnType<typeof setTimeout> | undefined;
	let pressed = false;
	/** Whether the row was last touched by a finger: then, while picking, a tap picks too. */
	let touched = false;

	function holdStart(event: PointerEvent, trackId: string): void {
		touched = event.pointerType === 'touch';
		if (!touched) {
			return;
		}
		clearTimeout(pressing);
		pressed = false;
		pressing = setTimeout(() => {
			pressed = true;
			navigator.vibrate?.(10);
			pick(trackId, 'toggle');
		}, longPressMs);
	}

	function holdEnd(): void {
		clearTimeout(pressing);
	}

	function tap(event: MouseEvent, trackId: string): void {
		if (pressed) {
			pressed = false;
			event.preventDefault();
			return;
		}
		if (event.shiftKey) {
			pick(trackId, 'range');
		} else if (event.ctrlKey || event.metaKey || (touched && selection.any)) {
			// On a phone, once anything is picked, a tap picks too: playing waits for the bar.
			pick(trackId, 'toggle');
		} else {
			play(trackId);
		}
	}

	const album = $derived(block.album);
	const hearted = $derived(library.marks.hearted('albums', album.id));
	const stars = $derived(albumStars(album.tracks, library.marks));
	/**
	 * The cover's glow, in its own color: the accent, or, for a cover with no color of its own,
	 * its ground lifted, so it glows its own way and not the frame's amber.
	 */
	const glow = $derived.by(() => {
		const palette = paletteFor(album);
		const tinted = album.artwork === undefined || album.artwork.accent !== '#f28c28';
		return tinted
			? `color-mix(in srgb, ${palette.accent} 45%, transparent)`
			: `color-mix(in srgb, ${palette.wash} 22%, transparent)`;
	});
	/** What says the same for every song of the album: a phone's line leaves it to the heading. */
	const shared = $derived.by(() => {
		if (!phone) {
			return new Set<string>();
		}
		const same = pref.order.filter((id) => {
			const texts = album.tracks.map((track) => {
				const cell = cellOf(id, {
					track,
					album,
					stars: null,
					hearted: false,
					artistWhenDifferent: pref.artistWhenDifferent,
					now
				});
				return cell.look === 'name' || cell.look === 'fact' ? cell.text : '';
			});
			return texts.every((text) => text === texts[0]);
		});
		return new Set<string>(same);
	});
</script>

<section
	class="album"
	aria-label={album.title}
	style:top={`${String(top)}px`}
	style:height={`${String(block.height)}px`}
>
	<div class="side">
		<div
			class="pin"
			data-morph-part="card"
			data-morph-key={`cover:${album.id}`}
			data-morph-pinned={stacked ? undefined : ''}
		>
			<a class="art" href={resolve('/album/[id]', { id: album.id })} aria-label={album.title}>
				<span
					class="square"
					style:--glow={glow}
					use:morph={{ id: `cover:${album.id}`, slot: 'card', kind: 'cover' }}
				>
					<Cover
						albumId={album.id}
						entry={album}
						place={stacked ? 'small' : 'card'}
						box={cover}
						lazy
					/>
				</span>
			</a>
			<div class="names">
				<div class="title-line">
					<a
						class="title"
						href={resolve('/album/[id]', { id: album.id })}
						title={album.title}
						use:morph={{ id: `title:${album.id}`, slot: 'card', kind: 'title' }}>{album.title}</a
					>
					{#if hearted}
						<span class="hearted" role="img" aria-label="Hearted"
							><Icon name="heart" size={16} /></span
						>
					{/if}
				</div>
				{#if trouble}
					<p class="facts trouble" role="status">Couldn't load this album</p>
				{:else}
					{@const hiding = block.shown < album.tracks.length}
					<!-- What a filter hides takes the room: the stars go under it, not off the end. -->
					<p class="facts">
						{albumHoldings(album.year, block.shown, album.tracks.length)}
						{#if !hiding}
							· <span class="stars"><StarsShown {stars} /></span>
						{/if}
					</p>
					{#if hiding}
						<p class="facts stars"><StarsShown {stars} /></p>
					{/if}
				{/if}
			</div>
		</div>
	</div>
	<div
		class="songs"
		data-morph-part="card"
		data-morph-key={`songs:${album.id}`}
		style:top={`${String(head)}px`}
	>
		{#each rows as row (row.kind === 'song' ? row.trackIndex : `disc-${String(row.disc)}`)}
			{@const at = `${String(row.top - block.top - head)}px`}
			{#if row.kind === 'disc'}
				<p class="disc" style:top={at} style:height={`${String(row.height)}px`}>
					DISC {row.disc}
				</p>
			{:else}
				{@const track = album.tracks.at(row.trackIndex)}
				{#if track !== undefined}
					{@const cued = track.id === cuedId}
					{@const picked = selection.has(track.id)}
					{@const context = {
						track,
						album,
						stars: library.marks.stars(track.id),
						hearted: library.marks.hearted('tracks', track.id),
						artistWhenDifferent: pref.artistWhenDifferent,
						now
					}}
					<div
						class="row"
						class:two={phone}
						class:cued
						class:picked
						class:picking={selection.any}
						role="group"
						aria-label={track.title}
						data-track={track.id}
						style:top={at}
						style:height={`${String(row.height)}px`}
						style:grid-template-columns={phone ? undefined : grid}
						oncontextmenu={(event) => {
							event.preventDefault();
							openMenu(track.id, event.clientX, event.clientY);
						}}
						onpointerdown={(event) => {
							holdStart(event, track.id);
						}}
						onpointerup={holdEnd}
						onpointercancel={holdEnd}
						onpointerleave={holdEnd}
					>
						<span class="box">
							<input
								type="checkbox"
								aria-label={`Select ${track.title}`}
								checked={picked}
								onclick={(event) => {
									pick(track.id, event.shiftKey ? 'range' : 'toggle');
									// The box says what the selection does, whatever the click did to it.
									event.currentTarget.checked = selection.has(track.id);
								}}
							/>
						</span>
						<button
							type="button"
							class="play"
							aria-label={`Play ${track.title}`}
							aria-current={cued ? 'true' : undefined}
							onclick={(event) => {
								tap(event, track.id);
							}}
							onkeydown={(event) => {
								rowKey(event, track.id);
							}}
						>
							<span class="mono number">
								{#if cued && sounding}
									<Icon name="play" size={12} />
								{:else}
									{trackLabel(track, false)}
								{/if}
							</span>
							{#if phone}
								<span class="lines">
									<span class="name">{track.title}</span>
									<span class="line"
										><span class="parts"
											>{fitLine(
												phoneLine(pref.order, context, (id) => shared.has(id)),
												timeOf(track),
												lineChars
											).join(' · ')}</span
										><span class="line-time">{timeOf(track)}</span></span
									>
								</span>
							{:else}
								<span class="name">{track.title}</span>
							{/if}
						</button>
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
												library.marks.press(track.id, star);
											}}>★</button
										>
									{/each}
								</span>
							{:else if cell.look === 'heart'}
								<span class="heart-cell"
									><HeartButton kind="tracks" id={track.id} name={track.title} size={16} /></span
								>
							{:else}
								<span class={['cell', cell.look]} class:end={id === 'plays' || id === 'bitrate'}
									>{cell.text}</span
								>
							{/if}
						{/each}
						{#if !phone}<span class="mono time">{timeOf(track)}</span>{/if}
						<button
							type="button"
							class="more"
							aria-label={`More for ${track.title}`}
							aria-haspopup="menu"
							onclick={(event) => {
								const { right, bottom } = event.currentTarget.getBoundingClientRect();
								openMenu(track.id, right - 300, bottom + 4);
							}}
							><svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"
								><circle cx="5" cy="12" r="1.8"></circle><circle cx="12" cy="12" r="1.8"
								></circle><circle cx="19" cy="12" r="1.8"></circle></svg
							></button
						>
					</div>
				{/if}
			{/if}
		{/each}
	</div>
</section>

<style>
	.album {
		position: absolute;
		left: 0;
		right: 0;
	}

	.side {
		position: absolute;
		top: 0;
		bottom: 0;
		left: 0;
		width: var(--cover, 208px);
	}

	/* Stuck under the header while the songs beside it scroll, and gone with the last of them. */
	.pin {
		position: sticky;
		top: var(--list-line, 152px);
	}

	.art {
		display: block;
	}

	/* Its own color glows under it: the boards' 0 32 80 -28 at 256, growing with the cover. */
	.square {
		position: relative;
		display: block;
		width: var(--cover, 208px);
		height: var(--cover, 208px);
		border-radius: clamp(18px, calc(var(--cover, 208px) * 0.085), 32px);
		overflow: hidden;
		box-shadow:
			0 calc(var(--cover, 208px) * 0.125) calc(var(--cover, 208px) * 0.31)
				calc(var(--cover, 208px) * -0.11) var(--glow, transparent),
			0 0 0 1px rgb(255 255 255 / 6%);
	}

	.names {
		margin-top: 12px;
		min-width: 0;
	}

	.title-line {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
	}

	.hearted {
		flex: none;
		display: flex;
		color: var(--text);
	}

	/* As wide as the words: a flight measures the element, and an empty right side would stretch. */
	.title {
		display: block;
		width: fit-content;
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: var(--type-title);
		font-weight: 700;
		letter-spacing: var(--tracking-title);
	}

	.title:hover {
		color: var(--fox);
	}

	.facts {
		margin-top: 2px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	.stars {
		color: var(--text-secondary);
	}

	.trouble {
		color: var(--fox);
	}

	.songs {
		position: absolute;
		bottom: 0;
		left: calc(var(--cover, 208px) + var(--cover-gap, 32px));
		right: 0;
	}

	.disc {
		position: absolute;
		left: 0;
		right: 0;
		display: flex;
		align-items: flex-end;
		padding: 0 14px 6px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	/* The grid is the header's: every cell sits under its label. */
	.row {
		--heart-on: var(--fox);
		--heart-off: rgb(255 255 255 / 20%);

		position: absolute;
		left: 0;
		right: 0;
		box-sizing: border-box;
		display: grid;
		align-items: center;
		column-gap: var(--col-gap, 12px);
		padding: 0 var(--row-pad, 14px);
		border-bottom: 1px solid var(--edge-card);
		font-size: var(--type-body);
	}

	/* The number and the title are one target: tapping either plays the song. */
	.play {
		grid-column: 2 / 4;
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
		color: rgb(255 255 255 / 16%);
		font: inherit;
		line-height: 1;
	}

	.rate button.on {
		color: var(--fox);
	}

	.rate button:hover,
	.rate:hover button:has(~ button:hover) {
		color: color-mix(in srgb, var(--fox) 70%, transparent);
	}

	.heart-cell {
		display: flex;
		justify-content: center;
	}

	.heart-cell :global(.heart) {
		width: 28px;
		height: 28px;
	}

	/* The box and ⋯ show on hover and on focus; once anything is picked, every box shows. */
	.box {
		display: flex;
		align-items: center;
	}

	.box input {
		width: var(--box-size, 16px);
		height: var(--box-size, 16px);
		margin: 0;
		accent-color: var(--fox);
		opacity: 0;
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
		background: rgb(255 255 255 / 14%);
	}

	.row:focus-within .box input,
	.row:focus-within .more,
	.row.picking .box input,
	.row.cued .more {
		opacity: 1;
	}

	.row.picked {
		border-radius: var(--radius-12);
		background: rgb(242 140 40 / 16%);
	}

	@media (hover: hover) {
		.row:hover {
			border-radius: var(--radius-12);
			background: rgb(255 255 255 / 8%);
		}

		.row:hover .box input,
		.row:hover .more {
			opacity: 1;
		}

		.row.picked:hover {
			background: rgb(242 140 40 / 22%);
		}
	}

	/* Nothing hovers on touch: ⋯ is always there. */
	@media (hover: none) {
		.more {
			opacity: 1;
		}
	}

	.row.cued,
	.row.cued:hover {
		--heart-on: var(--fox-ink);
		--heart-off: rgb(26 15 5 / 40%);

		border-radius: var(--radius-12);
		background: var(--fox);
		color: var(--fox-ink);
	}

	.cued .mono,
	.cued .cell,
	.cued .cell.name,
	.cued .rate button.on {
		color: var(--fox-ink);
	}

	.cued .rate button {
		color: rgb(26 15 5 / 30%);
	}

	/* Two lines a song: the title, then a line of the person's columns, and its length. */
	.row.two {
		display: flex;
		gap: 10px;
		padding: 0 0 0 10px;
	}

	.two .box {
		display: none;
	}

	.two.picking .box {
		display: flex;
	}

	.two .play {
		flex: 1;
		display: flex;
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

	.cued .line {
		color: var(--fox-ink);
	}

	/* Stacked, the cover heads its album, its names beside it on its last line. */
	@media (max-width: 1024px) {
		.side {
			bottom: auto;
			width: auto;
			right: 0;
			height: 112px;
		}

		.pin {
			position: static;
			height: 100%;
			display: flex;
			align-items: flex-end;
			gap: 16px;
		}

		.square {
			flex: none;
		}

		.names {
			margin-top: 0;
			padding-bottom: 2px;
		}

		.songs {
			left: 0;
		}
	}
</style>
