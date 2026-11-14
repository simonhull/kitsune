<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { ownColor, paletteFor } from '#lib/cover/palette.ts';
	import { theme } from '#lib/theme/theme.svelte.ts';
	import { albumStars } from '#lib/library/albumStars.ts';
	import {
		albumHoldings,
		discFacts,
		discKey,
		discLabel,
		discOf,
		noDisc
	} from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { ColumnId, ColumnsPref, MovableId } from '#lib/library/source.ts';
	import { runJoins } from '#lib/library/shuffleRules.ts';
	import { morph } from '#lib/morph/registry.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import StarsShown from '#lib/shell/StarsShown.svelte';
	import { numbered, sameOnAlbum } from './columns.ts';
	import CoverPlay from './CoverPlay.svelte';
	import type { AlbumBlock, Row } from './libraryRows.ts';
	import PickBox from './PickBox.svelte';
	import type { Selection } from './selection.svelte.ts';
	import SongRow from './SongRow.svelte';

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
		/** The columns that fit. */
		shown: readonly MovableId[];
		pref: ColumnsPref;
		/** When "Yesterday" is. */
		now: number;
		/** The song cued, if any. */
		cuedId: string | undefined;
		/** Whether that song is sounding now. */
		sounding: boolean;
		/** A tap on this album couldn't be played. */
		trouble: boolean;
		/** The list's selection. */
		selection: Selection;
		/** Plays one disc of the album. */
		playDisc: (disc: number) => void;
		/**
		 * The artist a page about one lists the album under: Artist leaves out its songs that are only
		 * theirs, whoever the album is by.
		 */
		pageArtist?: string | undefined;
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
		pref,
		now,
		cuedId,
		sounding,
		trouble,
		selection,
		playDisc,
		pageArtist
	}: Props = $props();

	const album = $derived(block.album);
	/** What the index says of each disc: its name, count and length. */
	const discs = $derived(new Map((album.discs ?? []).map((disc) => [discKey(disc), disc])));
	/** Each disc's songs, for its heading's box: the songs with no disc number as one more. */
	const discSongs = $derived(Map.groupBy(album.tracks, (track) => discOf(track.discNo)));
	const sharedHas = (id: ColumnId): boolean => shared.has(id);
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
	const shared = $derived(
		phone ? sameOnAlbum(pref.order, album, pref.artistWhenDifferent, now) : new Set<ColumnId>()
	);
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
			<!-- The cover and its play, beside the link: a button can't live inside one. -->
			<div class="cover-box">
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
				<CoverPlay {album} from="list" />
			</div>
			<div class="names">
				<div class="title-line">
					<a
						class="title"
						href={resolve('/album/[id]', { id: album.id })}
						title={album.title}
						use:morph={{ id: `title:${album.id}`, slot: 'card', kind: 'title' }}>{album.title}</a
					>
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
		<!-- A song keyed by its ID, so one moved to Trash folds out itself and the rest close up. -->
		{#each rows as row (row.kind === 'song' ? (album.tracks.at(row.trackIndex)?.id ?? row.trackIndex) : `disc-${String(row.disc)}`)}
			{@const at = `${String(row.top - block.top - head)}px`}
			{#if row.kind === 'disc'}
				{@const disc = discs.get(row.disc)}
				{@const facts = disc === undefined ? undefined : discFacts(disc)}
				{@const ids = (discSongs.get(row.disc) ?? []).map(({ id }) => id)}
				{@const none = row.disc === noDisc}
				{@const label = discLabel(row.disc)}
				<div
					class="disc"
					class:first={row.first}
					class:none
					style:top={at}
					style:height={`${String(row.height)}px`}
				>
					<span class="disc-box pick-gutter"
						><PickBox
							state={selection.boxFor(ids)}
							label={none
								? `Select every song with no disc number on ${album.title}`
								: `Select every song on disc ${String(row.disc)} of ${album.title}`}
							ontoggle={() => {
								selection.toggleAll(ids);
							}}
						/></span
					>
					<span class="disc-line">
						<h3 class="disc-no">{label.toUpperCase()}</h3>
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
						aria-label={none
							? `Play the songs with no disc number on ${album.title}`
							: disc?.name === undefined
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
					<SongRow
						{track}
						{album}
						{phone}
						parts={numbered}
						{shown}
						order={pref.order}
						artistWhenDifferent={pageArtist !== undefined || pref.artistWhenDifferent}
						{pageArtist}
						{lineChars}
						sameOnAlbum={sharedHas}
						top={row.top - block.top - head}
						height={row.height}
						{now}
						stars={library.marks.stars(track.id)}
						cued={track.id === cuedId}
						{sounding}
						picked={selection.has(track.id)}
						joins={track.shuffle?.run === undefined
							? undefined
							: runJoins(album.tracks, row.trackIndex)}
					/>
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

	/* A song folding out on its way to Trash: the albums after it close up with its rows. */
	:global(:root[data-folding]) .album,
	:global(:root[data-folding]) .disc {
		transition:
			top var(--morph-room) var(--curve-room),
			height var(--morph-room) var(--curve-room);
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

	/*
	 * The cover's play, sized by the cover: as the Albums cards have it at 208 (48 in 12 from the
	 * corner), the Recent row's at a stacked 112, and no bigger than a thumb's reach at 420.
	 */
	.cover-box {
		--cover-play-size: clamp(36px, calc(var(--cover, 208px) * 0.23), 72px);
		--cover-play-inset: clamp(8px, calc(var(--cover, 208px) * 0.058), 18px);

		position: relative;
		width: var(--cover, 208px);
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

	/* The songs with no disc number: a quiet warning, as "no RG" is. */
	.none .disc-no {
		color: var(--fox-text);
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

	/* A disc's box, in the rows' gutter, which reaches from the edge and down the whole heading. */
	.disc-box {
		flex: none;
		align-self: stretch;
		box-sizing: border-box;
		width: calc(var(--box-size, 16px) + 4px + var(--row-pad, 14px));
		display: flex;
		align-items: flex-end;
		margin: 0 -2px 0 calc(-1 * var(--row-pad, 14px));
		padding: 0 0 1px var(--row-pad, 14px);
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

		.cover-box {
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
