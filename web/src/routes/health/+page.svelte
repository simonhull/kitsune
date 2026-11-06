<script lang="ts">
	import { onMount, untrack } from 'svelte';
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
</script>

<svelte:head>
	<title>Health · Kitsune</title>
</svelte:head>

{#if facts !== null}
	<div class="health">
		<ScanPanel {facts} {admin} down={library.down} {now} />
		<IssuesPanel {facts} {admin} issues={library.issues} />
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
