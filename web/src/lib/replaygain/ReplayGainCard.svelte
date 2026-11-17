<!--
	Health's ReplayGain card (plan 070, boards A to C): the songs Kitsune measured that carry no
	ReplayGain of their own, and the button that writes the measure into their files. It becomes the
	run's head while one goes, then says what the run did, with Undo. Administrators only; missing
	ReplayGain isn't a fault, so nothing here raises the Health number.
-->
<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { library } from '#lib/library/library.svelte.ts';
	import { whenWords } from '#lib/organize/words.ts';
	import { replayGain } from './gain.svelte.ts';
	import type { GainAlbum, GainRun } from './types.ts';
	import {
		albumChip,
		albumLine,
		biggestLine,
		doneWords,
		headWords,
		leftHeading,
		leftWords,
		levelledWords,
		moreLeftLine,
		moreWords,
		nowLine,
		paceWords,
		runRowWords,
		tagsLine,
		undoAsk,
		waitingLine,
		whereLine,
		writeWords
	} from './words.ts';

	/** How often the pace and the times are worked out again. */
	const tickMs = 1000;
	/** How long after the library or the loudness pass moves the counts are read again. */
	const settleMs = 800;
	/** Songs left shown by name before "+ n MORE". */
	const leftShown = 3;

	const phone = new MediaQuery('(max-width: 720px)', false);
	const wide = new MediaQuery('(min-width: 2560px)', false);
	const g = replayGain;

	let now = $state(Date.now());
	/** The run whose Undo is being asked about. */
	let asking = $state('');

	onMount(() => {
		g.hear();
		void g.refresh();
		const clock = setInterval(() => {
			now = Date.now();
		}, tickMs);
		return () => {
			clearInterval(clock);
		};
	});

	// A scan, an edit or the loudness pass moves the counts: read them again once it settles.
	let settle: ReturnType<typeof setTimeout> | undefined;
	$effect(() => {
		void library.libraryHeard;
		void library.facts?.loudness?.measured;
		void library.facts?.loudness?.ungained;
		untrack(() => {
			if (g.status === null || g.going !== null) {
				return;
			}
			clearTimeout(settle);
			settle = setTimeout(() => void g.refresh(), settleMs);
		});
		return () => {
			clearTimeout(settle);
		};
	});

	const status = $derived(g.status);
	const run = $derived(g.run);
	const can = $derived(g.can);
	const view = $derived(g.view);
	const drawn = $derived(
		library.admin &&
			status !== null &&
			(status.songs > 0 || status.waiting > 0 || status.runs.length > 0 || view !== 'status')
	);
	const ready = $derived(view === 'status' && status !== null && status.songs > 0);
	const biggest = $derived(status?.biggest.slice(0, wide.current ? 4 : 3) ?? []);
	const elapsed = $derived.by(() => {
		if (run === null) {
			return 0;
		}
		const started = Date.parse(run.startedAt);
		return now - (Number.isNaN(started) ? g.heardSince : Math.min(started, g.heardSince));
	});
	const percent = $derived(
		run === null || run.total <= 0 ? 0 : Math.min(100, (run.done / run.total) * 100)
	);
	const inHand = $derived(
		g.albums.find((album) => album.id === run?.album) ?? g.albums.at(g.albumsDone)
	);
	/** The albums around the one in hand: the last three done, it, and the next two. */
	const shownAlbums = $derived.by(() => {
		const at = g.albumsDone;
		const from = Math.max(0, Math.min(at - 3, g.albums.length - 6));
		return g.albums.slice(from, from + 6).map((album, index) => ({
			album,
			state:
				from + index < at
					? album.state === 'left'
						? ('left' as const)
						: ('written' as const)
					: from + index === at
						? ('writing' as const)
						: ('waiting' as const)
		}));
	});
	const leftCount = $derived(run === null ? 0 : Math.max(run.left + run.failed, g.left.length));
	const undoMine = $derived(run !== null && run.undoable && can);

	function coverOf(album: GainAlbum): { artwork?: GainAlbum['artwork'] } {
		return album.artwork === undefined ? {} : { artwork: album.artwork };
	}

	function undoFromList(each: GainRun): void {
		if (run?.id !== each.id) {
			void g.show(each);
		}
		asking = each.id;
	}
</script>

{#if drawn && status !== null}
	<section
		class="card"
		class:quiet={view === 'status' && status.songs === 0}
		class:phone={phone.current}
		aria-labelledby="replaygain-card"
		data-testid="replaygain"
	>
		<div class="top">
			<span class="eyebrow" id="replaygain-card"
				>{view === 'run' ? 'REPLAYGAIN · WRITING TO THE FILES' : 'REPLAYGAIN · IN THE FILES'}</span
			>
			{#if !phone.current}
				<span class="small">{can ? 'TAG WRITES ON' : 'TAG WRITES OFF'} · ONLY ADMINS</span>
			{/if}
		</div>

		{#if view === 'run' && run !== null}
			<div class="head" role="status" aria-live="polite">
				<div class="head-row">
					<span class="head-title" data-testid="gain-head">{headWords(run)}</span>
					{#if !phone.current}
						<span class="small">{paceWords(run.done, run.total, elapsed)}</span>
					{/if}
					<button
						type="button"
						class="outline stop"
						disabled={run.stopping === true}
						onclick={() => void g.stop()}>{run.stopping === true ? 'Stopping…' : 'Stop'}</button
					>
				</div>
				<div
					class="bar"
					role="progressbar"
					aria-label={run.kind === 'undo' ? 'Songs taken out' : 'Songs written'}
					aria-valuemin={0}
					aria-valuemax={run.total}
					aria-valuenow={run.done}
				>
					<span style:width={`${String(percent)}%`}></span>
				</div>
				<span class="small">{nowLine(inHand?.title ?? '')}</span>
			</div>
			{#if g.problem !== ''}<p class="problem" role="alert">{g.problem}</p>{/if}
			<ul class="albums" aria-label="Albums">
				{#each shownAlbums as { album, state } (album.id)}
					<li class="album-row">
						<span class="cover"
							><Cover albumId={album.id} entry={coverOf(album)} place="small" box={36} /></span
						>
						<span class="words">
							<span class="album-title">{album.title}</span>
							<span class="small">{albumLine(album)}</span>
						</span>
						<span class="chip" class:hot={state === 'writing'} class:landed={state === 'written'}
							>{albumChip(state, run.kind === 'undo')}</span
						>
					</li>
				{/each}
			</ul>
		{:else if view === 'done' && run !== null}
			{@const words = doneWords(run, now)}
			<div
				class="done"
				class:undone={run.kind === 'undo' || run.state === 'undone'}
				data-testid="gain-done"
			>
				<span class="kicker">{words.kicker}</span>
				<span class="done-title">{words.title}<span class="dot">.</span></span>
				<span class="done-body">{words.body}</span>
				{#if asking === run.id && undoMine}
					<div class="ask">
						<span class="ask-words">{undoAsk(run)}</span>
						<span class="small">A SONG RETAGGED SINCE KEEPS ITS OWN, AND IS NAMED AFTER</span>
						<button
							type="button"
							class="outline keep"
							onclick={() => {
								asking = '';
							}}>Keep it</button
						>
						<button
							type="button"
							class="fox"
							disabled={g.asking}
							onclick={() => {
								asking = '';
								void g.undo(run.id);
							}}>Take it out</button
						>
					</div>
				{:else if undoMine}
					<div class="undo-row">
						<button
							type="button"
							class="strong-outline"
							onclick={() => {
								asking = run.id;
							}}>Undo this run</button
						>
						<span class="small">TAKES OUT ONLY WHAT THIS RUN WROTE · KEPT 90 DAYS</span>
					</div>
				{/if}
			</div>
			{#if g.problem !== ''}<p class="problem" role="alert">{g.problem}</p>{/if}
			{#if leftCount > 0 && g.left.length > 0}
				<div class="left">
					<span class="label">{leftHeading(leftCount)}</span>
					{#each g.left.slice(0, leftShown) as song (song.track)}
						{@const why = leftWords[song.reason]}
						<div class="left-row">
							<span class="left-title">{song.title === '' ? song.path : song.title}</span>
							<span class="chip">{why.chip}</span>
							<span class="left-path">{song.path} · <span class="then">{why.then}</span></span>
						</div>
					{/each}
					{#if leftCount > leftShown}
						<span class="small more"
							>{moreLeftLine(
								g.left.slice(leftShown),
								leftCount - Math.max(leftShown, g.left.length)
							)}</span
						>
					{/if}
				</div>
			{/if}
			{#if status.songs > 0 && can && run.kind === 'run' && run.state === 'stopped'}
				<div class="again">
					<button type="button" class="outline" disabled={g.asking} onclick={() => void g.start()}
						>{writeWords(status.songs)}</button
					>
				</div>
			{/if}
			{@render runsList()}
		{:else if ready}
			<div class="ready">
				<div class="said">
					<span class="number">{status.songs.toLocaleString('en-US')}</span>
					<span class="what">{levelledWords(status.songs)}</span>
					{#if status.waiting > 0 || status.unmeasurable > 0}
						<span class="small waiting">{waitingLine(status.waiting, status.unmeasurable)}</span>
					{/if}
					<span class="why"
						>Kitsune measured them, so they play level here. Writing it into the files levels them
						in every player. Nothing you hear in Kitsune changes.</span
					>
				</div>
				<dl class="writes" aria-label="What it writes">
					<div class="fact">
						<dt>TRACK</dt>
						<dd>Gain and peak, each song’s own</dd>
					</div>
					<div class="fact">
						<dt>ALBUM</dt>
						<dd>Gain and peak, pooled over the album</dd>
					</div>
					<div class="fact">
						<dt>LEVEL</dt>
						<dd>
							−18 LUFS, ReplayGain 2.0<span class="fact-mono"
								>AS KITSUNE PLAYS THEM NOW, TO 0.01 DB</span
							>
						</dd>
					</div>
					<div class="fact">
						<dt>TAGS</dt>
						<dd>
							The four REPLAYGAIN_ tags<span class="fact-mono">{tagsLine(status.formats)}</span>
						</dd>
					</div>
				</dl>
			</div>
			{#if phone.current}
				<span class="small where"
					>{whereLine(status.albums, status.formats)} · {biggestLine(biggest)}</span
				>
			{:else}
				<div class="where">
					<span class="small">{whereLine(status.albums, status.formats)}</span>
					{#each biggest as album (album.id)}
						<span class="big" title={album.title}>
							<span class="cover tiny"
								><Cover albumId={album.id} entry={coverOf(album)} place="small" box={24} /></span
							>
							<span class="big-title">{album.title}</span>
							<span class="small">{album.songs.toLocaleString('en-US')}</span>
						</span>
					{/each}
					<span class="small">{moreWords(status.albums, biggest.length)}</span>
				</div>
			{/if}
			{#if g.problem !== ''}<p class="problem" role="alert">{g.problem}</p>{/if}
			<div class="act">
				{#if can}
					<button type="button" class="fox write" disabled={g.asking} onclick={() => void g.start()}
						>{writeWords(status.songs)}</button
					>
					<span class="small note"
						>ONE ALBUM AT A TIME · STOP ANY TIME{#if phone.current}&nbsp;· UNDO{:else}<br />EVERY
							RUN CAN BE UNDONE{/if}</span
					>
				{:else}
					<span class="off"
						>Tag writes are off, so nothing on disk changes. Turn them on in <a
							href={resolve('/server')}>Server › Library</a
						>.</span
					>
				{/if}
			</div>
			{#if status.runs.length > 0}{@render runsList()}{/if}
		{:else}
			<span class="number quiet-number">0</span>
			<span class="what"
				>{status.waiting > 0
					? 'Nothing to write yet'
					: 'Every song carries its own ReplayGain'}</span
			>
			{#if status.waiting > 0 || status.unmeasurable > 0}
				<span class="small waiting">{waitingLine(status.waiting, status.unmeasurable)}</span>
			{/if}
			{#if status.runs.length > 0}{@render runsList()}{/if}
		{/if}
	</section>
{/if}

{#snippet runsList()}
	{#if status !== null && status.runs.length > 0}
		<div class="runs">
			<span class="label">RUNS</span>
			{#each status.runs as each (each.id)}
				{@const row = runRowWords(each)}
				<div class="run-row">
					<span class="small">{whenWords(each.startedAt, now)}</span>
					<span class="run-what">{row.what}</span>
					{#if each.undoable && can}
						<button
							type="button"
							class="run-state hot"
							aria-label={`Undo: ${row.what}`}
							onclick={() => {
								undoFromList(each);
							}}>{row.state}</button
						>
					{:else}
						<span class="run-state">{row.state}</span>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
{/snippet}

<style>
	.card {
		container-type: inline-size;
		min-height: calc(400px * var(--step));
		box-sizing: border-box;
		padding: calc(28px * var(--step));
		display: flex;
		flex-direction: column;
		border-radius: var(--radius-28);
		background: var(--card);
		border: 1px solid var(--fox-line);
	}

	.card.quiet {
		border-color: var(--edge);
	}

	.top {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 16px;
	}

	.eyebrow {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.small {
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.label {
		padding: 6px 0 8px;
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.1em;
		color: var(--text-meta);
	}

	.number {
		font-size: calc(72px * var(--step));
		font-weight: 900;
		letter-spacing: -0.05em;
		line-height: 0.85;
	}

	.quiet-number {
		margin-top: 18px;
	}

	.what {
		margin-top: 14px;
		font-size: calc(17px * var(--step));
		font-weight: 700;
	}

	.why {
		margin-top: 8px;
		font-size: calc(14px * var(--step));
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.waiting {
		margin-top: 8px;
		line-height: 1.5;
		color: var(--fox-text);
	}

	/* Board A: what's said beside what it'll write. */
	.ready {
		margin-top: 18px;
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1.08fr);
		gap: calc(28px * var(--step));
		align-items: start;
	}

	.said {
		display: flex;
		flex-direction: column;
	}

	.writes {
		margin: 0;
		display: flex;
		flex-direction: column;
		padding: 6px 16px;
		border-radius: var(--radius-16);
		background: var(--card-raised);
	}

	.fact {
		display: grid;
		grid-template-columns: calc(116px * var(--step)) minmax(0, 1fr);
		gap: 12px;
		align-items: baseline;
		padding: 10px 0;
	}

	.fact + .fact {
		border-top: 1px solid var(--edge);
	}

	dt {
		font-family: var(--font-mono);
		font-size: calc(10.5px * var(--step));
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	dd {
		margin: 0;
		font-size: calc(13.5px * var(--step));
		line-height: 1.35;
	}

	.fact-mono {
		display: block;
		margin-top: 2px;
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		color: var(--fox-text);
	}

	.where {
		margin-top: 20px;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		line-height: 1.5;
	}

	.big {
		max-width: calc(260px * var(--step));
		height: calc(32px * var(--step));
		box-sizing: border-box;
		display: inline-flex;
		align-items: center;
		gap: 8px;
		padding: 0 12px 0 4px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		font-size: calc(13px * var(--step));
		font-weight: 600;
	}

	.big-title {
		min-width: 0;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.cover {
		position: relative;
		flex: none;
		width: calc(36px * var(--step));
		height: calc(36px * var(--step));
		overflow: hidden;
		border-radius: calc(8px * var(--step));
		background: var(--card-raised);
	}

	.cover.tiny {
		width: calc(24px * var(--step));
		height: calc(24px * var(--step));
		border-radius: calc(6px * var(--step));
	}

	.act {
		margin-top: auto;
		padding-top: 24px;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 14px;
	}

	.note {
		line-height: 1.5;
	}

	.off {
		font-size: calc(14px * var(--step));
		line-height: 1.5;
		color: var(--text-secondary);
	}

	a {
		color: var(--fox-text);
	}

	button {
		cursor: pointer;
	}

	button:disabled {
		opacity: 0.5;
		cursor: progress;
	}

	.fox {
		height: calc(48px * var(--step));
		padding: 0 24px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font: 700 calc(15px * var(--step)) var(--font-sans);
	}

	.outline {
		height: calc(44px * var(--step));
		padding: 0 20px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font: 600 calc(14px * var(--step)) var(--font-sans);
	}

	.outline:hover:not(:disabled) {
		background: var(--hover);
	}

	.strong-outline {
		height: calc(44px * var(--step));
		padding: 0 22px;
		border-radius: var(--radius-pill);
		border: 1.5px solid var(--text);
		background: none;
		color: var(--text);
		font: 700 calc(15px * var(--step)) var(--font-sans);
	}

	.problem {
		margin: 12px 0 0;
		font-size: calc(14px * var(--step));
		color: var(--danger-ink);
	}

	/* Board B: the run's head. */
	.head {
		margin-top: 16px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.head-row {
		display: flex;
		align-items: baseline;
		gap: 14px;
	}

	.head-title {
		font-size: calc(26px * var(--step));
		font-weight: 900;
		letter-spacing: -0.03em;
		white-space: nowrap;
	}

	.stop {
		margin-left: auto;
		height: calc(40px * var(--step));
		font-weight: 700;
	}

	.bar {
		height: 8px;
		border-radius: 4px;
		background: var(--control-wash);
		overflow: hidden;
	}

	.bar span {
		display: block;
		height: 100%;
		background: var(--fox);
		transition: width 400ms var(--curve-settle);
	}

	.albums {
		margin: 14px 0 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
	}

	.album-row {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 12px;
		padding: 8px 0;
		border-top: 1px solid var(--edge);
	}

	.words {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.album-title {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font-size: calc(14px * var(--step));
		font-weight: 700;
	}

	.words .small {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font-size: calc(10.5px * var(--step));
		letter-spacing: 0.04em;
	}

	/* A stamped chip, as Organize's. */
	.chip {
		justify-self: end;
		padding: 3px 8px;
		border-radius: 6px;
		border: 1.5px dashed var(--edge-strong);
		font: 700 calc(10.5px * var(--step)) var(--font-mono);
		letter-spacing: 0.06em;
		color: var(--text-meta);
		white-space: nowrap;
		transform: rotate(-1deg);
	}

	.chip.landed {
		color: var(--text);
	}

	.chip.hot {
		border-color: var(--fox-line);
		color: var(--fox-text);
	}

	/* Board C: what the run did. */
	.done {
		margin-top: 16px;
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: calc(22px * var(--step)) calc(24px * var(--step));
		border-radius: var(--radius-24);
		background: color-mix(in srgb, var(--fox) 12%, transparent);
		border: 1.5px solid var(--fox-line);
	}

	.done.undone {
		background: var(--card-raised);
		border-color: var(--edge-strong);
	}

	.kicker {
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.1em;
		color: var(--text-meta);
	}

	.done-title {
		font-size: calc(40px * var(--step));
		font-weight: 900;
		letter-spacing: -0.04em;
		line-height: 1.05;
	}

	.dot {
		color: var(--fox);
	}

	.done-body {
		font-size: calc(15px * var(--step));
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.ask {
		display: flex;
		align-items: center;
		gap: 12px;
		flex-wrap: wrap;
		padding: 14px 16px;
		border-radius: var(--radius-16);
		border: 1.5px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
		background: color-mix(in srgb, var(--fox) 12%, transparent);
	}

	.ask-words {
		font-size: calc(15px * var(--step));
		font-weight: 600;
	}

	.ask .keep {
		margin-left: auto;
		height: calc(40px * var(--step));
	}

	.ask .fox {
		height: calc(40px * var(--step));
		padding: 0 18px;
		font-size: calc(14px * var(--step));
	}

	.undo-row {
		display: flex;
		align-items: center;
		gap: 14px;
		flex-wrap: wrap;
	}

	.left {
		margin-top: 14px;
		display: flex;
		flex-direction: column;
	}

	.left-row {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		gap: 4px 14px;
		align-items: baseline;
		padding: 9px 0;
		border-top: 1px solid var(--edge);
	}

	.left-title {
		font-size: calc(14px * var(--step));
		font-weight: 700;
	}

	.left-row .chip {
		font-size: calc(10px * var(--step));
		color: var(--text-secondary);
	}

	.left-path {
		grid-column: 1 / -1;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		line-height: 1.5;
		color: var(--text-meta);
	}

	.then {
		color: var(--text-secondary);
	}

	.more {
		padding-top: 6px;
	}

	.again {
		margin-top: 14px;
	}

	.runs {
		margin-top: 14px;
		display: flex;
		flex-direction: column;
	}

	.run-row {
		display: grid;
		grid-template-columns: calc(150px * var(--step)) minmax(0, 1fr) auto;
		align-items: center;
		gap: 14px;
		padding: 10px 0;
		border-top: 1px solid var(--edge);
	}

	.run-what {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font-size: calc(14px * var(--step));
		font-weight: 600;
	}

	.run-state {
		padding: 0;
		border: 0;
		background: none;
		font: 700 calc(11px * var(--step)) var(--font-mono);
		color: var(--text-meta);
	}

	.run-state.hot {
		color: var(--fox-text);
	}

	/* Narrow: what it'll write goes under what's said. */
	@container (max-width: 640px) {
		.ready {
			grid-template-columns: minmax(0, 1fr);
			gap: 16px;
		}
	}

	/* A phone (board Phone): one column, the button the card's width, the run's head sticky. */
	.phone {
		min-height: 0;
		padding: 24px;
		border-radius: var(--radius-24);
	}

	.phone .number {
		font-size: 56px;
	}

	.phone .what {
		margin-top: 12px;
	}

	.phone .ready {
		margin-top: 16px;
	}

	.phone .writes {
		padding: 4px 14px;
	}

	.phone .fact {
		grid-template-columns: minmax(0, 1fr);
		gap: 3px;
		padding: 9px 0;
	}

	.phone .where {
		display: block;
		margin-top: 14px;
		font-size: 10.5px;
	}

	.phone .act {
		flex-direction: column;
		align-items: stretch;
		gap: 10px;
		padding-top: 18px;
	}

	.phone .write {
		width: 100%;
	}

	.phone .note {
		text-align: center;
		font-size: 10.5px;
	}

	.phone .head {
		position: sticky;
		top: 72px;
		z-index: 1;
		margin: 12px -10px 0;
		padding: 12px 10px;
		border-radius: var(--radius-16);
		background: var(--card);
	}

	.phone .head-title {
		font-size: 20px;
	}

	.phone .done {
		padding: 20px;
	}

	.phone .done-title {
		font-size: 30px;
	}

	.phone .strong-outline {
		width: 100%;
	}

	.phone .ask .keep {
		margin-left: 0;
	}

	.phone .run-row {
		grid-template-columns: minmax(0, 1fr) auto;
	}

	.phone .run-row > .small {
		grid-column: 1 / -1;
	}

	@media (prefers-reduced-motion: reduce) {
		.bar span {
			transition: none;
		}
	}
</style>
