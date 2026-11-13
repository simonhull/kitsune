<!--
	A phone's way to its song line: there's no header under 700, and the order of the columns decides
	what a row's second line says. The pill opens the columns menu with a real row of this list at
	its top, which follows each move, and notes beside each column what the line leaves out.
-->
<script lang="ts">
	import type { ColumnId, LibraryTrack, MovableId } from '#lib/library/source.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import ColumnsMenu from './ColumnsMenu.svelte';
	import {
		lineNotes,
		numbered,
		sameOnAlbum,
		type ColumnsContext,
		type RowAlbum
	} from './columns.ts';
	import { columnsView } from './columnsView.svelte.ts';
	import SongRow from './SongRow.svelte';

	interface Props {
		/** The album whose first song is the preview. */
		album: (RowAlbum & { tracks: readonly LibraryTrack[] }) | undefined;
		context: ColumnsContext;
		/** How many characters the list's second line has room for. */
		lineChars: number;
		artistWhenDifferent: boolean;
		now: number;
	}

	let { album, context, lineChars, artistWhenDifferent, now }: Props = $props();

	let open = $state<{ x: number; y: number } | null>(null);
	let button: HTMLButtonElement | undefined = $state();

	const pref = $derived(columnsView.pref);
	const track = $derived(album?.tracks.at(0));
	const shared = $derived(
		album === undefined
			? new Set<ColumnId>()
			: sameOnAlbum(pref.order, album, artistWhenDifferent, now)
	);
	const sharedHas = (id: ColumnId): boolean => shared.has(id);

	/** Lifts the menu out of the page: the pill sits in a head that is its own layer. */
	function onTop(node: HTMLElement): { destroy: () => void } {
		document.body.append(node);
		return {
			destroy: () => {
				node.remove();
			}
		};
	}
	const notes = $derived(
		album === undefined || track === undefined
			? undefined
			: lineNotes(
					pref.order,
					{
						track,
						album,
						rating: library.marks.stars(track.id),
						artistWhenDifferent,
						now
					},
					sharedHas,
					lineChars
				)
	);
</script>

{#snippet preview(order: readonly MovableId[])}
	{#if album !== undefined && track !== undefined}
		<SongRow
			{track}
			{album}
			phone
			parts={numbered}
			shown={[]}
			{order}
			{artistWhenDifferent}
			{lineChars}
			sameOnAlbum={sharedHas}
			{now}
			stars={library.marks.stars(track.id)}
			cued={false}
			sounding={false}
			picked={false}
		/>
	{/if}
{/snippet}

<button
	type="button"
	class="line-pill"
	aria-haspopup="menu"
	aria-expanded={open !== null}
	bind:this={button}
	onclick={(event) => {
		const { left, bottom } = event.currentTarget.getBoundingClientRect();
		open = { x: left, y: bottom + 6 };
	}}
	><svg width="14" height="14" viewBox="0 0 24 24" aria-hidden="true"
		><path d="M4 7h16M4 12h16M4 17h10"></path></svg
	><span>LINE</span></button
>

{#if open !== null}
	<div use:onTop>
		<ColumnsMenu
			x={open.x}
			y={open.y}
			{context}
			preview={album === undefined ? undefined : preview}
			{notes}
			onclose={(refocus) => {
				open = null;
				if (refocus) {
					button?.focus();
				}
			}}
		/>
	</div>
{/if}

<style>
	.line-pill {
		position: relative;
		height: 28px;
		padding: 0 12px;
		display: inline-flex;
		flex: none;
		align-items: center;
		gap: 6px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text-secondary);
		font: 600 11px var(--font-mono);
		letter-spacing: 0.08em;
	}

	/* A finger's target reaches past the pill. */
	.line-pill::after {
		content: '';
		position: absolute;
		inset: -6px -2px;
	}

	.line-pill:hover,
	.line-pill[aria-expanded='true'] {
		border-color: var(--edge-hover);
		background: var(--fill);
		color: var(--text);
	}

	.line-pill path {
		fill: none;
		stroke: currentColor;
		stroke-width: 2;
		stroke-linecap: round;
	}
</style>
