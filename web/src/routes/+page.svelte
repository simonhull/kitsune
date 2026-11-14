<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import {
		count,
		emptyLine,
		everythingNote,
		greeting,
		homeRows,
		playsLine,
		summary
	} from '#lib/library/home.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import ContinueCard from '#lib/pages/ContinueCard.svelte';
	import Deck from '#lib/pages/Deck.svelte';
	import HomeCover from '#lib/pages/HomeCover.svelte';
	import HomeRow from '#lib/pages/HomeRow.svelte';
	import ShuffleTile from '#lib/pages/ShuffleTile.svelte';
	import { librarySource } from '#lib/shuffle/sources.ts';

	const facts = $derived(library.facts);
	const name = $derived(library.user?.name ?? '');
	/** Under a deck that's dealing, Home is already there. */
	const showsHome = $derived(!library.deck || library.dealing !== 'no');
	const rows = $derived(
		homeRows(
			library.trashed.empty
				? library.recent
				: library.recent.filter(({ id }) => !library.trashed.hidesAlbum(id)),
			library.history,
			library.playing?.album.id ?? null
		)
	);
	const artists = $derived(library.artists.kind === 'ready' ? library.artists.value.length : null);

	const everything = librarySource('everything');
	const favourites = librarySource('rated');

	// The copy here shows at once; one over a minute old is read again, and swapped in place.
	onMount(() => {
		void library.freshenHistory();
	});
</script>

<svelte:head>
	<title>Kitsune</title>
</svelte:head>

{#if facts !== null}
	<div class="page-frame home">
		<div class="lead">
			<div class="intro" data-morph-part="intro" data-morph-key="intro">
				<h1>{greeting(new Date().getHours(), name)}</h1>
				{#if facts.albums > 0 && (!library.deck || library.dealing === 'dealt')}
					{@const [albums, songs, hours] = summary(facts)}
					<span class="summary" class:arriving={library.deck}
						>{`${albums} · ${songs}`}<span class="hours">{` · ${hours}`}</span></span
					>
				{/if}
			</div>

			{#if library.deck}
				<Deck {facts} />
			{/if}
			{#if showsHome}
				{#if facts.albums === 0}
					<p class="empty" role="status">{emptyLine(facts, false)}</p>
				{:else}
					<section aria-label="Start" class="start">
						<ContinueCard feature={rows.feature} />
						<ShuffleTile
							source={everything}
							look="system"
							title="Everything"
							short="Everything"
							note={everythingNote(facts.tracks, artists)}
							morphKey="shuffle"
						/>
						<ShuffleTile
							source={favourites}
							look="card"
							title="Your favourites"
							short="Favourites"
							note="4★ and up"
							morphKey="favourites"
						/>
					</section>
				{/if}
			{/if}
		</div>

		<!--
			The rows come in together once the history is known, the first time only: below the start
			row, so nothing on screen moves for them. After that the copy here shows at once.
		-->
		{#if showsHome && facts.albums > 0 && library.history !== null}
			<div class="rows">
				{#if rows.played.length > 0}
					<HomeRow
						label="Recently played"
						morphKey="played"
						items={rows.played}
						key={(album) => album.id}
					>
						{#snippet item(album)}<HomeCover {album} />{/snippet}
					</HomeRow>
				{/if}
				{#if rows.onRepeat.length > 0}
					<HomeRow
						label="On repeat"
						note="the last 90 days"
						morphKey="repeat"
						items={rows.onRepeat}
						key={({ album }) => album.id}
					>
						{#snippet item({ album, plays })}<HomeCover
								{album}
								extra={playsLine(plays)}
							/>{/snippet}
					</HomeRow>
				{/if}
				<HomeRow label="Just added" morphKey="recent" items={rows.added} key={(album) => album.id}>
					{#snippet item(album)}<HomeCover {album} />{/snippet}
					{#snippet aside()}<a class="all-link" href={resolve('/albums')}>All albums</a>{/snippet}
					{#snippet end()}
						<a class="all" href={resolve('/albums')}>
							<span class="all-count">{count(facts.albums)} albums</span>
							<span class="all-name"
								>All albums<svg
									width="20"
									height="20"
									viewBox="0 0 24 24"
									fill="none"
									stroke-width="2.2"
									stroke-linecap="round"
									stroke-linejoin="round"
									aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6"></path></svg
								></span
							>
						</a>
					{/snippet}
				</HomeRow>
			</div>
		{/if}
	</div>
{/if}

<style>
	.home {
		display: flex;
		flex-direction: column;
		gap: calc(56px * var(--step));
	}

	.lead,
	.rows {
		display: flex;
		flex-direction: column;
		gap: calc(56px * var(--step));
		min-width: 0;
	}

	.intro {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 12px 24px;
	}

	h1 {
		font-size: clamp(44px, 7vw, var(--type-display));
		font-weight: 800;
		letter-spacing: var(--tracking-display);
		line-height: 0.9;
		text-wrap: balance;
	}

	.summary {
		font-family: var(--font-mono);
		font-size: calc(13px * var(--step));
		color: var(--text-meta);
	}

	/* It takes over from the deck's count, which fades as this comes up. */
	.arriving {
		animation: summary-arrives var(--morph) var(--curve-morph) 160ms backwards;
	}

	@keyframes summary-arrives {
		from {
			opacity: 0;
		}
	}

	.empty {
		max-width: 44ch;
		font-size: var(--type-title);
		line-height: 1.4;
		color: var(--text-secondary);
	}

	/* One big way back in, two ways to shuffle: three columns, two rows of 208. */
	.start {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		grid-template-rows: repeat(2, calc(208px * var(--step)));
		gap: calc(16px * var(--step));
		margin-top: calc(-8px * var(--step));
	}

	/* The row's last card: the way to every album, the size of a cover. */
	.all {
		box-sizing: border-box;
		width: var(--home-cover);
		height: var(--home-cover);
		padding: calc(18px * var(--step));
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-20);
		color: var(--text);
	}

	.all:hover {
		border-color: var(--edge-hover);
	}

	.all-count {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	.all-name {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		font-size: calc(20px * var(--step));
		font-weight: 800;
		letter-spacing: -0.03em;
		line-height: 1;
	}

	.all-name svg {
		flex: none;
		width: calc(20px * var(--step));
		height: calc(20px * var(--step));
		stroke: var(--fox-text);
	}

	/* A phone's way to every album: a link beside the heading, where the card would scroll away. */
	.all-link {
		display: none;
		align-items: center;
		min-height: var(--target);
		font-size: 14px;
		font-weight: 600;
		color: var(--fox-text);
	}

	/*
	 * From 2560 the page is two columns: the greeting and the ways in on the left, the rows on the
	 * right, so a 4K screen holds all of it at once.
	 */
	@media (min-width: 2560px) {
		.home {
			display: grid;
			grid-template-columns: minmax(0, 46fr) minmax(0, 70fr);
			align-items: start;
			gap: calc(96px * var(--step));
		}

		.lead {
			gap: calc(24px * var(--step));
		}

		.intro {
			flex-direction: column;
			align-items: flex-start;
			gap: calc(18px * var(--step));
			margin-bottom: calc(32px * var(--step));
		}

		.start {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			grid-template-rows: auto calc(200px * var(--step));
			gap: calc(24px * var(--step));
			margin-top: 0;
		}

		.rows {
			gap: calc(64px * var(--step));
			padding-top: calc(8px * var(--step));
		}
	}

	@media (max-width: 720px) {
		.home,
		.lead,
		.rows {
			gap: 40px;
		}

		.intro {
			flex-direction: column;
			align-items: flex-start;
			gap: 10px;
		}

		.summary {
			font-size: 12px;
		}

		.hours {
			display: none;
		}

		.start {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			grid-template-rows: none;
			gap: 12px;
			margin-top: -12px;
		}

		/* The way to every album moves up beside the heading, as a link. */
		.all {
			display: none;
		}

		.all-link {
			display: inline-flex;
		}
	}
</style>
