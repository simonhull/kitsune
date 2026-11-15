<!--
	New smart playlist: the rules editor on a fresh draft, "New smart playlist" with one rule, Rating
	is at least ★★★★. Nothing is made until Save rules, which lands on the new playlist's page; Leave
	it, Esc and Back go back to where it was opened from, with nothing made.
-->
<script lang="ts">
	import { afterNavigate, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import { newSmartName } from '#lib/library/smartlist.ts';
	import SmartEditor from '#lib/pages/SmartEditor.svelte';
	import { LivePreview, newSeed, SmartDraft } from '#lib/pages/smartRules.svelte.ts';

	const draft = SmartDraft.fresh();
	const seed = newSeed();
	const preview = new LivePreview((request, signal) => library.playlists.preview(request, signal));
	let saving = $state(false);
	let failed = $state(false);
	/** Whether this page was reached from another of the app's, which Leave it goes back to. */
	let cameFrom = false;

	afterNavigate(({ from }) => {
		cameFrom = from !== null;
	});

	const playlists = $derived(
		library.playlists.all
			.filter(({ id }) => !id.startsWith('pending:'))
			.map(({ id, name }) => ({ id, name }))
	);

	$effect(() => {
		void library.playlists.load();
	});

	function leave(): void {
		if (cameFrom) {
			history.back();
		} else {
			void goto(resolve('/playlists'), { replaceState: true });
		}
	}

	async function save(): Promise<void> {
		saving = true;
		failed = false;
		try {
			const made = await library.playlists.createSmart(
				draft.name.trim() === '' ? newSmartName : draft.name.trim(),
				draft.rules,
				seed
			);
			await goto(resolve('/playlists/[id]', { id: made.id }), { replaceState: true });
		} catch {
			failed = true;
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>New smart playlist · Kitsune</title>
</svelte:head>

<div class="page-frame smart">
	<SmartEditor
		{draft}
		{preview}
		requestOf={(rules) => ({ rules, seed })}
		mosaic={[]}
		savedCount={undefined}
		savedIds={null}
		{playlists}
		{saving}
		onleave={leave}
		onsave={() => void save()}
	/>
	{#if failed}
		<p class="note" role="status">The rules couldn't be saved. Try again in a moment.</p>
	{/if}
</div>

<style>
	.smart {
		--frame-top: 96px;
	}

	.note {
		margin-top: 12px;
		color: var(--danger);
	}

	@media (max-width: 699px) {
		.smart {
			--frame-top: 54px;
		}
	}
</style>
