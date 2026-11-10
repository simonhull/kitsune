<script lang="ts">
	import { untrack } from 'svelte';
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
	import type { PageProps } from './$types';

	let { params }: PageProps = $props();

	const id = $derived(params.id);
	const load = $derived(library.artist(id));
	const artist = $derived(load.kind === 'ready' ? load.value : undefined);
	/** Known from whatever linked here, so the first frame has its headline. */
	const name = $derived(artist?.name ?? library.artistName(id) ?? '');

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
	<div class="page-frame artist">
		<div class="words">
			<span class="meta">{artist === undefined ? '' : artistFacts(artist)}&nbsp;</span>
			<!-- The fit divides by the words' width, known before layout: the first frame is final. -->
			<h1 title={name} style:--width={emWidth(name, 900, -0.07)}>{name}</h1>
		</div>

		{#if load.kind === 'unavailable'}
			<p class="note" role="status">
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
									<div class="heading">
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
									<h2 id="appears-on">Appears on</h2>
									<AlbumShelf albums={artistAlbums(artist.appearsOn, order)} />
								</section>
							{/if}
							{#if artist.albums.length === 0 && artist.appearsOn.length === 0}
								<p class="note" role="status">Nothing of theirs is in the library right now.</p>
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
		gap: calc(18px * var(--step));
	}

	.meta {
		font-family: var(--font-mono);
		font-size: calc(13px * var(--step));
		color: var(--text-secondary);
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

		h2 {
			font-size: 32px;
		}
	}
</style>
