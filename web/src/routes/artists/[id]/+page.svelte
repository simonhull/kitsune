<script lang="ts">
	import { untrack } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import { imageCredits, phoneStagePicture } from '#lib/art/art.ts';
	import ArtistBio from '#lib/art/ArtistBio.svelte';
	import ArtistThumb from '#lib/art/ArtistThumb.svelte';
	import { heroStage } from '#lib/art/stage.svelte.ts';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { emWidth } from '#lib/library/fit.ts';
	import { artistFacts } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { artistAlbums } from '#lib/library/albumOrder.ts';
	import { artistAlbumOrders, wayOf } from '#lib/library/sorts.ts';
	import AlbumShelf from '#lib/pages/AlbumShelf.svelte';
	import ArtistSongs from '#lib/pages/ArtistSongs.svelte';
	import SortControl, { type SortGroup } from '#lib/pages/SortControl.svelte';
	import { sorts } from '#lib/pages/sortsView.svelte.ts';
	import { morph } from '#lib/morph/registry.ts';
	import { chooser } from '#lib/choose/chooser.svelte.ts';
	import type { PageProps } from './$types';

	let { params }: PageProps = $props();

	const id = $derived(params.id);
	const load = $derived(library.artist(id));
	const artist = $derived(load.kind === 'ready' ? load.value : undefined);
	/** Known from whatever linked here, so the first frame has its headline. */
	const name = $derived(artist?.name ?? library.artistName(id) ?? '');
	/** Its thumb: the page's own once read, the list's before, so the first frame has it too. */
	const thumb = $derived(
		artist === undefined ? library.artistThumb(id) : (artist.images.thumb ?? null)
	);
	/** The stage behind the hero, which the shell draws: the words stand at its foot. */
	const stage = $derived(heroStage(id));
	/** On a phone the square thumb is the stage, so the head has none of its own. */
	const phone = new MediaQuery('max-width: 430px');
	const credits = $derived(artist === undefined ? [] : imageCredits(artist.images));

	/** Its albums go as an artist's do on the Library: one order, `library.sort`'s albums. */
	const order = $derived(sorts.library.value.albums);
	const sortGroups: SortGroup[] = $derived([
		{
			label: 'SORT ALBUMS BY',
			options: artistAlbumOrders,
			current: order,
			choose: (next) => {
				sorts.library.set({ ...sorts.library.value, albums: next as typeof order });
			}
		}
	]);

	$effect(() => {
		void sorts.library.load();
	});

	// Never awaited, like an album's tracks. Untracked: a failure must wait for Try again. The songs
	// are asked for beside it, by the section, so neither waits for the other.
	$effect(() => {
		const wanted = id;
		untrack(() => void library.loadArtist(wanted));
	});

	/** Administrators choose the artist's pictures from the hero's ⋯. */
	const choosing = $derived(chooser.can && artist !== undefined);

	// Leaving the artist, or going to another, takes their menu with them.
	$effect(() => {
		const here = id;
		return () => {
			if (chooser.current?.artistId === here) {
				chooser.close(false);
			}
		};
	});

	function openMenu(event: MouseEvent & { currentTarget: HTMLButtonElement }): void {
		if (artist === undefined) {
			return;
		}
		const { left, right, top, bottom } = event.currentTarget.getBoundingClientRect();
		chooser.open({
			artistId: id,
			name: artist.name,
			mbid: artist.mbid,
			anchor: { left, right, top, bottom }
		});
	}

	/** The full list of songs shows: a view in the URL, so Back folds it and a link lands on it. */
	const inUrl = $derived(page.url.searchParams.has('songs'));
	/**
	 * A press shows its view at once, inside the press, and the URL follows: a navigation is a few
	 * promises long, and nothing a person sees waits for it.
	 */
	let pressed = $state<boolean | null>(null);
	const expanded = $derived(pressed ?? inUrl);
	// The URL has caught up, or Back or Forward moved it: it says again.
	$effect(() => {
		void inUrl;
		pressed = null;
	});
	/** Set when this visit added the `?songs` entry, so folding is going back to the one before. */
	let pushed = false;

	function expand(): void {
		pushed = true;
		pressed = true;
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- Built from resolve('/artists/[id]').
		void goto(`${resolve('/artists/[id]', { id })}?songs`, { reset: false });
	}

	function fold(): void {
		pressed = false;
		if (pushed) {
			pushed = false;
			history.back();
			return;
		}
		void goto(resolve('/artists/[id]', { id }), { replace: true, reset: false });
	}
</script>

<svelte:head>
	<title>{name === '' ? 'Artist' : name} · Kitsune</title>
</svelte:head>

{#if load.kind === 'missing'}
	<div class="missing">
		<h1>That artist isn't in the library</h1>
		<a href={resolve('/artists')}>All artists</a>
	</div>
{:else}
	<div
		class="page-frame artist"
		class:staged={stage?.wide === true}
		class:square={stage?.square === true}
	>
		<div class="words" data-morph-part="words" data-morph-key="words">
			<div class="head">
				{#if !(phone.current && stage !== null && phoneStagePicture(stage.images) !== undefined)}
					<ArtistThumb artistId={id} {name} {thumb} size={112} slot="hero" />
				{/if}
				<div class="facts">
					<span class="meta">{artist === undefined ? '' : artistFacts(artist)}&nbsp;</span>
					{#if credits.length > 0}
						<span class="credit">
							{#each credits as line, at (at)}
								<span class="line">
									{#each line as part, index (index)}
										{#if index > 0}<span class="dot" aria-hidden="true">·</span>{/if}
										{#if part.href === undefined}
											<span>{part.text}</span>
										{:else}
											<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- The source's own page, not a route. -->
											<a href={part.href} target="_blank" rel="noreferrer noopener">{part.text}</a>
										{/if}
									{/each}
								</span>
							{/each}
						</span>
					{/if}
				</div>
			</div>
			<div class="name" class:choosing>
				<!-- The fit divides by the words' width, known before layout: the first frame is final. -->
				<h1
					title={name}
					style:--width={emWidth(name, 900, -0.07)}
					use:morph={{ id: `name:${id}`, slot: 'hero', kind: 'title' }}
				>
					{name}
				</h1>
				{#if choosing}
					<button
						type="button"
						class="more"
						aria-label={`More for ${name}`}
						aria-haspopup="menu"
						aria-expanded={chooser.current?.artistId === id}
						onclick={openMenu}
						><svg width="20" height="20" viewBox="0 0 24 24" aria-hidden="true"
							><path
								d="M5 12h.01M12 12h.01M19 12h.01"
								fill="none"
								stroke="currentColor"
								stroke-width="3.4"
								stroke-linecap="round"
							></path></svg
						></button
					>
				{/if}
			</div>
		</div>

		{#if load.kind === 'unavailable'}
			<p class="note" role="status" data-morph-part="panel" data-morph-key="note">
				This artist couldn't be loaded.
				<button type="button" onclick={() => void library.loadArtist(id)}>Try again</button>
			</p>
		{:else}
			<!-- Before the artist comes, its songs may: the section holds its place either way. -->
			<div class="body">
				<ArtistSongs {id} {name} detail={artist} {expanded} {expand} {fold}>
					{#snippet shelves()}
						{#if artist !== undefined}
							{#if artist.albums.length > 0}
								<section aria-labelledby="own-albums">
									<!-- Its covers are units of their own: units can't nest. -->
									<div class="heading" data-morph-part="panel" data-morph-key="own-albums">
										<h2 id="own-albums">Albums</h2>
										<SortControl
											groups={sortGroups}
											words={artistAlbumOrders.find(({ by }) => by === order.by)?.label ?? ''}
											way={wayOf(order.by, order.dir)}
											look="pill"
											label="Sort the albums"
										/>
									</div>
									<AlbumShelf albums={artistAlbums(artist.albums, order)} />
								</section>
							{/if}
							{#if artist.appearsOn.length > 0}
								<section aria-labelledby="appears-on">
									<h2 id="appears-on" data-morph-part="panel" data-morph-key="appears-on">
										Appears on
									</h2>
									<AlbumShelf albums={artistAlbums(artist.appearsOn, order)} />
								</section>
							{/if}
							{#if artist.albums.length === 0 && artist.appearsOn.length === 0}
								<p class="note" role="status" data-morph-part="panel" data-morph-key="nothing">
									Nothing of theirs is in the library right now.
								</p>
							{/if}
							{#if artist.bio !== undefined}
								<ArtistBio {name} bio={artist.bio} />
							{/if}
						{/if}
					{/snippet}
				</ArtistSongs>
			</div>
		{/if}
	</div>
{/if}

<style>
	.artist {
		--frame-top: 120px;

		container-type: inline-size;
		display: flex;
		flex-direction: column;
		gap: calc(56px * var(--step));
	}

	.words {
		display: flex;
		flex-direction: column;
		gap: calc(24px * var(--step));
	}

	/* On a stage the words stand at its foot, under the picture, the name last. */
	.staged .words {
		min-height: calc(var(--artist-stage) - var(--frame-top) * var(--step) - 40px * var(--step));
		justify-content: flex-end;
	}

	/* The thumb, round, beside the facts and where they came from. */
	.head {
		display: flex;
		align-items: center;
		gap: calc(22px * var(--step));
		min-width: 0;
	}

	.head :global(.thumb) {
		box-shadow:
			inset 0 0 0 2px var(--edge-raised),
			0 18px 40px color-mix(in srgb, var(--ground) 45%, transparent);
	}

	.facts {
		display: flex;
		flex-direction: column;
		gap: calc(10px * var(--step));
		min-width: 0;
	}

	.meta {
		font-family: var(--font-mono);
		font-size: calc(13px * var(--step));
		color: var(--text-secondary);
	}

	.credit {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.credit .dot {
		margin: 0 0.6em;
	}

	.credit a {
		color: inherit;
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.credit a:hover {
		color: var(--fox-text);
	}

	/* One line, fit by length, while it can stay headline-sized; past that two lines at a floor. */
	.words h1 {
		--room: calc(100cqw - 64px);
		--fit: calc(var(--room) / max(var(--width), 1));

		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		overflow: hidden;
		max-width: var(--room);
		padding-bottom: 0.09em;
		margin-bottom: -0.09em;
		font-size: max(min(var(--type-hero), var(--fit)), clamp(32px, var(--fit) * 2, 44px));
		font-weight: 900;
		letter-spacing: var(--tracking-hero);
		line-height: 0.92;
		overflow-wrap: anywhere;
		text-wrap: balance;
	}

	/* The name, and for an administrator the ⋯ at the gutter beside it. */
	.name {
		position: relative;
	}

	.choosing h1 {
		--room: calc(100cqw - 64px - 60px * var(--step));
	}

	.more {
		position: absolute;
		right: 0;
		top: calc(41px * var(--step));
		width: calc(44px * var(--step));
		height: calc(44px * var(--step));
		display: grid;
		place-items: center;
		border: 0;
		border-radius: 50%;
		background: color-mix(in srgb, var(--ground) 35%, transparent);
		box-shadow: inset 0 0 0 1px var(--edge-strong);
		color: var(--text);
		cursor: pointer;
	}

	.more:hover,
	.more[aria-expanded='true'] {
		background: var(--hover);
	}

	.more:focus-visible {
		outline: 2px solid var(--fox-line);
		outline-offset: 3px;
	}

	.body {
		display: flex;
		flex-direction: column;
	}

	section {
		display: flex;
		flex-direction: column;
		gap: calc(24px * var(--step));
	}

	.heading {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 12px 24px;
	}

	h2 {
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.note {
		min-height: var(--target);
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

	.missing {
		min-height: 100dvh;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 16px;
		padding: 16px;
		text-align: center;
	}

	.missing h1 {
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.missing a {
		min-height: var(--target);
		display: flex;
		align-items: center;
		color: var(--fox-text);
	}

	@media (max-width: 720px) {
		.artist {
			--frame-top: 104px;

			gap: 40px;
		}

		.words h1 {
			--room: calc(100cqw - 32px);
		}

		.choosing h1 {
			--room: calc(100cqw - 32px - 56px);
		}

		.more {
			top: auto;
			bottom: 0;
		}

		h2 {
			font-size: 32px;
		}
	}

	/*
	 * A phone stands on the square thumb: the facts above the name and the credit under it, the head
	 * opened so the three are one column.
	 */
	@media (max-width: 430px) {
		.square .words {
			min-height: calc(var(--artist-stage) - 104px + 8px);
			justify-content: flex-end;
			gap: 10px;
		}

		.square .head,
		.square .facts {
			display: contents;
		}

		.square .meta {
			order: 1;
			font-size: 11px;
		}

		.square .words h1 {
			order: 2;
		}

		.square .credit {
			order: 3;
			font-size: 10px;
		}
	}
</style>
