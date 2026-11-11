<script lang="ts">
	import Cover from '#lib/cover/Cover.svelte';
	import { emWidth } from '#lib/library/fit.ts';
	import { resolve } from '$app/paths';
	import { albumStars } from '#lib/library/albumStars.ts';
	import { albumFacts } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { Album, AlbumEntry } from '#lib/library/source.ts';
	import { morph } from '#lib/morph/registry.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import { shuffles } from '#lib/shuffle/session.svelte.ts';
	import { albumSource } from '#lib/shuffle/sources.ts';
	import HeartButton from '#lib/shell/HeartButton.svelte';
	import Icon from '#lib/shell/Icon.svelte';
	import StarsShown from '#lib/shell/StarsShown.svelte';
	import { albumMenu } from './albumMenu.svelte.ts';
	import TagFields from './TagFields.svelte';
	import type { TagSession } from './tagEdits.svelte.ts';

	interface Props {
		albumId: string;
		/** The list entry: present on the first frame when we came from a list. */
		entry: AlbumEntry | undefined;
		/** The full album, once it has streamed in. */
		album: Album | undefined;
		/** Opens the editor. Left out for anyone who can't edit. */
		edit: (() => void) | undefined;
		editButton?: HTMLButtonElement | undefined;
		/** The edit under way on this album, if any. */
		session: TagSession | undefined;
		/** Where focus goes when the editor opens. */
		eyebrow?: HTMLElement | undefined;
		/** Many tracks at once are being edited: this many of them picked. */
		picking?: { picked: number; of: number } | undefined;
		/** Goes from the album page's editor to many tracks at once. */
		many?: (() => void) | undefined;
	}

	let {
		albumId,
		entry,
		album,
		edit,
		editButton = $bindable(),
		session,
		eyebrow = $bindable(),
		picking,
		many
	}: Props = $props();

	/** Editing of either kind: the page draws up the same way for both. */
	const editing = $derived(session !== undefined || picking !== undefined);

	const files = $derived(session?.tags?.tracks.length ?? album?.tracks.length ?? 0);
	const fileCount = $derived(`${String(files)} ${files === 1 ? 'FILE' : 'FILES'}`);
	const open = $derived(session?.open ?? null);
	const place = $derived((session?.tags?.tracks.findIndex(({ id }) => id === open) ?? -1) + 1);

	/** Until the album streams in, and on a compilation, the name is plain text. */
	const owners = $derived(album?.artists ?? []);
	const meta = $derived(album === undefined ? '' : albumFacts(album, album));
	const playable = $derived(album !== undefined && album.tracks.length > 0);
	const sounding = $derived(
		player.queue?.album.id === albumId && player.queue.albums === undefined && player.sounding
	);
	const stars = $derived(album === undefined ? null : albumStars(album.tracks, library.marks));

	function play(): void {
		if (album !== undefined) {
			player.playOrToggle(album);
		}
	}

	/** What Shuffle shuffles: this album, each song once, by the person's rules. */
	const shuffleSource = $derived(album === undefined ? undefined : albumSource(album));
	// The first song is picked as the album arrives, so a press plays at once.
	$effect(() => {
		if (shuffleSource !== undefined) {
			void shuffles.prime(shuffleSource);
		}
	});

	function shuffle(): void {
		if (shuffleSource !== undefined) {
			shuffles.start(shuffleSource);
			void shuffles.prime(shuffleSource);
		}
	}
</script>

<section aria-label="Album" class="hero stepped" class:editing class:one={open !== null}>
	<div
		class="art"
		data-morph-part="art"
		use:morph={{ id: `cover:${albumId}`, slot: 'hero', kind: 'cover' }}
	>
		<Cover {albumId} {entry} place="stage" />
	</div>
	<div class="words" data-morph-part="words">
		{#if picking !== undefined}
			<span class="meta ready eyebrow" tabindex="-1" bind:this={eyebrow}>
				EDITING TAGS · {picking.picked} OF {picking.of}
				{picking.of === 1 ? 'TRACK' : 'TRACKS'} PICKED
			</span>
		{:else if session === undefined}
			<span class="meta" class:ready={album !== undefined}>{meta}&nbsp;</span>
		{:else}
			<span class="meta ready eyebrow" tabindex="-1" bind:this={eyebrow}>
				{#if open === null}
					EDITING TAGS · <span class="writes">WRITES TO&nbsp;</span>{fileCount}
				{:else}
					EDITING TAGS · TRACK {place} OF {files}
				{/if}
			</span>
		{/if}
		<!-- The fit divides by the words' width, known before layout: the first frame is final. -->
		<h1
			title={entry?.title}
			style:--width={emWidth(entry?.title ?? '', 900, -0.07)}
			use:morph={{ id: `title:${albumId}`, slot: 'hero', kind: 'title' }}
		>
			{entry?.title ?? ''}
		</h1>
		<div class="actions">
			{#if !editing}
				<button
					type="button"
					class="play"
					aria-label={`${sounding ? 'Pause' : 'Play'} ${entry?.title ?? 'album'}`}
					disabled={!playable}
					onclick={play}
				>
					<Icon name={sounding ? 'pause' : 'play'} size={28} />
				</button>
			{/if}
			<span class="artist">
				<span class="name" use:morph={{ id: `artist:${albumId}`, slot: 'hero', kind: 'title' }}>
					{#if owners.length === 1}
						<!-- The album's own spelling of the name, which a shared artist row can differ from. -->
						<a href={resolve('/artists/[id]', { id: owners[0].id })}>{entry?.artist}</a>
					{:else if owners.length > 1}
						{#each owners as owner, index (owner.id)}
							{index === 0 ? '' : ', '}<a href={resolve('/artists/[id]', { id: owner.id })}
								>{owner.name}</a
							>
						{/each}
					{:else}
						{entry?.artist ?? ''}
					{/if}
				</span>
			</span>
			{#if many !== undefined}
				<button type="button" class="ghost many" onclick={many}>Many at once</button>
			{/if}
			{#if !editing}
				<button type="button" class="ghost" disabled={!playable} onclick={shuffle}>Shuffle</button>
				<span class="favorite"
					><HeartButton kind="albums" id={albumId} name={entry?.title ?? 'album'} size={18} /></span
				>
				{#if edit !== undefined}
					<button
						type="button"
						class="ghost"
						disabled={!playable}
						bind:this={editButton}
						onclick={edit}>Edit tags</button
					>
				{/if}
				{#if entry !== undefined}
					{@const held = entry}
					<button
						type="button"
						class="ghost round"
						aria-label={`More for ${held.title}`}
						aria-haspopup="menu"
						onclick={(event) => {
							const { left, bottom } = event.currentTarget.getBoundingClientRect();
							albumMenu.open(held, left, bottom + 6);
						}}
						><svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"
							><circle cx="5" cy="12" r="1.8"></circle><circle cx="12" cy="12" r="1.8"
							></circle><circle cx="19" cy="12" r="1.8"></circle></svg
						></button
					>
				{/if}
				{#if stars !== null}
					<!-- Worked out from the songs, never set: so it reads, and doesn't press. -->
					<span class="rating">
						<span class="glyphs"><StarsShown {stars} empty="dim" /></span>
						<span class="from" aria-hidden="true"
							>{stars.mean} · FROM {stars.rated} OF {stars.of}
							{stars.of === 1 ? 'SONG' : 'SONGS'}{stars.byHeart > 0
								? ` · ${String(stars.byHeart)} BY HEART`
								: ''}</span
						>
					</span>
				{/if}
			{/if}
		</div>
		{#if session?.wide != null && open === null}
			<TagFields {session} wide={session.wide} />
		{/if}
	</div>
</section>

<style>
	/* The stage spans the window: its words and cover keep to the frame's gutter, or 64 at least. */
	.hero {
		--hero-pad: max(64px, var(--gutter));
		--hero-cover: calc(380px * var(--step));
		--hero-type: var(--type-hero);

		container-type: inline-size;
		position: relative;
		height: var(--hero-height);
		transition: height var(--morph-slow) var(--curve-settle);
	}

	/* Editing takes the page in hand: the stage draws up, and the tracks rise to meet it. */
	.hero.editing {
		--hero-cover: 260px;

		transition-duration: var(--morph);
		transition-timing-function: var(--curve-morph);
	}

	.art {
		position: absolute;
		right: var(--hero-pad);
		top: calc(120px * var(--step));
		width: var(--hero-cover);
		height: var(--hero-cover);
		border-radius: var(--radius-24);
		overflow: hidden;
		box-shadow: var(--shadow-hero);
		transition:
			width var(--morph-slow) var(--curve-settle),
			height var(--morph-slow) var(--curve-settle),
			top var(--morph-slow) var(--curve-settle);
	}

	.editing .art {
		top: 104px;
		transition-duration: var(--morph);
		transition-timing-function: var(--curve-morph);
	}

	/* One track has the stage: its cover has flown down to the panel, so the words have the room. */
	.one {
		--hero-cover: 0px;
	}

	.one .art {
		visibility: hidden;
	}

	.one .actions {
		display: none;
	}

	.words {
		position: absolute;
		left: var(--hero-pad);
		right: var(--hero-pad);
		bottom: calc(120px * var(--step));
		display: flex;
		flex-direction: column;
		gap: calc(18px * var(--step));
		transition: bottom var(--morph-slow) var(--curve-settle);
	}

	.editing .words {
		bottom: 70px;
		gap: 0;
		transition-duration: var(--morph);
		transition-timing-function: var(--curve-morph);
	}

	.one .words {
		bottom: 119px;
	}

	.meta {
		font-family: var(--font-mono);
		font-size: calc(13px * var(--step));
		color: var(--wash);
		opacity: 0;
		transition: opacity var(--morph-fast);
	}

	.meta.ready {
		opacity: 1;
	}

	.eyebrow {
		margin-bottom: 8px;
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		outline: none;
	}

	/*
	 * One line, fit by length, while it can stay headline-sized; past that two lines at a floor.
	 * The padding keeps descenders out of the clip: this line height is tighter than the glyphs.
	 */
	h1 {
		--room: calc(100cqw - 2 * var(--hero-pad) - var(--hero-cover) - 32px);
		--fit: calc(var(--room) / max(var(--width), 1));

		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		overflow: hidden;
		width: fit-content;
		max-width: var(--room);
		padding-bottom: 0.09em;
		margin-bottom: -0.09em;
		font-size: max(min(var(--hero-type), var(--fit)), clamp(32px, var(--fit) * 2, 44px));
		font-weight: 900;
		letter-spacing: var(--tracking-hero);
		line-height: 0.92;
		text-wrap: balance;
		transition: font-size var(--morph-slow) var(--curve-settle);
	}

	.editing h1 {
		--hero-type: 120px;

		line-height: 0.9;
		transition-duration: var(--morph);
		transition-timing-function: var(--curve-morph);
	}

	.one h1 {
		--hero-type: 88px;

		letter-spacing: -0.06em;
	}

	.editing .actions {
		margin: 18px 0 26px;
	}

	.editing .artist {
		font-size: 20px;
		margin-right: 0;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 14px;
	}

	.play {
		width: calc(72px * var(--step));
		height: calc(72px * var(--step));
		border: 0;
		border-radius: 50%;
		background: var(--accent);
		color: var(--on-accent);
		display: flex;
		align-items: center;
		justify-content: center;
		box-shadow: 0 16px 40px color-mix(in srgb, var(--accent) 30%, transparent);
	}

	/* As wide as the words, whatever room the row gives the artist: a flight measures this. */
	.name {
		display: inline-block;
		max-width: 100%;
	}

	/* The name on the stage, and 44px to press however small it gets. */
	.name a {
		position: relative;
	}

	.name a::after {
		content: '';
		position: absolute;
		inset: -11px 0;
	}

	.name a:hover {
		color: var(--accent);
	}

	/* Only until the tracks stream in, which is usually before anyone could press it. */
	.play:disabled {
		cursor: default;
	}

	.artist {
		min-width: 0;
		font-size: calc(22px * var(--step));
		font-weight: 700;
		letter-spacing: var(--tracking-title);
		margin-right: 12px;
		overflow-wrap: anywhere;
	}

	.ghost {
		height: var(--target);
		padding: 0 calc(18px * var(--step));
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: color-mix(in srgb, var(--ground) 30%, transparent);
		color: var(--text);
		font-size: calc(14px * var(--step));
		font-weight: 600;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.ghost.round {
		width: var(--target);
		padding: 0;
	}

	.round svg {
		width: calc(18px * var(--step));
		height: calc(18px * var(--step));
		fill: currentColor;
	}

	.ghost:disabled {
		color: var(--text-label);
		cursor: default;
	}

	/* The ghost buttons' ring, on the art: so it wears the accent. */
	.favorite {
		--heart-on: var(--accent);
		--heart-off: var(--text);

		display: flex;
	}

	.favorite :global(.heart) {
		border: 1px solid var(--edge-strong);
		background: color-mix(in srgb, var(--ground) 30%, transparent);
	}

	.rating {
		margin-left: 14px;
		display: flex;
		align-items: baseline;
		gap: 8px;
	}

	.glyphs {
		font-size: calc(18px * var(--step));
		color: var(--accent);
	}

	.from {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-secondary);
		white-space: nowrap;
	}

	@media (max-width: 820px) {
		.hero {
			--hero-pad: 16px;

			height: auto;
			padding: 96px var(--hero-pad) 96px;
			display: flex;
			flex-direction: column;
			gap: 24px;
		}

		.art {
			position: relative;
			inset: auto;
			align-self: center;
			width: min(280px, 72cqw);
			height: auto;
			aspect-ratio: 1;
		}

		.words {
			position: static;
			gap: 14px;
		}

		h1 {
			--room: calc(100cqw - 2 * var(--hero-pad));

			font-size: max(min(96px, var(--fit)), clamp(22px, var(--fit) * 2, 30px));
		}

		.play {
			width: 56px;
			height: 56px;
		}

		/* A phone edits the words alone: there's no room for the cover, and the fields need it. */
		.hero.editing {
			display: block;
			height: var(--hero-height);
			padding: 0;
		}

		.editing .art {
			display: none;
		}

		.editing .words {
			position: absolute;
			left: var(--hero-pad);
			right: var(--hero-pad);
			bottom: 44px;
		}

		.editing h1 {
			--hero-type: 56px;

			font-size: max(min(var(--hero-type), var(--fit)), clamp(22px, var(--fit) * 2, 30px));
		}

		.editing .actions {
			margin: 12px 0 16px;
		}

		.editing .artist {
			flex-basis: auto;
			font-size: 15px;
		}

		.writes {
			display: none;
		}

		.artist {
			font-size: 18px;
			flex-basis: calc(100% - 70px);
			margin-right: 0;
		}
	}
</style>
