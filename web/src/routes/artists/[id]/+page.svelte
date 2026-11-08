<script lang="ts">
	import { untrack } from 'svelte';
	import { resolve } from '$app/paths';
	import { emWidth } from '#lib/library/fit.ts';
	import { artistFacts } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import AlbumShelf from '#lib/pages/AlbumShelf.svelte';
	import type { PageProps } from './$types';

	let { params }: PageProps = $props();

	const id = $derived(params.id);
	const load = $derived(library.artist(id));
	const artist = $derived(load.kind === 'ready' ? load.value : undefined);
	/** Known from whatever linked here, so the first frame has its headline. */
	const name = $derived(artist?.name ?? library.artistName(id) ?? '');

	// Never awaited, like an album's tracks. Untracked: a failure must wait for Try again.
	$effect(() => {
		const wanted = id;
		untrack(() => void library.loadArtist(wanted));
	});
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
		{:else if artist !== undefined}
			{#if artist.albums.length > 0}
				<section aria-labelledby="own-albums">
					<h2 id="own-albums">Albums</h2>
					<AlbumShelf albums={artist.albums} />
				</section>
			{/if}
			{#if artist.appearsOn.length > 0}
				<section aria-labelledby="appears-on">
					<h2 id="appears-on">Appears on</h2>
					<AlbumShelf albums={artist.appearsOn} />
				</section>
			{/if}
			{#if artist.albums.length === 0 && artist.appearsOn.length === 0}
				<p class="note" role="status">Nothing of theirs is in the library right now.</p>
			{/if}
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

	section {
		display: flex;
		flex-direction: column;
		gap: calc(24px * var(--step));
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
