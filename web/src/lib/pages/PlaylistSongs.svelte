<!--
	A playlist's songs in its own order, as SongRows: a grip, the number, the title with its cover,
	who it's by and its album, and its time. Each entry is its own row, so a song in twice is two.
	The grip moves a song (held a moment first by a finger): the row dims, a 2 px fox line marks
	where it lands, and the move saves after the drop. Alt with an arrow moves the focused song; Del
	takes the picked songs, or the focused one, out of the playlist and never out of the library.
	Rows are placed in a window over the list, so 10,000 of them cost what a screenful does.

	A smart playlist's songs come in the order its rules give: no grip, no Alt with an arrow, no Del
	and no Take out of playlist, and after the album the columns its rules read, headed in the fox.
-->
<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { clock } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type {
		LibraryTrack,
		MovableId,
		PlaylistAlbum,
		PlaylistDetail,
		PlaylistEntry,
		SmartRules
	} from '#lib/library/source.ts';
	import { ruleColumns } from '#lib/library/smartlist.ts';
	import { smartFoot } from './playlistWords.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import { rowWindow } from './artistSongs.ts';
	import type { RowWindow } from './libraryRows.ts';
	import {
		columnSpecs,
		columnsFrom,
		fit,
		largeFrom,
		leadingCount,
		monoAdvance,
		template,
		type RowParts
	} from './columns.ts';
	import { playlistActions } from './playlistActions.svelte.ts';
	import RowMenu from './RowMenu.svelte';
	import SelectionBar from './SelectionBar.svelte';
	import { onScreen, Selection } from './selection.svelte.ts';
	import {
		actionsFor,
		menuTarget,
		pickedFacts,
		type MenuTarget,
		type SongActions,
		type SongFacts
	} from './songActions.ts';
	import { songGrid } from './songGrid.ts';
	import SongRow from './SongRow.svelte';
	import { focusRow, isTrashKey, rowEvents, SongList } from './songRow.ts';

	interface Props {
		playlist: PlaylistDetail;
		/** Plays from an entry, through the playlist to its end. */
		onplay: (entryId: string) => void;
		/** A smart playlist's rules: its order is theirs, so nothing moves or comes out. */
		smartRules?: SmartRules | undefined;
	}

	let { playlist, onplay, smartRules }: Props = $props();

	const smart = $derived(smartRules !== undefined);
	const rulesRead = $derived(smartRules === undefined ? [] : ruleColumns(smartRules));

	/** How long a finger holds the grip before the song comes up to be moved. */
	const holdMs = 220;
	/** How near an edge of the screen a dragged song scrolls the page. */
	const edge = 72;

	let windowWidth = $state(1280);
	let windowHeight = $state(800);
	let listWidth = $state(0);
	let root: HTMLElement | undefined = $state();
	let rowsElement: HTMLElement | undefined = $state();

	const phone = $derived(windowWidth < columnsFrom);
	const step = $derived(windowWidth >= largeFrom ? 'large' : 'columns');
	/** A phone's rows have no number, as the board has them. */
	const parts: RowParts = $derived({ handle: !smart, cover: true, number: !phone });
	const rowHeight = $derived(phone ? 58 : step === 'large' ? 56 : 46);
	const order: MovableId[] = $derived(
		smart ? ['number', 'title', 'album', ...rulesRead] : ['number', 'title', 'artist', 'album']
	);
	/** What a phone's second line says: the album, then the time. */
	const phoneOrder: MovableId[] = $derived(
		smart ? ['title', 'album', ...rulesRead] : ['title', 'album']
	);
	const fitted = $derived(fit(order, listWidth, step, parts));
	const grid = $derived(template(fitted.shown, step, parts));
	const lineChars = $derived(Math.floor((listWidth - 150) / monoAdvance));
	const now = Date.now();
	const cuedId = $derived(library.playing?.track.id);

	const entries = $derived(playlist.entries);
	const keys = $derived(entries.map(({ id }) => id));
	const byEntry = $derived(new Map(entries.map((entry) => [entry.id, entry])));
	const albums = $derived(new Map(playlist.albums.map((album) => [album.id, album])));
	const albumOf = $derived(new Map(entries.map(({ track, albumId }) => [track.id, albumId])));

	// The stars came with the songs: the rows show them even before any index is read.
	$effect(() => {
		const tracks = entries.map(({ track }) => track);
		untrack(() => {
			library.marks.seedTracks(tracks);
		});
	});

	const selection = new Selection();
	onMount(() => onScreen.show(selection));
	const list = new SongList(selection, {
		order: () => keys,
		reveal: (key) => {
			reveal(key);
		}
	});
	// An entry picked and since gone is let go.
	$effect(() => {
		const held = new Set(keys);
		untrack(() => {
			for (const id of selection.ids) {
				if (!held.has(id)) {
					selection.toggle(id);
				}
			}
		});
	});
	const picked = $derived(selection.inOrder(keys));
	const pickedTracks = $derived(picked.flatMap((key) => byEntry.get(key)?.track.id ?? []));
	const pickedActions = $derived(actionsFor(pickedTracks, albumOf));
	const songFacts: SongFacts = (key) => {
		const entry = byEntry.get(key);
		return entry === undefined
			? undefined
			: { albumId: entry.albumId, durationMs: entry.track.durationMs };
	};

	// Only the rows near the screen are there; the list is as tall as all of them.
	let mounted = $state<RowWindow>({ first: 0, last: -1 });
	function measure(): void {
		if (rowsElement === undefined) {
			return;
		}
		const top = rowsElement.getBoundingClientRect().top;
		mounted = rowWindow(keys.length, rowHeight, -top, windowHeight, mounted);
	}
	$effect(() => {
		void keys.length;
		void rowHeight;
		void windowHeight;
		untrack(measure);
	});
	const placed = $derived(
		entries
			.slice(mounted.first, mounted.last + 1)
			.map((entry, i) => ({ entry, at: mounted.first + i }))
	);

	/** Brings an entry's row on screen, if it isn't, and gives it the focus. */
	function reveal(key: string): void {
		const at = keys.indexOf(key);
		if (at < 0 || rowsElement === undefined) {
			return;
		}
		const top = rowsElement.getBoundingClientRect().top + scrollY + at * rowHeight;
		if (top < scrollY + 120 || top + rowHeight > scrollY + windowHeight - 140) {
			scrollTo({ top: top - windowHeight / 2, behavior: 'instant' });
			measure();
		}
		requestAnimationFrame(() => {
			focusRow(root, key);
		});
	}

	/** A row's song, keyed by its entry: a song in twice is two rows. */
	function rowTrack(entry: PlaylistEntry, at: number): LibraryTrack {
		// The number is its place in the playlist, not on its album: the rest is what the rules read.
		// eslint-disable-next-line @typescript-eslint/no-unused-vars -- The row's own number is its place.
		const { id: _track, discNo: _disc, trackNo: _no, ...rest } = entry.track;
		return { ...rest, id: entry.id, trackNo: at + 1 };
	}

	const noAlbum: PlaylistAlbum = { id: '', title: '', artist: '', compilation: false };

	/** Takes the picked entries out, or the one given when it isn't among them. */
	function takeOut(key: string): void {
		const taking = selection.has(key) && picked.length > 0 ? picked : [key];
		const after = keys[Math.min(keys.length - 1, keys.indexOf(taking.at(-1) ?? key) + 1)];
		playlistActions.takeOut(playlist.id, taking);
		selection.clear();
		if (after !== undefined && !taking.includes(after)) {
			requestAnimationFrame(() => {
				focusRow(root, after);
			});
		}
	}

	/** Moves an entry one place up or down, keeping the focus on it. */
	function nudge(key: string, by: -1 | 1): void {
		const at = keys.indexOf(key);
		const to = at + by;
		if (at < 0 || to < 0 || to >= keys.length) {
			return;
		}
		void library.playlists.move(playlist.id, key, to).catch(() => undefined);
		requestAnimationFrame(() => {
			reveal(key);
			root?.querySelector<HTMLElement>(`[data-track='${key}'] .handle`)?.focus();
		});
	}

	let rowMenu = $state<{
		x: number;
		y: number;
		target: MenuTarget;
		actions: SongActions;
		takeOut: () => void;
	} | null>(null);
	let opener: HTMLElement | null = null;

	const events = rowEvents({
		play: (key) => {
			onplay(key);
		},
		pick: (key, how) => {
			if (byEntry.has(key)) {
				list.pick(key, how);
			}
		},
		menu: (key, x, y) => {
			const entry = byEntry.get(key);
			if (entry === undefined) {
				return;
			}
			opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
			const album = albums.get(entry.albumId);
			const target = menuTarget(
				key,
				selection,
				() => picked,
				songFacts,
				() => ({
					albumId: entry.albumId,
					title: entry.track.title,
					line: `${(album?.title ?? '').toUpperCase()} · ${clock(entry.track.durationMs / 1000)}`,
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
				target: many
					? { ...target, trackIds: pickedTracks }
					: { ...target, trackId: entry.track.id },
				actions: many ? pickedActions : actionsFor([entry.track.id], albumOf),
				takeOut: () => {
					takeOut(key);
				}
			};
		},
		key: (event, key) => {
			if (smart) {
				list.key(event, key);
				return;
			}
			if (event.altKey && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) {
				event.preventDefault();
				nudge(key, event.key === 'ArrowUp' ? -1 : 1);
				return;
			}
			if (isTrashKey(event)) {
				event.preventDefault();
				takeOut(key);
				return;
			}
			list.key(event, key);
		},
		rate: (key, stars) => {
			const entry = byEntry.get(key);
			if (entry !== undefined) {
				library.marks.rate(entry.track.id, stars);
			}
		},
		picked: (key) => selection.has(key)
	});

	/** The entry being moved by its grip, where it came from, and where it would land now. */
	let dragging = $state<{ key: string; from: number; to: number } | null>(null);
	/** A finger's grip, held but not yet long enough. */
	let holding: { timer: ReturnType<typeof setTimeout>; key: string; y: number } | null = null;
	let lastY = 0;
	let scrolling = 0;

	function landing(clientY: number): number {
		if (rowsElement === undefined) {
			return 0;
		}
		const top = rowsElement.getBoundingClientRect().top;
		return Math.max(0, Math.min(keys.length - 1, Math.floor((clientY - top) / rowHeight)));
	}

	function startDrag(key: string, event: PointerEvent): void {
		if (event.button !== 0) {
			return;
		}
		event.preventDefault();
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		lastY = event.clientY;
		const begin = (): void => {
			const from = keys.indexOf(key);
			if (from >= 0) {
				dragging = { key, from, to: from };
				follow();
			}
		};
		if (event.pointerType === 'touch') {
			holding = { key, y: event.clientY, timer: setTimeout(begin, holdMs) };
			return;
		}
		begin();
	}

	/** Scrolls the page while a dragged song is held near its top or bottom edge. */
	function follow(): void {
		cancelAnimationFrame(scrolling);
		if (dragging === null) {
			return;
		}
		const by = lastY < edge ? -12 : lastY > windowHeight - edge ? 12 : 0;
		if (by !== 0) {
			scrollBy({ top: by, behavior: 'instant' });
			measure();
			dragging = { ...dragging, to: landing(lastY) };
		}
		scrolling = requestAnimationFrame(follow);
	}

	function dragMove(event: PointerEvent): void {
		lastY = event.clientY;
		if (holding !== null && Math.abs(event.clientY - holding.y) > 10) {
			clearTimeout(holding.timer);
			holding = null;
		}
		if (dragging !== null) {
			const to = landing(event.clientY);
			if (to !== dragging.to) {
				dragging = { ...dragging, to };
			}
		}
	}

	function drop(): void {
		if (holding !== null) {
			clearTimeout(holding.timer);
			holding = null;
		}
		cancelAnimationFrame(scrolling);
		const held = dragging;
		dragging = null;
		if (held !== null && held.to !== held.from) {
			void library.playlists.move(playlist.id, held.key, held.to).catch(() => undefined);
		}
	}

	/** Where the fox line goes: above the row landed on going up, below it going down. */
	function markOf(at: number): 'above' | 'below' | null {
		if (dragging === null || dragging.to !== at || dragging.to === dragging.from) {
			return null;
		}
		return dragging.to < dragging.from ? 'above' : 'below';
	}

	function onWindowKey(event: KeyboardEvent): void {
		if (event.key === 'Escape' && dragging !== null) {
			dragging = null;
			return;
		}
		list.windowKey(event, rowMenu !== null);
	}

	const heads = $derived(
		fitted.shown.map((id) =>
			id === 'number'
				? { id, head: '#' }
				: id === 'title'
					? { id, head: 'TITLE' }
					: { id, head: columnSpecs.find((spec) => spec.id === id)?.head ?? '' }
		)
	);
</script>

<svelte:window
	bind:innerWidth={windowWidth}
	bind:innerHeight={windowHeight}
	onkeydown={onWindowKey}
	onscroll={measure}
	onpointermove={dragMove}
	onpointerup={drop}
	onpointercancel={() => {
		if (holding !== null) {
			clearTimeout(holding.timer);
			holding = null;
		}
		dragging = null;
	}}
/>

<section
	class="songs song-list"
	class:dragging={dragging !== null}
	aria-label={`Songs in ${playlist.name}`}
	bind:this={root}
	bind:clientWidth={listWidth}
	use:songGrid={grid}
	data-selecting={selection.any ? '' : undefined}
>
	{#if !phone}
		<div class="header" data-song-header aria-hidden="true">
			{#each { length: leadingCount(parts) }, i (i)}<span></span>{/each}
			{#each heads as { id, head } (id)}<span
					class={['head', id, rulesRead.includes(id as never) && 'rule']}>{head}</span
				>{/each}
			<span class="head end">TIME</span><span></span>
		</div>
	{/if}
	{#if entries.length === 0}
		<p class="empty">
			{smart
				? 'No song matches its rules yet. Rate or play some, or edit the rules.'
				: 'Nothing in it yet. Add songs from any ⋯ menu, or pick some and press Add to…'}
		</p>
	{/if}
	<div
		class="rows"
		bind:this={rowsElement}
		style:height={`${String(entries.length * rowHeight)}px`}
		{...events}
	>
		{#each placed as { entry, at } (entry.id)}
			{@const mark = markOf(at)}
			{@const album = albums.get(entry.albumId) ?? noAlbum}
			<div
				class="slot"
				class:moving={dragging?.key === entry.id}
				class:above={mark === 'above'}
				class:below={mark === 'below'}
				style:top={`${String(at * rowHeight)}px`}
				style:height={`${String(rowHeight)}px`}
			>
				<SongRow
					track={rowTrack(entry, at)}
					{album}
					{phone}
					{parts}
					shown={fitted.shown}
					order={phone ? phoneOrder : order}
					artistWhenDifferent={false}
					{lineChars}
					albumOnLine
					height={rowHeight}
					{now}
					stars={library.marks.stars(entry.track.id)}
					cued={cuedId === entry.track.id}
					sounding={cuedId === entry.track.id && player.sounding}
					picked={selection.has(entry.id)}
					ondrag={smart ? undefined : startDrag}
					onmove={smart ? undefined : nudge}
				/>
			</div>
		{/each}
	</div>
	{#if entries.length > 0}
		<p class="foot">
			{smart
				? phone
					? 'LIVE: RATE OR PLAY A SONG AND IT JOINS OR LEAVES'
					: smartFoot
				: phone
					? 'HOLD A GRIP TO MOVE A SONG'
					: 'DRAG A GRIP TO MOVE A SONG · DEL TAKES IT OUT OF THE PLAYLIST, NEVER THE LIBRARY'}
		</p>
	{/if}
</section>

{#if rowMenu !== null}
	<RowMenu
		x={rowMenu.x}
		y={rowMenu.y}
		target={rowMenu.target}
		actions={rowMenu.actions}
		takeOut={smart ? undefined : rowMenu.takeOut}
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
		count={picked.length}
		hidden={0}
		albums={picks.albums}
		durationMs={picks.durationMs}
		actions={pickedActions}
		compact={phone}
		edit={pickedActions.editTags}
		onclear={() => {
			selection.clear();
		}}
	/>
{/if}

<style>
	.songs {
		--song-row: 100%;
		--song-row-two: 100%;

		position: relative;
		padding: 6px calc(14px * var(--step)) 4px;
		border-radius: var(--radius-24);
		background: var(--card);
	}

	.header {
		display: grid;
		align-items: center;
		column-gap: var(--col-gap, 12px);
		height: calc(36px * var(--step));
		padding: 0 var(--row-pad, 14px);
		border-bottom: 1px solid var(--edge);
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.head {
		overflow: hidden;
		white-space: nowrap;
	}

	.end {
		text-align: right;
	}

	.rule {
		color: var(--fox-text);
	}

	.rows {
		position: relative;
	}

	.slot {
		position: absolute;
		left: 0;
		right: 0;
	}

	.slot :global(.row) {
		border-radius: var(--radius-12);
	}

	.slot.moving :global(.row) {
		background: color-mix(in srgb, var(--fox) 10%, transparent);
		opacity: 0.55;
	}

	.slot.above::before,
	.slot.below::after {
		content: '';
		position: absolute;
		z-index: 1;
		left: 6px;
		right: 6px;
		height: 2px;
		border-radius: 1px;
		background: var(--fox-line);
	}

	.slot.above::before {
		top: -1px;
	}

	.slot.below::after {
		bottom: -1px;
	}

	.dragging,
	.dragging :global(*) {
		cursor: grabbing;
	}

	.empty {
		padding: 28px 8px;
		color: var(--text-secondary);
	}

	.foot {
		padding: 10px 6px 8px;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	@media (max-width: 699px) {
		.songs {
			padding: 0;
			background: none;
		}
	}
</style>
