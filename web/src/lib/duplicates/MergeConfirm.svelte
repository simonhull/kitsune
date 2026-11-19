<!--
	The merge confirm (board I): what moves to the keeper and what doesn't, with Merge and trash and
	Keep both. It opens in place of the actions, on a phone too, budding from Keep…; there is no scrim.
	There is no Undo after, so this is the warning.
-->
<script lang="ts">
	import { on } from 'svelte/events';
	import { reshapeOn } from '#lib/morph/reshapeOn.svelte.ts';
	import { SurfaceSlot } from '#lib/morph/surface.ts';

	interface Props {
		question: string;
		moves: readonly string[];
		stays: readonly string[];
		/** Said above the lines when the library changed since the plan was shown. */
		changed: boolean;
		problem: string;
		busy: boolean;
		phone: boolean;
		onmerge: () => void;
		oncancel: () => void;
	}

	let { question, moves, stays, changed, problem, busy, phone, onmerge, oncancel }: Props =
		$props();

	let sheet: HTMLElement | undefined = $state();

	// Buds from Keep… in place of the actions, on every screen; on a 409 it becomes the new plan in
	// place; it gives way where it stands, and the actions are laid again in front of it.
	const surface = new SurfaceSlot();
	reshapeOn(
		surface,
		() => 1,
		() => JSON.stringify([moves, stays, changed])
	);

	$effect(() => {
		// On a phone it takes the focus, so a keyboard lands in it.
		if (phone) {
			sheet?.querySelector<HTMLButtonElement>('button')?.focus();
		}
	});

	function keydown(event: KeyboardEvent): void {
		if (event.key === 'Escape' && !busy) {
			event.preventDefault();
			oncancel();
		}
	}
</script>

<div
	bind:this={sheet}
	class="confirm"
	class:phone
	role="region"
	{@attach surface.attach}
	aria-label="Merge"
	tabindex="-1"
	{@attach (element) => on(element, 'keydown', keydown)}
	data-testid="confirm"
>
	<p class="question">{question}</p>
	{#if changed}
		<p class="changed mono" role="status">THE LIBRARY CHANGED SINCE · THIS IS WHAT MOVES NOW</p>
	{/if}
	<div class="columns">
		<div class="column">
			<span class="label fox mono">{phone ? 'WHAT MOVES' : 'WHAT MOVES TO THE KEEPER'}</span>
			{#each moves as line (line)}
				<span class="line"
					><span class="mark fox" aria-hidden="true">→</span><span>{line}</span></span
				>
			{/each}
		</div>
		<div class="column">
			<span class="label mono">WHAT DOESN’T</span>
			{#each stays as line (line)}
				<span class="line quiet"
					><span class="mark" aria-hidden="true">·</span><span>{line}</span></span
				>
			{/each}
		</div>
	</div>
	{#if problem !== ''}
		<p class="problem" role="alert">{problem}</p>
	{/if}
	<div class="buttons">
		<button type="button" class="fox-button" disabled={busy} onclick={onmerge}
			>Merge and trash</button
		>
		<button type="button" class="outline" disabled={busy} onclick={oncancel}>Keep both</button>
		{#if !phone}
			<span class="note mono">NO UNDO: A MERGE IS UNDONE BY HAND, SO THIS IS THE WARNING</span>
		{/if}
	</div>
</div>

<style>
	.mono {
		font-family: var(--font-mono);
	}

	.confirm {
		display: flex;
		flex-direction: column;
		gap: calc(14px * var(--step));
		padding: calc(20px * var(--step)) calc(24px * var(--step));
		border-radius: var(--radius-20);
		border: 1.5px solid var(--fox-line);
		background: color-mix(in srgb, var(--fox) 8%, var(--ground));
		outline: none;
	}

	.question {
		margin: 0;
		font-size: calc(20px * var(--step));
		font-weight: 800;
		letter-spacing: -0.02em;
		line-height: 1.2;
	}

	.changed {
		margin: 0;
		font-size: calc(11px * var(--step));
		letter-spacing: 0.06em;
		color: var(--fox-text);
	}

	.columns {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: calc(24px * var(--step));
	}

	.column {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.label {
		font-size: calc(11px * var(--step));
		letter-spacing: 0.1em;
		color: var(--text-meta);
	}

	.label.fox,
	.mark.fox {
		color: var(--fox-text);
	}

	.line {
		display: flex;
		gap: 10px;
		font-size: calc(14px * var(--step));
		line-height: 1.45;
	}

	.line.quiet {
		color: var(--text-secondary);
	}

	.problem {
		margin: 0;
		color: var(--danger-ink);
	}

	.buttons {
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.fox-button,
	.outline {
		height: calc(46px * var(--step));
		padding: 0 calc(22px * var(--step));
		border-radius: var(--radius-pill);
		font: 700 calc(15px * var(--step)) var(--font-sans);
		cursor: pointer;
	}

	.fox-button {
		border: 0;
		background: var(--fox);
		color: var(--fox-ink);
	}

	.outline {
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font-weight: 600;
	}

	.outline:hover {
		background: var(--hover);
	}

	button:disabled {
		opacity: 0.5;
		cursor: progress;
	}

	.note {
		margin-left: 8px;
		font-size: calc(11px * var(--step));
		letter-spacing: 0.04em;
		color: var(--text-meta);
	}

	.phone .question {
		font-size: 19px;
	}

	.phone .columns {
		grid-template-columns: minmax(0, 1fr);
		gap: 12px;
	}

	.phone .buttons {
		flex-direction: column;
		align-items: stretch;
		gap: 8px;
	}

	.phone .fox-button {
		height: 50px;
		font-size: 16px;
	}

	.phone .outline {
		height: 48px;
	}
</style>
