<script lang="ts">
	import type { Snippet } from 'svelte';

	/** One section of the Server page: its title, its fields, and Undo and Save once changed. */
	interface Props {
		id: string;
		title: string;
		changed: boolean;
		saving: boolean;
		/** What went wrong with the last save, or what would. */
		problem: string;
		/** Just saved: says so for a moment. */
		saved: boolean;
		onundo: () => void;
		onsave: () => void;
		/** Beside the title, such as Scan now. */
		action?: Snippet;
		children: Snippet;
	}

	let { id, title, changed, saving, problem, saved, onundo, onsave, action, children }: Props =
		$props();
</script>

<section
	{id}
	class="section"
	class:changed
	aria-labelledby={`${id}-title`}
	data-morph-part="panel"
	data-morph-key={id}
>
	<div class="head">
		<h2 id={`${id}-title`}>{title}</h2>
		{#if changed}
			<span class="mono state unsaved">UNSAVED</span>
		{:else if saved}
			<span class="mono state" role="status">SAVED</span>
		{:else if action !== undefined}
			{@render action()}
		{/if}
	</div>
	{@render children()}
	{#if problem !== ''}
		<p class="problem" role="alert">{problem}</p>
	{/if}
	{#if changed}
		<div class="buttons">
			<button type="button" class="undo" disabled={saving} onclick={onundo}>Undo</button>
			<button type="button" class="save" disabled={saving} onclick={onsave}>
				{saving ? 'Saving…' : 'Save'}
			</button>
		</div>
	{/if}
</section>

<style>
	.section {
		box-sizing: border-box;
		min-width: 0;
		padding: var(--section-pad);
		border-radius: var(--section-radius);
		background: var(--card);
		border: 1px solid var(--edge-card);
		display: flex;
		flex-direction: column;
		gap: var(--section-gap);
		scroll-margin-top: 120px;
		transition: border-color var(--morph-fast) var(--curve-morph);
	}

	.section.changed {
		border-color: color-mix(in srgb, var(--fox) 45%, transparent);
	}

	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		min-height: var(--target);
	}

	h2 {
		font-size: var(--section-title);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
		line-height: 1;
	}

	.mono {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
	}

	.state {
		color: var(--text-meta);
	}

	.unsaved {
		color: var(--fox-text);
	}

	.problem {
		font-size: 13px;
		line-height: 1.5;
		color: var(--problem);
	}

	.buttons {
		display: flex;
		justify-content: flex-end;
		gap: 10px;
		animation: rise var(--morph-fast) var(--curve-morph) both;
	}

	button {
		height: 48px;
		border: 0;
		border-radius: var(--radius-pill);
		font: inherit;
		font-size: 14px;
	}

	button:disabled {
		opacity: 0.6;
	}

	.undo {
		padding: 0 20px;
		background: transparent;
		color: var(--text-secondary);
		font-weight: 600;
	}

	.undo:hover:not(:disabled) {
		color: var(--text);
	}

	.save {
		padding: 0 24px;
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	@keyframes rise {
		from {
			opacity: 0;
			transform: translateY(6px);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.buttons {
			animation: none;
		}
	}
</style>
