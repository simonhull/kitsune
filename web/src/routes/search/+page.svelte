<script lang="ts">
	import { onDestroy, untrack } from 'svelte';
	import { afterNavigate, beforeNavigate, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { library } from '#lib/library/library.svelte.ts';
	import { resultsOf, searchLine } from '#lib/library/search.svelte.ts';
	import { queryIn, searchFor } from '#lib/library/searchUrl.ts';
	import AlbumShelf from '#lib/pages/AlbumShelf.svelte';
	import TrackHits from '#lib/pages/TrackHits.svelte';

	/** A word in progress waits this long, which is still inside keeping up with the fingers. */
	const debounceMs = 180;

	const { search } = library;

	const query = $derived(queryIn(page.url));
	const results = $derived(resultsOf(search.state));
	const line = $derived(searchLine(search.state));

	/** What's in the box. It runs ahead of the URL: the box must never wait for it. */
	let text = $state(untrack(() => query));
	let field: HTMLInputElement | undefined = $state();
	let pending: ReturnType<typeof setTimeout> | undefined;

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
		clearTimeout(pending);
		pending = setTimeout(write, debounceMs);
	}

	function submitted(event: SubmitEvent): void {
		event.preventDefault();
		write();
	}

	$effect(() => {
		const wanted = query;
		// Untracked: a search reads and writes its own state, which mustn't start it again.
		untrack(() => {
			search.run(wanted);
		});
		return () => {
			search.stop();
		};
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
	});
</script>

<svelte:head>
	<title>{query === '' ? 'Search' : query} · Kitsune</title>
</svelte:head>

<div class="search">
	<div class="ask">
		<h1>Search</h1>
		<form role="search" onsubmit={submitted}>
			<input
				bind:this={field}
				bind:value={text}
				oninput={typed}
				type="search"
				name="q"
				aria-label="Search your library"
				placeholder="Search"
				enterkeyhint="search"
				autocomplete="off"
				autocapitalize="off"
				spellcheck="false"
				maxlength="256"
			/>
		</form>
		<p class="line" class:fact={line.fact} role="status">
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
	</div>

	{#if results !== null}
		{#if results.artists.length > 0}
			<section aria-labelledby="found-artists">
				<h2 id="found-artists">Artists</h2>
				<ul class="artists">
					{#each results.artists as artist (artist.id)}
						<li>
							<a class="artist" href={resolve('/artists/[id]', { id: artist.id })}>{artist.name}</a>
						</li>
					{/each}
				</ul>
			</section>
		{/if}
		{#if results.albums.length > 0}
			<section aria-labelledby="found-albums">
				<h2 id="found-albums">Albums</h2>
				<AlbumShelf albums={results.albums} />
			</section>
		{/if}
		{#if results.tracks.length > 0}
			<section aria-labelledby="found-tracks">
				<h2 id="found-tracks">Tracks</h2>
				<TrackHits tracks={results.tracks} />
			</section>
		{/if}
	{/if}
</div>

<style>
	.search {
		max-width: 1400px;
		margin: 0 auto;
		padding: 106px 32px 140px;
		display: flex;
		flex-direction: column;
		gap: 56px;
	}

	.ask {
		display: flex;
		flex-direction: column;
		gap: 12px;
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

	input {
		width: 100%;
		padding: 18px 24px 20px;
		border: 1px solid var(--edge-card);
		border-radius: var(--radius-28);
		background: var(--card);
		color: var(--text);
		caret-color: var(--fox);
		font: inherit;
		font-size: clamp(32px, 5vw, 64px);
		font-weight: 800;
		letter-spacing: var(--tracking-display);
		line-height: 1.1;
		appearance: none;
	}

	input::placeholder {
		color: var(--text-label);
		opacity: 1;
	}

	input::-webkit-search-cancel-button {
		display: none;
	}

	.line {
		min-height: var(--target);
		padding: 0 24px;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0 8px;
		font-size: var(--type-body);
		color: var(--text-secondary);
		overflow-wrap: anywhere;
	}

	.line.fact {
		font-family: var(--font-mono);
		font-size: 13px;
		color: var(--text-meta);
	}

	.line button {
		min-height: var(--target);
		padding: 0;
		border: 0;
		background: none;
		color: var(--fox);
	}

	section {
		display: flex;
		flex-direction: column;
		gap: 24px;
	}

	h2 {
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.artists {
		margin: 0;
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
		color: var(--fox);
	}

	@media (max-width: 720px) {
		.search {
			padding: 98px 16px 128px;
			gap: 40px;
		}

		h2 {
			font-size: 32px;
		}
	}
</style>
