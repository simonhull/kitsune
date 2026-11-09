<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { ColumnId } from '#lib/library/source.ts';
	import type { ColumnsPref } from '#lib/library/source.ts';
	import {
		baseWidth,
		clampWidth,
		numbered,
		nudged,
		specOf,
		widthRange,
		widthScale,
		type RowParts,
		type Step
	} from './columns.ts';

	interface Props {
		shown: readonly ColumnId[];
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
	}

	let {
		shown,
		hiddenCount,
		parts = numbered,
		openMenu,
		box,
		widths,
		step = 'columns',
		onresize
	}: Props = $props();

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
		const cells = [...list.querySelectorAll<HTMLElement>(`[data-col="${id}"]`)];
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

	/** The keyboard's way to the menu: the context-menu key, or Shift+F10, on a label. */
	function onkeydown(event: KeyboardEvent, hiding?: ColumnId): void {
		if (
			event.key === 'ContextMenu' ||
			event.key === 'Enter' ||
			(event.key === 'F10' && event.shiftKey)
		) {
			event.preventDefault();
			const { left, bottom } = (event.currentTarget as HTMLElement).getBoundingClientRect();
			openMenu(left, bottom + 4, hiding);
		}
	}

	function oncontextmenu(event: MouseEvent, hiding?: ColumnId): void {
		event.preventDefault();
		event.stopPropagation();
		openMenu(event.clientX, event.clientY, hiding);
	}
</script>

<div
	class="header"
	role="group"
	aria-label="Columns. On a label, the menu key or Shift+F10 chooses them."
	oncontextmenu={(event) => {
		oncontextmenu(event);
	}}
>
	{#if parts.handle}<span></span>{/if}
	<span class="box pick-gutter">{@render box?.()}</span>
	{#if parts.cover}<span></span>{/if}
	{#if parts.number}
		<span
			class="label"
			role="button"
			aria-haspopup="menu"
			tabindex="0"
			onkeydown={(event) => {
				onkeydown(event);
			}}>#</span
		>
	{/if}
	<span
		class="label"
		role="button"
		aria-haspopup="menu"
		tabindex="0"
		onkeydown={(event) => {
			onkeydown(event);
		}}>TITLE</span
	>
	{#each shown as id (id)}
		{@const spec = specOf(id)}
		{@const range = widthRange(id)}
		<span class="col" class:end={spec.end === true}>
			<span
				class="label"
				role="button"
				aria-haspopup="menu"
				tabindex="0"
				aria-label={spec.label}
				oncontextmenu={(event) => {
					oncontextmenu(event, id);
				}}
				onkeydown={(event) => {
					onkeydown(event, id);
				}}>{spec.head}</span
			>
			{#if onresize !== undefined}
				<!--
					A divider at its right edge: drag, double-click to fit, or the arrows. A focusable
					separator is WAI-ARIA's window splitter, an interactive widget Svelte's lists don't know.
				-->
				<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
				<span
					class="divider"
					role="separator"
					aria-orientation="vertical"
					aria-label={`Resize ${spec.label}. Left and right arrows; Shift for more.`}
					aria-valuenow={baseWidth(id, widths)}
					aria-valuemin={range.min}
					aria-valuemax={range.max}
					tabindex="0"
					onpointerdown={(event) => {
						startResize(event, id);
					}}
					onpointermove={moveResize}
					onpointerup={endResize}
					onpointercancel={endResize}
					ondblclick={(event) => {
						fitColumn(event, id);
					}}
					onkeydown={(event) => {
						keyResize(event, id);
					}}
				></span>
			{/if}
		</span>
	{/each}
	<span
		class="label end"
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
