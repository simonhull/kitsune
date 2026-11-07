<script lang="ts">
	import { clock } from '#lib/library/format.ts';
	import Icon, { heartPath } from '#lib/shell/Icon.svelte';
	import type { SongActions } from './songActions.ts';

	interface Props {
		count: number;
		/** Picked, and hidden by the filter. */
		hidden: number;
		albums: number;
		durationMs: number;
		/** Whether every one is hearted, so the heart says it takes them off. */
		hearted: boolean;
		actions: SongActions;
		/** A phone: Play and Play next stay, and the rest go in More. */
		compact: boolean;
		onclear: () => void;
	}

	let { count, hidden, albums, durationMs, hearted, actions, compact, onclear }: Props = $props();

	let rating = $state(false);
	let more = $state(false);

	const label = $derived(
		`${count.toLocaleString('en-US')} selected${hidden > 0 ? `, ${hidden.toLocaleString('en-US')} hidden` : ''}`
	);
	const facts = $derived(
		`${albums.toLocaleString('en-US')} ${albums === 1 ? 'ALBUM' : 'ALBUMS'} · ${clock(durationMs / 1000)}`
	);

	function rate(stars: number | null): void {
		actions.rate(stars);
		rating = false;
		more = false;
	}
</script>

{#snippet heartButton()}
	<button
		type="button"
		class="icon"
		class:on={hearted}
		aria-label={hearted ? `Take the heart off all ${String(count)}` : `Heart all ${String(count)}`}
		aria-pressed={hearted}
		onclick={() => {
			actions.heart();
		}}
		><svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"
			><path d={heartPath}></path></svg
		></button
	>
{/snippet}

{#snippet stars()}
	<div class="stars" role="group" aria-label={`Rate all ${String(count)}`}>
		{#each [1, 2, 3, 4, 5] as star (star)}
			<button
				type="button"
				aria-label={`${String(star)} of 5`}
				onclick={() => {
					rate(star);
				}}>★</button
			>
		{/each}
		<button
			type="button"
			class="clear-stars"
			onclick={() => {
				rate(null);
			}}>Clear</button
		>
	</div>
{/snippet}

<div class="bar" class:compact role="toolbar" aria-label={`${label}. Esc clears.`}>
	<span class="count">{label}</span>
	{#if !compact}<span class="facts">{facts}</span>{/if}
	<button
		type="button"
		class="primary"
		onclick={() => {
			actions.play();
		}}><Icon name="play" size={14} />Play</button
	>
	<button
		type="button"
		class="text"
		onclick={() => {
			actions.playNext();
		}}>Play next</button
	>
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
		{@render heartButton()}
		<button
			type="button"
			class="text"
			aria-expanded={rating}
			onclick={() => {
				rating = !rating;
			}}>Rate</button
		>
		{#if actions.editTags !== undefined}
			<button type="button" class="text" onclick={actions.editTags}>Edit tags</button>
		{/if}
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
					actions.addToQueue();
					more = false;
				}}>Add to queue</button
			>
			<button
				type="button"
				role="menuitem"
				class="sheet-item"
				onclick={() => {
					actions.heart();
					more = false;
				}}>{hearted ? 'Take the hearts off' : 'Heart all'}</button
			>
			{#if actions.editTags !== undefined}
				<button type="button" role="menuitem" class="sheet-item" onclick={actions.editTags}
					>Edit tags</button
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
		border: 1px solid rgb(242 140 40 / 50%);
		box-shadow: 0 24px 64px rgb(0 0 0 / 60%);
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

	.text:hover,
	.icon:hover {
		background: rgb(255 255 255 / 8%);
	}

	.icon {
		width: 48px;
		display: grid;
		place-items: center;
	}

	.icon path {
		fill: none;
		stroke: var(--text);
		stroke-width: 2;
	}

	.icon.on path {
		fill: var(--fox);
		stroke: var(--fox);
	}

	.clear {
		margin-left: auto;
		width: 48px;
		display: grid;
		place-items: center;
		background: rgb(255 255 255 / 8%);
	}

	.pop {
		position: absolute;
		right: 120px;
		bottom: calc(100% + 10px);
		padding: 8px;
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid rgb(255 255 255 / 10%);
		box-shadow: 0 24px 64px rgb(0 0 0 / 60%);
	}

	.stars {
		display: flex;
		align-items: center;
	}

	.stars button {
		width: 40px;
		height: 44px;
		font-size: 20px;
		color: rgb(255 255 255 / 28%);
	}

	.stars button:hover,
	.stars button:has(~ button:not(.clear-stars):hover) {
		color: var(--fox);
	}

	.stars .clear-stars {
		width: auto;
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
