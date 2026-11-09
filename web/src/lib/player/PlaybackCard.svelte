<!-- The You page's Playback card: how loud a song plays before it has ReplayGain. -->
<script lang="ts">
	import { gain } from '#lib/library/format.ts';
	import { playbackSettings, untaggedGainRange } from './playback.svelte.ts';

	const id = $props.id();
	const level = $derived(playbackSettings.untaggedGain);
</script>

<section aria-label="Playback" class="card">
	<h3>Playback</h3>

	<div class="setting">
		<div class="top">
			<label for={`${id}-untagged`}>Songs not levelled yet</label>
			<span class="value">{level === 0 ? 'AS RECORDED' : `${gain(level)} dB`}</span>
		</div>
		<input
			id={`${id}-untagged`}
			type="range"
			min={untaggedGainRange.min}
			max={untaggedGainRange.max}
			step="1"
			value={level}
			aria-describedby={`${id}-untagged-note`}
			aria-valuetext={level === 0 ? 'as recorded' : `${String(level)} decibels`}
			oninput={(event) => {
				playbackSettings.setUntaggedGain(Number(event.currentTarget.value));
			}}
		/>
		<div class="ends" aria-hidden="true"><span>QUIETER</span><span>AS RECORDED</span></div>
		<p class="note" id={`${id}-untagged-note`}>
			Kitsune levels every song so one doesn't jump out over the next, from its ReplayGain or by
			measuring it in the background. Until a song is levelled, it plays this much quieter.
		</p>
	</div>
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
		font-variant-numeric: tabular-nums;
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
