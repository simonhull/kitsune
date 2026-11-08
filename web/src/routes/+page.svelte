<script lang="ts">
	import { resolve } from '$app/paths';
	import {
		emptyLine,
		greeting,
		healthNote,
		listeningTime,
		summary,
		trackCount
	} from '#lib/library/home.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import AlbumGrid from '#lib/pages/AlbumGrid.svelte';
	import Deck from '#lib/pages/Deck.svelte';
	import FeatureTile from '#lib/pages/FeatureTile.svelte';
	import { homeGrid } from '#lib/pages/gridWindow.svelte.ts';
	import RecentCover from '#lib/pages/RecentCover.svelte';
	import Icon from '#lib/shell/Icon.svelte';

	const facts = $derived(library.facts);
	const feature = $derived(library.recent.at(0));
	const name = $derived(library.user?.name ?? '');
	/** Under a deck that's dealing, Home is already there. */
	const showsHome = $derived(!library.deck || library.dealing !== 'no');
</script>

<svelte:head>
	<title>Kitsune</title>
</svelte:head>

{#if facts !== null}
	<div class="page-frame home">
		<div class="intro" data-morph-part="intro" data-morph-key="intro">
			<h1>{greeting(new Date().getHours(), name)}</h1>
			{#if facts.albums > 0 && (!library.deck || library.dealing === 'dealt')}
				<span class="summary" class:arriving={library.deck}>{summary(facts)}</span>
			{/if}
		</div>

		{#if library.deck}
			<Deck {facts} />
		{/if}
		{#if showsHome}
			{#if facts.albums === 0}
				<p class="empty" role="status">{emptyLine(facts, false)}</p>
			{:else}
				<section aria-label="Overview" class="overview">
					{#if feature !== undefined}<FeatureTile album={feature} />{/if}

					<a
						class="health"
						href={resolve('/health')}
						data-morph-part="card"
						data-morph-key="health"
					>
						<span class="fox-mark"><Icon name="fox" size={220} /></span>
						<span class="label">Library health</span>
						<span class="figure">
							<span class="number">{facts.issues.toLocaleString('en-US')}</span>
							<span class="note">{healthNote(facts.issues, facts.issueCounts)}</span>
						</span>
					</a>

					<div class="week" data-morph-part="card" data-morph-key="stats">
						<span class="label">Your library</span>
						<span class="figure">
							<span class="number">{listeningTime(facts.durationMs)}</span>
							<span class="note">{trackCount(facts.tracks)} of listening</span>
						</span>
					</div>

					<section
						aria-label="Recently added"
						class="recent"
						data-morph-part="card"
						data-morph-key="recent"
					>
						<h2>Recently added</h2>
						<div class="recent-row">
							{#each library.recent.slice(1) as album (album.id)}
								<RecentCover {album} />
							{/each}
						</div>
					</section>
				</section>

				<section aria-label="Your albums" class="repeat">
					<h2 data-morph-part="card" data-morph-key="albums">Your albums</h2>
					<AlbumGrid grid={homeGrid} />
				</section>
			{/if}
		{/if}
	</div>
{/if}

<style>
	.home {
		display: flex;
		flex-direction: column;
		gap: calc(56px * var(--step));
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
		font-size: calc(13px * var(--step));
		color: var(--text-meta);
	}

	/* It takes over from the deck's count, which fades as this comes up. */
	.arriving {
		animation: summary-arrives var(--morph) var(--curve-morph) 160ms backwards;
	}

	@keyframes summary-arrives {
		from {
			opacity: 0;
		}
	}

	.empty {
		max-width: 44ch;
		font-size: var(--type-title);
		line-height: 1.4;
		color: var(--text-secondary);
	}

	/*
	 * The hero row spans the frame: Just added takes two columns and two rows, and the rest share
	 * what's left, Recently added across the second row. A wider screen gives the rest more columns.
	 */
	.overview {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
		grid-auto-rows: calc(200px * var(--step));
		gap: calc(16px * var(--step));
	}

	@media (min-width: 1280px) {
		.overview {
			grid-template-columns: repeat(4, minmax(0, 1fr));
		}
	}

	.health,
	.week {
		position: relative;
		overflow: hidden;
		border-radius: var(--radius-28);
		padding: calc(24px * var(--step));
		display: flex;
		flex-direction: column;
		justify-content: space-between;
	}

	.health {
		background: var(--fox);
		color: var(--fox-ink);
	}

	.health:hover {
		background: color-mix(in srgb, var(--fox) 90%, var(--text));
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
		font-size: calc(13px * var(--step));
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
		font-size: calc(72px * var(--step));
		font-weight: 900;
		letter-spacing: -0.05em;
		line-height: 0.85;
	}

	.week .number {
		font-size: calc(56px * var(--step));
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
		padding: calc(24px * var(--step));
		display: flex;
		flex-direction: column;
		gap: calc(16px * var(--step));
		overflow: hidden;
	}

	/* As many covers as one row holds: the rest wrap out of sight below it. */
	.recent-row {
		--recent-size: calc(112px * var(--step));

		display: grid;
		grid-template-columns: repeat(auto-fill, var(--recent-size));
		grid-template-rows: var(--recent-size);
		grid-auto-rows: var(--recent-size);
		gap: var(--recent-size) calc(14px * var(--step));
		height: var(--recent-size);
		overflow: hidden;
	}

	.repeat {
		display: flex;
		flex-direction: column;
		gap: calc(20px * var(--step));
	}

	.repeat h2 {
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	/* After the cards' own rules, which it overrides. */
	@media (min-width: 2560px) {
		.overview {
			grid-template-columns: repeat(6, minmax(0, 1fr));
		}

		.health,
		.week {
			grid-column: span 2;
		}

		.recent {
			grid-column: 3 / -1;
		}
	}

	@media (max-width: 720px) {
		.home {
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
			display: flex;
			gap: 14px;
			height: auto;
			overflow-x: auto;
			margin-right: -24px;
			padding-right: 24px;
			scrollbar-width: none;
		}
	}
</style>
