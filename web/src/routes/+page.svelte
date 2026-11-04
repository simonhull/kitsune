<script lang="ts">
	import { library } from '#lib/library/library.svelte.ts';
	import AlbumCard from '#lib/pages/AlbumCard.svelte';
	import ContinueTile from '#lib/pages/ContinueTile.svelte';
	import RecentCover from '#lib/pages/RecentCover.svelte';
	import Icon from '#lib/shell/Icon.svelte';

	const home = $derived(library.home);
</script>

<svelte:head>
	<title>Kitsune</title>
</svelte:head>

{#if home !== null}
	<div class="home">
		<div class="intro" data-morph-part="card">
			<h1>{home.greeting}</h1>
			<span class="summary">{home.summary}</span>
		</div>

		<section aria-label="Overview" class="overview">
			<ContinueTile listening={home.continueListening} />

			<div class="health" data-morph-part="card">
				<span class="fox-mark"><Icon name="fox" size={220} /></span>
				<span class="label">Library health</span>
				<span class="figure">
					<span class="number">{home.health.issues}</span>
					<span class="note">{home.health.detail}</span>
				</span>
			</div>

			<div class="week" data-morph-part="card">
				<span class="label">This week</span>
				<span class="figure">
					<span class="number">{home.week.listened}</span>
					<span class="note">{home.week.detail}</span>
				</span>
			</div>

			<section aria-label="Recently added" class="recent" data-morph-part="card">
				<div class="recent-head">
					<h2>Recently added</h2>
					<span class="more">See all</span>
				</div>
				<div class="recent-row">
					{#each home.recentlyAdded as album (album.id)}
						<RecentCover {album} />
					{/each}
				</div>
			</section>
		</section>

		<section aria-label="On repeat" class="repeat">
			<div class="repeat-head" data-morph-part="card">
				<h2>On repeat</h2>
				<span class="more">All albums</span>
			</div>
			<div class="grid">
				{#each home.onRepeat as album (album.id)}
					<AlbumCard {album} />
				{/each}
			</div>
		</section>
	</div>
{/if}

<style>
	.home {
		max-width: 1400px;
		margin: 0 auto;
		padding: 106px 32px 140px;
		display: flex;
		flex-direction: column;
		gap: 56px;
	}

	.intro {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 12px 24px;
	}

	h1 {
		font-size: clamp(44px, 7vw, var(--type-display));
		font-weight: 800;
		letter-spacing: var(--tracking-display);
		line-height: 0.9;
		text-wrap: balance;
	}

	.summary {
		font-family: var(--font-mono);
		font-size: 13px;
		color: var(--text-meta);
	}

	.overview {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
		grid-auto-rows: 200px;
		gap: 16px;
	}

	.health,
	.week {
		position: relative;
		overflow: hidden;
		border-radius: var(--radius-28);
		padding: 24px;
		display: flex;
		flex-direction: column;
		justify-content: space-between;
	}

	.health {
		background: var(--fox);
		color: var(--fox-ink);
	}

	.fox-mark {
		display: flex;
		position: absolute;
		right: -30px;
		bottom: -40px;
		opacity: 0.18;
	}

	.week,
	.recent {
		background: var(--card);
		border: 1px solid var(--edge-card);
	}

	.label,
	.recent h2 {
		font-size: 13px;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}

	.health .label {
		font-weight: 700;
	}

	.week .label,
	.recent h2 {
		color: var(--text-meta);
	}

	.figure {
		position: relative;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.week .figure {
		gap: 6px;
	}

	.number {
		font-size: 72px;
		font-weight: 900;
		letter-spacing: -0.05em;
		line-height: 0.85;
	}

	.week .number {
		font-size: 56px;
		font-weight: 800;
		line-height: 0.9;
	}

	.note {
		font-size: var(--type-body);
		font-weight: 600;
	}

	.week .note {
		font-weight: 400;
		color: var(--text-secondary);
	}

	.recent {
		grid-column: span 2;
		border-radius: var(--radius-28);
		padding: 24px;
		display: flex;
		flex-direction: column;
		gap: 16px;
		overflow: hidden;
	}

	.recent-head,
	.repeat-head {
		display: flex;
		justify-content: space-between;
	}

	.recent-head {
		align-items: center;
	}

	.repeat-head {
		align-items: baseline;
	}

	.more {
		font-size: 14px;
		color: var(--text-secondary);
	}

	.recent-row {
		display: flex;
		gap: 14px;
	}

	.repeat {
		display: flex;
		flex-direction: column;
		gap: 20px;
	}

	.repeat h2 {
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 24px;
	}

	@media (max-width: 720px) {
		.home {
			padding: 98px 16px 128px;
			gap: 40px;
		}

		.overview {
			grid-template-columns: minmax(0, 1fr);
			grid-auto-rows: auto;
		}

		.health,
		.week {
			min-height: 180px;
		}

		.recent {
			grid-column: auto;
		}

		.recent-row {
			overflow-x: auto;
			margin-right: -24px;
			padding-right: 24px;
			scrollbar-width: none;
		}

		.grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 20px 16px;
		}
	}
</style>
