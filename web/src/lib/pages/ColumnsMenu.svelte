<script lang="ts">
	import type { ColumnId } from '#lib/library/source.ts';
	import { defaultColumns, menuList, moved, specOf, toggled } from './columns.ts';
	import { columnsView } from './columnsView.svelte.ts';
	import Floating from './Floating.svelte';

	interface Props {
		x: number;
		y: number;
		/** The label right-clicked, offered first as "Hide <label>". */
		hiding?: ColumnId | undefined;
		onclose: (refocus: boolean) => void;
	}

	let { x, y, hiding, onclose }: Props = $props();

	const pref = $derived(columnsView.pref);
	const list = $derived(menuList(pref));
	/** One row of the list: the boards' 36. */
	const itemHeight = 36;

	let dragging = $state<ColumnId | null>(null);
	let listElement: HTMLElement | undefined = $state();

	function moveTo(id: ColumnId, to: number): void {
		columnsView.set(moved(pref, id, to));
	}

	/** Alt with an arrow moves the focused column, the drag's keyboard twin. */
	function onkeydown(event: KeyboardEvent, id: ColumnId, index: number): void {
		if (!event.altKey || (event.key !== 'ArrowUp' && event.key !== 'ArrowDown')) {
			return;
		}
		event.preventDefault();
		event.stopPropagation();
		const to = index + (event.key === 'ArrowUp' ? -1 : 1);
		if (to < 0 || to >= list.length) {
			return;
		}
		moveTo(id, to);
		// The list redraws in its new order: the moved column keeps the focus.
		requestAnimationFrame(() => {
			listElement?.querySelector<HTMLElement>(`[data-column="${id}"] input`)?.focus();
		});
	}

	function startDrag(event: PointerEvent, id: ColumnId): void {
		if (event.button !== 0) {
			return;
		}
		event.preventDefault();
		dragging = id;
		const handle = event.currentTarget;
		if (handle instanceof HTMLElement) {
			handle.setPointerCapture(event.pointerId);
		}
	}

	function drag({ clientY }: PointerEvent): void {
		if (dragging === null || listElement === undefined) {
			return;
		}
		const { top } = listElement.getBoundingClientRect();
		const to = Math.max(0, Math.min(list.length - 1, Math.floor((clientY - top) / itemHeight)));
		if (list.at(to)?.id !== dragging) {
			moveTo(dragging, to);
		}
	}

	function drop(): void {
		dragging = null;
	}
</script>

<Floating {x} {y} label="Columns" width={320} {onclose}>
	{#if hiding !== undefined}
		<button
			type="button"
			role="menuitem"
			class="item"
			data-menu-item
			onclick={() => {
				columnsView.set(toggled(pref, hiding));
				onclose(true);
			}}>Hide {specOf(hiding).label}</button
		>
		<div class="rule"></div>
	{/if}
	<p class="eyebrow">COLUMNS · DRAG TO REORDER</p>
	{#each ['#', 'Title', 'Time'] as fixed (fixed)}
		<div class="always">
			<svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true"
				><rect x="6" y="10" width="12" height="10" rx="2"></rect><path d="M9 10V7a3 3 0 0 1 6 0v3"
				></path></svg
			>
			<span class="name">{fixed}</span>
			<span class="note">ALWAYS</span>
		</div>
	{/each}
	<div class="list" bind:this={listElement} role="group" aria-label="Choose and order the columns">
		{#each list as { id, on }, index (id)}
			<label class="column" class:on class:dragging={dragging === id} data-column={id}>
				<input
					type="checkbox"
					role="menuitemcheckbox"
					aria-checked={on}
					checked={on}
					data-menu-item
					aria-describedby="columns-reorder-hint"
					onchange={() => {
						columnsView.set(toggled(pref, id));
					}}
					onkeydown={(event) => {
						onkeydown(event, id, index);
					}}
				/>
				<span class="name">{specOf(id).label}</span>
				<span
					class="handle"
					aria-hidden="true"
					onpointerdown={(event) => {
						startDrag(event, id);
					}}
					onpointermove={drag}
					onpointerup={drop}
					onpointercancel={drop}
				>
					<svg width="16" height="16" viewBox="0 0 24 24"
						><circle cx="9" cy="6" r="1.6"></circle><circle cx="15" cy="6" r="1.6"></circle><circle
							cx="9"
							cy="12"
							r="1.6"
						></circle><circle cx="15" cy="12" r="1.6"></circle><circle cx="9" cy="18" r="1.6"
						></circle><circle cx="15" cy="18" r="1.6"></circle></svg
					>
				</span>
			</label>
		{/each}
	</div>
	<p id="columns-reorder-hint" class="hint">Alt with the arrow keys moves a column.</p>
	<div class="rule"></div>
	<label class="switch">
		<input
			type="checkbox"
			role="menuitemcheckbox"
			aria-checked={pref.artistWhenDifferent}
			checked={pref.artistWhenDifferent}
			data-menu-item
			onchange={() => {
				columnsView.set({ ...pref, artistWhenDifferent: !pref.artistWhenDifferent });
			}}
		/>
		<span>Artist only when it isn't the album's</span>
	</label>
	<button
		type="button"
		role="menuitem"
		class="item quiet"
		data-menu-item
		onclick={() => {
			columnsView.set(defaultColumns);
		}}>Reset to defaults</button
	>
</Floating>

<style>
	.item {
		height: 40px;
		padding: 0 12px;
		border: 0;
		border-radius: var(--radius-12);
		background: transparent;
		color: var(--text);
		font: 600 14px var(--font-sans);
		text-align: left;
	}

	.item:hover,
	.item:focus-visible {
		background: rgb(255 255 255 / 8%);
	}

	.quiet {
		color: var(--text-secondary);
	}

	.rule {
		height: 1px;
		margin: 8px 4px;
		background: var(--edge);
	}

	.eyebrow {
		padding: 4px 12px 8px;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.always {
		height: 36px;
		padding: 0 12px;
		display: flex;
		align-items: center;
		gap: 12px;
		font-size: 14px;
		color: var(--text-label);
	}

	.always svg {
		flex: none;
		fill: none;
		stroke: currentColor;
		stroke-width: 2;
	}

	.name {
		flex: 1;
		min-width: 0;
	}

	.note {
		font-family: var(--font-mono);
		font-size: 11px;
	}

	.list {
		display: flex;
		flex-direction: column;
	}

	.column {
		height: 36px;
		padding: 0 8px 0 12px;
		display: flex;
		align-items: center;
		gap: 12px;
		border-radius: 10px;
		font-size: 14px;
		color: var(--text-meta);
		user-select: none;
	}

	.column.on {
		color: var(--text);
	}

	.column:hover,
	.column:focus-within {
		background: rgb(255 255 255 / 6%);
	}

	.column.dragging {
		background: rgb(242 140 40 / 16%);
	}

	input {
		flex: none;
		width: 16px;
		height: 16px;
		margin: 0;
		accent-color: var(--fox);
	}

	.handle {
		flex: none;
		width: 28px;
		height: 32px;
		display: grid;
		place-items: center;
		cursor: grab;
		touch-action: none;
	}

	.dragging .handle {
		cursor: grabbing;
	}

	.handle svg {
		fill: var(--text-label);
	}

	.hint {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	.switch {
		min-height: 40px;
		padding: 6px 12px;
		display: flex;
		align-items: center;
		gap: 12px;
		font-size: 14px;
		color: var(--text-secondary);
	}
</style>
