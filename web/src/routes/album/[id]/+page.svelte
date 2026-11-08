<script lang="ts">
	import { tick, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import type { Album } from '#lib/library/source.ts';
	import { whenSettled } from '#lib/morph/settled.ts';
	import AlbumHero from '#lib/pages/AlbumHero.svelte';
	import BulkEditor from '#lib/pages/BulkEditor.svelte';
	import { bulkEdits } from '#lib/pages/bulkEdits.svelte.ts';
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
	/** Many tracks at once, board D: the album's own bulk edit. */
	const bulk = $derived(
		bulkEdits.current?.layout === 'album' && bulkEdits.current.albumIds.includes(id)
			? bulkEdits.current
			: null
	);

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

	/** An edit that moved every track took the album with it: follow it to where they went. */
	async function follow(to: string | undefined): Promise<boolean> {
		if (to === undefined || to === id) {
			return false;
		}
		await goto(resolve('/album/[id]', { id: to }), { replaceState: true });
		return true;
	}

	async function write(): Promise<void> {
		const written = await session?.write();
		if (written?.kind === 'done') {
			if (!(await follow(library.movedTo(id)))) {
				await leftEditor('Tags written.');
			}
		} else if (written?.kind === 'problem') {
			await tick();
			document.getElementById(written.field)?.focus();
		}
	}

	/** Leaves the album page's editor for many tracks at once, every one picked. */
	function pickTracks(): void {
		const tracks = session?.tags?.tracks ?? [];
		tagEdits.close(id);
		bulkEdits.open(
			'album',
			tracks.map((track) => ({
				id: track.id,
				albumId: id,
				title: track.fields.title?.at(0) ?? track.file
			})),
			library.user?.canEditTags === true
		);
	}

	function leaveBulk(): void {
		bulkEdits.close();
		void leftEditor('');
	}

	async function writeBulk(): Promise<void> {
		const written = await bulk?.write();
		if (written?.kind === 'done') {
			bulkEdits.close();
			// Merged away: the tracks are all on another album now.
			if (!(library.failed(id) && (await follow(written.albumId)))) {
				await leftEditor('Tags written.');
			}
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
	<div class="album" class:editing={session !== undefined || bulk !== null}>
		<AlbumHero
			albumId={id}
			{entry}
			{album}
			edit={library.user?.canEditTags === true ? () => void openEditor() : undefined}
			bind:editButton
			{session}
			picking={bulk === null
				? undefined
				: { picked: bulk.pickedTracks.length, of: bulk.tracks.length }}
			many={session !== undefined && !session.dirty && session.open === null
				? pickTracks
				: undefined}
			bind:eyebrow
		/>
		{#if bulk !== null}
			<BulkEditor session={bulk} />
			<TagBar session={bulk} what="these tracks" leave={leaveBulk} write={() => void writeBulk()} />
		{:else}
			<TrackList
				{album}
				{cuedId}
				sounding={player.sounding}
				play={(index) => {
					if (album !== undefined) {
						player.play(album, index);
					}
				}}
				playDisc={(disc) => {
					if (album !== undefined) {
						player.playDisc(album, disc);
					}
				}}
				gone={library.failed(id)}
				unavailable={library.unavailable(id)}
				retry={() => void library.loadAlbum(id)}
				editing={session !== undefined}
				one={session?.open != null}
				editor={session?.tags == null ? undefined : editor}
			/>
		{/if}
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
