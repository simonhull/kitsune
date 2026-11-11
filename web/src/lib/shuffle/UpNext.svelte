<!--
	What comes up in a shuffle, as SongRows: a drag handle, the box, a cover, the title with who it's
	by, and a second line that only this list fills (a shuffle's why, given as `why`). No number,
	header or columns. Each song's menu adds Remove from queue.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { untrack } from 'svelte';
	import { library } from '#lib/library/library.svelte.ts';
	import type { AlbumEntry, LibraryTrack, ShuffleMode } from '#lib/library/source.ts';
	import { columnsFrom, largeFrom, template, type RowParts } from '#lib/pages/columns.ts';
	import { songGrid } from '#lib/pages/songGrid.ts';
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
	import SongRow from '#lib/pages/SongRow.svelte';
	import { focusRow, rowEvents, SongList } from '#lib/pages/songRow.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import { clock } from '#lib/library/format.ts';
	import { onMount } from 'svelte';
	import type { Shuffle, UpNextItem } from './session.svelte.ts';

	interface Props {
		shuffle: Shuffle;
		items: readonly UpNextItem[];
		mode: ShuffleMode;
		/** How many songs the shuffle picks from, once the server has said. */
		songs: number | null;
		/** The song playing, which heads the list. */
		playing: { id: string; title: string; artist: string; durationMs: number; album: AlbumEntry };
		/** What a row says of why it came up, if anything does. */
		why?: Snippet<[UpNextItem]>;
	}

	let { shuffle, items, mode, songs, playing, why }: Props = $props();

	const parts: RowParts = { handle: true, cover: true, number: false };
	const now = Date.now();
	let windowWidth = $state(1280);
	let root: HTMLElement | undefined = $state();
	const step = $derived(windowWidth >= largeFrom ? 'large' : 'columns');
	const grid = $derived(template([], step, parts, 0));
	const phone = $derived(windowWidth < columnsFrom);

	const facts = $derived(
		[
			mode === 'endless' ? 'ENDLESS' : 'EACH ONCE',
			songs === null ? null : `${songs.toLocaleString('en-US')} ${songs === 1 ? 'SONG' : 'SONGS'}`
		]
			.filter((each) => each !== null)
			.join(' · ')
	);

	/** A row's song: a combined run is one row, its titles joined. */
	function trackOf(item: UpNextItem): LibraryTrack {
		return {
			id: item.key,
			title: item.titles.join(' + '),
			artists: [item.artist],
			durationMs: item.durationMs,
			hearted: false
		};
	}

	const keys = $derived(items.map(({ key }) => key));
	const byKey = $derived(new Map(items.map((item) => [item.key, item])));
	/** Each song's album, for the actions: a run's songs go with its first. */
	const albumOf = $derived(
		new Map(items.flatMap((item) => item.trackIds.map((id) => [id, item.album.id] as const)))
	);
	const songsOfKeys = (chosen: readonly string[]): string[] =>
		chosen.flatMap((key) => byKey.get(key)?.trackIds ?? []);

	const selection = new Selection();
	onMount(() => onScreen.show(selection));
	const list = new SongList(selection, {
		order: () => keys,
		reveal: (key) => {
			focusRow(root, key);
		}
	});
	// What was picked and has since played or gone is let go.
	$effect(() => {
		const shown = new Set(keys);
		untrack(() => {
			for (const id of selection.ids) {
				if (!shown.has(id)) {
					selection.toggle(id);
				}
			}
		});
	});
	const picked = $derived(selection.inOrder(keys));
	const pickedSongs = $derived(songsOfKeys(picked));
	const pickedActions = $derived(actionsFor(pickedSongs, albumOf));
	const songFacts: SongFacts = (key) => {
		const item = byKey.get(key);
		return item === undefined ? undefined : { albumId: item.album.id, durationMs: item.durationMs };
	};

	let rowMenu = $state<{
		x: number;
		y: number;
		target: MenuTarget;
		actions: SongActions;
		remove: () => void;
	} | null>(null);
	let opener: HTMLElement | null = null;

	const events = rowEvents({
		play: (key) => {
			// Up next is the shuffle's own order: a press skips on to that song.
			const at = player.queue?.tracks.findIndex(({ id }) => id === byKey.get(key)?.trackIds[0]);
			if (at !== undefined && at >= 0) {
				player.skipTo(at);
			}
		},
		pick: (key, how) => {
			if (byKey.has(key)) {
				list.pick(key, how);
			}
		},
		menu: (key, x, y) => {
			const item = byKey.get(key);
			if (item === undefined) {
				return;
			}
			opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
			const target = menuTarget(
				key,
				selection,
				() => picked,
				songFacts,
				() => ({
					albumId: item.album.id,
					title: item.titles.join(' + '),
					line: `${item.album.title.toUpperCase()} · ${clock(item.durationMs / 1000)}`,
					artistId: undefined
				})
			);
			if (target === undefined) {
				return;
			}
			const many = target.kind === 'picked';
			rowMenu = {
				x,
				y,
				target: many ? { ...target, trackIds: pickedSongs } : target,
				actions: many ? pickedActions : actionsFor(item.trackIds, albumOf),
				remove: () => {
					shuffle.remove(many ? pickedSongs : item.trackIds);
					selection.clear();
				}
			};
		},
		key: (event, key) => {
			if (event.altKey && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) {
				event.preventDefault();
				void shuffle.move(key, event.key === 'ArrowUp' ? -1 : 1);
				return;
			}
			list.key(event, key);
		},
		rate: () => {
			// No columns here, so no stars to press.
		},
		picked: (key) => selection.has(key)
	});

	/** The row being dragged by its handle, and how far. It moves when it's let go. */
	let dragging = $state<{ key: string; from: number; dy: number; height: number } | null>(null);

	function startDrag(key: string, event: PointerEvent): void {
		if (event.button !== 0) {
			return;
		}
		event.preventDefault();
		const row = (event.currentTarget as HTMLElement).closest('[data-track]');
		dragging = {
			key,
			from: event.clientY,
			dy: 0,
			height: row?.getBoundingClientRect().height ?? 64
		};
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
	}

	function dragMove(event: PointerEvent): void {
		if (dragging !== null) {
			dragging = { ...dragging, dy: event.clientY - dragging.from };
		}
	}

	async function drop(): Promise<void> {
		const held = dragging;
		dragging = null;
		if (held === null) {
			return;
		}
		const steps = Math.round(held.dy / held.height);
		const by = steps < 0 ? -1 : 1;
		for (let moved = 0; moved < Math.abs(steps); moved += 1) {
			await shuffle.move(held.key, by);
		}
	}

	function onWindowKey(event: KeyboardEvent): void {
		list.windowKey(event, rowMenu !== null);
	}
</script>

<svelte:window
	bind:innerWidth={windowWidth}
	onkeydown={onWindowKey}
	onpointermove={dragMove}
	onpointerup={() => void drop()}
	onpointercancel={() => {
		dragging = null;
	}}
/>

{#snippet nowPlaying()}<span class="now">NOW PLAYING</span>{/snippet}

<section
	class="up-next song-list"
	aria-label="Up next"
	bind:this={root}
	use:songGrid={grid}
	data-selecting={selection.any ? '' : undefined}
>
	<div class="head">
		<h2>Up next</h2>
		<span class="facts">{facts}</span>
	</div>
	<div class="rows" {...events}>
		<SongRow
			track={{
				id: playing.id,
				title: playing.title,
				artists: [playing.artist],
				durationMs: playing.durationMs,
				hearted: false
			}}
			album={playing.album}
			phone={false}
			{parts}
			shown={[]}
			order={[]}
			artistWhenDifferent={false}
			lineChars={0}
			credit={playing.artist}
			second={nowPlaying}
			{now}
			rating={null}
			hearted={false}
			cued
			sounding={player.sounding}
			picked={false}
		/>
		{#if items.length === 0}
			<p class="empty">Picking what comes next…</p>
		{/if}
		{#each items as item (item.key)}
			{@const moving = dragging?.key === item.key}
			{#snippet reason()}{@render why?.(item)}{/snippet}
			<div
				class="slot"
				class:moving
				style:translate={moving && dragging !== null ? `0 ${String(dragging.dy)}px` : undefined}
			>
				<SongRow
					track={trackOf(item)}
					album={item.album}
					{phone}
					{parts}
					shown={[]}
					order={[]}
					artistWhenDifferent={false}
					lineChars={0}
					credit={item.artist}
					second={why === undefined ? undefined : reason}
					{now}
					rating={null}
					hearted={false}
					cued={false}
					sounding={false}
					picked={selection.has(item.key)}
					ondrag={startDrag}
					onmove={(key, by) => void shuffle.move(key, by)}
				/>
			</div>
		{/each}
	</div>
</section>

{#if rowMenu !== null}
	<RowMenu
		x={rowMenu.x}
		y={rowMenu.y}
		target={rowMenu.target}
		actions={rowMenu.actions}
		remove={rowMenu.remove}
		onclose={(refocus) => {
			rowMenu = null;
			if (refocus) {
				opener?.focus();
			}
		}}
	/>
{/if}
{#if selection.any}
	{@const picks = pickedFacts(picked, songFacts)}
	<SelectionBar
		count={pickedSongs.length}
		hidden={0}
		albums={picks.albums}
		durationMs={picks.durationMs}
		hearted={pickedSongs.every((id) => library.marks.hearted('tracks', id))}
		actions={pickedActions}
		compact={phone}
		edit={undefined}
		onclear={() => {
			selection.clear();
		}}
	/>
{/if}

<style>
	.up-next {
		--song-row-lined: 64px;

		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 16px;
		padding: 0 14px 12px;
	}

	h2 {
		font-size: 40px;
		font-weight: 800;
		letter-spacing: -0.04em;
	}

	.facts,
	.empty,
	.now {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.06em;
		color: var(--text-meta);
		white-space: nowrap;
	}

	.now {
		font-size: 11px;
		color: inherit;
	}

	.empty {
		padding: 20px 14px;
	}

	.rows {
		display: flex;
		flex-direction: column;
	}

	.slot.moving {
		position: relative;
		z-index: 1;
		opacity: 0.9;
	}

	@media (max-width: 720px) {
		h2 {
			font-size: 28px;
		}

		.head {
			padding: 0 8px 8px;
		}
	}

	@media (min-width: 2560px) {
		.up-next {
			--song-row-lined: 88px;
		}

		h2 {
			font-size: 56px;
		}
	}
</style>
