<!--
	One group (board H): its copies side by side, the suggested keeper in the fox ring, then Keep this
	one and Not duplicates, which open the confirm (board I) in place. On a phone the copies stack,
	the keeper first, and the confirm is a sheet. With tag writes off it still compares, and says so
	where Keep would be.
-->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import CopyCard from './CopyCard.svelte';
	import { duplicates } from './duplicates.svelte.ts';
	import MergeConfirm from './MergeConfirm.svelte';
	import { queuedCopy } from './play.ts';
	import { DuplicatesError, type DuplicateGroup, type MergePlan } from './types.ts';
	import {
		chipWords,
		evidenceWords,
		mergedWords,
		movesLines,
		shortAlbum,
		staysLines,
		whyWords
	} from './words.ts';

	interface Props {
		group: DuplicateGroup;
		phone: boolean;
	}

	let { group, phone }: Props = $props();

	let kept = $state('');
	let confirming = $state(false);
	let busy = $state(false);
	let problem = $state('');
	/** A plan the server sent back because the library changed, by the copy it keeps. */
	let fresher = $state<Record<string, MergePlan>>({});

	$effect.pre(() => {
		// A group read afresh starts from the server's suggestion.
		kept = group.keeper;
		fresher = {};
	});

	const keeper = $derived(group.copies.find((c) => c.track === kept));
	const others = $derived(group.copies.filter((c) => c.track !== kept));
	const ordered = $derived(phone && keeper !== undefined ? [keeper, ...others] : group.copies);
	const plan = $derived(fresher[kept] ?? group.plans.find((p) => p.keep === kept)?.plan);
	const keeperName = $derived(keeper === undefined ? '' : shortAlbum(keeper.album.title));
	const otherName = $derived(others.length === 1 ? shortAlbum(others[0]?.album.title ?? '') : '');
	const writes = $derived(library.user?.canTrash === true);
	const why = $derived(whyWords(group.reasons, group.copies, group.keeper, kept));

	const question = $derived(
		otherName === ''
			? `Merge into ${keeperName}, then trash the other ${String(others.length)} copies?`
			: phone
				? `Merge into ${keeperName}, then trash the other copy?`
				: `Merge into ${keeperName}, then trash the copy from ${otherName}?`
	);

	function choose(track: string): void {
		kept = track;
		problem = '';
	}

	async function merge(): Promise<void> {
		if (plan === undefined || keeper === undefined) {
			return;
		}
		busy = true;
		problem = '';
		const title = group.title;
		const album = keeper.album.title;
		try {
			await duplicates.merge(
				group.key,
				{ keep: kept, trash: others.map((c) => c.track), plan },
				(merged) => mergedWords(title, album, merged.trashed.tracks.length)
			);
			await goto(resolve('/server/duplicates'));
		} catch (error) {
			if (error instanceof DuplicatesError && error.status === 409 && error.plan !== undefined) {
				fresher = { ...fresher, [kept]: error.plan };
			} else {
				problem =
					error instanceof DuplicatesError && error.detail !== ''
						? error.detail
						: 'The merge didn’t happen. Nothing moved.';
			}
		} finally {
			busy = false;
		}
	}

	async function notDuplicates(): Promise<void> {
		busy = true;
		try {
			if (group.dismissed === true) {
				await duplicates.bringBack(group.key);
			} else {
				await duplicates.dismiss(group.key);
			}
			await goto(resolve('/server/duplicates'));
		} catch (error) {
			problem = error instanceof DuplicatesError ? error.detail : 'That didn’t happen.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="group" class:phone>
	<a class="back mono" href={resolve('/server/duplicates')}>‹ ALL DUPLICATES</a>
	<div class="head">
		<h1>{group.title}<span class="dot">.</span></h1>
		<span class="evidence">
			<span class="chip mono" class:sure={group.confidence !== 'probably'}
				>{chipWords(group.confidence)}</span
			>
			<span class="words mono"
				>{evidenceWords(
					group.confidence,
					group.artist,
					group.copies.map((c) => c.durationMs)
				)}</span
			>
		</span>
	</div>
	{#if group.pair !== undefined}
		<p class="pair-note mono">
			ONE OF AN ALBUM PAIR ·
			<a href={resolve('/server/duplicates/[group]', { group: group.pair })}>COMPARE THE ALBUMS ›</a
			>
		</p>
	{/if}

	<div class="copies" style:--copies={Math.min(group.copies.length, 4)}>
		{#each ordered as copy (copy.track)}
			<CopyCard
				{copy}
				{phone}
				keeper={copy.track === kept}
				{why}
				choosing={!confirming && writes}
				onkeep={() => {
					choose(copy.track);
				}}
				onplay={() => {
					player.playSongs([queuedCopy(copy, group.artist)]);
				}}
			/>
		{/each}
	</div>

	{#if confirming && plan !== undefined && keeper !== undefined}
		<MergeConfirm
			{question}
			moves={movesLines(plan, {
				name: keeperName,
				plays: keeper.plays,
				stars: keeper.stars,
				...(otherName === '' ? {} : { other: otherName })
			})}
			stays={staysLines(keeperName, others.length)}
			changed={fresher[kept] !== undefined}
			{problem}
			{busy}
			{phone}
			onmerge={() => void merge()}
			oncancel={() => {
				confirming = false;
				problem = '';
			}}
		/>
	{/if}
	{#if !confirming || phone}
		<div class="actions">
			{#if writes}
				<button
					type="button"
					class="fox-button"
					disabled={busy}
					onclick={() => {
						confirming = true;
					}}
					>{others.length === 1
						? `Keep ${keeperName}, trash the other`
						: `Keep ${keeperName}, trash the others`}</button
				>
			{/if}
			<button type="button" class="outline" disabled={busy} onclick={() => void notDuplicates()}
				>{group.dismissed === true ? 'Bring back' : 'Not duplicates'}</button
			>
			{#if !writes}
				<span class="note"
					>Tag writes are off, so nothing on disk changes. Turn them on in Server › Library.</span
				>
			{:else if !phone}
				<span class="note mono">NOTHING HAPPENS UNTIL YOU CONFIRM</span>
			{/if}
		</div>
		{#if problem !== '' && !confirming}
			<p class="problem" role="alert">{problem}</p>
		{/if}
	{/if}
</div>

<style>
	.mono {
		font-family: var(--font-mono);
	}

	.group {
		display: flex;
		flex-direction: column;
		gap: calc(16px * var(--step));
	}

	.back {
		align-self: flex-start;
		font-size: calc(12px * var(--step));
		letter-spacing: 0.08em;
		color: var(--text-meta);
		text-decoration: none;
	}

	.back:hover {
		color: var(--fox-text);
	}

	.head {
		display: flex;
		align-items: flex-end;
		gap: 18px;
		flex-wrap: wrap;
	}

	h1 {
		margin: 0;
		font-size: calc(72px * var(--step));
		font-weight: 900;
		letter-spacing: -0.06em;
		line-height: 1;
		overflow-wrap: anywhere;
	}

	.dot {
		color: var(--fox);
	}

	.evidence {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding-bottom: 6px;
	}

	.chip {
		align-self: flex-start;
		padding: 3px 8px;
		border-radius: 6px;
		border: 1.5px dashed var(--edge-strong);
		font-size: calc(11px * var(--step));
		font-weight: 700;
		letter-spacing: 0.06em;
		color: var(--text-secondary);
		transform: rotate(-1deg);
	}

	.chip.sure {
		border-color: color-mix(in srgb, var(--fox-line) 55%, transparent);
		color: var(--fox-text);
	}

	.words,
	.pair-note {
		font-size: calc(11px * var(--step));
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.pair-note {
		margin: 0;
	}

	.pair-note a {
		color: var(--fox-text);
		text-decoration: none;
	}

	.copies {
		display: grid;
		grid-template-columns: repeat(var(--copies), minmax(0, 1fr));
		gap: calc(20px * var(--step));
		padding-top: calc(14px * var(--step));
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 12px;
		flex-wrap: wrap;
	}

	.fox-button,
	.outline {
		height: calc(48px * var(--step));
		padding: 0 calc(24px * var(--step));
		border-radius: var(--radius-pill);
		font: 700 calc(16px * var(--step)) var(--font-sans);
		cursor: pointer;
	}

	.fox-button {
		border: 0;
		background: var(--fox);
		color: var(--fox-ink);
	}

	.outline {
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font-weight: 600;
	}

	.outline:hover {
		background: var(--hover);
	}

	button:disabled {
		opacity: 0.5;
	}

	.note {
		margin-left: 8px;
		font-size: calc(11px * var(--step));
		letter-spacing: 0.04em;
		color: var(--text-meta);
	}

	.problem {
		margin: 0;
		color: var(--danger-ink);
	}

	.phone h1 {
		font-size: 48px;
	}

	.phone .head {
		flex-direction: column;
		align-items: flex-start;
		gap: 8px;
	}

	.phone .copies {
		grid-template-columns: minmax(0, 1fr);
		gap: 12px;
		padding-top: 8px;
	}

	.phone .actions {
		flex-direction: column;
		align-items: stretch;
	}

	.phone .note {
		margin: 0;
	}
</style>
