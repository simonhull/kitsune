<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { ownColor, paletteFor } from '#lib/cover/palette.ts';
	import { theme } from '#lib/theme/theme.svelte.ts';
	import { albumStars } from '#lib/library/albumStars.ts';
	import { albumHoldings, discFacts, trackLabel } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { ColumnId, ColumnsPref, ShuffleMark } from '#lib/library/source.ts';
	import { runJoins } from '#lib/library/shuffleRules.ts';
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
		/** Plays one disc of the album. */
		playDisc: (disc: number) => void;
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
		playDisc,
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
	/** What the index says of each disc: its name, count and length. */
	const discs = $derived(new Map((album.discs ?? []).map((disc) => [disc.no, disc])));
	const hearted = $derived(library.marks.hearted('albums', album.id));
	const stars = $derived(albumStars(album.tracks, library.marks));
	/**
	 * The cover's glow, in its own color: the accent, or, for a cover with no color of its own,
	 * its ground lifted, so it glows its own way and not the frame's amber.
	 */
	const glow = $derived.by(() => {
		const palette = paletteFor(album, theme.current);
		return ownColor(album.artwork)
			? `color-mix(in srgb, ${palette.accent} var(--cover-glow-alpha), transparent)`
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
		{#snippet marks(mark: ShuffleMark | undefined)}
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
		{#each rows as row (row.kind === 'song' ? row.trackIndex : `disc-${String(row.disc)}`)}
			{@const at = `${String(row.top - block.top - head)}px`}
			{#if row.kind === 'disc'}
				{@const disc = discs.get(row.disc)}
				{@const facts = disc === undefined ? undefined : discFacts(disc)}
				<div
					class="disc"
					class:first={row.first}
					style:top={at}
					style:height={`${String(row.height)}px`}
				>
					<span class="disc-line">
						<h3 class="disc-no">DISC {row.disc}</h3>
						{#if disc?.name !== undefined}
							<span class="disc-name" title={disc.name}>{disc.name}</span>
						{/if}
						{#if facts !== undefined}
							<span class="disc-facts"
								>{#if !phone}{`${facts.songs} · `}{/if}{facts.length}</span
							>
						{/if}
					</span>
					<button
						type="button"
						class="disc-play"
						aria-label={disc?.name === undefined
							? `Play disc ${String(row.disc)} of ${album.title}`
							: `Play disc ${String(row.disc)}, ${disc.name}`}
						onclick={() => {
							playDisc(row.disc);
						}}
					>
						<Icon name="play" size={10} />
					</button>
				</div>
			{:else}
				{@const track = album.tracks.at(row.trackIndex)}
				{#if track !== undefined}
					{@const mark = track.shuffle}
					{@const joins = runJoins(album.tracks, row.trackIndex)}
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
						{#if mark?.run !== undefined}
							<span
								class="bracket"
								class:start={!joins.up}
								class:end={!joins.down}
								aria-hidden="true"
							></span>
						{/if}
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
									<span class="titled"
										><span class="name">{track.title}</span>{@render marks(mark)}</span
									>
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
								<span class="titled"
									><span class="name">{track.title}</span>{@render marks(mark)}</span
								>
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
			0 0 0 1px var(--cover-edge);
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
		color: var(--fox-text);
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
		color: var(--fox-text);
	}

	.songs {
		position: absolute;
		bottom: 0;
		left: calc(var(--cover, 208px) + var(--cover-gap, 32px));
		right: 0;
	}

	/* A disc's heading: quiet, for scanning, with a play that shows on hover and focus. */
	.disc {
		position: absolute;
		left: 0;
		right: 0;
		box-sizing: border-box;
		display: flex;
		align-items: flex-end;
		gap: 14px;
		padding: 0 var(--row-pad, 14px) 8px;
		border-bottom: 1px solid var(--edge-field);
	}

	/* Number, name and facts share one baseline, at the foot of the heading. */
	.disc-line {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: baseline;
		gap: 14px;
	}

	.disc-no {
		flex: none;
		margin: 0;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 600;
		letter-spacing: 0.1em;
		color: var(--text);
	}

	.disc-name {
		min-width: 0;
		overflow: hidden;
		font-size: calc(var(--type-mono) + 2px);
		font-weight: 600;
		line-height: 1.2;
		color: var(--text-secondary);
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.disc-facts {
		flex: none;
		margin-left: auto;
		font-family: var(--font-mono);
		font-size: calc(var(--type-mono) - 1px);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.disc-play {
		flex: none;
		width: 32px;
		height: 32px;
		margin: 0 0 -4px;
		display: grid;
		place-items: center;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: none;
		color: var(--text);
		opacity: 0;
	}

	.disc:hover .disc-play,
	.disc-play:focus-visible {
		opacity: 1;
	}

	/* Without hover, a play nobody can find isn't there: it shows, and a finger gets its 44. */
	@media (hover: none) {
		.disc-play {
			opacity: 1;
			width: var(--target);
			height: var(--target);
			margin-bottom: -8px;
		}
	}

	/* The grid is the header's: every cell sits under its label. */
	.row {
		--heart-on: var(--star);
		--heart-off: var(--star-off);

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
		background: var(--pressed);
	}

	.row:focus-within .box input,
	.row:focus-within .more,
	.row.picking .box input,
	.row.cued .more {
		opacity: 1;
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

		.row:hover .box input,
		.row:hover .more {
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
	.cued .rate button.on {
		color: var(--fox-ink);
	}

	.cued .rate button {
		color: color-mix(in srgb, var(--fox-ink) 30%, transparent);
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
