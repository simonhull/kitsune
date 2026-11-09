<!--
	An album page's songs: the same SongRow as the Library's, with the person's columns (never Album,
	which is the page), a disc heading with its own box and play, and the facts at the foot.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { addedOn, discFacts, formatLine } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { Album, ColumnId, ColumnsPref, LibraryAlbum } from '#lib/library/source.ts';
	import { runJoins } from '#lib/library/shuffleRules.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import { discGroups } from './albumSongs.ts';
	import { columnsView } from './columnsView.svelte.ts';
	import {
		artistWhenDifferentIn,
		columnVars,
		columnsFrom,
		columnsIn,
		fit,
		largeFrom,
		monoAdvance,
		numbered,
		sameOnAlbum,
		template
	} from './columns.ts';
	import PickBox from './PickBox.svelte';
	import type { Selection } from './selection.svelte.ts';
	import SongHeader from './SongHeader.svelte';
	import SongRow from './SongRow.svelte';
	import { rowEvents, type SongList } from './songRow.ts';

	interface Props {
		album: Album | undefined;
		/** Its songs as rows show them, with the columns' fields once they've come. */
		rows: LibraryAlbum | undefined;
		pref: ColumnsPref;
		/** The ID of the song cued from this album, if any, and whether it's sounding. */
		cuedId: string | undefined;
		sounding: boolean;
		selection: Selection;
		list: SongList;
		/** Plays the album from this song. */
		play: (trackId: string) => void;
		/** Plays one disc of the album. */
		playDisc: (disc: number) => void;
		openMenu: (trackId: string, x: number, y: number) => void;
		/** Opens the columns menu at a place, offering to hide a column if one was pointed at. */
		openColumns: (x: number, y: number, hiding?: ColumnId) => void;
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
		rows,
		pref,
		cuedId,
		sounding,
		selection,
		list,
		play,
		playDisc,
		openMenu,
		openColumns,
		gone,
		unavailable,
		retry,
		editing,
		one,
		editor
	}: Props = $props();

	/** When "Yesterday" is, for Last played: read once a visit. */
	const now = Date.now();
	let width = $state(0);
	let windowWidth = $state(1280);
	const phone = $derived(windowWidth < columnsFrom);
	const step = $derived(windowWidth >= largeFrom ? 'large' : 'columns');
	const order = $derived(columnsIn(pref.order, 'album'));
	const artistWhenDifferent = $derived(artistWhenDifferentIn(pref, 'album'));
	const fitted = $derived(fit(order, width, step, numbered, pref.widths));
	const grid = $derived(template(fitted.shown, step));
	const groups = $derived(rows === undefined ? [] : discGroups(rows));
	const shared = $derived(
		phone && rows !== undefined
			? sameOnAlbum(order, rows, artistWhenDifferent, now)
			: new Set<ColumnId>()
	);
	const sharedHas = (id: ColumnId): boolean => shared.has(id);
	const allIds = $derived(rows?.tracks.map(({ id }) => id) ?? []);
	/** The rows' one set of handlers. */
	const events = rowEvents({
		play: (trackId) => {
			play(trackId);
		},
		pick: (trackId, how) => {
			list.pick(trackId, how);
		},
		menu: (trackId, x, y) => {
			openMenu(trackId, x, y);
		},
		key: (event, trackId) => {
			list.key(event, trackId);
		},
		rate: (trackId, stars) => {
			library.marks.press(trackId, stars);
		},
		picked: (trackId) => selection.has(trackId)
	});
	const lineChars = $derived(Math.floor((width - 96 - (selection.any ? 26 : 0)) / monoAdvance));
</script>

<svelte:window bind:innerWidth={windowWidth} />

{#snippet headerBox()}
	<PickBox
		state={selection.boxFor(allIds)}
		label={selection.boxFor(allIds) === 'all'
			? 'Clear every song on this album'
			: 'Select every song on this album'}
		ontoggle={() => {
			list.toggleAll();
		}}
	/>
{/snippet}

<section
	aria-label="Tracks"
	class="panel song-list stepped"
	class:editing
	class:one
	data-morph-part="panel"
	data-selecting={selection.any ? '' : undefined}
	style:--song-grid={grid}
	style={columnVars(fitted.shown, pref.widths, step)}
>
	{#if album !== undefined && editor !== undefined}
		{@render editor(album)}
	{:else if gone}
		<p class="note">This album isn't in the library any more.</p>
	{:else if unavailable}
		<p class="note">
			The tracks couldn't be loaded.
			<button type="button" onclick={retry}>Try again</button>
		</p>
	{:else if rows !== undefined}
		<div class="rows" bind:clientWidth={width} {...events}>
			{#if !phone}
				<SongHeader
					shown={fitted.shown}
					hiddenCount={fitted.hiddenCount}
					openMenu={openColumns}
					box={headerBox}
					widths={pref.widths}
					{step}
					onresize={(id, to) => {
						columnsView.resize(id, to);
					}}
				/>
			{/if}
			{#each groups as group, at (group.disc?.no ?? 0)}
				{#if group.disc !== undefined}
					{@const disc = group.disc}
					{@const label = `Disc ${String(disc.no)}`}
					{@const facts = discFacts(disc)}
					{@const ids = group.indexes.flatMap((index) => rows.tracks.at(index)?.id ?? [])}
					<div class="disc" class:first={at === 0}>
						<span class="disc-box pick-gutter"
							><PickBox
								state={selection.boxFor(ids)}
								label={`Select every song on ${label}`}
								ontoggle={() => {
									selection.toggleAll(ids);
								}}
							/></span
						>
						<h2 class="disc-no">{label}</h2>
						<p class="disc-about">
							{#if disc.name !== undefined}
								<span class="disc-name">{disc.name}</span>
							{/if}
							<span class="disc-facts"
								><span class="disc-songs">{`${facts.songs} · `}</span>{facts.length}</span
							>
						</p>
						<button
							type="button"
							class="disc-play"
							aria-label={disc.name === undefined ? `Play ${label}` : `Play ${label}, ${disc.name}`}
							onclick={() => {
								playDisc(disc.no);
							}}
						>
							<Icon name="play" size={12} />
							<span class="disc-play-words">Play disc</span>
						</button>
					</div>
				{/if}
				{#each group.indexes as index (index)}
					{@const track = rows.tracks.at(index)}
					{#if track !== undefined}
						<SongRow
							{track}
							album={rows}
							{phone}
							parts={numbered}
							shown={fitted.shown}
							order={pref.order}
							{artistWhenDifferent}
							{lineChars}
							sameOnAlbum={sharedHas}
							{now}
							stars={library.marks.stars(track.id)}
							hearted={library.marks.hearted('tracks', track.id)}
							cued={track.id === cuedId}
							{sounding}
							picked={selection.has(track.id)}
							joins={track.shuffle?.run === undefined ? undefined : runJoins(rows.tracks, index)}
						/>
					{/if}
				{/each}
			{/each}
			{#if album !== undefined}
				<div class="facts">
					<span>{formatLine(album.tracks)}</span>
					<span>added {addedOn(album.addedAt)}</span>
				</div>
			{/if}
		</div>
	{/if}
</section>

<style>
	.panel {
		position: relative;
		/* Under the stage, in line with its words: the hero's gutter on each side, never a centred column. */
		margin: calc(-72px * var(--step)) auto 0;
		width: calc(100% - 2 * max(64px, var(--gutter)));
		min-height: calc(560px * var(--step));
		box-sizing: border-box;
		padding: calc(20px * var(--step)) calc(24px * var(--step)) calc(24px * var(--step));
		border-radius: var(--radius-32);
		background: color-mix(in srgb, var(--card) 82%, transparent);
		border: 1px solid var(--edge);
		backdrop-filter: blur(24px);
		transition:
			margin-top var(--morph-slow) var(--curve-settle),
			width var(--morph-slow) var(--curve-settle);
	}

	/* Editing, it reaches 32 past the words on each side. */
	.editing {
		margin-top: -34px;
		width: calc(100% - 2 * max(32px, var(--gutter) - 32px));
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

	/* A disc's heading: the album's own section, with its box, its name, its length and its play. */
	.disc {
		display: flex;
		align-items: flex-end;
		gap: var(--col-gap, 12px);
		padding: calc(40px * var(--step)) var(--row-pad, 14px) calc(14px * var(--step));
		margin-bottom: 4px;
		border-bottom: 1px solid var(--edge-strong);
	}

	.disc.first {
		padding-top: calc(12px * var(--step));
	}

	/* In the rows' gutter, which reaches from the panel's edge and down the whole heading. */
	.disc-box {
		flex: none;
		align-self: stretch;
		width: calc(var(--box-size, 16px) + 4px + var(--row-pad, 14px));
		box-sizing: border-box;
		display: flex;
		align-items: flex-end;
		margin: calc(-40px * var(--step)) 0 calc(-14px * var(--step)) calc(-1 * var(--row-pad, 14px));
		padding: 0 0 calc(26px * var(--step)) var(--row-pad, 14px);
	}

	.disc.first .disc-box {
		margin-top: calc(-12px * var(--step));
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
		gap: calc(20px * var(--step));
		padding-left: calc(8px * var(--step));
	}

	.disc-name {
		min-width: 0;
		overflow: hidden;
		padding-bottom: 2px;
		font-size: calc(20px * var(--step));
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
		padding: 0 calc(18px * var(--step));
		display: flex;
		align-items: center;
		gap: calc(8px * var(--step));
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: none;
		color: var(--text);
		font-size: calc(14px * var(--step));
		font-weight: 600;
	}

	@media (hover: hover) {
		.disc-play:hover {
			border-color: var(--edge-hover);
		}
	}

	.facts {
		display: flex;
		flex-wrap: wrap;
		gap: calc(8px * var(--step)) calc(24px * var(--step));
		margin-top: calc(8px * var(--step));
		padding: calc(16px * var(--step)) var(--row-pad, 14px) 0;
		border-top: 1px solid var(--edge);
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
	}

	/* The phone's heading: the number over its name and length, and a round play to thumb. */
	@media (max-width: 699px) {
		.disc {
			display: grid;
			grid-template-columns: auto minmax(0, 1fr) var(--target);
			grid-template-areas: 'box no play' 'box about play';
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

		.disc-box {
			grid-area: box;
			display: none;
			margin: 0;
			padding: 0;
			width: auto;
		}

		:global([data-selecting]) .disc-box {
			display: flex;
			align-items: center;
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
			padding: 0;
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

		.facts {
			padding: 16px 10px 6px;
		}
	}
</style>
