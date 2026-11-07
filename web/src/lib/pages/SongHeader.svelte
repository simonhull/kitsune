<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { ColumnId } from '#lib/library/source.ts';
	import { specOf } from './columns.ts';

	interface Props {
		shown: readonly ColumnId[];
		hiddenCount: number;
		/** The grid the rows have. */
		grid: string;
		/** Opens the columns menu at a place, offering to hide a column if one was pointed at. */
		openMenu: (x: number, y: number, hiding?: ColumnId) => void;
		/** The header's box, where there's one. */
		box?: Snippet;
	}

	let { shown, hiddenCount, grid, openMenu, box }: Props = $props();

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
	style:grid-template-columns={grid}
	oncontextmenu={(event) => {
		oncontextmenu(event);
	}}
>
	<span class="box">{@render box?.()}</span>
	<span
		class="label"
		role="button"
		aria-haspopup="menu"
		tabindex="0"
		onkeydown={(event) => {
			onkeydown(event);
		}}>#</span
	>
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
		<span
			class="label"
			class:end={spec.end === true}
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
		align-items: center;
		column-gap: var(--col-gap, 12px);
		padding: 0 var(--row-pad, 14px);
		border-bottom: 1px solid rgb(255 255 255 / 12%);
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

	.end {
		text-align: right;
	}

	.box,
	.chip-cell {
		display: flex;
		align-items: center;
	}

	.chip-cell {
		justify-content: flex-end;
	}

	.chip {
		height: 24px;
		padding: 0 6px;
		border: 1px solid rgb(255 255 255 / 16%);
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text-secondary);
		font: 500 11px var(--font-mono);
	}

	.chip:hover {
		border-color: var(--fox);
		color: var(--fox);
	}
</style>
