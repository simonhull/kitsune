<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import type { ColumnId } from '#lib/library/source.ts';
	import AlbumHero from '#lib/pages/AlbumHero.svelte';
	import { tagEditor } from '#lib/pages/tagEditor.svelte.ts';
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
	/** The tag editor is open over the page. */
	const editing = $derived(tagEditor.current !== null);

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
		},
		trash: (trackIds) => {
			actionsFor(trackIds, albumOf).trash?.();
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

	/** Edit N songs: the album in the editor, only the songs picked ticked. */
	function editPicked(): void {
		const chosen = rows?.tracks.filter((track) => selection.has(track.id)) ?? [];
		selection.clear();
		tagEditor.open({ kind: 'album', albumId: id, ticked: chosen.map((track) => track.id) }, true);
	}

	function onWindowKey(event: KeyboardEvent): void {
		list.windowKey(event, rowMenu !== null || columnsMenu !== null || editing);
	}

	let innerWidth = $state(1280);

	// Never awaited: the first frame is built from the list entry, and tracks stream in.
	$effect(() => {
		const wanted = id;
		// A first scan ending forgets the album it was half way through, even with this page open.
		void library.album(wanted);
		// Untracked: a failure must wait for Try again, not re-run this and ask forever.
		untrack(() => void library.loadAlbum(wanted));
	});

	/** Edit tags: the one editor, over the page, on every song of the album. */
	function openEditor(): void {
		tagEditor.open({ kind: 'album', albumId: id }, true);
	}
</script>

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
		<AlbumHero albumId={id} {entry} {album} edit={canEditTags ? openEditor : undefined} />
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
			/>
		</div>
		{#if selection.any && !editing}
			{@const facts = pickedFacts(picked, songFacts)}
			<SelectionBar
				count={selection.size}
				hidden={0}
				albums={facts.albums}
				durationMs={facts.durationMs}
				actions={pickedActions}
				compact={innerWidth < 700}
				edit={canEditTags ? editPicked : undefined}
				onclear={() => {
					selection.clear();
				}}
			/>
		{/if}
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
