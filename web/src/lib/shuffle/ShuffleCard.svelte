<!-- The You page's Shuffle card, as the Settings board has it: how ratings count, flow, and skips. -->
<script lang="ts">
	import { onMount } from 'svelte';
	import Switch from '#lib/pages/Switch.svelte';
	import { shuffleSettings } from './settings.svelte.ts';
	import { flowWord, ratingsSentence, ratingsWord } from './words.ts';

	const settings = $derived(shuffleSettings.current);
	const ratingsId = $props.id();

	onMount(() => {
		void shuffleSettings.load();
	});

	/** A slider's place, 0 to 100, as the setting's 0 to 1. */
	function amount(event: Event & { currentTarget: HTMLInputElement }): number {
		return Number(event.currentTarget.value) / 100;
	}
</script>

<section aria-label="Shuffle" class="card">
	<h3>Shuffle</h3>

	<div class="setting">
		<div class="top">
			<label for={`${ratingsId}-ratings`}>How much ratings matter</label>
			<span class="value">{ratingsWord(settings.ratings)}</span>
		</div>
		<input
			id={`${ratingsId}-ratings`}
			type="range"
			min="0"
			max="100"
			step="5"
			value={Math.round(settings.ratings * 100)}
			aria-describedby={`${ratingsId}-ratings-note`}
			aria-valuetext={ratingsWord(settings.ratings).toLowerCase()}
			oninput={(event) => {
				shuffleSettings.set('ratings', amount(event));
			}}
		/>
		<div class="ends" aria-hidden="true"><span>NOT AT ALL</span><span>STRONGLY</span></div>
		<p class="note" id={`${ratingsId}-ratings-note`} aria-live="polite">
			{ratingsSentence(settings.ratings)}
		</p>
	</div>

	<div class="setting">
		<div class="top">
			<label for={`${ratingsId}-flow`}>Flow</label>
			<span class="value">{flowWord(settings.flow)}</span>
		</div>
		<input
			id={`${ratingsId}-flow`}
			type="range"
			min="0"
			max="100"
			step="5"
			value={Math.round(settings.flow * 100)}
			aria-describedby={`${ratingsId}-flow-note`}
			aria-valuetext={flowWord(settings.flow).toLowerCase()}
			oninput={(event) => {
				shuffleSettings.set('flow', amount(event));
			}}
		/>
		<div class="ends" aria-hidden="true">
			<span>ANYTHING NEXT</span><span>CLOSE NEIGHBOURS</span>
		</div>
		<p class="note" id={`${ratingsId}-flow-note`}>
			Leans the next song toward one that suits the last. Never the same artist twice in a row.
		</p>
	</div>

	<Switch
		label="Learn from what I skip and how much I hear"
		note="A song you skip comes up less for a while, and less again if you keep skipping it. Stopping playback never counts as a skip."
		on={settings.memory}
		toggle={(on) => {
			shuffleSettings.set('memory', on);
		}}
	/>
</section>

<style>
	.card {
		margin-top: 16px;
		padding: 32px;
		display: flex;
		flex-direction: column;
		gap: 28px;
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid var(--edge-card);
	}

	/* The You page's other card heading, Theme, sets the size. */
	h3 {
		margin: 0;
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
		line-height: 1;
	}

	.setting {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.top {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 12px;
	}

	label {
		font-size: 15px;
		font-weight: 600;
	}

	.value,
	.ends {
		font-family: var(--font-mono);
		letter-spacing: 0.06em;
	}

	.value {
		font-size: 12px;
		color: var(--fox-text);
	}

	input[type='range'] {
		width: 100%;
		height: var(--target);
		margin: -10px 0;
		accent-color: var(--fox);
	}

	.ends {
		display: flex;
		justify-content: space-between;
		font-size: 11px;
		color: var(--text-meta);
	}

	.note {
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	@media (max-width: 720px) {
		.card {
			padding: 24px 20px;
			gap: 24px;
		}
	}
</style>
