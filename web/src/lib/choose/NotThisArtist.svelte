<!--
	Not this artist's confirm: the picker (or the menu) reshaped in place into what it takes away, at
	the button that asked. Keep it goes back; Not this artist forgets what the sources gave and holds
	the artist back until its MusicBrainz ID changes. Art from the library's own files stays.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { chooser, type ChooserOpen } from './chooser.svelte.ts';
	import { confirmWords } from './words.ts';

	interface Props {
		open: ChooserOpen;
		phone: boolean;
	}

	let { open, phone }: Props = $props();

	const words = $derived(confirmWords(open.name, open.mbid));
	const busy = $derived(chooser.busy?.what === 'reject');

	let panel: HTMLElement | undefined = $state();

	onMount(() => {
		panel?.querySelector<HTMLButtonElement>('button.keep')?.focus();
	});
</script>

<div
	bind:this={panel}
	class="confirm"
	class:phone
	role="group"
	aria-label="Not this artist"
	data-testid="not-this-artist"
>
	<span class="kicker">{words.kicker}</span>
	<p class="question">{words.title}</p>
	<p class="body">{words.body}</p>
	<div class="buttons">
		<button
			type="button"
			class="keep"
			data-menu-item
			disabled={busy}
			onclick={() => {
				chooser.keep();
			}}>Keep it</button
		>
		<button
			type="button"
			class="go"
			data-menu-item
			disabled={busy}
			onclick={() => void chooser.reject()}>{busy ? 'Holding it back…' : 'Not this artist'}</button
		>
	</div>
</div>

<style>
	.confirm {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.kicker {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.1em;
		color: var(--danger);
	}

	.question {
		font-size: 24px;
		font-weight: 800;
		letter-spacing: -0.03em;
		line-height: 1.15;
	}

	.body {
		font-size: 14px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.buttons {
		display: flex;
		gap: 12px;
		margin-top: 8px;
	}

	button {
		height: 44px;
		border-radius: var(--radius-pill);
		font: 700 14px var(--font-sans);
		cursor: pointer;
	}

	button:disabled {
		opacity: 0.5;
		cursor: progress;
	}

	.keep {
		padding: 0 22px;
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font-weight: 600;
	}

	.keep:hover:not(:disabled) {
		background: var(--hover);
	}

	.go {
		padding: 0 24px;
		border: 0;
		background: var(--danger);
		color: var(--danger-ink);
	}

	.phone .question {
		font-size: 20px;
	}

	.phone .buttons {
		flex-direction: column-reverse;
		gap: 8px;
	}

	.phone .go {
		height: 50px;
		font-size: 16px;
	}

	.phone .keep {
		height: 48px;
	}
</style>
