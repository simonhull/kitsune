<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { staleFor } from '#lib/library/health.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import MatchingCard from '#lib/matching/MatchingCard.svelte';
	import ReplayGainCard from '#lib/replaygain/ReplayGainCard.svelte';
	import MetadataCard from '#lib/metadata/MetadataCard.svelte';
	import IssuesPanel from '#lib/pages/IssuesPanel.svelte';
	import ScanPanel from '#lib/pages/ScanPanel.svelte';

	/** How often the words that depend on the clock are worked out again. */
	const tickMs = 1000;

	const facts = $derived(library.facts);
	const admin = $derived(library.admin);
	const lastEnded = $derived(library.facts?.scan.last?.endedAt);
	let now = $state(Date.now());

	onMount(() => {
		// Every visit asks: a scan may have begun with nobody polling.
		void library.refreshFacts();
		const clock = setInterval(() => {
			now = Date.now();
		}, tickMs);
		return () => {
			clearInterval(clock);
		};
	});

	// Every visit asks too, and so does the end of every scan: that's what changes the list.
	$effect(() => {
		void lastEnded;
		if (admin) {
			untrack(() => void library.loadIssues());
		}
	});

	// A scan finds files between those asks: a list behind the counts asks again, once for each
	// count. Still behind after that answer, it gives up and offers Try again.
	const stale = $derived.by(() => {
		const issues = library.issues;
		return admin && facts !== null && issues.kind === 'ready'
			? staleFor(facts.issueCounts, issues.value.issues)
			: null;
	});
	let askedFor: string | null = null;
	let answeredFor = $state<string | null>(null);
	const gaveUp = $derived(stale !== null && stale === answeredFor);

	async function askAgain(key: string): Promise<void> {
		askedFor = key;
		answeredFor = null;
		await library.loadIssues();
		answeredFor = key;
	}

	function retry(): void {
		void (stale === null ? library.loadIssues() : askAgain(stale));
	}

	$effect(() => {
		if (stale !== null && stale !== askedFor) {
			const key = stale;
			untrack(() => void askAgain(key));
		}
	});
</script>

<svelte:head>
	<title>Health · Kitsune</title>
</svelte:head>

{#if facts !== null}
	<div class="page-frame health">
		<ScanPanel {facts} {admin} down={library.down} {now} />
		<IssuesPanel {facts} {admin} issues={library.issues} {gaveUp} {retry} />
		{#if admin}
			<div class="replaygain"><ReplayGainCard /></div>
			<div class="matching"><MatchingCard /></div>
			<div class="metadata"><MetadataCard /></div>
		{/if}
	</div>
{/if}

<style>
	.health {
		--frame-top: 96px;
		--panel-height: clamp(
			calc(560px * var(--step)),
			calc(100dvh - 200px * var(--step)),
			max(calc(700px * var(--step)), 65dvh)
		);

		display: grid;
		grid-template-columns: minmax(0, 552fr) minmax(0, 800fr);
		align-items: start;
		gap: calc(24px * var(--step));
	}

	/* Under the panels: MusicBrainz as wide as the scan, ReplayGain as wide as the issues. */
	.matching {
		grid-row: 2;
		grid-column: 1;
	}

	.replaygain {
		grid-row: 2;
		grid-column: 2;
	}

	/* Artist art and bios in the second column, under ReplayGain, or in its place when it isn't drawn. */
	.metadata {
		grid-column: 2;
	}

	/* An empty slot takes no room: no card is drawn on every library. */
	.matching:empty,
	.replaygain:empty,
	.metadata:empty {
		display: none;
	}

	@media (max-width: 900px) {
		.health {
			--panel-height: auto;

			grid-template-columns: minmax(0, 1fr);
			gap: 16px;
		}

		.matching,
		.replaygain,
		.metadata {
			grid-row: auto;
			grid-column: auto;
		}
	}

	@media (max-width: 720px) {
		.health {
			--frame-top: 98px;
		}
	}
</style>
