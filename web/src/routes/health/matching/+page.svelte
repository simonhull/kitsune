<!--
	The matching queue, board C: albums whose files carry no MusicBrainz IDs, one on show with its
	releases already asked for, next and skip after each. Esc stops for now; the place is kept.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { library } from '#lib/library/library.svelte.ts';
	import { matchQueue, setMatchState } from '#lib/matching/api.ts';
	import { queuePlace, type Asked } from '#lib/matching/queue.svelte.ts';
	import { matchSheet } from '#lib/matching/sheet.svelte.ts';
	import type { MatchCandidate, MatchQueue, MatchState, QueuedAlbum } from '#lib/matching/types.ts';
	import { chipOf, factsLine, labelOf } from '#lib/matching/words.ts';

	let queue = $state.raw<MatchQueue | null>(null);
	let failed = $state(false);
	let tab = $state<MatchState>('todo');

	const albums = $derived(queue?.albums ?? []);
	const todo = $derived(albums.filter((a) => a.state === 'todo'));
	const shown = $derived(albums.filter((a) => a.state === tab));
	const counts = $derived(queue?.counts ?? { todo: 0, skipped: 0, matched: 0 });
	const total = $derived(counts.todo + counts.skipped + counts.matched);
	const current = $derived(
		albums.find((a) => a.id === queuePlace.current && a.state !== 'matched') ?? todo.at(0)
	);
	const asked = $derived(current === undefined ? undefined : queuePlace.asked[current.id]);
	const best = $derived(asked?.kind === 'ready' ? asked.candidates.at(0) : undefined);
	const position = $derived(
		current === undefined ? 0 : albums.findIndex((a) => a.id === current.id) + 1
	);

	async function load(): Promise<void> {
		try {
			queue = await matchQueue();
			failed = false;
		} catch {
			failed = true;
		}
	}

	onMount(() => {
		void load();
	});

	// The album on show, and the next ones, are asked for while the person reviews.
	$effect(() => {
		if (current !== undefined) {
			queuePlace.current = current.id;
			queuePlace.askFor(current.state === 'todo' ? todo : [current]);
		}
	});

	function stateLine(
		a: QueuedAlbum,
		ask: Asked | undefined
	): { text: string; tone: 'ready' | 'done' | 'quiet'; asking: boolean } {
		if (a.state === 'matched') {
			return { text: '✓ MATCHED', tone: 'done', asking: false };
		}
		if (a.state === 'skipped') {
			return { text: 'SKIPPED', tone: 'quiet', asking: false };
		}
		switch (ask?.kind) {
			case 'asking':
				return { text: 'ASKING MUSICBRAINZ…', tone: 'quiet', asking: true };
			case 'failed':
				return { text: 'NOT ANSWERED · ASKS AGAIN', tone: 'quiet', asking: false };
			case 'ready': {
				const top = ask.candidates.at(0);
				const lead = a.id === current?.id ? 'NEXT' : 'READY';
				return top === undefined
					? { text: `${lead} · NOTHING FOUND`, tone: 'ready', asking: false }
					: { text: `${lead} · BEST ${String(top.score)}%`, tone: 'ready', asking: false };
			}
		}
		return { text: 'WAITING', tone: 'quiet', asking: false };
	}

	/** Opens the sheet over the queue, at the review of `candidate` when one is given. */
	async function review(a: QueuedAlbum, candidate?: MatchCandidate): Promise<void> {
		matchSheet.open({
			albumId: a.id,
			from: 'queue',
			applied: (landed, files) => {
				queuePlace.done = { id: landed, title: a.title, files };
				queuePlace.forget(a.id);
				queuePlace.current = null;
				void load();
			}
		});
		if (candidate !== undefined) {
			await matchSheet.pick(candidate);
		}
	}

	async function skip(a: QueuedAlbum): Promise<void> {
		await setMatchState(a.id, 'skipped').catch(() => undefined);
		queuePlace.current = null;
		await load();
	}

	async function choose(a: QueuedAlbum): Promise<void> {
		queuePlace.current = a.id;
		if (matchMedia('(max-width: 900px)').matches) {
			await review(a);
		}
	}

	function onkeydown(event: KeyboardEvent): void {
		if (
			matchSheet.current !== null ||
			event.target instanceof HTMLInputElement ||
			current === undefined
		) {
			return;
		}
		if (event.key === 'Enter' && event.target === document.body) {
			event.preventDefault();
			void review(current, best);
		} else if (event.key.toLowerCase() === 's' && !event.metaKey && !event.ctrlKey) {
			event.preventDefault();
			void skip(current);
		} else if (event.key === 'Escape') {
			void goto(resolve('/health'));
		}
	}
</script>

<svelte:window {onkeydown} />

<svelte:head>
	<title>Albums to match · Kitsune</title>
</svelte:head>

{#if !library.admin}
	<p class="gone">Matching is for administrators.</p>
{:else}
	<div class="page-frame queue">
		<header>
			<div class="words">
				<a class="crumb" href={resolve('/health')}>HEALTH › NO MUSICBRAINZ IDS</a>
				<h1>
					{#if queue === null}Albums to match{:else}{counts.todo}
						{counts.todo === 1 ? 'album' : 'albums'} to match{/if}
				</h1>
				<p class="lede">
					Their files carry no MusicBrainz IDs. Match one, apply what you agree with, and the next
					one is already waiting. Nothing changes without you.
				</p>
			</div>
			{#if total > 0}
				<div class="progress">
					<p class="mono">
						<span
							><b>{counts.matched} MATCHED</b> · {counts.skipped} SKIPPED · {counts.todo} TO GO</span
						><span>OF {total}</span>
					</p>
					<div
						class="track"
						role="progressbar"
						aria-valuemin={0}
						aria-valuemax={total}
						aria-valuenow={counts.matched + counts.skipped}
						aria-label="Albums done"
					>
						<span class="matched" style:width={`${String((100 * counts.matched) / total)}%`}></span>
						<span class="skipped" style:width={`${String((100 * counts.skipped) / total)}%`}></span>
						<span class="rest"></span>
					</div>
				</div>
			{/if}
		</header>

		{#if failed}
			<p class="trouble" role="alert">
				The queue can’t be read just now. <button
					type="button"
					class="link"
					onclick={() => void load()}>Try again</button
				>
			</p>
		{:else if queue !== null}
			<div class="panels">
				<section class="card list" aria-label="The queue">
					<div class="tabs" role="tablist" aria-label="Show">
						{#each [['todo', 'To do', counts.todo], ['skipped', 'Skipped', counts.skipped], ['matched', 'Matched', counts.matched]] as const as [state, name, n] (state)}
							<button
								type="button"
								role="tab"
								aria-selected={tab === state}
								onclick={() => (tab = state)}>{name} {n}</button
							>
						{/each}
					</div>
					<div class="rows">
						{#each shown as a (a.id)}
							{@const line = stateLine(a, queuePlace.asked[a.id])}
							<button
								type="button"
								class="row"
								class:now={a.id === current?.id}
								class:done={a.state === 'matched'}
								onclick={() => void choose(a)}
							>
								<span class="art"
									><Cover albumId={a.id} entry={library.entry(a.id)} place="card" box={48} /></span
								>
								<span class="names">
									<span class="title">{a.title}</span>
									<span class="by">{a.artist} · {a.songs} songs</span>
								</span>
								<span class="state {line.tone}">
									{#if line.asking}<span class="dot" aria-hidden="true"></span>{/if}{line.text}
								</span>
							</button>
						{:else}
							<p class="none">
								{tab === 'todo'
									? 'Every album has its MusicBrainz IDs.'
									: tab === 'skipped'
										? 'Nothing skipped.'
										: 'Nothing matched here yet.'}
							</p>
						{/each}
					</div>
				</section>

				<section
					class="card next"
					aria-label={current === undefined ? 'Nothing to match' : `${current.title}, next`}
				>
					{#if queuePlace.done !== null}
						<div class="strip" role="status">
							<span class="tick" aria-hidden="true"
								><svg width="14" height="14" viewBox="0 0 24 24"
									><path d="M4 12.5l5 5L20 6.5" /></svg
								></span
							>
							<span class="said"
								><b>{queuePlace.done.title} is matched.</b>
								Wrote {queuePlace.done.files}
								{queuePlace.done.files === 1 ? 'file' : 'files'}, MusicBrainz IDs on each.</span
							>
							<a href={resolve('/album/[id]', { id: queuePlace.done.id })}>Open the album</a>
						</div>
					{/if}
					{#if current === undefined}
						<p class="none big">Nothing left to match. Albums matched here stay under Matched.</p>
					{:else}
						<div class="album">
							<span class="art big"
								><Cover
									albumId={current.id}
									entry={library.entry(current.id)}
									place="card"
									box={120}
								/></span
							>
							<div class="names">
								<p class="mono">
									NEXT · {position} OF {albums.length} · {current.artist.toUpperCase()} · {current.songs}
									SONGS
								</p>
								<h2>{current.title}</h2>
							</div>
							<button type="button" class="ghost" onclick={() => void skip(current)}>Skip</button>
							<button type="button" class="fox" onclick={() => void review(current, best)}>
								{best === undefined ? 'Find matches' : 'Next: review the best'}
							</button>
						</div>
						<p class="mono asked">
							{#if asked?.kind === 'ready'}ALREADY ASKED{:else if asked?.kind === 'failed'}MUSICBRAINZ
								DIDN’T ANSWER · IT ASKS AGAIN ON OPEN{:else}<span class="dot" aria-hidden="true"
								></span> ASKING MUSICBRAINZ…{/if}
						</p>
						{#if asked?.kind === 'ready'}
							{#each asked.candidates.slice(0, 3) as c, index (c.id)}
								{@const { label, catalog } = labelOf(c.release)}
								<button
									type="button"
									class="candidate"
									class:first={index === 0}
									onclick={() => void review(current, c)}
								>
									<span class="score">{c.score}<span>%</span></span>
									<span class="cover"
										>{#if c.cover !== undefined}<img
												src={c.cover}
												alt=""
												width="56"
												height="56"
											/>{/if}</span
									>
									<span class="names">
										<span class="title">{c.release.title}</span>
										<span class="by">{factsLine(c.release)}</span>
									</span>
									<span class="label">{[label, catalog].filter((s) => s !== '').join(' · ')}</span>
									<span class="chips">
										{#each c.evidence.slice(0, 2) as e, at (at)}
											{@const chip = chipOf(e)}
											<span class="chip {chip.verdict}">{chip.mark} {chip.text}</span>
										{/each}
									</span>
								</button>
							{:else}
								<p class="none">
									Nothing on MusicBrainz looks like it. Open it to search by other terms.
								</p>
							{/each}
						{/if}
						<p class="keys mono" aria-hidden="true">
							<span>ENTER · REVIEW THE BEST</span><span>S · SKIP</span><span
								>ESC · STOP FOR NOW, YOUR PLACE IS KEPT</span
							>
						</p>
					{/if}
				</section>
			</div>
		{/if}
	</div>
{/if}

<style>
	.queue {
		--frame-top: 120px;
		--frame-bottom: 160px;
	}

	button {
		font: inherit;
		color: inherit;
		cursor: pointer;
	}

	header {
		display: flex;
		align-items: flex-end;
		gap: 48px;
	}

	.words {
		flex: 1;
		min-width: 0;
	}

	.crumb,
	.mono {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
		text-decoration: none;
	}

	h1 {
		margin-top: 16px;
		font-size: var(--type-display);
		font-weight: 900;
		letter-spacing: -0.06em;
		line-height: 0.9;
	}

	.lede {
		margin-top: 16px;
		max-width: 720px;
		font-size: 17px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.progress {
		width: min(640px, 40%);
	}

	.progress .mono {
		display: flex;
		justify-content: space-between;
	}

	.progress b {
		font-weight: 400;
		color: var(--text);
	}

	.track {
		margin-top: 10px;
		display: flex;
		gap: 3px;
		height: 10px;
	}

	.matched {
		border-radius: 5px 0 0 5px;
		background: var(--fox);
	}

	.skipped {
		background: var(--edge-strong);
	}

	.rest {
		flex: 1;
		border-radius: 0 5px 5px 0;
		background: var(--input);
	}

	.panels {
		margin-top: 56px;
		display: grid;
		grid-template-columns: minmax(0, calc(620px * var(--step))) minmax(0, 1fr);
		gap: 32px;
		align-items: start;
	}

	.card {
		padding: 16px;
		border-radius: var(--radius-32);
		background: var(--card);
		border: 1px solid var(--edge);
	}

	.tabs {
		display: flex;
		gap: 6px;
		width: fit-content;
		padding: 4px;
		border-radius: var(--radius-pill);
		background: var(--input);
		border: 1px solid var(--edge);
	}

	.tabs button {
		height: 40px;
		padding: 0 16px;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text-secondary);
		font-size: 13px;
		font-weight: 600;
	}

	.tabs [aria-selected='true'] {
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	.rows {
		margin-top: 12px;
		max-height: 70dvh;
		overflow-y: auto;
	}

	.row {
		width: 100%;
		height: 68px;
		display: flex;
		align-items: center;
		gap: 14px;
		padding: 0 12px;
		border: 0;
		border-bottom: 1px solid var(--edge-card);
		background: transparent;
		text-align: left;
	}

	.row:hover {
		background: var(--hover);
	}

	.row.now {
		border-radius: 14px;
		background: color-mix(in srgb, var(--fox) 12%, transparent);
		box-shadow: inset 0 0 0 1px var(--fox-line);
	}

	.row.done {
		opacity: 0.6;
	}

	.art {
		position: relative;
		width: 48px;
		height: 48px;
		flex: none;
		overflow: hidden;
		border-radius: 10px;
		background: var(--fill);
	}

	.art.big {
		width: 120px;
		height: 120px;
		border-radius: 18px;
	}

	.names {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}

	.title {
		overflow: hidden;
		font-size: 15px;
		font-weight: 700;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.by {
		margin-top: 2px;
		overflow: hidden;
		font-size: 13px;
		color: var(--text-secondary);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.state {
		display: flex;
		align-items: center;
		gap: 8px;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.06em;
		color: var(--text-meta);
		white-space: nowrap;
	}

	.state.ready {
		color: var(--text);
	}

	.state.done {
		color: var(--fox-text);
	}

	.dot {
		display: inline-block;
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--text-meta);
		animation: ask 1.2s ease-in-out infinite;
	}

	@keyframes ask {
		0%,
		100% {
			opacity: 0.25;
		}

		50% {
			opacity: 1;
		}
	}

	.next {
		padding: 24px 28px;
	}

	.strip {
		display: flex;
		align-items: center;
		gap: 14px;
		min-height: 64px;
		margin-bottom: 28px;
		padding: 0 18px;
		border-radius: var(--radius-20);
		background: var(--input);
	}

	.strip .tick {
		width: 28px;
		height: 28px;
		flex: none;
		display: grid;
		place-items: center;
		border-radius: 50%;
		background: var(--fox);
	}

	.strip svg {
		fill: none;
		stroke: var(--fox-ink);
		stroke-width: 3.4;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	.said {
		flex: 1;
		font-size: 14px;
		color: var(--text-secondary);
	}

	.said b {
		font-size: 15px;
		color: var(--text);
	}

	.strip a {
		font-size: 14px;
		font-weight: 600;
		color: var(--fox-text);
		text-decoration: none;
	}

	.album {
		display: flex;
		align-items: flex-end;
		gap: 20px;
	}

	h2 {
		margin-top: 8px;
		overflow: hidden;
		font-size: 56px;
		font-weight: 900;
		letter-spacing: -0.05em;
		line-height: 0.95;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.ghost,
	.fox {
		height: 48px;
		padding: 0 20px;
		border-radius: var(--radius-pill);
		font-size: 14px;
		white-space: nowrap;
	}

	.ghost {
		border: 1px solid var(--edge-strong);
		background: transparent;
		font-weight: 600;
	}

	.fox {
		border: 0;
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	.asked {
		margin-top: 24px;
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 11px;
	}

	.candidate {
		width: 100%;
		display: grid;
		grid-template-columns: 64px 56px minmax(0, 1fr) 210px minmax(0, 1.3fr);
		align-items: center;
		column-gap: 16px;
		min-height: 84px;
		padding: 0 14px;
		border: 0;
		border-bottom: 1px solid var(--edge-card);
		background: transparent;
		text-align: left;
	}

	.candidate:hover,
	.candidate.first {
		border-radius: var(--radius-16);
		background: var(--hover);
	}

	.score {
		font-size: 28px;
		font-weight: 900;
		letter-spacing: -0.05em;
	}

	.score span {
		font-size: 13px;
		font-weight: 700;
		color: var(--text-meta);
	}

	.cover {
		width: 56px;
		height: 56px;
		overflow: hidden;
		border-radius: 10px;
		background: var(--fill);
	}

	.cover img {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.label {
		font-size: 13px;
		color: var(--text-secondary);
	}

	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.chip {
		height: 24px;
		line-height: 24px;
		padding: 0 10px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: var(--input);
		font-family: var(--font-mono);
		font-size: 11px;
		white-space: nowrap;
	}

	.chip.differs {
		border-style: dashed;
		background: transparent;
		color: var(--text-meta);
	}

	.chip.neutral {
		background: transparent;
		color: var(--text-secondary);
	}

	.keys {
		margin-top: 32px;
		display: flex;
		flex-wrap: wrap;
		gap: 24px;
		color: var(--text-label);
	}

	.none {
		padding: 24px 12px;
		font-size: 14px;
		color: var(--text-meta);
	}

	.none.big {
		font-size: 17px;
	}

	.trouble,
	.gone {
		margin-top: 48px;
		font-size: 15px;
		color: var(--text-secondary);
	}

	.gone {
		padding: 140px 16px;
		text-align: center;
	}

	.link {
		padding: 0;
		border: 0;
		background: none;
		font-weight: 600;
		color: var(--fox-text);
	}

	@media (max-width: 1400px) {
		.candidate {
			grid-template-columns: 56px 56px minmax(0, 1fr) minmax(0, 1fr);
		}

		.label {
			display: none;
		}

		header {
			flex-direction: column;
			align-items: stretch;
			gap: 24px;
		}

		.progress {
			width: 100%;
		}
	}

	/* A phone: the list, full width. Tapping an album opens its releases. */
	@media (max-width: 900px) {
		.queue {
			--frame-top: 98px;
			--frame-bottom: 140px;
		}

		h1 {
			font-size: 44px;
		}

		.lede {
			font-size: 15px;
		}

		.panels {
			margin-top: 28px;
			grid-template-columns: minmax(0, 1fr);
		}

		.next {
			display: none;
		}

		.card {
			padding: 12px;
			border-radius: var(--radius-24);
		}

		.rows {
			max-height: none;
		}

		.state {
			font-size: 10px;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dot {
			animation: none;
		}
	}
</style>
