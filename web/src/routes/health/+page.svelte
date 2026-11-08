<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { staleFor } from '#lib/library/health.ts';
	import { library } from '#lib/library/library.svelte.ts';
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
	<div class="health">
		<ScanPanel {facts} {admin} down={library.down} {now} />
		<IssuesPanel {facts} {admin} issues={library.issues} {gaveUp} {retry} />
	</div>
{/if}

<style>
	.health {
		--panel-height: clamp(560px, calc(100dvh - 200px), 700px);

		max-width: 1440px;
		margin: 0 auto;
		padding: 96px 32px 140px;
		display: grid;
		grid-template-columns: minmax(0, 552fr) minmax(0, 800fr);
		align-items: start;
		gap: 24px;
	}

	@media (max-width: 900px) {
		.health {
			--panel-height: auto;

			grid-template-columns: minmax(0, 1fr);
			gap: 16px;
		}
	}

	@media (max-width: 720px) {
		.health {
			padding: 98px 16px 128px;
		}
	}
</style>
