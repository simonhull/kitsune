<script lang="ts">
	import { tick, untrack } from 'svelte';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import type { Album } from '#lib/library/source.ts';
	import { whenSettled } from '#lib/morph/settled.ts';
	import AlbumHero from '#lib/pages/AlbumHero.svelte';
	import TagBar from '#lib/pages/TagBar.svelte';
	import TagRows from '#lib/pages/TagRows.svelte';
	import { tagEdits } from '#lib/pages/tagEdits.svelte.ts';
	import TrackList from '#lib/pages/TrackList.svelte';
	import { player } from '#lib/player/player.svelte.ts';
	import type { PageProps } from './$types';

	let { params }: PageProps = $props();

	const id = $derived(params.id);
	const entry = $derived(library.entry(id));
	const album = $derived(library.album(id));
	const cuedId = $derived(library.playing?.album.id === id ? library.playing.track.id : undefined);
	const session = $derived(tagEdits.of(id));

	let editButton: HTMLButtonElement | undefined = $state();
	let eyebrow: HTMLElement | undefined = $state();
	let announcement = $state('');

	// Never awaited: the first frame is built from the list entry, and tracks stream in.
	$effect(() => {
		const wanted = id;
		// A first scan ending forgets the album it was half way through, even with this page open.
		void library.album(wanted);
		// Untracked: a failure must wait for Try again, not re-run this and ask forever.
		untrack(() => void library.loadAlbum(wanted));
	});

	// Edits outlive the page. Only a page left with something unsaved comes back to them.
	$effect(() => {
		const wanted = id;
		untrack(() => {
			tagEdits.arrive(wanted);
		});
		return () => {
			tagEdits.depart(wanted);
		};
	});

	// The album last opened is the one cued, but the island mustn't change under a flight.
	$effect(() => {
		const loaded = album;
		if (loaded !== undefined) {
			return whenSettled(() => {
				library.cueAlbum(loaded);
			});
		}
	});

	async function openEditor(): Promise<void> {
		tagEdits.open(id);
		announcement = '';
		await tick();
		eyebrow?.focus();
	}

	async function leftEditor(words: string): Promise<void> {
		announcement = words;
		await tick();
		editButton?.focus();
	}

	function leave(): void {
		tagEdits.close(id);
		void leftEditor('');
	}

	async function write(): Promise<void> {
		const written = await session?.write();
		if (written?.kind === 'done') {
			await leftEditor('Tags written.');
		} else if (written?.kind === 'problem') {
			await tick();
			document.getElementById(written.field)?.focus();
		}
	}
</script>

{#snippet editor(held: Album)}
	{#if session?.tags != null}
		<TagRows album={held} tags={session.tags} {session} />
	{/if}
{/snippet}

<svelte:head>
	<title>{entry?.title ?? 'Album'} · Kitsune</title>
</svelte:head>

{#if library.failed(id) && entry === undefined}
	<div class="missing">
		<h1>That album isn't in the library</h1>
		<a href={resolve('/')} data-sveltekit-reset="false">Back to Home</a>
	</div>
{:else}
	<div class="album" class:editing={session !== undefined}>
		<AlbumHero
			albumId={id}
			{entry}
			{album}
			edit={library.user?.canEditTags === true ? () => void openEditor() : undefined}
			bind:editButton
			{session}
			bind:eyebrow
		/>
		<TrackList
			{album}
			{cuedId}
			sounding={player.sounding}
			play={(index) => {
				if (album !== undefined) {
					player.play(album, index);
				}
			}}
			gone={library.failed(id)}
			unavailable={library.unavailable(id)}
			retry={() => void library.loadAlbum(id)}
			editing={session !== undefined}
			one={session?.open != null}
			editor={session?.tags == null ? undefined : editor}
		/>
		{#if session !== undefined}
			<TagBar {session} open={session.open} {leave} write={() => void write()} />
		{/if}
		<p class="visually-hidden" role="status">{announcement}</p>
	</div>
{/if}

<style>
	.album {
		padding-bottom: 140px;
	}

	/* The last row must clear the bar as well as the island. */
	.album.editing {
		padding-bottom: 220px;
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
</style>
