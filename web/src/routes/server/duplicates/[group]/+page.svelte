<!--
	Server › Duplicates › a group or an album pair (plan 069), in place of the list: the copies side
	by side, the keeper suggested, and the merge's confirm. Administrators only.
-->
<script lang="ts">
	import { MediaQuery } from 'svelte/reactivity';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import GroupView from '#lib/duplicates/GroupView.svelte';
	import PairView from '#lib/duplicates/PairView.svelte';
	import { DuplicatesError, type DuplicateDetail } from '#lib/duplicates/types.ts';
	import { duplicates } from '#lib/duplicates/duplicates.svelte.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import ToolTabs from '#lib/organize/ToolTabs.svelte';

	const phone = new MediaQuery('(max-width: 720px)', false);
	const key = $derived(page.params.group ?? '');

	// What the group said when last read, if it was: its title is there for the press to land on.
	let detail = $state<DuplicateDetail | null>(duplicates.known(page.params.group ?? ''));
	let problem = $state('');

	$effect(() => {
		const asked = key;
		detail = duplicates.known(asked);
		problem = '';
		duplicates.detail(asked).then(
			(read) => {
				if (asked === key) {
					detail = read;
				}
			},
			(error: unknown) => {
				if (asked === key) {
					problem =
						error instanceof DuplicatesError && error.status === 404
							? 'This group isn’t duplicates any more: the library changed, or it was merged.'
							: 'This group couldn’t be read.';
				}
			}
		);
	});

	const title = $derived(
		detail === null ? 'Duplicates' : detail.kind === 'group' ? detail.group.title : 'Album pair'
	);
</script>

<svelte:head>
	<title>{title} · Duplicates · Kitsune</title>
</svelte:head>

{#if !library.admin}
	<p class="gone">Duplicates is for administrators. <a href={resolve('/you')}>Back to You</a></p>
{:else}
	<div class="page-frame detail" class:phone={phone.current}>
		{#if !phone.current}
			<ToolTabs current="duplicates" />
		{/if}
		{#if detail?.kind === 'group'}
			<GroupView group={detail.group} phone={phone.current} />
		{:else if detail?.kind === 'pair'}
			<PairView pair={detail.pair} phone={phone.current} />
		{:else if problem !== ''}
			<p class="problem" role="alert">
				{problem} <a href={resolve('/server/duplicates')}>All duplicates ›</a>
			</p>
		{/if}
	</div>
{/if}

<style>
	.gone {
		padding: calc(var(--frame-top) * var(--step)) var(--gutter);
		color: var(--text-secondary);
	}

	.detail {
		display: flex;
		flex-direction: column;
		gap: calc(16px * var(--step));
		padding-bottom: calc(140px * var(--step));
	}

	.problem {
		margin: 0;
		color: var(--text-secondary);
	}

	.problem a {
		color: var(--fox-text);
		text-decoration: none;
	}
</style>
