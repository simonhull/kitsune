<!--
	Write ReplayGain's confirm (board D): the album ⋯ grown in place into what it writes, never a
	modal; on a phone, a sheet from the bottom with a fox top edge, as Duplicates' confirm is.
-->
<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { confirmWords } from './words.ts';

	interface Props {
		/** Where the menu it grows from opened. */
		x: number;
		y: number;
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

	let { x, y, count, formats, albumGain, albumPeak, phone, busy, onwrite, oncancel }: Props =
		$props();

	/** As wide as the board's, and clear of the screen's edges by this much. */
	const width = 420;
	const margin = 12;

	const words = $derived(confirmWords(count, formats, albumGain, albumPeak));

	let panel: HTMLElement | undefined = $state();
	let left = $state(0);
	let top = $state(0);
	let placed = $state(false);

	onMount(() => {
		if (!phone && panel !== undefined) {
			const { height } = panel.getBoundingClientRect();
			left = Math.max(margin, Math.min(x, innerWidth - width - margin));
			top = Math.max(margin, Math.min(y, innerHeight - height - margin));
		}
		placed = true;
		void tick().then(() => panel?.querySelector<HTMLButtonElement>('button.fox')?.focus());
		function outside(event: PointerEvent): void {
			if (
				!phone &&
				panel !== undefined &&
				event.target instanceof Node &&
				!panel.contains(event.target)
			) {
				oncancel(false);
			}
		}
		addEventListener('pointerdown', outside, true);
		return () => {
			removeEventListener('pointerdown', outside, true);
		};
	});

	function keydown(event: KeyboardEvent): void {
		if (event.key === 'Escape' && !busy) {
			event.preventDefault();
			event.stopPropagation();
			oncancel(true);
		}
	}
</script>

{#if phone}
	<button
		type="button"
		class="scrim"
		aria-label="Leave them"
		tabindex="-1"
		onclick={() => {
			oncancel(false);
		}}
	></button>
{/if}
<div
	bind:this={panel}
	class="confirm"
	class:sheet={phone}
	class:placed
	role="dialog"
	aria-modal={phone ? 'true' : undefined}
	aria-label="Write ReplayGain"
	tabindex="-1"
	style:left={phone ? undefined : `${String(left)}px`}
	style:top={phone ? undefined : `${String(top)}px`}
	onkeydown={keydown}
	data-testid="gain-confirm"
>
	{#if phone}<span class="grip" aria-hidden="true"></span>{/if}
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
			disabled={busy}
			onclick={() => {
				oncancel(true);
			}}>Leave them</button
		>
		<button type="button" class="fox" disabled={busy} onclick={onwrite}>{words.button}</button>
	</div>
	<span class="note">UNDO FROM THE TOAST, OR FROM HEALTH’S RUNS</span>
</div>

<style>
	.confirm {
		position: fixed;
		/* Over the player island, as the menu it grows from is. */
		z-index: 6;
		width: 420px;
		max-height: calc(100dvh - 24px);
		overflow-y: auto;
		box-sizing: border-box;
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 20px 22px;
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1.5px solid var(--fox-line);
		box-shadow:
			3px 3px 0 color-mix(in srgb, var(--accent, var(--fox)) 50%, transparent),
			var(--shadow-float);
		outline: none;
		visibility: hidden;
		opacity: 0;
		transform: scale(0.96);
		transform-origin: top left;
	}

	.confirm.placed {
		visibility: visible;
		opacity: 1;
		transform: none;
		transition:
			opacity 140ms var(--curve-morph),
			transform 200ms var(--curve-settle);
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

	.scrim {
		position: fixed;
		inset: 0;
		z-index: 40;
		border: 0;
		background: color-mix(in srgb, var(--ground) 60%, transparent);
	}

	.sheet {
		left: 0;
		right: 0;
		bottom: 0;
		z-index: 41;
		width: auto;
		max-height: 85dvh;
		padding: 10px 18px calc(26px + env(safe-area-inset-bottom));
		border-radius: 28px 28px 0 0;
		border: 0;
		border-top: 1.5px solid var(--fox-line);
		background: var(--card-raised);
		box-shadow: none;
		transform: translateY(24px);
		transform-origin: bottom center;
	}

	.sheet .facts {
		background: var(--card);
	}

	.grip {
		align-self: center;
		width: 40px;
		height: 4px;
		border-radius: 2px;
		background: var(--edge-strong);
	}

	.sheet .question {
		font-size: 19px;
	}

	.sheet .buttons {
		flex-direction: column-reverse;
		gap: 8px;
	}

	.sheet .fox {
		height: 50px;
		font-size: 16px;
	}

	.sheet .outline {
		height: 48px;
	}

	.sheet .note {
		text-align: center;
	}

	@media (prefers-reduced-motion: reduce) {
		.confirm.placed {
			transition: opacity 120ms linear;
		}
	}
</style>
