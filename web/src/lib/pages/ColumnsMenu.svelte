<script lang="ts">
	import { flushSync, onDestroy, type Snippet } from 'svelte';
	import type { ColumnId, MovableId } from '#lib/library/source.ts';
	import { flip, listsOnPage, reducedMotion, say } from './columnDrag.ts';
	import { moveFrame, type MoveFrame } from './columnMove.ts';
	import {
		columnsIn,
		isColumn,
		labelOf,
		menuList,
		moved,
		specOf,
		toggled,
		type ColumnsContext,
		type LineNote
	} from './columns.ts';
	import { columnsView, undoResetMs } from './columnsView.svelte.ts';
	import Floating from './Floating.svelte';

	interface Props {
		x: number;
		y: number;
		/** The label right-clicked, offered first as "Hide <label>". */
		hiding?: ColumnId | undefined;
		/** Where it was opened: on an album page, Album is the page, so it says it shows elsewhere. */
		context?: ColumnsContext;
		/**
		 * On a phone: a real song's row as the line shows it now, above the list, and what the line
		 * leaves out of it, noted beside each column.
		 */
		preview?: Snippet<[readonly MovableId[]]> | undefined;
		notes?: ReadonlyMap<ColumnId, LineNote> | undefined;
		onclose: (refocus: boolean) => void;
	}

	let { x, y, hiding, context = 'library', preview, notes, onclose }: Props = $props();
	/** What this list leaves out of the person's columns, which still shows elsewhere. */
	const elsewhere = $derived(
		new Set(
			menuList(columnsView.pref)
				.map(({ id }) => id)
				.filter((id) => !columnsIn([id], context).includes(id))
		)
	);

	const pref = $derived(columnsView.pref);
	const list = $derived(menuList(pref));
	/** One row of the list: the boards' 36, and a finger's 44 on a phone. */
	const itemHeight = $derived(preview === undefined ? 36 : 44);

	let listElement: HTMLElement | undefined = $state();
	/** The order a drag would make, which the phone's preview shows as each row is crossed. */
	let tentative = $state<readonly MovableId[] | null>(null);

	function moveTo(id: MovableId, to: number): void {
		columnsView.set(moved(pref, id, to));
	}

	function itemOf(id: MovableId): HTMLElement | null {
		return listElement?.querySelector<HTMLElement>(`[data-column="${id}"]`) ?? null;
	}

	/** Stamps a row that has landed, as a header label is. */
	function land(id: MovableId): void {
		const item = itemOf(id);
		if (item === null) {
			return;
		}
		delete item.dataset['landed'];
		void item.offsetWidth;
		item.dataset['landed'] = '';
		setTimeout(() => {
			delete item.dataset['landed'];
		}, 700);
	}

	/** Moves a column to `to`, every row sliding from where it was. */
	function flipTo(id: MovableId, to: number): void {
		const element = listElement;
		if (element === undefined) {
			return;
		}
		const items = (): HTMLElement[] => [...element.querySelectorAll<HTMLElement>('[data-column]')];
		const before = new Map(items().map((item) => [item.dataset['column'], item.offsetTop]));
		element.dataset['phase'] = 'commit';
		flushSync(() => {
			moveTo(id, to);
		});
		if (!reducedMotion()) {
			for (const item of items()) {
				const was = before.get(item.dataset['column']);
				if (was !== undefined && was !== item.offsetTop) {
					item.style.translate = `0 ${String(was - item.offsetTop)}px`;
				}
			}
			void element.offsetWidth;
		}
		element.dataset['phase'] = 'flip';
		for (const item of items()) {
			item.style.translate = '';
		}
		land(id);
		settle();
	}

	let settling: ReturnType<typeof setTimeout> | undefined;
	function settle(): void {
		clearTimeout(settling);
		settling = setTimeout(() => {
			if (listElement !== undefined) {
				delete listElement.dataset['phase'];
			}
		}, 240);
	}

	/** Alt with an arrow moves the focused column, the drag's keyboard twin. */
	function onkeydown(event: KeyboardEvent, id: MovableId, index: number): void {
		if (!event.altKey || (event.key !== 'ArrowUp' && event.key !== 'ArrowDown')) {
			return;
		}
		event.preventDefault();
		event.stopPropagation();
		const to = index + (event.key === 'ArrowUp' ? -1 : 1);
		if (to < 0 || to >= list.length) {
			say(`${labelOf(id)} is already ${to < 0 ? 'first' : 'last'}.`);
			return;
		}
		flipTo(id, to);
		// The list is in its new order: the moved column keeps the focus.
		itemOf(id)?.querySelector<HTMLElement>('input')?.focus();
		say(`${labelOf(id)} moved to ${String(to + 1)} of ${String(list.length)}.`);
	}

	/** A row under the pointer: it lifts and follows, and the others make room. */
	let lift: {
		id: MovableId;
		pointer: number;
		y: number;
		from: number;
		item: HTMLElement;
		frame: MoveFrame;
	} | null = null;

	function slots(): { left: number; width: number }[] {
		return list.map((_, index) => ({ left: index * itemHeight, width: itemHeight }));
	}

	function startDrag(event: PointerEvent, id: MovableId, index: number): void {
		const item = itemOf(id);
		if (event.button !== 0 || item === null || listElement === undefined || lift !== null) {
			return;
		}
		event.preventDefault();
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		lift = {
			id,
			pointer: event.pointerId,
			y: event.clientY,
			from: index,
			item,
			frame: moveFrame(slots(), index, 0, 0)
		};
		listElement.dataset['phase'] = 'drag';
		item.dataset['lifted'] = '';
		say(`Picked up ${labelOf(id)}, ${String(index + 1)} of ${String(list.length)}.`);
	}

	function drag(event: PointerEvent): void {
		if (lift?.pointer !== event.pointerId || listElement === undefined) {
			return;
		}
		const frame = moveFrame(slots(), lift.from, event.clientY - lift.y, 0);
		if (frame.target !== lift.frame.target && preview !== undefined) {
			tentative = moved(pref, lift.id, frame.target).order;
		}
		lift.frame = frame;
		const items = listElement.querySelectorAll<HTMLElement>('[data-column]');
		items.forEach((item, index) => {
			item.style.translate = `0 ${String(frame.shifts[index] ?? 0)}px`;
		});
	}

	function drop(event: PointerEvent): void {
		if (lift?.pointer !== event.pointerId || listElement === undefined) {
			return;
		}
		const { id, from, item, frame } = lift;
		lift = null;
		delete item.dataset['lifted'];
		tentative = null;
		const element = listElement;
		const items = (): HTMLElement[] => [...element.querySelectorAll<HTMLElement>('[data-column]')];
		if (event.type === 'pointercancel' || frame.target === from) {
			element.dataset['phase'] = 'flip';
			for (const each of items()) {
				each.style.translate = '';
			}
			settle();
			say(`${labelOf(id)} stays ${String(from + 1)} of ${String(list.length)}.`);
			return;
		}
		// The new order in this task, then only the dropped row closes what's left to its place.
		element.dataset['phase'] = 'commit';
		flushSync(() => {
			moveTo(id, frame.target);
		});
		for (const each of items()) {
			each.style.translate = '';
		}
		const left = frame.offset - (frame.target - from) * itemHeight;
		if (!reducedMotion()) {
			item.style.translate = `0 ${String(left)}px`;
			void element.offsetWidth;
		}
		element.dataset['phase'] = 'settle';
		item.style.translate = '';
		land(id);
		settle();
		say(`${labelOf(id)} moved to ${String(frame.target + 1)} of ${String(list.length)}.`);
	}

	/** Reset to defaults, and its Undo: every column travels home together, left to right. */
	function reset(): void {
		const undo = columnsView.undoable !== null;
		flip(
			listsOnPage(),
			() => {
				if (undo) {
					columnsView.undoReset();
				} else {
					columnsView.resetToDefaults();
				}
			},
			{ phase: 'reset', stagger: 24 }
		);
		say(
			undo
				? 'Columns put back as they were.'
				: 'Columns back to their defaults. Undo reset for six seconds.'
		);
	}

	/** The seconds Undo has left, counted down while it's offered. */
	let left = $state(0);
	$effect(() => {
		if (columnsView.undoable === null) {
			return;
		}
		const until = Date.now() + undoResetMs;
		left = Math.ceil(undoResetMs / 1000);
		const tick = setInterval(() => {
			left = Math.max(0, Math.ceil((until - Date.now()) / 1000));
		}, 250);
		return () => {
			clearInterval(tick);
		};
	});

	onDestroy(() => {
		clearTimeout(settling);
	});

	const noteWords: Record<LineNote, string> = {
		heading: 'ON THE HEADING',
		over: "DOESN'T FIT"
	};
</script>

<Floating {x} {y} label="Columns" width={preview === undefined ? 320 : 360} {onclose}>
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
	<p class="eyebrow">
		{preview === undefined ? 'YOUR SONG COLUMNS' : 'YOUR SONG LINE'} · DRAG TO REORDER
	</p>
	{#if preview !== undefined}
		<div class="preview">{@render preview(tentative ?? pref.order)}</div>
	{/if}
	<div
		class="list"
		class:phone={preview !== undefined}
		bind:this={listElement}
		role="group"
		aria-label="Choose and order the columns"
		style:--item={`${String(itemHeight)}px`}
	>
		{#each list as { id, on }, index (id)}
			{@const fixed = !isColumn(id)}
			{@const note = isColumn(id) ? notes?.get(id) : undefined}
			<label class="column" class:on class:elsewhere={elsewhere.has(id)} data-column={id}>
				<input
					type="checkbox"
					role="menuitemcheckbox"
					aria-checked={on}
					aria-disabled={fixed ? 'true' : undefined}
					checked={on}
					data-menu-item
					aria-describedby="columns-reorder-hint"
					onclick={(event) => {
						// The number and the title are always there: they move, but never go.
						if (fixed) {
							event.preventDefault();
						}
					}}
					onchange={() => {
						if (isColumn(id)) {
							columnsView.set(toggled(pref, id));
						}
					}}
					onkeydown={(event) => {
						onkeydown(event, id, index);
					}}
				/>
				<span class="name">{labelOf(id)}</span>
				{#if fixed}
					<span class="note">ALWAYS</span>
				{:else if note !== undefined}
					<span class="note">{noteWords[note]}</span>
				{:else if elsewhere.has(id)}
					<span class="note">LIBRARY ONLY</span>
				{/if}
				<span
					class="handle"
					aria-hidden="true"
					onpointerdown={(event) => {
						startDrag(event, id, index);
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
	<div class="always">
		<svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true"
			><rect x="6" y="10" width="12" height="10" rx="2"></rect><path d="M9 10V7a3 3 0 0 1 6 0v3"
			></path></svg
		>
		<span class="name">Time</span>
		<span class="note">ALWAYS LAST</span>
	</div>
	<p id="columns-reorder-hint" class="hint">
		Alt with the arrow keys moves a column. The number and the title move, but are always shown.
	</p>
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
		class:undo={columnsView.undoable !== null}
		data-menu-item
		onclick={reset}
		>{#if columnsView.undoable === null}Reset to defaults{:else}Undo reset <span class="left"
				>{left} S</span
			>{/if}</button
	>
	<p class="eyebrow shared">THE SAME IN THE LIBRARY AND ON EVERY ALBUM</p>
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
		background: var(--hover);
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

	.shared {
		padding: 8px 12px 4px;
	}

	.elsewhere .name,
	.elsewhere .note {
		color: var(--text-label);
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
		position: relative;
		height: var(--item, 36px);
		box-sizing: border-box;
		background: var(--card);
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
		background: var(--hover);
	}

	/* Lifted, as a header label is: raised, tilted, with the hard offset. */
	.column:global([data-lifted]) {
		z-index: 3;
		background: var(--card-raised);
		box-shadow:
			3px 3px 0 var(--lift-offset),
			var(--shadow-float);
		rotate: -0.8deg;
		scale: 1.02;
		transition:
			rotate 120ms var(--curve-morph),
			scale 120ms var(--curve-morph);
	}

	.column:global([data-lifted]) .handle {
		color: var(--fox-text);
		cursor: grabbing;
	}

	.column:global([data-landed]) {
		animation: thud var(--morph-drop) var(--curve-settle);
	}

	.column:global([data-landed]) .name {
		animation: stamp 640ms ease-out;
	}

	.list:global([data-phase='drag']) .column:not(:global([data-lifted])),
	.list:global([data-phase='flip']) .column {
		transition: translate var(--morph-room) var(--curve-room);
	}

	.list:global([data-phase='settle']) .column {
		transition: translate var(--morph-drop) var(--curve-settle);
	}

	@keyframes thud {
		0% {
			rotate: -0.8deg;
			scale: 1.02;
		}

		55% {
			rotate: 0.6deg;
			scale: 0.98;
		}

		100% {
			rotate: 0deg;
			scale: 1;
		}
	}

	@keyframes stamp {
		18%,
		60% {
			color: var(--fox-text);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.column,
		.column:global([data-lifted]) {
			transition: none !important;
			rotate: none;
			scale: none;
		}

		.column:global([data-landed]),
		.column:global([data-landed]) .name {
			animation: none;
		}
	}

	/* On a phone: a finger's grip, and a real row at the top. */
	.phone .handle {
		width: var(--target);
		height: var(--target);
	}

	/* It stays in sight as the list scrolls under it: what a move does shows as it's made. */
	.preview {
		position: sticky;
		z-index: 4;
		top: -8px;
		margin: 0 4px 8px;
		padding: 0 12px;
		border-radius: var(--radius-16);
		background: var(--ground);
		border: 1px dashed var(--edge-strong);
	}

	.preview :global(.row) {
		border-bottom: 0;
	}

	.undo {
		display: flex;
		align-items: center;
		justify-content: space-between;
		color: var(--fox-text);
	}

	.left {
		font: 500 11px var(--font-mono);
		letter-spacing: 0.06em;
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
		color: var(--text-label);
		cursor: grab;
		touch-action: none;
	}

	.handle svg {
		fill: currentColor;
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
