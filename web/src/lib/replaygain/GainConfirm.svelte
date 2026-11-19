<!--
	Write ReplayGain's confirm: the album ⋯'s own box reshaped in place into what it writes (M4), never
	a second surface or a modal. On a phone it is the menu at the phone's whole width, never a sheet.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { confirmWords } from './words.ts';

	interface Props {
		/** The songs it writes, their formats, and the album's pooled measure. */
		count: number;
		formats: readonly string[];
		albumGain: number | undefined;
		albumPeak: number | undefined;
		phone: boolean;
		busy: boolean;
		onwrite: () => void;
		oncancel: (refocus: boolean) => void;
	}

	let { count, formats, albumGain, albumPeak, phone, busy, onwrite, oncancel }: Props = $props();

	const words = $derived(confirmWords(count, formats, albumGain, albumPeak));

	let panel: HTMLElement | undefined = $state();

	onMount(() => {
		panel?.querySelector<HTMLButtonElement>('button.fox')?.focus();
	});
</script>

<div
	bind:this={panel}
	class="confirm"
	class:phone
	role="group"
	aria-label="Write ReplayGain"
	data-testid="gain-confirm"
>
	<span class="kicker">{words.kicker}</span>
	<p class="question">{words.title}</p>
	<dl class="facts">
		{#if words.album !== ''}
			<div class="fact">
				<dt>ALBUM</dt>
				<dd>{words.album}</dd>
			</div>
		{/if}
		<div class="fact">
			<dt>EACH SONG</dt>
			<dd>ITS OWN TRACK GAIN AND PEAK</dd>
		</div>
		<div class="fact">
			<dt>LEVEL</dt>
			<dd>−18 LUFS · REPLAYGAIN 2.0</dd>
		</div>
	</dl>
	<p class="body">Nothing you hear in Kitsune changes. Every other player levels them too.</p>
	<div class="buttons">
		<button
			type="button"
			class="outline"
			data-menu-item
			disabled={busy}
			onclick={() => {
				oncancel(true);
			}}>Leave them</button
		>
		<button type="button" class="fox" data-menu-item disabled={busy} onclick={onwrite}
			>{words.button}</button
		>
	</div>
	<span class="note">UNDO FROM THE TOAST, OR FROM HEALTH’S RUNS</span>
</div>

<style>
	.confirm {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.kicker,
	.note,
	dt,
	dd {
		font-family: var(--font-mono);
		letter-spacing: 0.06em;
	}

	.kicker {
		font-size: 11px;
		letter-spacing: 0.1em;
		color: var(--fox-text);
	}

	.question {
		margin: 0;
		font-size: 22px;
		font-weight: 800;
		letter-spacing: -0.03em;
		line-height: 1.15;
	}

	.facts {
		margin: 0;
		padding: 4px 14px;
		border-radius: var(--radius-12);
		background: var(--card-raised);
	}

	.fact {
		display: grid;
		grid-template-columns: 80px minmax(0, 1fr);
		gap: 10px;
		padding: 8px 0;
		font-size: 11.5px;
	}

	.fact + .fact {
		border-top: 1px solid var(--edge);
	}

	dt {
		color: var(--text-meta);
	}

	dd {
		margin: 0;
	}

	.body {
		margin: 0;
		font-size: 14px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.buttons {
		display: flex;
		gap: 10px;
	}

	button {
		cursor: pointer;
	}

	button:disabled {
		opacity: 0.5;
		cursor: progress;
	}

	.outline,
	.fox {
		height: 44px;
		border-radius: var(--radius-pill);
		font: 700 14px var(--font-sans);
	}

	.outline {
		padding: 0 18px;
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font-weight: 600;
	}

	.outline:hover:not(:disabled) {
		background: var(--hover);
	}

	.fox {
		padding: 0 22px;
		border: 0;
		background: var(--fox);
		color: var(--fox-ink);
	}

	.note {
		font-size: 10px;
		color: var(--text-meta);
	}

	.phone .question {
		font-size: 19px;
	}

	.phone .buttons {
		flex-direction: column-reverse;
		gap: 8px;
	}

	.phone .fox {
		height: 50px;
		font-size: 16px;
	}

	.phone .outline {
		height: 48px;
	}

	.phone .note {
		text-align: center;
	}
</style>
