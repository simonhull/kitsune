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
	/** Where the menu takes a phone's whole width, with bigger rows: still the menu, never a sheet. */
	const phoneQuery = '(max-width: 600px)';

	let button: HTMLButtonElement | undefined = $state();
	let open = $state<{ x: number; y: number; phone: boolean } | null>(null);
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
			phone: matchMedia(phoneQuery).matches
		};
	}

	function close(refocus: boolean): void {
		open = null;
		// A press outside closes it with no refocus: if that press was on the button, its click must
		// not open it again. Choosing or Esc leaves the button free at once.
		if (!refocus) {
			closedAt = performance.now();
		}
		if (refocus) {
			button?.focus();
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

{#if open !== null}
	<Floating
		x={open.x}
		y={open.y}
		{label}
		width={menuWidth}
		full={open.phone}
		face={open.phone ? 'phone' : 'menu'}
		onclose={close}
	>
		<div class="items" class:phone={open.phone}>
			{@render items()}
		</div>
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

	/* On a page's frame: a step up with the page. */
	.pill {
		height: calc(40px * var(--step));
		padding: 0 calc(14px * var(--step)) 0 calc(12px * var(--step));
		border-radius: var(--radius-pill);
		font: 600 calc(13px * var(--step)) var(--font-sans);
	}

	.pill svg {
		width: calc(16px * var(--step));
		height: calc(16px * var(--step));
	}

	.pill .way {
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
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

	.items {
		display: flex;
		flex-direction: column;
	}

	.phone .item {
		height: 52px;
		border-radius: var(--radius-16);
		font-size: 16px;
	}

	.phone .item.on {
		background: var(--hover);
	}
</style>
