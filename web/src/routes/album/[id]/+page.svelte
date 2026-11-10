<script lang="ts">
	import { onMount, tick, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import type { Album, ColumnId } from '#lib/library/source.ts';
	import { whenSettled } from '#lib/morph/settled.ts';
	import AlbumHero from '#lib/pages/AlbumHero.svelte';
	import BulkEditor from '#lib/pages/BulkEditor.svelte';
	import { bulkEdits } from '#lib/pages/bulkEdits.svelte.ts';
	import { matchFromBulk, matchFromEditor } from '#lib/matching/hooks.ts';
	import TagBar from '#lib/pages/TagBar.svelte';
	import TagRows from '#lib/pages/TagRows.svelte';
	import { tagEdits } from '#lib/pages/tagEdits.svelte.ts';
	import AlbumSongs from '#lib/pages/AlbumSongs.svelte';
	import { rowsOf } from '#lib/pages/albumSongs.ts';
	import { clock, discLabel, discOf } from '#lib/library/format.ts';
	import ColumnsMenu from '#lib/pages/ColumnsMenu.svelte';
	import { columnsView } from '#lib/pages/columnsView.svelte.ts';
	import RowMenu from '#lib/pages/RowMenu.svelte';
	import SelectionBar from '#lib/pages/SelectionBar.svelte';
	import { onScreen, Selection } from '#lib/pages/selection.svelte.ts';
	import {
		actionsFor,
		menuTarget,
		pickedFacts,
		type MenuTarget,
		type SongActions,
		type SongFacts
	} from '#lib/pages/songActions.ts';
	import { focusRow, SongList } from '#lib/pages/songRow.ts';
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

	/** The album's songs as rows show them: the index's, with the columns' fields, once here. */
	const rows = $derived(
		library.albumRows(id, columnsView.fields) ?? (album === undefined ? undefined : rowsOf(album))
	);
	$effect(() => {
		const wanted = id;
		const fields = columnsView.fields;
		if (columnsView.known && !untrack(() => library.indexHolds(wanted, fields))) {
			untrack(() => void library.loadAlbumRows([wanted], fields));
		}
	});
	onMount(() => {
		void columnsView.load();
	});

	/** The songs picked on this page: its own, and gone with it. */
	const selection = new Selection();
	onMount(() => onScreen.show(selection));
	const order = $derived(rows?.tracks.map((track) => track.id) ?? []);
	let songsElement: HTMLElement | undefined = $state();
	const list = new SongList(selection, {
		order: () => order,
		reveal: (trackId) => {
			focusRow(songsElement, trackId);
		}
	});
	// Another album is another list.
	$effect(() => {
		void id;
		untrack(() => {
			selection.clear();
		});
	});
	const albumOf = $derived(new Map(order.map((trackId) => [trackId, id])));
	const songFacts: SongFacts = (trackId) => {
		const track = rows?.tracks.find((each) => each.id === trackId);
		return track === undefined ? undefined : { albumId: id, durationMs: track.durationMs };
	};
	const picked = $derived(selection.inOrder(order));
	const pickedActions = $derived(actionsFor(picked, albumOf));
	const canEditTags = $derived(library.user?.canEditTags === true);

	/** The ⋯ menu and the columns menu, where each was opened, and what opened it, to go back to. */
	let rowMenu = $state<{ x: number; y: number; target: MenuTarget; actions: SongActions } | null>(
		null
	);
	let columnsMenu = $state<{ x: number; y: number; hiding: ColumnId | undefined } | null>(null);
	let opener: HTMLElement | null = null;

	function openRowMenu(trackId: string, x: number, y: number): void {
		opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		const target = menuTarget(
			trackId,
			selection,
			() => picked,
			songFacts,
			(each) => {
				const track = rows?.tracks.find((one) => one.id === each);
				if (track === undefined || entry === undefined) {
					return undefined;
				}
				const disc =
					(rows?.discs?.length ?? 0) > 1
						? ` · ${discLabel(discOf(track.discNo)).toUpperCase()}`
						: '';
				return {
					albumId: id,
					title: track.title,
					line: `${entry.title.toUpperCase()}${disc} · ${clock(track.durationMs / 1000)}`,
					artistId: album?.artists.at(0)?.id
				};
			}
		);
		if (target !== undefined) {
			rowMenu = {
				x,
				y,
				target,
				actions: target.kind === 'picked' ? pickedActions : actionsFor([trackId], albumOf)
			};
		}
	}

	function openColumns(x: number, y: number, hiding?: ColumnId): void {
		opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		columnsMenu = { x, y, hiding };
	}

	function closeMenus(refocus: boolean): void {
		rowMenu = null;
		columnsMenu = null;
		if (refocus) {
			opener?.focus();
		}
	}

	/** Edit N songs: the album's own editor for many, on the songs picked. */
	function editPicked(): void {
		const chosen = rows?.tracks.filter((track) => selection.has(track.id)) ?? [];
		selection.clear();
		bulkEdits.open(
			'album',
			chosen.map((track) => ({ id: track.id, albumId: id, title: track.title })),
			canEditTags
		);
	}

	function onWindowKey(event: KeyboardEvent): void {
		list.windowKey(
			event,
			rowMenu !== null || columnsMenu !== null || bulk !== null || session !== undefined
		);
	}

	let editButton: HTMLButtonElement | undefined = $state();
	let eyebrow: HTMLElement | undefined = $state();
	let announcement = $state('');
	let innerWidth = $state(1280);

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

<svelte:window onkeydown={onWindowKey} bind:innerWidth />

<svelte:head>
	<title>{entry?.title ?? 'Album'} · Kitsune</title>
</svelte:head>

{#if library.failed(id) && entry === undefined}
	<div class="missing">
		<h1>That album isn't in the library</h1>
		<a href={resolve('/')} data-sveltekit-reset="false">Back to Home</a>
	</div>
{:else}
	<div class="album">
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
			<BulkEditor session={bulk}>
				{#snippet bar()}
					<TagBar
						session={bulk}
						what="these tracks"
						leave={leaveBulk}
						write={() => void writeBulk()}
						match={matchFromBulk(bulk)}
					/>
				{/snippet}
			</BulkEditor>
		{:else}
			<div class="songs" bind:this={songsElement}>
				<AlbumSongs
					{album}
					{rows}
					pref={columnsView.pref}
					{cuedId}
					sounding={player.sounding}
					{selection}
					{list}
					play={(trackId) => {
						const index = album?.tracks.findIndex((track) => track.id === trackId) ?? -1;
						if (album !== undefined && index >= 0) {
							player.play(album, index);
						}
					}}
					playDisc={(disc) => {
						if (album !== undefined) {
							player.playDisc(album, disc);
						}
					}}
					openMenu={openRowMenu}
					{openColumns}
					gone={library.failed(id)}
					unavailable={library.unavailable(id)}
					retry={() => void library.loadAlbum(id)}
					editing={session !== undefined}
					one={session?.open != null}
					editor={session?.tags == null ? undefined : editor}
				/>
				{#if session !== undefined}
					<TagBar
						{session}
						open={session.open}
						{leave}
						write={() => void write()}
						match={() => matchFromEditor(id)}
					/>
				{/if}
			</div>
			{#if selection.any && session === undefined}
				{@const facts = pickedFacts(picked, songFacts)}
				<SelectionBar
					count={selection.size}
					hidden={0}
					albums={facts.albums}
					durationMs={facts.durationMs}
					hearted={picked.every((trackId) => library.marks.hearted('tracks', trackId))}
					actions={pickedActions}
					compact={innerWidth < 700}
					edit={canEditTags ? editPicked : undefined}
					onclear={() => {
						selection.clear();
					}}
				/>
			{/if}
		{/if}
		<p class="visually-hidden" role="status">{announcement}</p>
	</div>
	{#if rowMenu !== null}
		<RowMenu
			x={rowMenu.x}
			y={rowMenu.y}
			target={rowMenu.target}
			actions={rowMenu.actions}
			here="album"
			onclose={closeMenus}
		/>
	{/if}
	{#if columnsMenu !== null}
		<ColumnsMenu
			x={columnsMenu.x}
			y={columnsMenu.y}
			hiding={columnsMenu.hiding}
			context="album"
			onclose={closeMenus}
		/>
	{/if}
{/if}

<style>
	.album {
		padding-bottom: 140px;
	}

	/*
	 * The songs keep the share of the window they have at 1920, two thirds, and never less than a
	 * width a row reads across at the step, so a 4K screen gets the room and not a gap. They stand at
	 * the hero's words: the list centres itself in its gutters, so the cap counts both of them in.
	 */
	.songs {
		max-width: calc(max(1280px * var(--step), 200vw / 3) + 2 * max(64px, var(--gutter)));
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
