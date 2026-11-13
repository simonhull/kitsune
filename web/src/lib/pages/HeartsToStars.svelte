<!--
	The one-time card on the You page that turns the hearts someone kept into stars: how many there
	are, the stars their unrated songs get (the person's own most common rating, else 4★), five of
	those songs, and two ways out, both final. Either button converts; the card is gone for good after.
	Nothing it does reaches Last.fm: those songs were loved through their hearts.
-->
<script lang="ts">
	import { library } from '#lib/library/library.svelte.ts';
	import type { HeartsSummary } from '#lib/library/source.ts';
	import StarPick from '#lib/shell/StarPick.svelte';
	import { commonLine, goesToo, heartsLine, rateLabel } from './heartsCard.ts';

	interface Props {
		hearts: HeartsSummary;
		/** The stars the unrated songs would get: the page holds it, for the Last.fm line's count. */
		stars: number;
		choose: (stars: number) => void;
		/** Told once the hearts are stars, or left unrated: the card goes. */
		done: () => void;
	}

	let { hearts, stars, choose, done }: Props = $props();

	let busy = $state(false);
	let trouble = $state('');
	const unrated = $derived(hearts.tracksUnrated);
	const common = $derived(commonLine(hearts.byStars));
	const foot = $derived(goesToo(hearts));

	async function convert(given: number | null): Promise<void> {
		busy = true;
		trouble = '';
		try {
			await library.convertHearts(given);
			done();
		} catch {
			trouble = "That didn't go through, so your hearts are as they were. Try again.";
		} finally {
			busy = false;
		}
	}
</script>

<section class="change" aria-labelledby="change-head">
	<span class="kicker">YOUR HEARTS BECOME STARS</span>
	<h2 id="change-head">Stars say it better.</h2>
	<p class="lead">{heartsLine(hearts)}</p>

	{#if unrated > 0}
		<div class="give">
			<span class="give-words"
				>Give {unrated === 1 ? 'that song' : `those ${unrated.toLocaleString('en-US')} songs`}</span
			>
			<StarPick
				size="card"
				value={stars}
				label={`Stars for the ${unrated.toLocaleString('en-US')} ${unrated === 1 ? 'song' : 'songs'}`}
				onchange={choose}
			/>
			{#if common !== ''}<span class="common">{common}</span>{/if}
		</div>

		{#if hearts.preview.length > 0}
			<div class="among">
				<span class="among-head">AMONG THEM</span>
				<ul>
					{#each hearts.preview as song (song.id)}
						<li>
							<span class="title">{song.title}</span>
							<span class="artist">{song.artist ?? ''}</span>
							<span class="stars" role="img" aria-label={`${String(stars)} stars`}
								>{'★'.repeat(stars)}<span class="off">{'★'.repeat(5 - stars)}</span></span
							>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
	{/if}

	<div class="actions">
		{#if unrated > 0}
			<button type="button" class="solid" disabled={busy} onclick={() => void convert(stars)}
				>{rateLabel(unrated, stars)}</button
			>
			<button type="button" class="ghost" disabled={busy} onclick={() => void convert(null)}
				>Leave them unrated</button
			>
		{:else}
			<button type="button" class="solid" disabled={busy} onclick={() => void convert(null)}
				>Take the hearts off</button
			>
		{/if}
	</div>
	{#if foot !== ''}<p class="foot">{foot}</p>{/if}
	{#if trouble !== ''}<p class="trouble" role="status">{trouble}</p>{/if}
</section>

<style>
	.change {
		display: flex;
		flex-direction: column;
		gap: 20px;
		padding: 32px;
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid color-mix(in srgb, var(--fox-line) 45%, transparent);
	}

	.kicker,
	.among-head,
	.common {
		font-family: var(--font-mono);
		letter-spacing: 0.08em;
	}

	.kicker {
		font-size: var(--type-mono);
		color: var(--fox-text);
	}

	h2 {
		font-size: 40px;
		font-weight: 800;
		letter-spacing: -0.04em;
		line-height: 1;
	}

	.lead {
		max-width: 60ch;
		font-size: 16px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.give {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 12px 20px;
		padding: 20px;
		border-radius: var(--radius-20);
		background: var(--card-raised);
	}

	.give-words {
		font-size: 16px;
		font-weight: 600;
	}

	.common {
		font-size: 13px;
		letter-spacing: 0;
		color: var(--text-meta);
	}

	.among {
		display: flex;
		flex-direction: column;
	}

	.among-head {
		margin-bottom: 6px;
		font-size: 11px;
		color: var(--text-label);
	}

	li {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 200px) 96px;
		align-items: center;
		height: 40px;
		border-bottom: 1px solid var(--edge-card);
		font-size: 14px;
	}

	.title {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-weight: 600;
	}

	.artist {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		padding-right: 12px;
		color: var(--text-secondary);
	}

	.stars {
		font-size: 13px;
		letter-spacing: 0.2em;
		color: var(--star);
	}

	.stars .off {
		color: var(--star-off);
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 12px;
	}

	button {
		height: 48px;
		border-radius: var(--radius-pill);
		font-weight: 700;
	}

	button:disabled {
		opacity: 0.6;
		cursor: default;
	}

	.solid {
		padding: 0 24px;
		border: 0;
		background: var(--fox);
		color: var(--fox-ink);
		font-size: 15px;
	}

	.ghost {
		padding: 0 20px;
		border: 1px solid var(--edge-strong);
		background: transparent;
		color: var(--text);
		font-size: 14px;
		font-weight: 600;
	}

	.foot,
	.trouble {
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	.trouble {
		color: var(--fox-text);
	}

	/* A phone keeps every word: the preview's stars go under the names. */
	@media (max-width: 600px) {
		.change {
			padding: 24px 20px;
		}

		h2 {
			font-size: 32px;
		}

		li {
			grid-template-columns: minmax(0, 1fr) auto;
			height: auto;
			padding: 8px 0;
			row-gap: 2px;
		}

		.artist {
			grid-row: 2;
		}

		.stars {
			grid-row: 1 / span 2;
			grid-column: 2;
		}
	}
</style>
