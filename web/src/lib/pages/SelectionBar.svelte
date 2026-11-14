<script lang="ts">
	import { floor } from '#lib/shell/floor.svelte.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import type { StarsBy } from '#lib/library/stars.ts';
	import StarsInput from '#lib/shell/StarsInput.svelte';
	import { factsLine, type SongActions } from './songActions.ts';

	interface Props {
		count: number;
		/** Picked, and hidden by the filter. */
		hidden: number;
		albums: number;
		durationMs: number;
		actions: SongActions;
		/** A phone: Play stays, and the rest go in More. */
		compact: boolean;
		/** Opens bulk editing on the songs. Left out for anyone who can't. */
		edit: (() => void) | undefined;
		onclear: () => void;
	}

	let { count, hidden, albums, durationMs, actions, compact, edit, onclear }: Props = $props();

	const editLabel = $derived(
		`Edit ${count.toLocaleString('en-US')} ${count === 1 ? 'song' : 'songs'}`
	);

	const trashLabel = $derived(
		`Move ${count.toLocaleString('en-US')} ${count === 1 ? 'song' : 'songs'} to Trash`
	);

	let rating = $state(false);
	let more = $state(false);

	const label = $derived(
		`${count.toLocaleString('en-US')} selected${hidden > 0 ? `, ${hidden.toLocaleString('en-US')} hidden` : ''}`
	);
	const facts = $derived(factsLine({ albums, durationMs }));

	/** What the stars last set, while they are open: the keys go on from it. */
	let chosen = $state<number | null>(null);

	/** Rates them all. A pointer or a finger is done, and closes the stars; a key may go on. */
	function rate(stars: number | null, by: StarsBy = 'pointer'): void {
		actions.rate(stars);
		chosen = stars;
		if (by === 'pointer') {
			rating = false;
			more = false;
			chosen = null;
		}
	}
</script>

{#snippet stars()}
	<div class="stars">
		<StarsInput size="bar" value={chosen} label={`Rate all ${String(count)}`} onchange={rate} />
		<button
			type="button"
			class="clear-stars"
			onclick={() => {
				rate(null);
			}}>Clear</button
		>
	</div>
{/snippet}

<div class="bar" class:compact role="toolbar" use:floor aria-label={`${label}. Esc clears.`}>
	<span class="count">{label}</span>
	{#if !compact}<span class="facts">{facts}</span>{/if}
	<button
		type="button"
		class={edit === undefined ? 'primary' : 'text'}
		onclick={() => {
			actions.play();
		}}
		>{#if edit === undefined}<Icon name="play" size={14} />{/if}Play</button
	>
	<button
		type="button"
		class="text"
		onclick={() => {
			actions.shuffle();
		}}>Shuffle</button
	>
	{#if !compact}
		<button
			type="button"
			class="text"
			onclick={() => {
				actions.playNext();
			}}>Play next</button
		>
	{/if}
	{#if compact}
		<button
			type="button"
			class="text"
			aria-expanded={more}
			onclick={() => {
				more = !more;
			}}>More</button
		>
	{:else}
		<button
			type="button"
			class="text"
			onclick={() => {
				actions.addToQueue();
			}}>Add to queue</button
		>
		{#if edit !== undefined}
			<button type="button" class="primary" onclick={edit}>{editLabel}</button>
		{/if}
		{#if actions.trash !== undefined}
			{@const trash = actions.trash}
			<button
				type="button"
				class="text danger"
				aria-label={trashLabel}
				aria-keyshortcuts="Delete"
				onclick={() => {
					trash();
				}}><Icon name="trash" size={16} />Trash</button
			>
		{/if}
		<button
			type="button"
			class="text"
			aria-expanded={rating}
			onclick={() => {
				rating = !rating;
			}}>Rate</button
		>
	{/if}
	<button type="button" class="clear" aria-label="Clear the selection" onclick={onclear}
		><Icon name="close" size={16} /></button
	>
	{#if rating && !compact}
		<div class="pop">{@render stars()}</div>
	{/if}
	{#if more && compact}
		<div class="pop sheet" role="menu" aria-label="More for the selection">
			<p class="sheet-facts">{facts}</p>
			<button
				type="button"
				role="menuitem"
				class="sheet-item"
				onclick={() => {
					actions.playNext();
					more = false;
				}}>Play next</button
			>
			<button
				type="button"
				role="menuitem"
				class="sheet-item"
				onclick={() => {
					actions.addToQueue();
					more = false;
				}}>Add to queue</button
			>
			{#if edit !== undefined}
				<button
					type="button"
					role="menuitem"
					class="sheet-item"
					onclick={() => {
						more = false;
						edit();
					}}>{editLabel}</button
				>
			{/if}
			{#if actions.trash !== undefined}
				{@const trash = actions.trash}
				<button
					type="button"
					role="menuitem"
					class="sheet-item danger"
					onclick={() => {
						more = false;
						trash();
					}}><Icon name="trash" size={16} />{trashLabel}</button
				>
			{/if}
			{@render stars()}
		</div>
	{/if}
</div>

<style>
	.bar {
		position: fixed;
		z-index: 3;
		left: 50%;
		bottom: calc(var(--bar-bottom, 24px) + env(safe-area-inset-bottom));
		transform: translateX(-50%);
		width: min(1024px, calc(100% - 32px));
		height: 76px;
		box-sizing: border-box;
		padding: 10px 10px 10px 24px;
		display: flex;
		align-items: center;
		gap: 6px;
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid color-mix(in srgb, var(--fox) 50%, transparent);
		box-shadow: var(--shadow-float);
		animation: bar-in var(--morph-fast) var(--curve-settle);
	}

	@keyframes bar-in {
		from {
			opacity: 0;
			transform: translate(-50%, 16px);
		}
	}

	.count {
		font-size: var(--type-title);
		font-weight: 800;
		letter-spacing: var(--tracking-title);
		white-space: nowrap;
	}

	.facts {
		margin: 0 8px 0 4px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
		white-space: nowrap;
	}

	button {
		flex: none;
		height: 48px;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text);
		font: 600 14px var(--font-sans);
		white-space: nowrap;
	}

	.primary {
		padding: 0 20px;
		display: flex;
		align-items: center;
		gap: 8px;
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	.text {
		padding: 0 14px;
	}

	.text:hover {
		background: var(--hover);
	}

	.danger {
		display: flex;
		align-items: center;
		gap: 8px;
		color: var(--danger);
	}

	.clear {
		margin-left: auto;
		width: 48px;
		display: grid;
		place-items: center;
		background: var(--hover);
	}

	.pop {
		position: absolute;
		right: 120px;
		bottom: calc(100% + 10px);
		padding: 8px;
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid var(--edge-raised);
		box-shadow: var(--shadow-float);
	}

	.stars {
		display: flex;
		align-items: center;
	}

	.stars .clear-stars {
		height: 44px;
		padding: 0 14px;
		font-size: 14px;
		color: var(--text-secondary);
	}

	.compact {
		height: 64px;
		padding: 6px 6px 6px 16px;
		gap: 2px;
		border-radius: 24px;
	}

	.compact .count {
		margin-right: auto;
		font-size: 15px;
	}

	.compact .clear {
		margin-left: 0;
	}

	.compact button {
		height: 44px;
	}

	.compact .primary {
		padding: 0 14px;
	}

	.compact .text {
		padding: 0 10px;
	}

	.sheet {
		left: 0;
		right: 0;
		display: flex;
		flex-direction: column;
	}

	.sheet-facts {
		padding: 6px 12px 8px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	.sheet .sheet-item {
		height: 48px;
		padding: 0 12px;
		border-radius: var(--radius-12);
		text-align: left;
	}

	@media (prefers-reduced-motion: reduce) {
		.bar {
			animation-duration: 120ms;
		}
	}
</style>
