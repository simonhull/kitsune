<!--
	The songs a search found, as SongRows: a cover beside each, no number, and the person's columns
	with Artist and Album always on, since results come from everywhere. A press plays the results in
	order from that song.
-->
<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { clock } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { ColumnId, LibraryTrack, MovableId, TrackHit } from '#lib/library/source.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import {
		artistWhenDifferentIn,
		columnVars,
		columnsFrom,
		columnsIn,
		fit,
		largeFrom,
		monoAdvance,
		movable,
		reorderIn,
		template,
		type RowAlbum,
		type RowParts
	} from './columns.ts';
	import ColumnsMenu from './ColumnsMenu.svelte';
	import { columnsView } from './columnsView.svelte.ts';
	import { songGrid } from './songGrid.ts';
	import PickBox from './PickBox.svelte';
	import RowMenu from './RowMenu.svelte';
	import SelectionBar from './SelectionBar.svelte';
	import { onScreen, Selection } from './selection.svelte.ts';
	import {
		actionsFor,
		menuTarget,
		pickedFacts,
		songsOf,
		type MenuTarget,
		type SongActions,
		type SongFacts
	} from './songActions.ts';
	import SongHeader from './SongHeader.svelte';
	import SongRow from './SongRow.svelte';
	import { focusRow, rowEvents, SongList } from './songRow.ts';

	interface Props {
		tracks: readonly TrackHit[];
	}

	let { tracks }: Props = $props();

	const parts: RowParts = { handle: false, cover: true, number: false };
	const now = Date.now();
	let width = $state(0);
	let windowWidth = $state(1280);
	let root: HTMLElement | undefined = $state();
	const phone = $derived(windowWidth < columnsFrom);
	const step = $derived(windowWidth >= largeFrom ? 'large' : 'columns');
	const order = $derived(columnsIn(columnsView.pref.order, 'search'));
	const fitted = $derived(fit(order, width, step, parts, columnsView.pref.widths));
	const grid = $derived(template(fitted.shown, step, parts));
	const artistWhenDifferent = $derived(artistWhenDifferentIn(columnsView.pref, 'search'));

	/** Each hit as a row: the index's own song, with the columns' fields, once it has come. */
	const rows = $derived(
		tracks.map((hit): { track: LibraryTrack; album: RowAlbum } => {
			const album = library.albumRows(hit.albumId, columnsView.fields);
			const track = album?.tracks.find(({ id }) => id === hit.id);
			return {
				track: track ?? {
					id: hit.id,
					title: hit.title,
					artists: hit.artists,
					durationMs: hit.durationMs,
					hearted: library.marks.hearted('tracks', hit.id)
				},
				album: album ?? {
					id: hit.albumId,
					title: hit.album,
					artist: '',
					artwork: hit.artwork
				}
			};
		})
	);
	const ids = $derived(tracks.map(({ id }) => id));
	const albumOf = $derived(new Map(tracks.map((hit) => [hit.id, hit.albumId])));

	// The columns' fields for the albums found, in one request; a later search asks again.
	$effect(() => {
		const albums = [...new Set(tracks.map(({ albumId }) => albumId))];
		const fields = columnsView.fields;
		untrack(() => void library.loadAlbumRows(albums, fields));
	});

	const selection = new Selection();
	onMount(() => onScreen.show(selection));
	onMount(() => {
		void columnsView.load();
	});
	// New results are a new list.
	$effect(() => {
		void tracks;
		untrack(() => {
			selection.clear();
		});
	});
	const list = new SongList(selection, {
		order: () => ids,
		reveal: (trackId) => {
			focusRow(root, trackId);
		}
	});
	const picked = $derived(selection.inOrder(ids));
	const pickedActions = $derived(actionsFor(picked, albumOf));
	const songFacts: SongFacts = (trackId) => {
		const hit = tracks.find(({ id }) => id === trackId);
		return hit === undefined ? undefined : { albumId: hit.albumId, durationMs: hit.durationMs };
	};

	let rowMenu = $state<{ x: number; y: number; target: MenuTarget; actions: SongActions } | null>(
		null
	);
	let columnsMenu = $state<{ x: number; y: number; hiding: ColumnId | undefined } | null>(null);
	let opener: HTMLElement | null = null;

	function closeMenus(refocus: boolean): void {
		rowMenu = null;
		columnsMenu = null;
		if (refocus) {
			opener?.focus();
		}
	}

	/** Plays the results in order, from this one. */
	function play(trackId: string): void {
		const from = ids.indexOf(trackId);
		void songsOf(ids.slice(Math.max(from, 0)), albumOf).then((songs) => {
			player.playSongs(songs);
		});
	}

	const events = rowEvents({
		play,
		pick: (trackId, how) => {
			list.pick(trackId, how);
		},
		menu: (trackId, x, y) => {
			opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
			const target = menuTarget(
				trackId,
				selection,
				() => picked,
				songFacts,
				(each) => {
					const hit = tracks.find(({ id }) => id === each);
					return hit === undefined
						? undefined
						: {
								albumId: hit.albumId,
								title: hit.title,
								line: `${hit.album.toUpperCase()} · ${clock(hit.durationMs / 1000)}`,
								artistId: undefined
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
		},
		key: (event, trackId) => {
			list.key(event, trackId);
		},
		rate: (trackId, stars) => {
			library.marks.press(trackId, stars);
		},
		picked: (trackId) => selection.has(trackId)
	});

	function onWindowKey(event: KeyboardEvent): void {
		list.windowKey(event, rowMenu !== null || columnsMenu !== null);
	}
</script>

<svelte:window bind:innerWidth={windowWidth} onkeydown={onWindowKey} />

{#snippet headerBox()}
	<PickBox
		state={selection.boxFor(ids)}
		label={selection.boxFor(ids) === 'all' ? 'Clear every song found' : 'Select every song found'}
		ontoggle={() => {
			list.toggleAll();
		}}
	/>
{/snippet}

<div
	class="songs song-list"
	bind:this={root}
	bind:clientWidth={width}
	use:songGrid={grid}
	style={columnVars(fitted.shown, columnsView.pref.widths, step)}
	data-selecting={selection.any ? '' : undefined}
>
	{#if !phone}
		<SongHeader
			shown={fitted.shown}
			hiddenCount={fitted.hiddenCount}
			{parts}
			openMenu={(x, y, hiding) => {
				opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
				columnsMenu = { x, y, hiding };
			}}
			box={headerBox}
			widths={columnsView.pref.widths}
			{step}
			onresize={(id, to) => {
				columnsView.resize(id, to);
			}}
			onreorder={(id: MovableId, to: number, keys: boolean) => {
				columnsView.reorder(reorderIn(columnsView.pref, 'search', fitted.shown, id, to), {
					debounce: keys
				});
			}}
			movable={(id: MovableId) => movable(id, columnsView.pref)}
		/>
	{/if}
	<div class="rows" {...events}>
		{#each rows as { track, album } (track.id)}
			<SongRow
				{track}
				{album}
				{phone}
				{parts}
				shown={fitted.shown}
				{order}
				{artistWhenDifferent}
				lineChars={Math.floor((width - 120) / monoAdvance)}
				{now}
				stars={library.marks.stars(track.id)}
				hearted={library.marks.hearted('tracks', track.id)}
				cued={track.id === library.playing?.track.id}
				sounding={player.sounding}
				picked={selection.has(track.id)}
			/>
		{/each}
	</div>
</div>

{#if rowMenu !== null}
	<RowMenu
		x={rowMenu.x}
		y={rowMenu.y}
		target={rowMenu.target}
		actions={rowMenu.actions}
		onclose={closeMenus}
	/>
{/if}
{#if columnsMenu !== null}
	<ColumnsMenu
		x={columnsMenu.x}
		y={columnsMenu.y}
		hiding={columnsMenu.hiding}
		context="search"
		onclose={closeMenus}
	/>
{/if}
{#if selection.any}
	{@const facts = pickedFacts(picked, songFacts)}
	<SelectionBar
		count={selection.size}
		hidden={0}
		albums={facts.albums}
		durationMs={facts.durationMs}
		hearted={picked.every((trackId) => library.marks.hearted('tracks', trackId))}
		actions={pickedActions}
		compact={phone}
		edit={undefined}
		onclear={() => {
			selection.clear();
		}}
	/>
{/if}

<style>
	.songs {
		--song-row: 56px;

		min-width: 0;
	}

	.rows {
		display: flex;
		flex-direction: column;
	}

	@media (max-width: 699px) {
		.songs {
			margin: 0 -8px;
		}

		.rows :global(.row.two) {
			--song-row-two: 60px;
		}
	}
</style>
