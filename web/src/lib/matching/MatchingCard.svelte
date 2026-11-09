<!--
	The Health page's card for albums without MusicBrainz IDs, board C. Missing IDs aren't a fault,
	so nothing here raises Home's Health number. It opens the queue.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { matchQueue } from './api.ts';
	import type { MatchQueue } from './types.ts';

	let queue = $state.raw<MatchQueue | null>(null);

	onMount(() => {
		matchQueue()
			.then((q) => {
				queue = q;
			})
			.catch(() => undefined); // The card stays away: nothing waits on it.
	});

	const todo = $derived(queue?.counts.todo ?? 0);
</script>

{#if queue !== null && (todo > 0 || queue.counts.matched > 0)}
	<section class="card" class:quiet={todo === 0} aria-labelledby="matching-card">
		<span class="eyebrow">MUSICBRAINZ IDS</span>
		<span class="number">{todo}</span>
		<span class="what" id="matching-card"
			>{todo === 1 ? 'album has' : 'albums have'} no MusicBrainz IDs</span
		>
		<span class="why"
			>Matching finds each one’s release and writes its IDs, so later matches are exact and
			ListenBrainz and Last.fm know the songs.</span
		>
		<span class="act">
			{#if todo > 0}<a class="fox" href={resolve('/health/matching')}>Work through them</a>{/if}
			<span class="mono">{queue.counts.matched} MATCHED SO FAR</span>
		</span>
	</section>
{/if}

<style>
	.card {
		min-height: 400px;
		box-sizing: border-box;
		padding: 28px;
		display: flex;
		flex-direction: column;
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid var(--fox-line);
	}

	.card.quiet {
		border-color: var(--edge);
	}

	.eyebrow,
	.mono {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.number {
		margin-top: 18px;
		font-size: 72px;
		font-weight: 900;
		letter-spacing: -0.05em;
		line-height: 0.85;
	}

	.what {
		margin-top: 14px;
		font-size: 17px;
		font-weight: 700;
	}

	.why {
		margin-top: 8px;
		font-size: 14px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.act {
		margin-top: auto;
		padding-top: 24px;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 14px;
	}

	.fox {
		height: 48px;
		padding: 0 22px;
		display: inline-flex;
		align-items: center;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font-size: 14px;
		font-weight: 700;
		text-decoration: none;
	}

	@media (max-width: 720px) {
		.card {
			min-height: 0;
			padding: 24px;
			border-radius: var(--radius-24);
		}

		.number {
			font-size: 56px;
		}
	}
</style>
