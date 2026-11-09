<script lang="ts" module>
	import type { Sort, SortOption } from '#lib/library/sorts.ts';

	/** One part of a list's order, such as the Library's artists or their albums. */
	export interface SortGroup {
		label: string;
		options: readonly SortOption<string>[];
		current: Sort;
		choose: (next: Sort) => void;
	}
</script>

<script lang="ts">
	import { tick } from 'svelte';
	import { pick, wayOf } from '#lib/library/sorts.ts';
	import Floating from './Floating.svelte';

	interface Props {
		groups: readonly SortGroup[];
		/** What the button says: the order, and the way it runs. */
		words: string;
		way?: string;
		/** On a frame line, as a page's control, or a round button with only its icon. */
		look: 'line' | 'pill' | 'round';
		/** Whose order it is, for a screen reader: "Sort the library". */
		label: string;
	}

	let { groups, words, way = '', look, label }: Props = $props();

	const menuWidth = 320;
	/** Where the menu becomes a sheet from below. */
	const sheetQuery = '(max-width: 600px)';

	let button: HTMLButtonElement | undefined = $state();
	let open = $state<{ x: number; y: number; sheet: boolean } | null>(null);
	let sheet: HTMLElement | undefined = $state();
	/**
	 * When the menu last closed: a press on the button that closed it mustn't open it again. Never,
	 * to begin with: `performance.now()` counts from the page's load, so a 0 here would swallow a
	 * press in the page's first 300 ms.
	 */
	let closedAt = Number.NEGATIVE_INFINITY;

	function show(): void {
		if (open !== null) {
			close(false);
			return;
		}
		if (performance.now() - closedAt < 300) {
			return;
		}
		const box = button?.getBoundingClientRect();
		const left = box?.left ?? 0;
		open = {
			// Under the button, from its left edge; from its right, when it stands at the end of a line.
			x:
				left + menuWidth > document.documentElement.clientWidth
					? (box?.right ?? 0) - menuWidth
					: left,
			y: (box?.bottom ?? 0) + 8,
			sheet: matchMedia(sheetQuery).matches
		};
		if (open.sheet) {
			void tick().then(() => sheet?.querySelector<HTMLElement>('[data-menu-item]')?.focus());
		}
	}

	function close(refocus: boolean): void {
		open = null;
		closedAt = performance.now();
		if (refocus) {
			button?.focus();
		}
	}

	/** Lifts the sheet out of the page, to stand over the player island and everything else. */
	function onTop(node: HTMLElement): { destroy: () => void } {
		document.body.append(node);
		return {
			destroy: () => {
				node.remove();
			}
		};
	}

	function sheetKey(event: KeyboardEvent): void {
		if (event.key === 'Escape') {
			event.preventDefault();
			close(true);
		}
	}
</script>

{#snippet items()}
	{#each groups as group, index (group.label)}
		{#if index > 0}
			<div class="rule" role="separator"></div>
		{/if}
		<div class="eyebrow" aria-hidden="true">{group.label}</div>
		<div role="group" aria-label={group.label}>
			{#each group.options as option (option.by)}
				{@const on = option.by === group.current.by}
				<button
					type="button"
					class="item"
					class:on
					role="menuitemradio"
					aria-checked={on}
					data-menu-item
					onclick={() => {
						group.choose(pick(group.options, group.current, option.by));
					}}
				>
					<span class="check" aria-hidden="true">{on ? '✓' : ''}</span>
					<span class="name">{option.label}</span>
					<span class="way">{wayOf(option.by, on ? group.current.dir : option.first)}</span>
				</button>
			{/each}
		</div>
	{/each}
{/snippet}

<button
	type="button"
	class="sort {look}"
	aria-haspopup="menu"
	aria-expanded={open !== null}
	aria-label={look === 'round' ? `${label}: ${words}${way === '' ? '' : `, ${way}`}` : undefined}
	title={look === 'round' ? label : undefined}
	bind:this={button}
	onclick={show}
>
	<svg
		width={look === 'line' ? 14 : 16}
		height={look === 'line' ? 14 : 16}
		viewBox="0 0 24 24"
		aria-hidden="true"><path d="M4 6h16M4 12h11M4 18h6"></path></svg
	>
	{#if look !== 'round'}
		<span class="words">{words}</span>
		{#if way !== ''}
			<span class="way">{way}</span>
		{/if}
	{/if}
</button>

{#if open?.sheet === true}
	<div class="sheet-layer" use:onTop>
		<button
			type="button"
			class="scrim"
			aria-label="Close"
			tabindex="-1"
			onclick={() => {
				close(true);
			}}
		></button>
		<div
			class="sheet"
			role="menu"
			aria-label={label}
			tabindex="-1"
			bind:this={sheet}
			onkeydown={sheetKey}
		>
			<div class="sheet-head">
				<span class="sheet-title">Sort</span>
				<button
					type="button"
					class="done"
					onclick={() => {
						close(true);
					}}>Done</button
				>
			</div>
			{@render items()}
		</div>
	</div>
{:else if open !== null}
	<Floating x={open.x} y={open.y} {label} width={menuWidth} onclose={close}>
		{@render items()}
		<p class="hint">Pick the one you're on again to turn it round.</p>
	</Floating>
{/if}

<style>
	.sort {
		display: flex;
		align-items: center;
		gap: 8px;
		border: 1px solid var(--edge-strong);
		background: var(--fill);
		color: var(--text);
		white-space: nowrap;
	}

	.sort:hover,
	.sort[aria-expanded='true'] {
		background: var(--pressed);
	}

	.sort svg {
		flex: none;
	}

	.sort path {
		fill: none;
		stroke: currentcolor;
		stroke-width: 2;
		stroke-linecap: round;
	}

	/* On the Library's frame line: as small as the line, its target reaching past it. */
	.line {
		position: relative;
		height: 28px;
		padding: 0 10px;
		gap: 6px;
		border-radius: var(--radius-pill);
		font: 600 11px var(--font-mono);
		letter-spacing: 0.08em;
	}

	.line::after {
		content: '';
		position: absolute;
		inset: -8px -4px;
	}

	.pill {
		height: 40px;
		padding: 0 14px 0 12px;
		border-radius: var(--radius-pill);
		font: 600 13px var(--font-sans);
	}

	.pill .way {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.06em;
		color: var(--fox-text);
	}

	.round {
		width: 44px;
		height: 44px;
		flex: none;
		justify-content: center;
		padding: 0;
		border-radius: 50%;
	}

	.round svg {
		width: 18px;
		height: 18px;
	}

	.eyebrow {
		padding: 10px 12px 6px;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.rule {
		height: 1px;
		margin: 6px 4px;
		background: var(--edge);
	}

	.item {
		width: 100%;
		height: 40px;
		padding: 0 12px;
		display: flex;
		align-items: center;
		gap: 12px;
		border: 0;
		border-radius: var(--radius-12);
		background: transparent;
		color: var(--text-secondary);
		font: 500 14px var(--font-sans);
		text-align: left;
	}

	.item:hover,
	.item:focus-visible {
		background: var(--hover);
		color: var(--text);
	}

	.item.on {
		color: var(--text);
		font-weight: 700;
	}

	.check {
		width: 16px;
		flex: none;
		color: var(--fox-text);
		font-weight: 700;
	}

	.name {
		flex: 1;
		min-width: 0;
	}

	.item .way {
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 400;
		letter-spacing: 0.06em;
		color: var(--text-label);
	}

	.item.on .way {
		color: var(--fox-text);
	}

	/* The menu may open inside a mono line: its words are the page's. */
	.hint {
		padding: 6px 12px 6px;
		font-family: var(--font-sans);
		font-size: 12px;
		letter-spacing: normal;
		line-height: 1.5;
		color: var(--text-label);
	}

	.sheet-layer {
		position: fixed;
		z-index: 20;
		inset: 0;
	}

	.scrim {
		position: absolute;
		inset: 0;
		padding: 0;
		border: 0;
		background: color-mix(in srgb, var(--ground) 60%, transparent);
	}

	.sheet {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		max-height: 85dvh;
		overflow-y: auto;
		box-sizing: border-box;
		padding: 10px 8px max(24px, env(safe-area-inset-bottom));
		border-radius: var(--radius-28) var(--radius-28) 0 0;
		background: var(--card);
		border-top: 1px solid var(--edge-raised);
		box-shadow: var(--shadow-float);
		animation: rise 240ms var(--curve-settle);
	}

	.sheet:focus {
		outline: none;
	}

	.sheet-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0 12px 4px;
	}

	.sheet-title {
		font-size: 20px;
		font-weight: 800;
		letter-spacing: -0.03em;
	}

	.done {
		min-height: var(--target);
		padding: 0 8px;
		border: 0;
		background: none;
		color: var(--fox-text);
		font: 600 15px var(--font-sans);
	}

	.sheet .item {
		height: 52px;
		border-radius: var(--radius-16);
		font-size: 16px;
	}

	.sheet .item.on {
		background: var(--hover);
	}

	@keyframes rise {
		from {
			translate: 0 100%;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.sheet {
			animation-duration: 1ms;
		}
	}
</style>
