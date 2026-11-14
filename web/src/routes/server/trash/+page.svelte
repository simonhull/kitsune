<!--
	Server › Trash: the songs moved to Trash, by album, the album moved to most lately first. Each
	can go back where it was, or all of an album's at once; Empty Trash deletes them for good, and only
	on a second press. Administrators only, on a server that may change its files.
-->
<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { library } from '#lib/library/library.svelte.ts';
	import type { TrashAlbum } from '#lib/library/source.ts';
	import { trashBin } from '#lib/pages/trashBin.svelte.ts';
	import {
		groupLine,
		leftWords,
		restoreLabel,
		summaryWords,
		whenWords
	} from '#lib/pages/trashWords.ts';
	import Icon from '#lib/shell/Icon.svelte';

	/** How often the days left are worked out again. */
	const tickMs = 60_000;

	const phone = new MediaQuery('(max-width: 720px)', false);
	const load = $derived(trashBin.contents);
	const trash = $derived(load.kind === 'ready' ? load.value : null);
	let now = $state(Date.now());
	let confirming = $state(false);
	let problem = $state('');
	/** What the second press is about to delete, held while it's asked: the list may change under it. */
	let emptying = $state(0);

	$effect(() => {
		// Another device moved songs in or out: the list follows.
		if (library.libraryHeard > 0) {
			untrack(() => void trashBin.read());
		}
	});

	onMount(() => {
		void trashBin.read();
		const clock = setInterval(() => {
			now = Date.now();
		}, tickMs);
		return () => {
			clearInterval(clock);
		};
	});

	async function restore(ids: readonly string[]): Promise<void> {
		problem = (await trashBin.restore(ids)) ?? '';
	}

	function ask(): void {
		emptying = trash?.count ?? 0;
		confirming = true;
	}

	async function empty(): Promise<void> {
		confirming = false;
		problem = (await trashBin.empty({ all: true })) ?? '';
	}

	const songIds = (album: TrashAlbum): string[] => album.tracks.map(({ id }) => id);
</script>

<svelte:head>
	<title>Trash · Kitsune</title>
</svelte:head>

{#if !library.admin}
	<p class="gone">Trash is for administrators. <a href={resolve('/you')}>Back to You</a></p>
{:else}
	<div class="page-frame trash" class:phone={phone.current}>
		<div class="intro" data-morph-part="intro" data-morph-key="intro">
			<a class="crumb mono" href={resolve('/server')}>SERVER › TRASH</a>
			<div class="head">
				<h1>Trash<span class="dot">.</span></h1>
				{#if trashBin.can && trash !== null && trash.count > 0 && !phone.current}
					{@render emptyControl()}
				{/if}
			</div>
			<p class="summary mono">
				{trash === null ? '' : summaryWords(trash, phone.current)}
			</p>
			{#if trashBin.can && trash !== null && trash.count > 0 && phone.current}
				{@render emptyControl()}
			{/if}
			{#if problem !== ''}
				<p class="problem" role="alert">{problem}</p>
			{/if}
		</div>

		{#if load.kind === 'unavailable'}
			<p class="note" role="status">
				Trash couldn’t be read.
				<button type="button" class="link" onclick={() => void trashBin.read()}>Try again</button>
			</p>
		{:else if trash !== null && trash.count === 0}
			<p class="none">Nothing in Trash. Songs moved here stay {trash.retentionDays} days.</p>
		{:else if trash !== null}
			<div class="groups" data-morph-part="card" data-morph-key="trash">
				{#each trash.albums as album (album.id)}
					<section class="group" aria-label={album.title}>
						<div class="group-head">
							<span class="cover"
								><Cover
									albumId={album.id}
									entry={album}
									place="small"
									box={phone.current ? 44 : 56}
								/></span
							>
							<span class="group-words">
								<span class="album">{album.title}</span>
								<span class="group-line mono">{groupLine(album.artist, album.tracks.length)}</span>
							</span>
							{#if trashBin.can}
								<button
									type="button"
									class="restore-all"
									aria-label={album.tracks.length === 1
										? `Restore the one song of ${album.title}`
										: `Restore ${String(album.tracks.length)} songs of ${album.title}`}
									onclick={() => void restore(songIds(album))}
									>{restoreLabel(album.tracks.length)}</button
								>
							{/if}
						</div>
						{#each album.tracks as track (track.id)}
							{@const left = leftWords(track.purgeAt, now)}
							<div class="song">
								<span class="title">{track.title}</span>
								{#if !phone.current}
									<span class="when mono">{whenWords(track.trashedAt, track.trashedBy, now)}</span>
								{/if}
								<span class="left mono" class:danger={left.danger}>{left.text}</span>
								{#if trashBin.can}
									<button
										type="button"
										class="restore"
										aria-label={`Restore ${track.title}`}
										onclick={() => void restore([track.id])}
										>{#if phone.current}<Icon name="restore" size={18} />{:else}Restore{/if}</button
									>
								{/if}
							</div>
						{/each}
					</section>
				{/each}
			</div>
			<p class="about">
				Restore puts each file back where it was, with its plays, stars and listens, as if it never
				left. A song in Trash plays nowhere, is in no shuffle and no list. Files wait in
				<span class="code">.kitsune-trash</span> inside their music folder, so nothing crosses
				drives; after {trash.retentionDays} days the nightly pass deletes them for good.
			</p>
		{/if}
	</div>
{/if}

{#snippet emptyControl()}
	{#if confirming}
		<span class="confirm" role="group" aria-label="Empty Trash">
			<span class="confirm-words"
				>Delete {emptying.toLocaleString('en-US')}
				{emptying === 1 ? 'song' : 'songs'} for good? This can’t be undone.</span
			>
			<span class="confirm-buttons">
				<button
					type="button"
					class="keep"
					onclick={() => {
						confirming = false;
					}}>Keep them</button
				>
				<button type="button" class="delete" onclick={() => void empty()}>Delete for good</button>
			</span>
		</span>
	{:else}
		<button type="button" class="empty" onclick={ask}>Empty Trash…</button>
	{/if}
{/snippet}

<style>
	.trash {
		--cover: calc(56px * var(--step));
		--row: calc(48px * var(--step));
	}

	.gone,
	.note {
		padding: calc(var(--frame-top) + 40px) var(--gutter);
		color: var(--text-secondary);
	}

	.mono {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		letter-spacing: 0.06em;
	}

	.code {
		font-family: var(--font-mono);
		color: var(--text-secondary);
	}

	a {
		color: var(--fox-text);
	}

	.crumb {
		color: var(--text-meta);
		letter-spacing: 0.08em;
		text-decoration: none;
	}

	.crumb:hover {
		color: var(--fox-text);
	}

	.head {
		margin-top: calc(16px * var(--step));
		display: flex;
		align-items: flex-end;
		gap: 24px;
	}

	h1 {
		flex: 1;
		font-size: calc(56px * var(--step));
		font-weight: 800;
		letter-spacing: -0.05em;
		line-height: 1;
	}

	.dot {
		color: var(--fox-text);
	}

	.summary {
		margin: calc(8px * var(--step)) 0 calc(32px * var(--step));
		color: var(--text-meta);
	}

	button {
		flex: none;
		border: 0;
		border-radius: var(--radius-pill);
		background: none;
		color: inherit;
		font: 600 calc(14px * var(--step)) var(--font-sans);
		cursor: pointer;
	}

	.empty {
		height: calc(40px * var(--step));
		padding: 0 calc(18px * var(--step));
		border: 1px solid var(--danger-edge);
		color: var(--danger);
	}

	.empty:hover {
		background: var(--danger-wash);
	}

	.confirm {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 6px 6px 6px calc(16px * var(--step));
		border-radius: var(--radius-pill);
		background: var(--danger-wash);
		border: 1px solid color-mix(in srgb, var(--danger) 32%, transparent);
		animation: confirm-in var(--morph-room) var(--curve-room);
	}

	@keyframes confirm-in {
		from {
			opacity: 0;
			transform: translateX(12px);
		}
	}

	.confirm-words {
		font-size: calc(14px * var(--step));
	}

	.confirm-buttons {
		display: flex;
		gap: 4px;
	}

	.keep,
	.delete {
		height: calc(36px * var(--step));
		padding: 0 calc(14px * var(--step));
	}

	.keep {
		color: var(--text-secondary);
	}

	.keep:hover {
		background: var(--hover);
	}

	.delete {
		padding: 0 calc(16px * var(--step));
		background: var(--danger);
		color: var(--danger-ink);
		font-weight: 700;
	}

	.problem {
		margin-bottom: 24px;
		color: var(--problem);
		font-size: var(--type-body);
	}

	.none {
		padding: 80px 0;
		text-align: center;
		color: var(--text-meta);
		font-size: var(--type-body);
	}

	.groups {
		display: flex;
		flex-direction: column;
		gap: calc(28px * var(--step));
	}

	.group {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.group-head {
		display: flex;
		align-items: center;
		gap: calc(16px * var(--step));
		padding-bottom: calc(10px * var(--step));
		border-bottom: 1px solid var(--edge);
	}

	/* Greyed: these songs are out of the library. */
	.cover {
		--cover-radius: calc(14px * var(--step));

		flex: none;
		width: var(--cover);
		height: var(--cover);
		border-radius: var(--cover-radius);
		overflow: hidden;
		filter: grayscale(0.6);
		opacity: 0.85;
	}

	.cover :global(img),
	.cover :global(svg) {
		width: 100%;
		height: 100%;
	}

	.group-words {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 3px;
	}

	.album {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: calc(18px * var(--step));
		font-weight: 700;
	}

	.group-line {
		color: var(--text-meta);
		letter-spacing: 0.02em;
	}

	.restore-all {
		height: calc(40px * var(--step));
		padding: 0 calc(16px * var(--step));
		background: color-mix(in srgb, var(--fox) 16%, transparent);
		color: var(--fox-text);
		font-weight: 700;
	}

	.restore-all:hover {
		background: color-mix(in srgb, var(--fox) 24%, transparent);
	}

	.song {
		display: grid;
		grid-template-columns: minmax(0, 1fr) calc(210px * var(--step)) calc(150px * var(--step)) calc(
				120px * var(--step)
			);
		align-items: center;
		gap: 12px;
		height: var(--row);
		padding: 0 8px 0 calc(var(--cover) + 16px * var(--step));
		border-radius: var(--radius-12);
	}

	.song:hover {
		background: var(--hover);
	}

	.title {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: calc(14px * var(--step));
		font-weight: 600;
	}

	.when,
	.left {
		color: var(--text-meta);
		letter-spacing: 0.02em;
	}

	.left.danger {
		color: var(--danger);
	}

	.restore {
		justify-self: end;
		height: calc(36px * var(--step));
		padding: 0 calc(14px * var(--step));
		color: var(--text);
	}

	.restore:hover {
		background: var(--hover);
	}

	.about {
		margin-top: calc(36px * var(--step));
		max-width: calc(760px * var(--step));
		font-size: calc(13px * var(--step));
		line-height: 1.55;
		color: var(--text-meta);
	}

	.link {
		color: var(--fox-text);
		text-decoration: underline;
	}

	/* A phone: groups stacked, a glyph to restore each song, Empty Trash the width of the page. */
	.phone h1 {
		font-size: 40px;
	}

	.phone .summary {
		margin: 6px 0 16px;
		font-size: 11px;
	}

	.phone .empty {
		width: 100%;
		height: 44px;
		margin-bottom: 24px;
	}

	.phone .confirm {
		margin-bottom: 24px;
		flex-direction: column;
		align-items: stretch;
		gap: 8px;
		padding: 14px 14px 8px;
		border-radius: var(--radius-24);
	}

	.phone .confirm-buttons {
		justify-content: flex-end;
	}

	.phone .keep,
	.phone .delete {
		height: 44px;
	}

	.phone .groups {
		gap: 22px;
	}

	.phone .group {
		gap: 0;
	}

	.phone .group-head {
		gap: 12px;
		padding-bottom: 8px;
	}

	.phone {
		--cover: 44px;
	}

	.phone .cover {
		--cover-radius: 12px;
	}

	.phone .album {
		font-size: 15px;
	}

	.phone .group-line {
		font-size: 11px;
	}

	.phone .restore-all {
		height: 44px;
		padding: 0 14px;
		font-size: 13px;
	}

	.phone .song {
		grid-template-columns: minmax(0, 1fr) 44px;
		grid-template-rows: auto auto;
		column-gap: 8px;
		row-gap: 2px;
		height: auto;
		min-height: 52px;
		padding: 4px 0 4px 56px;
	}

	.phone .title {
		grid-row: 1;
		align-self: end;
	}

	.phone .left {
		grid-row: 2;
		align-self: start;
		font-size: 11px;
	}

	.phone .restore {
		grid-column: 2;
		grid-row: 1 / span 2;
		width: 44px;
		height: 44px;
		padding: 0;
		display: grid;
		place-items: center;
	}

	@media (prefers-reduced-motion: reduce) {
		.confirm {
			animation: none;
		}
	}
</style>
