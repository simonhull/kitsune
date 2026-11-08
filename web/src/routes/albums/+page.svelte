<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import {
		albumOrders,
		albumsQueryIn,
		albumsSearch,
		markAt,
		sameGenre,
		type AlbumsQuery
	} from '#lib/library/albumOrder.ts';
	import { albumsLine } from '#lib/library/format.ts';
	import { emptyLine } from '#lib/library/home.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import AlbumGrid from '#lib/pages/AlbumGrid.svelte';
	import AlbumRail from '#lib/pages/AlbumRail.svelte';
	import { albumsView as view } from '#lib/pages/albumsView.svelte.ts';

	/** How many genres get a chip of their own: as many as the board's row holds. */
	const chipCount = 5;

	let grid: AlbumGrid | undefined = $state();
	let everyGenre = $state(false);

	const facts = $derived(library.facts);
	/** Until the list is whole, its counts and its filter would only be half true. */
	const whole = $derived(!library.hasMoreAlbums);
	const chips = $derived.by(() => {
		const top = view.genres.slice(0, chipCount).map(({ name }) => name);
		const { genre } = view;
		const chosen = view.genres.find(({ name }) => genre !== null && sameGenre(name, genre));
		return chosen === undefined || top.includes(chosen.name) ? top : [...top, chosen.name];
	});
	const total = $derived(whole ? library.albums.length : (facts?.albums ?? 0));
	const line = $derived(
		albumsLine(view.albums.length, total, whole ? view.genres.length : 0, whole ? view.genre : null)
	);
	const here = $derived(markAt(view.marks, view.grid.reached));
	const railLabel = $derived(view.order === 'year' ? 'Jump to a decade' : 'Jump to a letter');

	// Before the template, so a first frame already shows what the URL asks for.
	$effect.pre(() => {
		if (page.route.id === '/albums') {
			view.show(albumsQueryIn(page.url));
		}
	});

	$effect(() => {
		// Sorting and filtering need all of it, and a scan ending can bring more.
		if (library.hasMoreAlbums) {
			void library.loadAllAlbums();
		}
	});

	function ask(query: AlbumsQuery): void {
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- Built from resolve('/albums').
		void goto(`${resolve('/albums')}${albumsSearch(query)}`, { replace: true, reset: false });
		// A different list starts at its top, and all genres open can leave the page far from there.
		scrollTo({ top: 0, behavior: 'instant' });
	}

	function chooseGenre(name: string): void {
		const { order, genre } = view;
		everyGenre = false;
		ask({ order, genre: genre !== null && sameGenre(name, genre) ? null : name });
	}

	function isChosen(name: string): boolean {
		return view.genre !== null && sameGenre(name, view.genre);
	}
</script>

<svelte:head>
	<title>Albums · Kitsune</title>
</svelte:head>

<div class="page-frame albums">
	<div class="intro">
		<h1>Albums</h1>
		{#if total > 0}
			<p class="facts">{line}</p>
		{/if}
	</div>

	{#if facts !== null && facts.albums === 0}
		<p class="note" role="status">
			{emptyLine(facts, library.deck)}
		</p>
	{:else if library.albums.length > 0}
		<div class="controls">
			<div class="sorts" role="group" aria-label="Sort by">
				{#each albumOrders as { order, label } (order)}
					<button
						type="button"
						class="sort"
						aria-pressed={view.order === order}
						onclick={() => {
							ask({ order, genre: view.genre });
						}}>{label}</button
					>
				{/each}
			</div>
			{#if chips.length > 0}
				<span class="rule" aria-hidden="true"></span>
				<div class="chips" role="group" aria-label="Filter by genre">
					{#each chips as name (name)}
						<button
							type="button"
							class="chip"
							aria-pressed={isChosen(name)}
							title={name}
							onclick={() => {
								chooseGenre(name);
							}}>{name}</button
						>
					{/each}
				</div>
				<button
					type="button"
					class="all"
					aria-expanded={everyGenre}
					aria-controls="every-genre"
					onclick={() => {
						everyGenre = !everyGenre;
					}}>All genres</button
				>
			{/if}
		</div>

		{#if everyGenre}
			<div class="every" id="every-genre" role="group" aria-label="Every genre">
				{#each view.genres.toSorted( (a, b) => a.name.localeCompare(b.name) ) as { name, albums } (name)}
					<button
						type="button"
						class="chip"
						aria-pressed={isChosen(name)}
						title={name}
						onclick={() => {
							chooseGenre(name);
						}}>{name} <span class="count">{albums.toLocaleString('en-US')}</span></button
					>
				{/each}
			</div>
		{/if}

		{#if view.albums.length > 0}
			<AlbumGrid grid={view.grid} bind:this={grid} />
		{:else if whole}
			<p class="note" role="status">
				No album here is tagged {view.genre}.
				<button
					type="button"
					onclick={() => {
						ask({ order: view.order, genre: null });
					}}>Show every album</button
				>
			</p>
		{/if}
	{/if}
</div>

{#if view.albums.length > 0 && view.marks.length > 1}
	<AlbumRail
		label={railLabel}
		marks={view.marks}
		{here}
		onjump={(index) => {
			grid?.jump(index);
		}}
	/>
{/if}

<style>
	/* The rail stands in the right gutter, widened for it, so the covers never run under it. */
	.albums {
		--frame-top: 96px;

		padding-right: max(var(--gutter), 104px);
		display: flex;
		flex-direction: column;
		gap: calc(21px * var(--step));
	}

	.intro {
		display: flex;
		flex-direction: column;
		gap: 11px;
	}

	h1 {
		font-size: clamp(44px, 7vw, var(--type-display));
		font-weight: 800;
		letter-spacing: var(--tracking-display);
		line-height: 0.9;
	}

	.facts {
		margin-left: 4px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.controls {
		min-width: 0;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
	}

	.sorts,
	.chips {
		min-width: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	.sort,
	.chip,
	.all {
		height: var(--target);
		flex: none;
		max-width: calc(240px * var(--step));
		border-radius: var(--radius-pill);
		font-size: calc(14px * var(--step));
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sort {
		padding: 0 calc(18px * var(--step));
		border: 1px solid var(--edge-strong);
		background: transparent;
	}

	.sort[aria-pressed='true'] {
		border-color: var(--text);
		background: var(--text);
		color: var(--ground);
	}

	.rule {
		width: 1px;
		height: 28px;
		margin: 0 8px;
		background: var(--edge-field);
	}

	.chip {
		padding: 0 calc(16px * var(--step));
		border: 0;
		background: var(--edge);
	}

	.chip[aria-pressed='true'] {
		background: var(--fox);
		color: var(--fox-ink);
	}

	.all {
		padding: 0 12px;
		border: 0;
		background: transparent;
		color: var(--text-secondary);
	}

	.all[aria-expanded='true'] {
		color: var(--fox-text);
	}

	.sort[aria-pressed='false']:hover,
	.chip[aria-pressed='false']:hover,
	.all:hover {
		color: var(--fox-text);
	}

	.every {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	.count {
		margin-left: 4px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		font-variant-numeric: tabular-nums;
		color: var(--text-meta);
	}

	[aria-pressed='true'] .count {
		color: inherit;
	}

	.note {
		min-height: var(--target);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0 8px;
		max-width: 44ch;
		font-size: var(--type-title);
		line-height: 1.4;
		color: var(--text-secondary);
	}

	.note button {
		min-height: var(--target);
		padding: 0;
		border: 0;
		background: none;
		color: var(--fox-text);
	}

	@media (max-width: 1100px) {
		.albums {
			padding-right: max(var(--gutter), 84px);
		}

		.rule {
			display: none;
		}
	}

	/* The chips run off the edge and scroll, and All genres stays in reach beside them. */
	@media (max-width: 720px) {
		.albums {
			--frame-top: 98px;

			padding-right: 36px;
			gap: 20px;
		}

		.sorts {
			flex: 1 0 100%;
			flex-wrap: nowrap;
			gap: 6px;
		}

		.sort {
			padding: 0 14px;
		}

		.chips {
			flex: 1 1 0;
			flex-wrap: nowrap;
			overflow-x: auto;
			scrollbar-width: none;
			/* raw-color: a mask reads only alpha. */
			mask-image: linear-gradient(to right, #000 calc(100% - 20px), transparent);
		}

		.all {
			padding: 0 0 0 4px;
		}
	}
</style>
