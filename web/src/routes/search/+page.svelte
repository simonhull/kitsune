<script lang="ts">
	import { onDestroy, untrack } from 'svelte';
	import { afterNavigate, beforeNavigate, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { library } from '#lib/library/library.svelte.ts';
	import { emWidth } from '#lib/library/fit.ts';
	import { resultsOf, searchLine, spotlight } from '#lib/library/search.svelte.ts';
	import { queryIn, searchFor } from '#lib/library/searchUrl.ts';
	import AlbumCard from '#lib/pages/AlbumCard.svelte';
	import TopResult from '#lib/pages/TopResult.svelte';
	import TrackHits from '#lib/pages/TrackHits.svelte';

	/** The URL waits this long on a word in progress, so history isn't a key-by-key record. */
	const debounceMs = 180;

	const { search } = library;

	const query = $derived(queryIn(page.url));
	const results = $derived(resultsOf(search.state));
	const found = $derived(results === null ? null : spotlight(results, search.answered));
	const line = $derived(searchLine(search.state));
	/** Looking is a passing note over results. Everything else has the page to itself. */
	const speaks = $derived(!line.fact && search.state.kind !== 'looking');

	/** What's in the box. It runs ahead of the URL: the box must never wait for it. */
	let text = $state(untrack(() => query));
	let field: HTMLTextAreaElement | undefined = $state();
	let pending: ReturnType<typeof setTimeout> | undefined;
	/** The query last sent, which the box sends ahead of the URL: the URL catching up asks nothing. */
	let asked: string | null = null;

	/** Looks for `wanted`, unless that's already been asked. A new question takes back the last. */
	function ask(wanted: string): void {
		if (wanted !== asked) {
			asked = wanted;
			search.run(wanted);
		}
	}

	/** The URL says what's searched, so a search can be shared, and a reload lands on it. */
	function write(): void {
		clearTimeout(pending);
		if (text.trim() !== query) {
			// `reset: false`, or every update drops focus from the box and scrolls the page.
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- Built from resolve('/search').
			void goto(`${resolve('/search')}${searchFor(text, page.url.search)}`, {
				replaceState: true,
				reset: false
			});
		}
	}

	function typed(): void {
		// A pasted line break: a query is one line, however many it wraps to.
		text = text.replace(/\s*\n\s*/gu, ' ');
		// The search never waits for the URL: every keystroke asks at once.
		ask(text.trim());
		clearTimeout(pending);
		pending = setTimeout(write, debounceMs);
	}

	function submitted(event: SubmitEvent): void {
		event.preventDefault();
		write();
	}

	/** Enter searches, as it would in a one-line box. It's still choosing a word mid-composition. */
	function pressed(event: KeyboardEvent): void {
		if (event.key === 'Enter' && !event.isComposing) {
			event.preventDefault();
			write();
		}
	}

	// A link, a reload, Back or Forward: whatever the URL asks that the box hasn't.
	$effect(() => {
		const wanted = query;
		// Untracked: a search reads and writes its own state, which mustn't start it again.
		untrack(() => {
			ask(wanted);
		});
	});

	beforeNavigate(({ type }) => {
		// Leaving, a word still waiting would drag the user back here.
		if (type !== 'goto') {
			clearTimeout(pending);
		}
	});

	afterNavigate(({ type }) => {
		// Our own update: the box is ahead of it, and copying it back would lose keystrokes.
		if (type === 'goto') {
			return;
		}
		text = query;
		if (type !== 'popstate') {
			field?.focus();
		}
	});

	onDestroy(() => {
		clearTimeout(pending);
		search.stop();
	});
</script>

<svelte:head>
	<title>{query === '' ? 'Search' : query} · Kitsune</title>
</svelte:head>

<div class="page-frame search" data-answered={search.answered}>
	<h1>Search</h1>
	<form role="search" onsubmit={submitted}>
		<!-- A box that wraps, so a query past what shrinking can fit still shows every word. -->
		<div class="box" style:--width={emWidth(text, 900, -0.06)}>
			<div class="fit">
				<span class="ruler" aria-hidden="true">{text === '' ? 'Search' : text}</span>
				<textarea
					bind:this={field}
					bind:value={text}
					oninput={typed}
					onkeydown={pressed}
					name="q"
					rows="1"
					aria-label="Search your library"
					placeholder="Search"
					enterkeyhint="search"
					autocomplete="off"
					autocapitalize="off"
					spellcheck="false"
					maxlength="256"></textarea>
			</div>
		</div>
	</form>
	<p class="line" class:fact={line.fact} class:speaks role="status">
		{line.text}
		{#if search.state.kind === 'failed'}
			<button
				type="button"
				onclick={() => {
					search.run(query);
				}}>Try again</button
			>
		{/if}
	</p>

	{#if results !== null && found !== null}
		<div class="found" class:alone={found.albums.length === 0}>
			<TopResult top={found.top} {results} />
			{#if found.albums.length > 0}
				<section class="albums" aria-labelledby="found-albums">
					<h2 id="found-albums">Albums</h2>
					{#each found.albums as album (album.id)}
						<AlbumCard {album} />
					{/each}
				</section>
			{/if}
			{#if found.artists.length > 0}
				<section class="wide" aria-labelledby="found-artists">
					<h2 id="found-artists">Artists</h2>
					<ul class="artists">
						{#each found.artists as artist (artist.id)}
							<li>
								<a class="artist" href={resolve('/artists/[id]', { id: artist.id })}
									>{artist.name}</a
								>
							</li>
						{/each}
					</ul>
				</section>
			{/if}
			{#if found.tracks.length > 0}
				<section class="wide" aria-labelledby="found-tracks">
					<h2 id="found-tracks">Tracks</h2>
					<TrackHits tracks={found.tracks} />
				</section>
			{/if}
		</div>
	{/if}
</div>

<style>
	.search {
		--frame-top: 96px;

		container-type: inline-size;
	}

	/* The box says what the page is: this one is for whoever can't see it. */
	h1 {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	/* Everything inside sizes from here, so the box and its ruler can't disagree. */
	.box {
		--largest: clamp(56px, 8cqw, calc(96px * var(--step)));
		--smallest: clamp(28px, 3.4cqw, calc(44px * var(--step)));
		--fit: calc(100cqw / max(var(--width), 1));

		min-height: calc(var(--largest) * 1.28 + 2px);
		display: flex;
		flex-direction: column;
		justify-content: flex-end;
		border-bottom: 2px solid var(--fox-line);
		font-size: clamp(var(--smallest), var(--fit), var(--largest));
		font-weight: 900;
		letter-spacing: -0.06em;
		line-height: 1.25;
	}

	.fit {
		display: grid;
	}

	/* It sets the height: a textarea can't grow to its words without help everywhere. */
	.ruler,
	textarea {
		grid-area: 1 / 1;
		min-width: 0;
		/* Geist's descenders run past the line box, which would give the box something to scroll. */
		padding: 0 0 0.03em;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}

	.ruler {
		visibility: hidden;
	}

	textarea {
		display: block;
		width: 100%;
		height: 100%;
		border: 0;
		border-radius: 0;
		outline: none;
		background: transparent;
		color: var(--text);
		caret-color: var(--fox-line);
		font: inherit;
		letter-spacing: inherit;
		overflow: hidden;
		resize: none;
		appearance: none;
	}

	textarea::placeholder {
		color: var(--text-label);
		opacity: 1;
	}

	.line {
		margin-top: 8px;
		min-height: 16px;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 20px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		line-height: calc(16px * var(--step));
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-meta);
		overflow-wrap: anywhere;
	}

	/* The app talking, with nothing else on the page to compete with. */
	.line.speaks {
		margin-top: 32px;
		max-width: 22em;
		font-family: var(--font-sans);
		font-size: var(--type-section);
		font-weight: 800;
		line-height: 1.05;
		letter-spacing: var(--tracking-section);
		text-transform: none;
		color: var(--text-secondary);
	}

	.line button {
		min-height: var(--target);
		padding: 0 18px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: color-mix(in srgb, var(--ground) 30%, transparent);
		font-size: 14px;
		font-weight: 600;
		letter-spacing: normal;
	}

	.line button:hover {
		color: var(--fox-text);
	}

	/*
	 * One grid, so albums past the first row come back under the Top result. It has as many columns
	 * as fit, seven at 1280, so a wider screen shows more albums beside it rather than bigger ones.
	 */
	.found {
		margin-top: calc(24px * var(--step));
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(calc(150px * var(--step)), 1fr));
		grid-template-rows: calc(16px * var(--step));
		gap: calc(12px * var(--step)) calc(16px * var(--step));
	}

	.found :global(.top) {
		grid-column: 1 / span 3;
		grid-row: 1 / span 2;
		margin-right: 16px;
	}

	.found.alone {
		grid-template-rows: none;
	}

	.found.alone :global(.top) {
		grid-column: 1 / -1;
		grid-row: auto;
		margin-right: 0;
	}

	.albums {
		display: contents;
	}

	.albums h2 {
		grid-column: 4 / -1;
		grid-row: 1;
	}

	/* A smaller card than a grid's, so a smaller play on it. */
	.albums :global(.card) {
		--cover-play-size: calc(40px * var(--step));
		--cover-play-inset: calc(10px * var(--step));

		margin-bottom: 16px;
	}

	.wide {
		grid-column: 1 / -1;
		min-width: 0;
		margin-top: 16px;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	h2 {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		line-height: calc(16px * var(--step));
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-meta);
	}

	.artists {
		margin: 8px 0 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	.artist {
		min-height: var(--target);
		padding: 0 18px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		display: flex;
		align-items: center;
		font-size: 14px;
		font-weight: 600;
		overflow-wrap: anywhere;
	}

	.artist:hover {
		color: var(--fox-text);
	}

	@media (max-width: 1160px) {
		.found {
			grid-template-columns: repeat(4, minmax(0, 1fr));
			grid-template-rows: none;
		}

		.found :global(.top) {
			grid-column: 1 / -1;
			grid-row: auto;
			margin-right: 0;
		}

		.albums h2 {
			grid-column: 1 / -1;
			grid-row: auto;
			margin-top: 16px;
		}
	}

	@media (max-width: 720px) {
		.search {
			--frame-top: 98px;
		}

		.found {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}

		.line.speaks {
			font-size: 32px;
		}
	}
</style>
