<!--
	An album pair: two albums whose songs three or more groups pair, compared as two columns with
	their track lists matched row by row. The songs only one album has are marked, and never
	trashed. Keeping one album merges each matched song of the other into it, in one go.
-->
<script lang="ts">
	import { layInFront } from '#lib/morph/surface.ts';
	import { goto } from '$app/navigation';
	import { unfoldFrom } from '#lib/morph/journey.ts';
	import { resolve } from '$app/paths';
	import { clock } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import CoverStack from './CoverStack.svelte';
	import { duplicates } from './duplicates.svelte.ts';
	import MergeConfirm from './MergeConfirm.svelte';
	import { queuedCopy } from './play.ts';
	import {
		DuplicatesError,
		type DuplicateCopy,
		type DuplicatePair,
		type MergePlan
	} from './types.ts';
	import { movesLines, staysLines } from './words.ts';

	interface Props {
		pair: DuplicatePair;
		phone: boolean;
	}

	let { pair, phone }: Props = $props();

	let kept = $state('');
	let confirming = $state(false);
	let busy = $state(false);
	let back = $state<HTMLAnchorElement>();
	let problem = $state('');
	let fresher = $state<Record<string, MergePlan>>({});

	$effect.pre(() => {
		kept = pair.keeper;
		fresher = {};
	});

	const side = $derived(pair.albums[0].id === kept ? 0 : 1);
	const keeper = $derived(pair.albums[side]);
	const other = $derived(pair.albums[1 - side]);
	const matched = $derived(
		pair.rows.filter((r) => r.copies[0] !== null && r.copies[1] !== null).length
	);
	const plan = $derived(fresher[kept] ?? pair.plans.find((p) => p.keep === kept)?.plan);
	const writes = $derived(library.user?.canTrash === true);
	const keeperPlays = $derived(
		pair.rows.reduce(
			(sum, r) =>
				sum + (r.copies[0] !== null && r.copies[1] !== null ? (r.copies[side]?.plays ?? 0) : 0),
			0
		)
	);
	const counts = $derived([0, 1].map((s) => pair.rows.filter((r) => r.copies[s] !== null).length));

	function line(c: DuplicateCopy): string {
		const parts = [c.format.toUpperCase()];
		if (c.bitrate !== undefined) {
			parts.push(`${c.bitrate.toLocaleString('en-US')} KB/S`);
		}
		parts.push(clock(Math.round(c.durationMs / 1000)));
		if (c.plays > 0) {
			parts.push(`${String(c.plays)} ${c.plays === 1 ? 'PLAY' : 'PLAYS'}`);
		}
		return parts.join(' · ');
	}

	async function merge(): Promise<void> {
		if (plan === undefined || other === undefined) {
			return;
		}
		busy = true;
		problem = '';
		const into = keeper?.title ?? '';
		try {
			await duplicates.merge(
				pair.key,
				{ keep: kept, trash: [other.id], plan },
				(merged) =>
					`Kept ${into}. ${String(merged.trashed.tracks.length)} ${merged.trashed.tracks.length === 1 ? 'copy' : 'copies'} moved to the Trash`
			);
			// It is gone, so no row holds it: the list is born at ‹ All duplicates' place.
			unfoldFrom(back);
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
			await (pair.dismissed === true
				? duplicates.bringBack(pair.key)
				: duplicates.dismiss(pair.key));
			// It is gone, so no row holds it: the list is born at ‹ All duplicates' place.
			unfoldFrom(back);
			await goto(resolve('/server/duplicates'));
		} catch (error) {
			problem = error instanceof DuplicatesError ? error.detail : 'That didn’t happen.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="pair" class:phone>
	<a class="back mono" href={resolve('/server/duplicates')} bind:this={back}>‹ ALL DUPLICATES</a>
	<div class="head">
		<h1 data-morph-part="intro" data-morph-key="headline" data-morph-headline>
			Album pair<span class="dot">.</span>
		</h1>
		<span class="words mono"
			>{pair.albums[0].artist.toUpperCase()} · {matched} SONGS IN BOTH · KEEP ONE ALBUM</span
		>
	</div>

	<div class="albums">
		{#each pair.albums as album, s (album.id)}
			<section class="album" class:keeper={album.id === kept} aria-label={album.title}>
				{#if album.id === kept}
					<span class="tag mono">KEEP THIS ONE</span>
				{/if}
				<CoverStack albums={[album]} size={phone ? 44 : 56} />
				<span class="album-words">
					<span class="album-title">{album.title}</span>
					<span class="album-line mono"
						>{album.year === undefined ? '' : `${String(album.year)} · `}{counts[s]} SONGS</span
					>
				</span>
				{#if album.id !== kept && writes && !confirming}
					<button
						type="button"
						class="outline small"
						onclick={() => {
							kept = album.id;
						}}>Keep this one instead</button
					>
				{/if}
			</section>
		{/each}
	</div>

	<ol class="rows" aria-label="Track lists">
		{#each pair.rows as row, i (row.group ?? `only-${String(i)}`)}
			<li class="row" class:lone={row.copies[0] === null || row.copies[1] === null}>
				{#each row.copies as copy, s (s)}
					<span class="cell" class:gone={copy !== null && s !== side && row.copies[1 - s] !== null}>
						{#if copy !== null}
							<button
								type="button"
								class="play"
								aria-label={`Play ${copy.title} from ${copy.album.title}`}
								onclick={() => {
									player.playSongs([queuedCopy(copy, pair.albums[s]?.artist ?? '')]);
								}}>{copy.trackNo === undefined ? '' : String(copy.trackNo).padStart(2, '0')}</button
							>
							<span class="cell-words">
								<span class="song">{copy.title}</span>
								<span class="facts mono">{line(copy)}</span>
							</span>
							{#if row.copies[1 - s] === null}
								<span class="only mono">ONLY IN THIS ONE</span>
							{/if}
						{/if}
					</span>
				{/each}
			</li>
		{/each}
	</ol>

	{#if confirming && plan !== undefined && keeper !== undefined}
		<MergeConfirm
			question={`Merge ${String(matched)} songs into ${keeper.title} and trash the others?`}
			moves={[
				...movesLines(plan, { name: keeper.title, plays: keeperPlays, stars: 0 }, true),
				'The songs only one album has stay where they are'
			]}
			stays={staysLines(keeper.title, matched)}
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
		<div class="actions" {@attach layInFront}>
			{#if writes}
				<button
					type="button"
					data-buds
					class="fox-button"
					disabled={busy}
					onclick={() => {
						confirming = true;
					}}>Keep {keeper?.title}, trash the other’s {matched}</button
				>
			{/if}
			<button type="button" class="outline" disabled={busy} onclick={() => void notDuplicates()}
				>{pair.dismissed === true ? 'Bring back' : 'Not duplicates'}</button
			>
			{#if !writes}
				<span class="note"
					>Tag writes are off, so nothing on disk changes. Turn them on in Server › Library.</span
				>
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

	.pair {
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
	}

	.dot {
		color: var(--fox);
	}

	.words {
		padding-bottom: 8px;
		font-size: calc(11px * var(--step));
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.albums {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: calc(20px * var(--step));
		padding-top: calc(14px * var(--step));
	}

	.album {
		position: relative;
		display: flex;
		align-items: center;
		gap: 16px;
		flex-wrap: wrap;
		padding: calc(18px * var(--step)) calc(22px * var(--step));
		border-radius: calc(22px * var(--step));
		background: var(--card);
		border: 1px solid var(--edge);
	}

	.album.keeper {
		border: 2px solid var(--fox-line);
		box-shadow: 0 0 0 6px color-mix(in srgb, var(--fox) 12%, transparent);
	}

	.tag {
		position: absolute;
		right: calc(22px * var(--step));
		top: calc(-14px * var(--step));
		padding: 5px 10px;
		border-radius: 6px;
		background: var(--fox);
		color: var(--fox-ink);
		font-size: calc(11px * var(--step));
		font-weight: 800;
		letter-spacing: 0.08em;
		transform: rotate(-3deg);
		box-shadow: 3px 3px 0 color-mix(in srgb, var(--misprint) 55%, transparent);
	}

	.album-words {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 3px;
		min-width: 0;
	}

	.album-title {
		font-size: calc(19px * var(--step));
		font-weight: 800;
	}

	.album-line,
	.facts {
		font-size: calc(11px * var(--step));
		color: var(--text-meta);
	}

	.rows {
		margin: 0;
		padding: 0;
		list-style: none;
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid var(--edge);
		overflow: hidden;
	}

	.row {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: calc(20px * var(--step));
		padding: 8px calc(22px * var(--step));
	}

	.row + .row {
		border-top: 1px solid var(--edge);
	}

	.cell {
		display: flex;
		align-items: center;
		gap: 12px;
		min-width: 0;
		min-height: calc(44px * var(--step));
	}

	.cell.gone .song {
		color: var(--text-meta);
		text-decoration: line-through;
		text-decoration-color: var(--text-meta);
	}

	.play {
		flex: none;
		width: calc(36px * var(--step));
		height: calc(36px * var(--step));
		border-radius: 50%;
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text-secondary);
		font: 600 calc(11px * var(--step)) var(--font-mono);
		cursor: pointer;
	}

	.play:hover {
		background: var(--hover);
		color: var(--text);
	}

	.cell-words {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.song {
		overflow: hidden;
		font-size: calc(14px * var(--step));
		font-weight: 600;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.only {
		flex: none;
		padding: 2px 7px;
		border-radius: 5px;
		border: 1.5px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
		font-size: calc(10px * var(--step));
		font-weight: 700;
		color: var(--fox-text);
		transform: rotate(-1deg);
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

	.outline.small {
		height: calc(36px * var(--step));
		padding: 0 14px;
		font-size: calc(13px * var(--step));
	}

	.outline:hover {
		background: var(--hover);
	}

	button:disabled {
		opacity: 0.5;
	}

	.note {
		font-size: calc(13px * var(--step));
		color: var(--text-meta);
	}

	.problem {
		margin: 0;
		color: var(--danger-ink);
	}

	.phone h1 {
		font-size: 48px;
	}

	.phone .albums,
	.phone .row {
		grid-template-columns: minmax(0, 1fr);
		gap: 12px;
	}

	.phone .row {
		gap: 4px;
		padding: 10px 14px;
	}

	.phone .cell:empty {
		display: none;
	}

	.phone .actions {
		flex-direction: column;
		align-items: stretch;
	}
</style>
