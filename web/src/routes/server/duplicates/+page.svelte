<!--
	Server › Duplicates (plan 069): copies of one recording, found across the library, an album pair
	first, then each group by how sure it is. Compare opens a group in place of the list. Not
	duplicates hides one for good, under its own filter, with Bring back. Administrators only.
-->
<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import { resolve } from '$app/paths';
	import CoverStack from '#lib/duplicates/CoverStack.svelte';
	import { duplicates } from '#lib/duplicates/duplicates.svelte.ts';
	import type { DuplicateFilter, DuplicateGroupSummary } from '#lib/duplicates/types.ts';
	import {
		checkedNote,
		chipWords,
		countsLine,
		filterChips,
		groupFacts,
		pairLine,
		pairSentence
	} from '#lib/duplicates/words.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import ToolTabs from '#lib/organize/ToolTabs.svelte';

	const phone = new MediaQuery('(max-width: 720px)', false);
	const d = duplicates;
	const list = $derived(d.list);
	const counts = $derived(list?.counts ?? null);
	const pairs = $derived((list?.pairs ?? []).filter((p) => !d.hidden.has(p.key)));
	const groups = $derived((list?.groups ?? []).filter((g) => !d.hidden.has(g.key)));
	const dismissedView = $derived(d.filter === 'dismissed');
	let bringing = $state('');

	onMount(() => {
		void d.read();
	});

	$effect(() => {
		// The library changed, here or on another device: the find is worked out again.
		if (library.libraryHeard > 0) {
			untrack(() => void d.read());
		}
	});

	function pick(filter: DuplicateFilter): void {
		void d.read(filter);
	}

	async function bringBack(key: string): Promise<void> {
		bringing = key;
		try {
			await d.bringBack(key);
		} finally {
			bringing = '';
		}
	}

	const albumNames = (g: DuplicateGroupSummary): string => g.albums.map((a) => a.title).join(' · ');
</script>

<svelte:head>
	<title>Duplicates · Kitsune</title>
</svelte:head>

{#if !library.admin}
	<p class="gone">Duplicates is for administrators. <a href={resolve('/you')}>Back to You</a></p>
{:else}
	<div class="page-frame dupes" class:phone={phone.current}>
		<header class="intro">
			<div class="intro-words">
				<ToolTabs current="duplicates" phone={phone.current} />
				<h1>Duplicates<span class="dot">.</span></h1>
				<p class="counts mono" data-testid="counts">
					{counts === null ? ' ' : countsLine(counts, phone.current)}
				</p>
			</div>
			{#if !phone.current}
				<p class="promise mono">
					KEEP ONE COPY · ITS PLAYS, STARS AND PLAYLIST PLACES GATHER ON IT · THE OTHERS GO TO THE
					TRASH FOR 30 DAYS
				</p>
			{/if}
		</header>

		{#if counts !== null && checkedNote(counts) !== ''}
			<p class="checked mono">{checkedNote(counts)}</p>
		{/if}

		{#if counts !== null}
			<div class="chips" role="group" aria-label="Show">
				{#each filterChips(counts) as chip (chip.filter)}
					<button
						type="button"
						class="filter"
						aria-pressed={d.filter === chip.filter}
						onclick={() => {
							pick(chip.filter);
						}}
						>{chip.label}
						<span class="count mono">{chip.count.toLocaleString('en-US')}</span></button
					>
				{/each}
			</div>
		{/if}

		{#if d.problem !== '' && list === null}
			<p class="problem" role="alert">{d.problem}</p>
		{/if}

		{#each pairs as pair (pair.key)}
			{@const sentence = pairSentence(pair)}
			<article
				class="pair"
				aria-label={`Album pair: ${pair.albums[0].title} and ${pair.albums[1].title}`}
			>
				<CoverStack albums={pair.albums} size={phone.current ? 44 : 60} />
				<span class="pair-words">
					<span class="pair-kicker mono">ALBUM PAIR · {pair.albums[0].artist.toUpperCase()}</span>
					<span class="pair-title"
						>{sentence.lead}<em>{sentence.a}</em>{sentence.mid}<em>{sentence.b}</em></span
					>
					<span class="pair-line mono">{pairLine(pair)}</span>
				</span>
				{#if dismissedView}
					<button
						type="button"
						class="outline"
						disabled={bringing === pair.key}
						onclick={() => void bringBack(pair.key)}>Bring back</button
					>
				{:else}
					<a class="outline" href={resolve('/server/duplicates/[group]', { group: pair.key })}
						>Compare albums ›</a
					>
				{/if}
			</article>
		{/each}

		{#if list !== null && groups.length === 0 && pairs.length === 0}
			<p class="empty">
				{dismissedView
					? 'Nothing is dismissed.'
					: d.filter === 'all'
						? 'No duplicates. Every song is here once.'
						: 'None of these.'}
			</p>
		{/if}

		{#if groups.length > 0}
			<ul class="groups" aria-label="Groups">
				{#each groups as g (g.key)}
					<li class="group">
						<span class="song">
							<span class="title">{g.title}</span>
							<span class="artist mono">{g.artist.toUpperCase()}</span>
						</span>
						<span class="chip mono" class:sure={g.confidence !== 'probably'}
							>{chipWords(g.confidence)}</span
						>
						<span class="where">
							<CoverStack albums={g.albums} size={phone.current ? 32 : 36} />
							<span class="where-words">
								<span class="albums">{albumNames(g)}</span>
								<span class="facts mono">{groupFacts(g)}</span>
							</span>
						</span>
						<span class="actions">
							{#if dismissedView}
								<button
									type="button"
									class="outline small"
									disabled={bringing === g.key}
									onclick={() => void bringBack(g.key)}>Bring back</button
								>
							{/if}
							<a
								class="outline small"
								href={resolve('/server/duplicates/[group]', { group: g.key })}
								aria-label={`Compare ${g.title}`}>Compare ›</a
							>
						</span>
					</li>
				{/each}
			</ul>
		{/if}

		{#if list?.nextCursor !== undefined}
			<button type="button" class="outline more" disabled={d.loading} onclick={() => void d.more()}
				>Show more</button
			>
		{/if}

		<p class="foot mono">NOT DUPLICATES HIDES ONE FOR GOOD · A NEW COPY RAISES IT AGAIN</p>
	</div>

	{#if d.toast !== null}
		<div class="toast" class:phone={phone.current} role="status">
			<span class="toast-dot" aria-hidden="true"></span>
			<span>{d.toast}</span>
			<a href={resolve('/server/trash')}>See the Trash</a>
		</div>
	{/if}
{/if}

<style>
	.mono {
		font-family: var(--font-mono);
	}

	.gone {
		padding: calc(var(--frame-top) * var(--step)) var(--gutter);
		color: var(--text-secondary);
	}

	.dupes {
		display: flex;
		flex-direction: column;
		gap: calc(14px * var(--step));
		padding-bottom: calc(140px * var(--step));
	}

	.intro {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 24px;
	}

	.intro-words {
		display: flex;
		flex-direction: column;
		gap: calc(10px * var(--step));
		min-width: 0;
	}

	h1 {
		margin: 0;
		font-size: calc(72px * var(--step));
		font-weight: 900;
		letter-spacing: -0.06em;
		line-height: 1;
	}

	.dot {
		color: var(--fox);
	}

	.counts,
	.checked,
	.foot {
		margin: 0;
		font-size: calc(12px * var(--step));
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.checked {
		color: var(--fox-text);
		font-size: calc(11px * var(--step));
	}

	.foot {
		font-size: calc(11px * var(--step));
		letter-spacing: 0.04em;
	}

	.promise {
		max-width: calc(420px * var(--step));
		margin: 0 0 4px;
		text-align: right;
		font-size: calc(11px * var(--step));
		line-height: 1.6;
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.chips {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}

	.filter {
		height: calc(34px * var(--step));
		padding: 0 12px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: transparent;
		color: var(--text-secondary);
		font: 600 calc(13px * var(--step)) var(--font-sans);
		cursor: pointer;
	}

	.filter[aria-pressed='true'] {
		border-color: var(--fox-line);
		background: color-mix(in srgb, var(--fox) 12%, transparent);
		color: var(--text);
	}

	.count {
		font-size: calc(11px * var(--step));
		color: var(--fox-text);
	}

	.problem,
	.empty {
		margin: 0;
		color: var(--text-secondary);
	}

	.pair {
		display: flex;
		align-items: center;
		gap: calc(20px * var(--step));
		padding: calc(18px * var(--step)) calc(22px * var(--step));
		border-radius: var(--radius-20);
		background: color-mix(in srgb, var(--fox) 8%, transparent);
		border: 1.5px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
	}

	.pair-words {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}

	.pair-kicker {
		font-size: calc(10.5px * var(--step));
		font-weight: 700;
		letter-spacing: 0.08em;
		color: var(--fox-text);
	}

	.pair-title {
		font-size: calc(18px * var(--step));
		font-weight: 700;
	}

	.pair-title em {
		font-style: normal;
		color: var(--fox-text);
	}

	.pair-line {
		font-size: calc(11px * var(--step));
		letter-spacing: 0.04em;
		color: var(--text-meta);
	}

	.outline {
		display: inline-flex;
		align-items: center;
		flex: none;
		height: calc(42px * var(--step));
		padding: 0 18px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font: 700 calc(14px * var(--step)) var(--font-sans);
		text-decoration: none;
		cursor: pointer;
	}

	.outline:hover {
		background: var(--hover);
	}

	.outline.small {
		height: calc(36px * var(--step));
		padding: 0 14px;
		font-size: calc(13px * var(--step));
	}

	.more {
		align-self: flex-start;
	}

	.groups {
		margin: 0;
		padding: 0;
		list-style: none;
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid var(--edge);
		overflow: hidden;
	}

	.group {
		display: grid;
		grid-template-columns: minmax(0, 1.4fr) calc(170px * var(--step)) minmax(0, 1.5fr) auto;
		align-items: center;
		gap: calc(18px * var(--step));
		min-height: calc(64px * var(--step));
		padding: 8px calc(22px * var(--step));
	}

	.group + .group {
		border-top: 1px solid var(--edge);
	}

	.song,
	.where-words {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.title {
		overflow: hidden;
		font-size: calc(16px * var(--step));
		font-weight: 700;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.artist,
	.facts {
		font-size: calc(10.5px * var(--step));
		color: var(--text-meta);
	}

	.chip {
		justify-self: start;
		padding: 3px 8px;
		border-radius: 6px;
		border: 1.5px dashed var(--edge-strong);
		font-size: calc(10px * var(--step));
		font-weight: 700;
		letter-spacing: 0.06em;
		color: var(--text-secondary);
		transform: rotate(-1deg);
		white-space: nowrap;
	}

	.chip.sure {
		border-color: color-mix(in srgb, var(--fox-line) 55%, transparent);
		color: var(--fox-text);
	}

	.where {
		display: flex;
		align-items: center;
		gap: 12px;
		min-width: 0;
	}

	.albums {
		overflow: hidden;
		font-size: calc(13px * var(--step));
		color: var(--text-secondary);
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.actions {
		display: flex;
		gap: 8px;
		justify-self: end;
	}

	.toast {
		position: fixed;
		left: 50%;
		bottom: calc(110px * var(--step));
		z-index: 20;
		display: flex;
		align-items: center;
		gap: 16px;
		max-width: calc(100vw - 2 * var(--gutter));
		padding: 14px 20px;
		border-radius: var(--radius-16);
		background: var(--card-raised);
		border: 1px solid var(--edge-raised);
		box-shadow: var(--shadow-float);
		font-size: calc(14px * var(--step));
		transform: translateX(-50%);
	}

	.toast-dot {
		flex: none;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--fox);
	}

	.toast a {
		font-weight: 700;
		color: var(--fox-text);
		text-decoration: none;
		white-space: nowrap;
	}

	/* A phone: one card per group, and the pair's card wraps. */
	.phone h1 {
		font-size: 48px;
	}

	.phone .counts {
		font-size: 10.5px;
		line-height: 1.5;
	}

	.phone .chips {
		flex-wrap: nowrap;
		overflow-x: auto;
		margin: 0 calc(-1 * var(--gutter));
		padding: 0 var(--gutter);
		scrollbar-width: none;
	}

	.phone .filter {
		flex: none;
	}

	.phone .pair {
		flex-wrap: wrap;
		padding: 14px 16px;
	}

	.phone .pair-title {
		font-size: 16px;
	}

	.phone .pair .outline {
		width: 100%;
		justify-content: center;
	}

	.phone .groups {
		display: flex;
		flex-direction: column;
		gap: 12px;
		border: 0;
		background: none;
		border-radius: 0;
		overflow: visible;
	}

	.phone .group {
		grid-template-columns: minmax(0, 1fr) auto;
		grid-template-areas:
			'song chip'
			'where where'
			'actions actions';
		gap: 10px;
		padding: 14px 16px;
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid var(--edge);
	}

	.phone .group + .group {
		border-top: 1px solid var(--edge);
	}

	.phone .song {
		grid-area: song;
	}

	.phone .chip {
		grid-area: chip;
		justify-self: end;
	}

	.phone .where {
		grid-area: where;
	}

	.phone .actions {
		grid-area: actions;
		justify-self: stretch;
	}

	.phone .actions .outline {
		flex: 1;
		justify-content: center;
	}

	.phone .albums {
		white-space: normal;
	}

	.toast.phone {
		left: var(--gutter);
		right: var(--gutter);
		bottom: 96px;
		flex-wrap: wrap;
		transform: none;
	}
</style>
