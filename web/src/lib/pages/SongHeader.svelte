<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { ColumnId, MovableId } from '#lib/library/source.ts';
	import type { ColumnsPref } from '#lib/library/source.ts';
	import { ColumnDrag, keyMove, liftAfter, type MoveHost } from './columnDrag.ts';
	import './columnMotion.css';
	import {
		baseWidth,
		clampWidth,
		columnName,
		isColumn,
		labelOf,
		numbered,
		nudged,
		specOf,
		widthRange,
		widthScale,
		type RowParts,
		type Step
	} from './columns.ts';
	import { columnsView } from './columnsView.svelte.ts';

	interface Props {
		/** What the rows place, in order: the number, the title and the columns that fit. */
		shown: readonly MovableId[];
		hiddenCount: number;
		/** The parts the rows have, whose grid is the list's `--song-grid`. */
		parts?: RowParts;
		/** Opens the columns menu at a place, offering to hide a column if one was pointed at. */
		openMenu: (x: number, y: number, hiding?: ColumnId) => void;
		/** The header's box, where there's one. */
		box?: Snippet;
		/** The person's column widths, at the base step, and the step the list is at. */
		widths?: ColumnsPref['widths'];
		step?: Exclude<Step, 'phone'>;
		/**
		 * A column dragged, double-clicked or nudged to a width at the base step: once a drag ends,
		 * at each key. While a drag goes on, only the list's `--col-<id>` follows the pointer.
		 */
		onresize?: ((id: ColumnId, width: number) => void) | undefined;
		/**
		 * A column dropped at `to` among `shown`, or moved there by a key (`keys`, which saves once a
		 * run of them stops). It must update the page before it returns: the drop commits in its task.
		 * Without it, nothing moves.
		 */
		onreorder?: ((id: MovableId, to: number, keys: boolean) => void) | undefined;
		/** Whether a column may be dragged here: search's own Artist and Album may not. */
		movable?: (id: MovableId) => boolean;
	}

	let {
		shown,
		hiddenCount,
		parts = numbered,
		openMenu,
		box,
		widths,
		step = 'columns',
		onresize,
		onreorder,
		movable = () => true
	}: Props = $props();

	let header: HTMLElement | undefined = $state();

	/** What a label says: `#`, `TITLE`, or its column's heading. */
	function headOf(id: MovableId): string {
		if (id === 'number') {
			return '#';
		}
		return id === 'title' ? 'TITLE' : specOf(id).head;
	}

	/** This header's side of a move: the list it moves, and how a move lands. */
	function host(): MoveHost | null {
		const list = header?.closest<HTMLElement>('.song-list');
		if (header === undefined || list === null || list === undefined || onreorder === undefined) {
			return null;
		}
		const commit = onreorder;
		return {
			list,
			header,
			lineup: [...shown],
			commit,
			hold: (holding) => {
				columnsView.hold(holding);
			}
		};
	}

	/** A press on a label that may become a drag, once it has gone far enough sideways. */
	let press: {
		id: MovableId;
		x: number;
		pointer: number;
		cell: HTMLElement;
		drag: ColumnDrag | null;
		dx: number;
		frame: number;
	} | null = null;

	function startMove(event: PointerEvent, id: MovableId): void {
		const cell = event.currentTarget as HTMLElement;
		if (event.button !== 0 || press !== null || onreorder === undefined || !movable(id)) {
			return;
		}
		cell.setPointerCapture(event.pointerId);
		cell.dataset['pressing'] = '';
		press = { id, x: event.clientX, pointer: event.pointerId, cell, drag: null, dx: 0, frame: 0 };
	}

	function pointerMove(event: PointerEvent): void {
		if (press?.pointer !== event.pointerId) {
			return;
		}
		const dx = event.clientX - press.x;
		if (press.drag === null) {
			const threshold = event.pointerType === 'touch' ? liftAfter.touch : liftAfter.mouse;
			const moving = host();
			if (Math.abs(dx) < threshold || moving === null) {
				return;
			}
			press.drag = new ColumnDrag(moving, press.id);
			addEventListener('keydown', escape, true);
		}
		press.dx = dx;
		// One write a frame, however often the pointer reports.
		press.frame ||= requestAnimationFrame(() => {
			if (press !== null) {
				press.frame = 0;
				press.drag?.move(press.dx);
			}
		});
	}

	function endMove(event: PointerEvent, cancel: boolean): void {
		if (press?.pointer !== event.pointerId) {
			return;
		}
		finish(cancel);
	}

	function finish(cancel: boolean): void {
		if (press === null) {
			return;
		}
		const { drag, dx, frame, cell } = press;
		press = null;
		cancelAnimationFrame(frame);
		delete cell.dataset['pressing'];
		removeEventListener('keydown', escape, true);
		if (drag === null) {
			return;
		}
		if (cancel) {
			drag.cancel();
		} else {
			drag.move(dx);
			drag.drop();
		}
	}

	function escape(event: KeyboardEvent): void {
		if (event.key === 'Escape' && press?.drag != null) {
			event.preventDefault();
			event.stopPropagation();
			finish(true);
		}
	}

	/** The drag under way: the column, where it began, its width then, and the list it sizes. */
	let drag: { id: ColumnId; x: number; width: number; list: HTMLElement; scale: number } | null =
		null;

	function startResize(event: PointerEvent, id: ColumnId): void {
		const divider = event.currentTarget as HTMLElement;
		const cell = divider.parentElement;
		const list = divider.closest<HTMLElement>('.song-list');
		if (event.button !== 0 || cell === null || list === null) {
			return;
		}
		event.preventDefault();
		event.stopPropagation();
		divider.setPointerCapture(event.pointerId);
		drag = {
			id,
			x: event.clientX,
			width: cell.getBoundingClientRect().width,
			list,
			scale: widthScale(id, step)
		};
		list.dataset['resizing'] = '';
	}

	/** The pointer's width, in base-step pixels, kept in the column's range. */
	function dragged(event: PointerEvent): number {
		if (drag === null) {
			return 0;
		}
		return clampWidth(drag.id, (drag.width + event.clientX - drag.x) / drag.scale);
	}

	function moveResize(event: PointerEvent): void {
		if (drag !== null) {
			// The one property the grid reads: no row, and no state, changes while the pointer moves.
			const width = Math.round(dragged(event) * drag.scale);
			drag.list.style.setProperty(`--col-${drag.id}`, `${String(width)}px`);
		}
	}

	function endResize(event: PointerEvent): void {
		if (drag === null) {
			return;
		}
		const { id, list } = drag;
		const width = dragged(event);
		delete list.dataset['resizing'];
		drag = null;
		onresize?.(id, width);
	}

	/** A double-click fits the column to the widest of what it shows now. */
	function fitColumn(event: MouseEvent, id: ColumnId): void {
		const list = (event.currentTarget as HTMLElement).closest('.song-list');
		if (list === null) {
			return;
		}
		const cells = [...list.querySelectorAll<HTMLElement>(`.row [data-col="${id}"]`)];
		const widest = Math.max(0, ...cells.map((cell) => cell.scrollWidth));
		if (widest > 0) {
			onresize?.(id, (widest + 2) / widthScale(id, step));
		}
	}

	function keyResize(event: KeyboardEvent, id: ColumnId): void {
		if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') {
			return;
		}
		event.preventDefault();
		event.stopPropagation();
		onresize?.(id, nudged(baseWidth(id, widths), event.key, event.shiftKey));
	}

	/**
	 * A label's keys: the context-menu key, Shift+F10 or Enter open the menu; Alt with an arrow,
	 * Home or End moves its column.
	 */
	function onkeydown(event: KeyboardEvent, id?: MovableId): void {
		if (
			event.key === 'ContextMenu' ||
			event.key === 'Enter' ||
			(event.key === 'F10' && event.shiftKey)
		) {
			event.preventDefault();
			const { left, bottom } = (event.currentTarget as HTMLElement).getBoundingClientRect();
			openMenu(left, bottom + 4, id !== undefined && isColumn(id) ? id : undefined);
			return;
		}
		const keys = ['ArrowLeft', 'ArrowRight', 'Home', 'End'];
		if (!event.altKey || !keys.includes(event.key) || id === undefined) {
			return;
		}
		event.preventDefault();
		const moving = host();
		const cell = (event.currentTarget as HTMLElement).parentElement;
		if (moving === null || cell === null || !movable(id) || press !== null) {
			return;
		}
		keyMove(moving, id, event.key, cell);
	}

	function oncontextmenu(event: MouseEvent, hiding?: ColumnId): void {
		event.preventDefault();
		event.stopPropagation();
		openMenu(event.clientX, event.clientY, hiding);
	}
</script>

{#snippet grip()}
	<svg class="grip" width="5" height="9" viewBox="0 0 5 9" aria-hidden="true"
		><circle cx="1" cy="1" r="0.9"></circle><circle cx="4" cy="1" r="0.9"></circle><circle
			cx="1"
			cy="4.5"
			r="0.9"
		></circle><circle cx="4" cy="4.5" r="0.9"></circle><circle cx="1" cy="8" r="0.9"
		></circle><circle cx="4" cy="8" r="0.9"></circle></svg
	>
{/snippet}

<div
	class="header"
	role="group"
	aria-label={onreorder === undefined
		? 'Columns. On a label, the menu key or Shift+F10 chooses them.'
		: 'Columns. On a label, Alt with the arrow keys moves it; the menu key or Shift+F10 chooses them.'}
	bind:this={header}
	oncontextmenu={(event) => {
		oncontextmenu(event);
	}}
>
	{#if parts.handle}<span></span>{/if}
	<span class="box pick-gutter">{@render box?.()}</span>
	{#if parts.cover}<span></span>{/if}
	{#each shown as id (id)}
		{@const column = isColumn(id) ? specOf(id) : undefined}
		{@const moves = onreorder !== undefined && movable(id)}
		{@const range = column === undefined ? undefined : widthRange(column.id)}
		<!-- The cell is the handle: its label is the control the keyboard and a screen reader use. -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<span
			class="col"
			class:end={column?.end === true}
			class:movable={moves}
			data-col={id}
			onpointerdown={(event) => {
				startMove(event, id);
			}}
			onpointermove={pointerMove}
			onpointerup={(event) => {
				endMove(event, false);
			}}
			onpointercancel={(event) => {
				endMove(event, true);
			}}
			onlostpointercapture={(event) => {
				endMove(event, true);
			}}
		>
			{#if moves}{@render grip()}{/if}
			<span
				class="label"
				role="button"
				aria-haspopup="menu"
				tabindex="0"
				aria-label={moves ? columnName(shown, id) : labelOf(id)}
				oncontextmenu={(event) => {
					oncontextmenu(event, column?.id);
				}}
				onkeydown={(event) => {
					onkeydown(event, id);
				}}>{headOf(id)}</span
			>
			{#if column !== undefined && range !== undefined && onresize !== undefined}
				<!--
					A divider at its right edge: drag, double-click to fit, or the arrows. A focusable
					separator is WAI-ARIA's window splitter, an interactive widget Svelte's lists don't know.
				-->
				<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
				<span
					class="divider"
					role="separator"
					aria-orientation="vertical"
					aria-label={`Resize ${column.label}. Left and right arrows; Shift for more.`}
					aria-valuenow={baseWidth(column.id, widths)}
					aria-valuemin={range.min}
					aria-valuemax={range.max}
					tabindex="0"
					onpointerdown={(event) => {
						startResize(event, column.id);
					}}
					onpointermove={moveResize}
					onpointerup={endResize}
					onpointercancel={endResize}
					ondblclick={(event) => {
						fitColumn(event, column.id);
					}}
					onkeydown={(event) => {
						keyResize(event, column.id);
					}}
				></span>
			{/if}
		</span>
	{/each}
	<span
		class="label end fixed"
		role="button"
		aria-haspopup="menu"
		tabindex="0"
		onkeydown={(event) => {
			onkeydown(event);
		}}>TIME</span
	>
	<span class="chip-cell">
		{#if hiddenCount > 0}
			<button
				type="button"
				class="chip"
				aria-label={`${String(hiddenCount)} more ${hiddenCount === 1 ? 'column is' : 'columns are'} hidden at this width: choose columns`}
				onclick={(event) => {
					const { right, bottom } = event.currentTarget.getBoundingClientRect();
					openMenu(right - 320, bottom + 6);
				}}>+{hiddenCount}</button
			>
		{/if}
	</span>
</div>

<style>
	.header {
		height: var(--head-row, 40px);
		box-sizing: border-box;
		display: grid;
		grid-template-columns: var(--song-grid);
		align-items: center;
		column-gap: var(--col-gap, 12px);
		padding: 0 var(--row-pad, 14px);
		border-bottom: 1px solid var(--edge-raised);
		font-family: var(--font-mono);
		font-size: var(--head-type, 11px);
		letter-spacing: 0.08em;
		color: var(--text-meta);
		white-space: nowrap;
	}

	.label {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		border-radius: 6px;
	}

	.label:focus-visible {
		outline-offset: 2px;
	}

	.col {
		position: relative;
		min-width: 0;
		height: 100%;
		display: flex;
		align-items: center;
	}

	.col.end {
		justify-content: flex-end;
	}

	/*
	 * A column that moves: its whole cell is the handle. The pill is drawn by ::before, so the cell's
	 * box never moves; the grip sits at its start edge, in the gap, and takes no pointer.
	 */
	.col::before {
		content: '';
		position: absolute;
		inset: 7px -7px;
		border-radius: 8px;
		transition: background-color 120ms var(--curve-morph);
	}

	.col.movable {
		cursor: grab;
		touch-action: pan-y;
		user-select: none;
	}

	.col.movable .label {
		position: relative;
		z-index: 1;
	}

	.col.movable:hover::before {
		background: var(--hover);
	}

	.col.movable:global([data-pressing])::before {
		background: var(--pressed);
	}

	.grip {
		position: absolute;
		z-index: 1;
		left: -6px;
		top: 50%;
		translate: 0 -50%;
		opacity: 0;
		color: var(--text-label);
		pointer-events: none;
		transition: opacity 120ms var(--curve-morph);
	}

	.grip circle {
		fill: currentColor;
	}

	.col.movable:hover .grip,
	.col.movable:focus-within .grip {
		opacity: 1;
	}

	/* While one moves, the frame dims and the dividers go. */
	:global(.song-list[data-moving]) .col:not(.movable),
	:global(.song-list[data-moving]) .fixed {
		color: var(--text-label);
		transition: color 160ms var(--curve-morph);
	}

	:global(.song-list[data-moving]) .divider {
		visibility: hidden;
	}

	/* Lifted: a fox chip, tilted, with a hard offset like a sticker printed off register. */
	.col.movable:global([data-lifted]) {
		z-index: 5;
		color: var(--fox-ink);
		cursor: grabbing;
		rotate: -1.2deg;
		scale: 1.05;
		transition:
			rotate 120ms var(--curve-morph),
			scale 120ms var(--curve-morph);
	}

	.col.movable:global([data-lifted])::before {
		background: var(--fox);
		box-shadow: calc(3px * var(--step)) calc(3px * var(--step)) 0 var(--lift-offset);
		transition: none;
	}

	.col.movable:global([data-lifted]) .grip {
		opacity: 1;
		color: var(--fox-ink);
	}

	/* Landed: a thud, then a stamp of fox ink and a skewed scuff that wipes under it and fades. */
	.col.movable:global([data-landed]) {
		animation: thud var(--morph-drop) var(--curve-settle);
	}

	.col.movable:global([data-landed])::before {
		transition:
			background-color 240ms var(--curve-morph),
			box-shadow 120ms var(--curve-morph);
	}

	.col.movable:global([data-landed]) .label {
		animation: stamp 640ms ease-out;
	}

	.col.movable:global([data-landed])::after {
		content: '';
		position: absolute;
		left: -5px;
		right: -5px;
		bottom: calc(3px * var(--step));
		height: calc(3px * var(--step));
		border-radius: 1px;
		background: var(--fox-line);
		transform: skewX(-24deg);
		pointer-events: none;
		animation: scuff 560ms ease-out forwards;
	}

	.col:global([data-bump='left']) {
		animation: bump-left 180ms var(--curve-settle);
	}

	.col:global([data-bump='right']) {
		animation: bump-right 180ms var(--curve-settle);
	}

	@keyframes thud {
		0% {
			rotate: -1.2deg;
			scale: 1.05;
		}

		55% {
			rotate: 0.6deg;
			scale: 0.96;
		}

		100% {
			rotate: 0deg;
			scale: 1;
		}
	}

	@keyframes stamp {
		0% {
			color: var(--fox-ink);
		}

		18%,
		60% {
			color: var(--fox-text);
		}

		100% {
			color: inherit;
		}
	}

	@keyframes scuff {
		0% {
			clip-path: inset(0 100% 0 0);
			opacity: 1;
		}

		28% {
			clip-path: inset(0);
			opacity: 1;
		}

		100% {
			clip-path: inset(0);
			opacity: 0;
		}
	}

	@keyframes bump-left {
		35% {
			translate: -5px 0;
			rotate: -1deg;
		}
	}

	@keyframes bump-right {
		35% {
			translate: 5px 0;
			rotate: 1deg;
		}
	}

	@keyframes fade {
		from {
			opacity: 1;
		}

		to {
			opacity: 0;
		}
	}

	/* Reduced motion: no tilt, thud or bump; the stamp is the scuff's fade alone. */
	@media (prefers-reduced-motion: reduce) {
		.col.movable:global([data-lifted]) {
			rotate: none;
			scale: none;
		}

		.col.movable:global([data-landed]),
		.col.movable:global([data-landed]) .label,
		.col:global([data-bump]) {
			animation: none;
		}

		.col.movable:global([data-landed])::after {
			clip-path: none;
			animation: fade 560ms ease-out forwards;
		}

		.col::before,
		.grip {
			transition: none;
		}
	}

	/*
	 * Over the gap after the column: 8 wide, with a line that shows on hover, on focus and while
	 * it's dragged. Title takes the rest, so it has none.
	 */
	.divider {
		position: absolute;
		z-index: 1;
		top: 6px;
		bottom: 6px;
		right: calc(-4px - var(--col-gap, 12px) / 2);
		width: 8px;
		cursor: col-resize;
		touch-action: none;
	}

	.divider::after {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		left: 3px;
		width: 2px;
		border-radius: 1px;
		background: var(--fox-line);
		opacity: 0;
		transition: opacity 120ms var(--curve-morph);
	}

	.divider:hover::after,
	.divider:focus-visible::after,
	:global([data-resizing]) .divider:active::after {
		opacity: 1;
	}

	.divider:focus-visible {
		outline: none;
	}

	:global([data-resizing]) {
		cursor: col-resize;
		user-select: none;
	}

	@media (prefers-reduced-motion: reduce) {
		.divider::after {
			transition: none;
		}
	}

	.end {
		text-align: right;
	}

	.box,
	.chip-cell {
		display: flex;
		align-items: center;
	}

	/* The header's gutter reaches from the edge and down the whole header, as a row's does. */
	.box {
		align-self: stretch;
		margin-left: calc(-1 * var(--row-pad, 14px));
		padding-left: var(--row-pad, 14px);
	}

	.chip-cell {
		justify-content: flex-end;
	}

	.chip {
		height: 24px;
		padding: 0 6px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text-secondary);
		font: 500 11px var(--font-mono);
	}

	.chip:hover {
		border-color: var(--fox-line);
		color: var(--fox-text);
	}
</style>
