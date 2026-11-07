<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { afterNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { emWidth } from '#lib/library/fit.ts';
	import { artistHoldings, libraryLine } from '#lib/library/format.ts';
	import { emptyLine } from '#lib/library/home.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import LibraryAlbum from '#lib/pages/LibraryAlbum.svelte';
	import LibraryRail from '#lib/pages/LibraryRail.svelte';
	import { letterOf, railHere, railLetters } from '#lib/pages/libraryRows.ts';
	import { libraryView as view } from '#lib/pages/libraryView.svelte.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import HeartButton from '#lib/shell/HeartButton.svelte';

	/** The headline's weight and tracking: the fit is worked out for exactly these. */
	const nameWeight = 900;
	const nameTracking = -0.06;

	const facts = $derived(library.facts);
	const load = $derived(library.index);
	const layout = $derived(view.layout);
	const artist = $derived(layout.artists.at(view.here)?.artist);
	const next = $derived(layout.artists.at(view.here + 1)?.artist);
	const letter = $derived(artist === undefined ? undefined : letterOf(artist.sortName));
	const letters = $derived(railLetters(layout, view.stacked));
	const songs = $derived(layout.rows.filter(({ kind }) => kind === 'song').length);
	const cuedId = $derived(library.playing?.track.id);

	/**
	 * Whether the page has landed. Before that its scroll isn't final, and reading it now would swap
	 * the song the user came back to for the top of the list.
	 */
	let landed = false;

	function track(): void {
		view.track(scrollY, innerHeight, landed);
	}

	function land(): void {
		scrollTo({ top: view.landing(), behavior: 'instant' });
		view.track(scrollY, innerHeight, false);
	}

	/** Puts a letter's first artist at the top, at once: a jump never animates a scroll. */
	function jump(to: string): void {
		scrollTo({ top: view.jump(to), behavior: 'instant' });
		track();
	}

	function widest(name: string): number {
		return Math.max(...name.split(' ').map((word) => emWidth(word, nameWeight, nameTracking)), 0.1);
	}

	// A scan or an edit ending asks again. Untracked: a failure must wait for Try again.
	$effect(() => {
		if (facts !== null && (library.indexStale || library.index.kind === 'loading')) {
			untrack(() => void library.loadIndex());
		}
	});

	// A new layout has moved every pixel: a new width, or a new list, finds the same song again.
	$effect(() => {
		void view.layout;
		if (landed) {
			untrack(land);
		}
	});

	// Here and not at mount: Kit scrolls, for a link or from history, after the page has mounted.
	afterNavigate(() => {
		land();
		landed = true;
	});

	onMount(() => {
		let frame = 0;
		function onScroll(): void {
			frame ||= requestAnimationFrame(() => {
				frame = 0;
				track();
			});
		}
		addEventListener('scroll', onScroll, { passive: true });
		addEventListener('resize', onScroll, { passive: true });
		const stopWatching = view.watch();
		return () => {
			cancelAnimationFrame(frame);
			removeEventListener('scroll', onScroll);
			removeEventListener('resize', onScroll);
			stopWatching();
		};
	});
</script>

<svelte:head>
	<title>Library · Kitsune</title>
</svelte:head>

<div class="library">
	<h1>Library</h1>

	{#if facts !== null && facts.albums === 0}
		<p class="note" role="status">{emptyLine(facts, library.deck)}</p>
	{:else if load.kind === 'unavailable'}
		<p class="note" role="status">
			The library couldn't be loaded.
			<button type="button" onclick={() => void library.loadIndex()}>Try again</button>
		</p>
	{:else if load.kind === 'ready'}
		<div class="veil" aria-hidden="true"></div>
		{#if letter !== undefined}
			<p class="frame" data-morph-part="intro" data-morph-key="frame">
				{libraryLine(letter, layout.artists.length, songs)}
			</p>
		{/if}
		{#if next !== undefined}
			<div class="next" class:ahead={view.nextAhead} data-morph-part="intro" data-morph-key="next">
				<span class="next-label">NEXT</span>
				<span class="next-name" title={next.name}>{next.name}</span>
			</div>
		{/if}

		<div class="list" style:height={`${String(layout.height)}px`}>
			{#each view.mounted as { index, block, albums } (index)}
				{@const { name, id } = block.artist}
				<section
					class="artist"
					aria-label={name}
					style:top={`${String(block.top)}px`}
					style:height={`${String(block.height)}px`}
				>
					<div class="head-column">
						<div
							class="head"
							data-morph-part="intro"
							data-morph-key={`head:${name}`}
							data-morph-pinned
						>
							<!-- The fit divides by the words' width, known before layout: the first frame is final. -->
							<h2
								title={name}
								style:--all={emWidth(name, nameWeight, nameTracking)}
								style:--word={widest(name)}
							>
								{#if id === undefined}
									{name}
								{:else}
									<a href={resolve('/artists/[id]', { id })}>{name}</a>
								{/if}
							</h2>
							<div class="facts">
								{#if id !== undefined}
									<HeartButton kind="artists" {id} {name} size={18} />
								{/if}
								<p class="holdings">{artistHoldings(block.artist.albums)}</p>
							</div>
						</div>
					</div>
					<div class="albums">
						{#each albums as { block: album, rows } (album.album.id)}
							<LibraryAlbum
								block={album}
								top={album.top - block.top}
								{rows}
								head={view.frame.metrics.albumHead}
								stacked={view.stacked}
								{cuedId}
								sounding={player.sounding}
								trouble={view.trouble === album.album.id}
								play={(trackId) => {
									view.play(album.album.id, trackId);
								}}
							/>
						{/each}
					</div>
				</section>
			{/each}
		</div>

		<LibraryRail {letters} here={letter && railHere(letters, letter)} onjump={jump} />
	{/if}
</div>

<style>
	.library {
		--name-column: clamp(220px, 23.6vw, 340px);
		--page-edge: max(64px, calc((100vw - 1440px) / 2 + 64px));

		position: relative;
		max-width: 1440px;
		margin: 0 auto;
		padding: 96px 0 140px;
	}

	/* The boards have no page title: the pinned artist is the headline. Screen readers still get one. */
	h1 {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	.note {
		min-height: var(--target);
		margin-left: var(--page-edge);
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
		color: var(--fox);
	}

	.veil {
		display: none;
	}

	/* Solid, because the name it stands over leaves upward, past it. */
	.frame {
		position: fixed;
		z-index: 2;
		top: 96px;
		left: var(--page-edge);
		width: var(--name-column);
		padding: 8px 0 16px;
		background: var(--ground);
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.next {
		position: fixed;
		bottom: 200px;
		left: var(--page-edge);
		width: var(--name-column);
		padding-top: 16px;
		border-top: 1px solid var(--edge);
		display: flex;
		flex-direction: column;
		gap: 6px;
		opacity: 0;
		visibility: hidden;
		transition:
			opacity var(--morph-fast) var(--curve-morph),
			visibility 0s var(--morph-fast);
	}

	.next.ahead {
		opacity: 1;
		visibility: visible;
		transition: opacity var(--morph-fast) var(--curve-morph);
	}

	.next-label {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.next-name {
		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		overflow: hidden;
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
		line-height: 1.1;
		overflow-wrap: anywhere;
		color: var(--text-label);
	}

	.list {
		position: relative;
	}

	.artist {
		position: absolute;
		left: 0;
		right: 0;
	}

	.head-column {
		position: absolute;
		z-index: 1;
		top: 0;
		bottom: 0;
		left: 64px;
		width: var(--name-column);
		pointer-events: none;
	}

	/* The name stays while its albums scroll; the next one pushes it out. */
	.head {
		position: sticky;
		top: 134px;
		pointer-events: auto;
	}

	/* Three lines at the most, and no word cut: the board's size for any name that fits it. */
	h2 {
		--fit: min(calc(var(--name-column) / var(--word)), calc(var(--name-column) * 2.5 / var(--all)));

		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		overflow: hidden;
		padding-bottom: 0.09em;
		margin-bottom: -0.09em;
		font-size: clamp(24px, var(--fit), var(--type-display));
		font-weight: 900;
		letter-spacing: -0.06em;
		line-height: 0.88;
		overflow-wrap: anywhere;
	}

	h2 a:hover {
		color: var(--fox);
	}

	/* Amber: the artist is the frame's, not a cover's. */
	.facts {
		--heart-on: var(--fox);
		--heart-off: var(--text-secondary);

		margin-top: 14px;
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.facts :global(.heart) {
		margin-left: -12px;
	}

	.holdings {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-secondary);
	}

	.albums {
		position: absolute;
		top: 0;
		bottom: 0;
		left: calc(64px + var(--name-column) + 44px);
		right: 92px;
	}

	@media (max-height: 720px) {
		.next {
			display: none;
		}
	}

	/* The columns stack. The list starts under the nav, and a veil keeps songs from passing behind it. */
	@media (max-width: 1024px) {
		.library {
			padding: 76px 0 128px;
		}

		.veil {
			display: block;
			position: fixed;
			z-index: 2;
			inset: 0 0 auto;
			height: 76px;
			background: var(--ground);
		}

		.frame,
		.next {
			display: none;
		}

		.head-column {
			left: 0;
			right: 0;
			width: auto;
		}

		.head {
			top: 76px;
			height: 80px;
			padding: 8px 48px 12px 16px;
			background: var(--ground);
			border-bottom: 1px solid var(--edge);
		}

		h2 {
			display: block;
			padding: 0;
			margin: 0;
			overflow: hidden;
			text-overflow: ellipsis;
			white-space: nowrap;
			font-size: clamp(22px, calc((100vw - 64px) / var(--all)), var(--type-section));
			letter-spacing: -0.05em;
			line-height: 0.95;
		}

		/* The head is 80 tall: the heart keeps its target, and lets it overhang the line. */
		.facts {
			height: 22px;
			margin-top: 2px;
		}

		.albums {
			left: 16px;
			right: 48px;
		}

		.note {
			margin-left: 16px;
		}
	}
</style>
