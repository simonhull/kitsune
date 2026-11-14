<!--
	The releases step of the match sheet, boards A, D and F: releases best first, each with its
	confidence and the evidence for it; other terms; and what to do when nothing is found.
-->
<script lang="ts">
	import { library } from '#lib/library/library.svelte.ts';
	import { tagEditor } from '#lib/pages/tagEditor.svelte.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import { matchSheet } from './sheet.svelte.ts';
	import type { MatchCandidate } from './types.ts';
	import { chipOf, factsLine, formatLine, labelOf, scoreLook } from './words.ts';

	interface Props {
		/** One step up, beside the review: the list is a column of cards. */
		beside?: boolean;
		close: () => void;
	}

	let { beside = false, close }: Props = $props();

	const opening = $derived(matchSheet.current);
	const album = $derived(opening === null ? undefined : library.album(opening.albumId));
	const entry = $derived(opening === null ? undefined : library.entry(opening.albumId));
	const songs = $derived(album?.tracks.length ?? entry?.trackCount ?? 0);
	const discs = $derived(album?.discs.length ?? 1);
	const list = $derived(matchSheet.list);
	const candidates = $derived(list.kind === 'ready' ? list.value.candidates : []);
	const picked = $derived(
		matchSheet.review.kind === 'ready' ? matchSheet.review.value.candidate.id : null
	);

	let cursor = $state(0);
	let asking = $state(false);
	let artist = $state('');
	let title = $state('');
	let code = $state('');
	let rows: HTMLElement[] = $state([]);

	const eyebrow = $derived(
		[
			'FIND MATCHES',
			(entry?.title ?? '').toUpperCase(),
			`${String(songs)} ${songs === 1 ? 'SONG' : 'SONGS'}`,
			...(discs > 1 ? [`${String(discs)} DISCS`] : [])
		]
			.filter((part) => part !== '')
			.join(' · ')
	);

	function openTerms(): void {
		const asked = list.kind === 'ready' ? list.value.asked : undefined;
		artist = matchSheet.terms.artist ?? asked?.artist ?? entry?.artist ?? '';
		title = matchSheet.terms.album ?? asked?.album ?? entry?.title ?? '';
		code = matchSheet.terms.code ?? '';
		asking = true;
	}

	function search(event: SubmitEvent): void {
		event.preventDefault();
		asking = false;
		void matchSheet.find({ artist: artist.trim(), album: title.trim(), code: code.trim() });
	}

	function pick(c: MatchCandidate): void {
		void matchSheet.pick(c);
	}

	/** ↑ ↓ move, Enter reviews. */
	function onkeydown(event: KeyboardEvent): void {
		if (candidates.length === 0 || event.target instanceof HTMLInputElement) {
			return;
		}
		if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
			event.preventDefault();
			cursor =
				(cursor + (event.key === 'ArrowDown' ? 1 : -1) + candidates.length) % candidates.length;
			rows[cursor]?.focus();
		}
	}

	/** The editor opens over the page, on this album; from the editor itself, it's already there. */
	function editByHand(): void {
		const was = opening;
		close();
		if (was !== null && was.from !== 'editor') {
			tagEditor.open({ kind: 'album', albumId: was.albumId }, true);
		}
	}

	const retryLine = (seconds: number): string =>
		seconds <= 60 ? `Try again in ${String(seconds)} s.` : 'Try again in a minute.';
</script>

{#snippet cover(c: MatchCandidate, size: number)}
	<span class="cover" style:--size={`${String(size)}px`}>
		{#if c.cover !== undefined}<img
				src={c.cover}
				alt=""
				loading="lazy"
				width={size}
				height={size}
			/>{/if}
	</span>
{/snippet}

{#snippet chips(c: MatchCandidate, short: boolean)}
	<span class="chips">
		{#each c.evidence as e, index (index)}
			{@const chip = chipOf(e, short)}
			<span class="chip {chip.verdict}">{chip.mark} {chip.text}</span>
		{/each}
	</span>
{/snippet}

{#snippet score(c: MatchCandidate)}
	{@const look = scoreLook(c.score)}
	<span class="score">
		<span class="number">{c.score}<span class="percent">%</span></span>
		<span class="bar" aria-hidden="true"
			><span class:fox={look.fox} style:width={`${String(c.score)}%`}></span></span
		>
	</span>
{/snippet}

<div class="list" class:beside {onkeydown} role="presentation">
	<header>
		<div class="words">
			<p class="eyebrow">{eyebrow}</p>
			<h2 id="match-heading">
				{#if list.kind === 'ready' && candidates.length === 0}
					Nothing on MusicBrainz looks like this album
				{:else if list.kind === 'ready'}
					{candidates.length}
					{candidates.length === 1 ? 'release' : 'releases'}<span class="where"
						>&nbsp;on MusicBrainz</span
					>
				{:else if list.kind === 'failed'}
					MusicBrainz can’t be reached
				{:else}
					Asking MusicBrainz…
				{/if}
			</h2>
			<p class="sub">
				{#if list.kind === 'ready' && candidates.length === 0}
					Asked for <b
						>{list.value.asked.artist ||
							list.value.asked.catalogNumber ||
							list.value.asked.barcode}</b
					>
					{#if list.value.asked.album !== ''}· <b>{list.value.asked.album}</b>{/if} · {songs} songs. Its
					tags may be off, or the release isn’t there yet.
				{:else if beside}
					Pick one; its review opens beside the list. Nothing is written until you Apply.
				{:else}
					<span class="long">Asked by album artist, title, song count and lengths.</span> Pick one to
					see what would change; nothing is written until you Apply.
				{/if}
			</p>
		</div>
		{#if !(list.kind === 'ready' && candidates.length === 0)}
			<button
				type="button"
				class="ghost terms"
				aria-expanded={asking}
				onclick={() => (asking ? (asking = false) : openTerms())}>Search by other terms</button
			>
		{/if}
		{#if !beside}
			<button type="button" class="close" aria-label="Close" onclick={close}
				><Icon name="close" size={16} /></button
			>
		{/if}
	</header>

	{#if asking}
		<form class="ask" onsubmit={search}>
			<label><span>Artist</span><input bind:value={artist} autocomplete="off" /></label>
			<label><span>Album</span><input bind:value={title} autocomplete="off" /></label>
			<label
				><span>Catalog number or barcode</span><input
					bind:value={code}
					placeholder="From the spine or the back"
					autocomplete="off"
				/></label
			>
			<button type="submit" class="fox">Search again</button>
		</form>
	{/if}

	{#if list.kind === 'loading'}
		<p class="asking" role="status">
			<span class="dot" aria-hidden="true"></span>Asking MusicBrainz…
		</p>
	{:else if list.kind === 'failed'}
		<div class="trouble" role="alert">
			<p>
				{#if list.failure.kind === 'unavailable'}
					It didn’t answer, or it’s busy. {retryLine(list.failure.retryAfterSeconds)}
				{:else if list.failure.kind === 'gone'}
					This album isn’t in the library any more.
				{:else}
					Only an administrator can find matches.
				{/if}
			</p>
			{#if list.failure.kind === 'unavailable'}
				<button type="button" class="ghost" onclick={() => void matchSheet.find(matchSheet.terms)}
					>Try again</button
				>
			{/if}
		</div>
	{:else if candidates.length === 0}
		<div class="ways">
			<section aria-labelledby="way-terms">
				<p class="mono" id="way-terms">1 · SEARCH BY OTHER TERMS</p>
				<p class="note">Change what Kitsune asks for. Your files don’t change.</p>
				{#if !asking}<button type="button" class="fox wide" onclick={openTerms}
						>Change the terms</button
					>{/if}
			</section>
			<section aria-labelledby="way-sound">
				<p class="mono" id="way-sound">2 · MATCH BY SOUND</p>
				<p class="note">
					Kitsune listens to the first two minutes of each song and asks AcoustID what they are. It
					works when the tags are wrong or empty.
				</p>
				<p class="later">Comes in a later version.</p>
			</section>
			<section aria-labelledby="way-hand">
				<p class="mono" id="way-hand">3 · EDIT BY HAND</p>
				<p class="note">
					You know this album better than any database. A later match starts from what you write.
				</p>
				<button type="button" class="ghost wide" onclick={editByHand}>Open the tag editor</button>
			</section>
		</div>
	{:else}
		<div class="head" aria-hidden="true">
			<span>MATCH</span><span></span><span>RELEASE</span><span class="wide-only">DATE</span><span
				class="wide-only">COUNTRY</span
			><span class="wide-only">LABEL</span><span class="wide-only">FORMAT</span><span
				class="wide-only songs">SONGS</span
			><span>WHY</span><span></span>
		</div>
		<div class="rows" role="listbox" aria-labelledby="match-heading">
			{#each candidates as c, index (c.id)}
				{@const { label, catalog } = labelOf(c.release)}
				<button
					type="button"
					role="option"
					class="row"
					class:dim={scoreLook(c.score).dim}
					class:picked={picked === c.id}
					aria-selected={picked === c.id || (picked === null && index === cursor)}
					bind:this={rows[index]}
					onfocus={() => (cursor = index)}
					onclick={() => {
						pick(c);
					}}
				>
					{@render score(c)}
					{@render cover(c, beside ? 110 : 88)}
					<span class="release">
						<span class="title">{c.release.title}</span>
						<span class="artist" class:off={c.evidence.some((e) => e.kind === 'artist')}
							>{c.release.artist}</span
						>
						<span class="facts">{factsLine(c.release)}</span>
						<span class="facts mono"
							>{[label, catalog]
								.filter((s) => s !== '')
								.join(' · ')
								.toUpperCase()}</span
						>
						<span class="narrow-chips">{@render chips(c, true)}</span>
					</span>
					<span class="mono wide-only">{c.release.date ?? ''}</span>
					<span class="mono wide-only">{c.release.country ?? ''}</span>
					<span class="label wide-only"
						><span>{label}</span><span class="mono">{catalog}</span></span
					>
					<span class="wide-only">{formatLine(c.release.media)}</span>
					<span class="mono wide-only songs">{c.release.trackCount}</span>
					<span class="why">{@render chips(c, false)}</span>
					<span class="go" aria-hidden="true"
						><svg width="16" height="16" viewBox="0 0 24 24"><path d="M9 5l7 7-7 7" /></svg></span
					>
				</button>
			{/each}
		</div>
		<p class="keys mono" aria-hidden="true">
			<span>MUSICBRAINZ · ANSWERS KEPT FOR A DAY</span>
			<span class="hint">↑ ↓ TO MOVE · ENTER TO REVIEW · ESC TO CLOSE</span>
		</p>
	{/if}
</div>

<style>
	.list {
		container: list / inline-size;
		display: flex;
		flex-direction: column;
		min-height: 0;
	}

	header {
		display: flex;
		align-items: flex-end;
		gap: 16px;
	}

	.words {
		flex: 1;
		min-width: 0;
	}

	.eyebrow,
	.mono,
	.head,
	.keys {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	h2 {
		margin-top: 10px;
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
		line-height: 1;
	}

	.sub {
		margin-top: 10px;
		font-size: 14px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.sub b {
		color: var(--text);
	}

	button {
		font: inherit;
		color: inherit;
		cursor: pointer;
	}

	.ghost {
		height: var(--target);
		padding: 0 18px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: transparent;
		font-size: 14px;
		font-weight: 600;
		white-space: nowrap;
	}

	.ghost:hover {
		background: var(--hover);
	}

	.fox {
		height: 48px;
		padding: 0 22px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font-size: 14px;
		font-weight: 700;
	}

	.wide {
		width: 100%;
		margin-top: auto;
	}

	.close {
		width: var(--target);
		height: var(--target);
		flex: none;
		display: grid;
		place-items: center;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--input);
	}

	.ask {
		margin-top: 20px;
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr)) auto;
		align-items: end;
		gap: 12px;
	}

	.ask label {
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 13px;
		font-weight: 600;
		color: var(--text-secondary);
	}

	input {
		height: 48px;
		box-sizing: border-box;
		padding: 0 18px;
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: var(--input);
		color: var(--text);
		font: inherit;
		font-size: 15px;
		outline: none;
	}

	input:focus {
		border-color: var(--fox-line);
	}

	input::placeholder {
		color: var(--text-meta);
	}

	.asking {
		margin: 40px 16px;
		display: flex;
		align-items: center;
		gap: 10px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--fox);
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

	.trouble {
		margin: 32px 0;
		display: flex;
		align-items: center;
		gap: 16px;
		font-size: 15px;
		color: var(--text-secondary);
	}

	.ways {
		margin-top: 28px;
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 24px;
	}

	.ways section {
		display: flex;
		flex-direction: column;
		gap: 14px;
		min-height: 220px;
		padding: 24px;
		border-radius: var(--radius-24);
		background: var(--input);
	}

	.note {
		font-size: 14px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.later {
		margin-top: auto;
		padding: 16px 18px;
		border-radius: 18px;
		border: 1px dashed var(--edge-strong);
		font-size: 13px;
		color: var(--text-meta);
	}

	.head,
	.row {
		display: grid;
		grid-template-columns:
			84px 88px minmax(0, 1fr) 112px 72px 200px 120px 56px minmax(0, 2fr)
			44px;
		align-items: center;
		column-gap: 20px;
	}

	.head {
		margin-top: 24px;
		height: 36px;
		padding: 0 16px;
		border-bottom: 1px solid var(--edge-strong);
		font-size: 11px;
	}

	.songs {
		text-align: right;
	}

	.rows {
		min-height: 0;
		overflow-y: auto;
	}

	.row {
		width: 100%;
		min-height: 120px;
		padding: 14px 16px;
		box-sizing: border-box;
		border: 0;
		border-bottom: 1px solid var(--edge-card);
		background: transparent;
		text-align: left;
		transition: background-color var(--morph-fast) var(--curve-morph);
	}

	.row:hover,
	.row:focus-visible,
	.row[aria-selected='true'] {
		border-radius: 18px;
		background: var(--hover);
		outline: none;
	}

	.row:focus-visible {
		box-shadow: inset 0 0 0 2px var(--fox-line);
	}

	.row.picked {
		box-shadow: inset 0 0 0 2px var(--fox-line);
		border-radius: 18px;
	}

	.row.dim {
		opacity: 0.72;
	}

	.score {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.number {
		font-size: 36px;
		font-weight: 900;
		letter-spacing: -0.05em;
		line-height: 0.9;
	}

	.percent {
		font-size: 16px;
		font-weight: 700;
		color: var(--text-meta);
	}

	.bar {
		display: block;
		width: 72px;
		height: 4px;
		border-radius: 2px;
		background: var(--edge-strong);
	}

	.bar span {
		display: block;
		height: 4px;
		border-radius: 2px;
		background: var(--text-meta);
	}

	.bar .fox {
		background: var(--fox);
	}

	.cover {
		display: block;
		width: var(--size);
		height: var(--size);
		flex: none;
		overflow: hidden;
		border-radius: 14px;
		background: var(--fill);
	}

	.cover img {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.release {
		min-width: 0;
		display: flex;
		flex-direction: column;
	}

	.title {
		overflow: hidden;
		font-size: 20px;
		font-weight: 700;
		letter-spacing: var(--tracking-title);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.artist {
		margin-top: 4px;
		font-size: 14px;
		color: var(--text-secondary);
	}

	.artist.off {
		color: var(--fox-text);
	}

	.facts,
	.narrow-chips {
		display: none;
	}

	.label {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 3px;
		font-size: 14px;
		font-weight: 600;
	}

	.label span:first-child {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.label .mono {
		font-weight: 400;
		letter-spacing: 0;
	}

	.mono.wide-only {
		font-size: 13px;
		letter-spacing: 0;
		color: var(--text);
	}

	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.chip {
		display: inline-flex;
		align-items: center;
		height: 26px;
		padding: 0 10px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: var(--input);
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.04em;
		white-space: nowrap;
	}

	.chip.neutral {
		border-color: var(--edge);
		background: transparent;
		color: var(--text-secondary);
	}

	.chip.differs {
		border-style: dashed;
		background: transparent;
		color: var(--text-meta);
	}

	.go {
		display: grid;
		width: var(--target);
		height: var(--target);
		place-items: center;
	}

	.go svg {
		fill: none;
		stroke: var(--text-secondary);
		stroke-width: 2;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	.keys {
		display: flex;
		flex-wrap: wrap;
		gap: 24px;
		padding: 16px 16px 0;
		color: var(--text-label);
	}

	.hint {
		margin-left: auto;
	}

	/* Narrower: the facts gather under the title, and the columns go. */
	@container list (max-width: 1400px) {
		.head,
		.row {
			grid-template-columns: 72px 88px minmax(0, 1fr) minmax(0, 1fr) 32px;
		}

		.wide-only {
			display: none;
		}

		.facts {
			display: block;
			margin-top: 4px;
			overflow: hidden;
			font-size: 13px;
			color: var(--text-secondary);
			text-overflow: ellipsis;
			white-space: nowrap;
		}

		.facts.mono {
			font-size: 11px;
			color: var(--text-meta);
		}
	}

	/* One step up, beside the review: cards. */
	.beside .head {
		display: none;
	}

	.beside .rows {
		margin-top: 16px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.beside .row {
		grid-template-columns: 100px 110px minmax(0, 1fr);
		align-items: start;
		column-gap: 22px;
		padding: 22px;
		border-radius: 26px;
		border: 0;
		box-shadow: inset 0 0 0 1px var(--edge);
	}

	.beside .row.picked {
		box-shadow: inset 0 0 0 2px var(--fox-line);
	}

	.beside .wide-only,
	.beside .why,
	.beside .go {
		display: none;
	}

	.beside .facts,
	.beside .narrow-chips {
		display: block;
	}

	.beside .narrow-chips {
		margin-top: 14px;
	}

	/* A phone: cards, and the long words go. */
	@container list (max-width: 640px) {
		header {
			align-items: flex-start;
		}

		h2 {
			margin-top: 6px;
			font-size: 26px;
			letter-spacing: -0.03em;
		}

		.where,
		.long {
			display: none;
		}

		.terms {
			display: none;
		}

		.sub {
			font-size: 13px;
		}

		.ask,
		.ways {
			grid-template-columns: minmax(0, 1fr);
		}

		.head {
			display: none;
		}

		.rows {
			margin-top: 12px;
			display: flex;
			flex-direction: column;
			gap: 8px;
		}

		.row {
			grid-template-columns: 64px minmax(0, 1fr) auto;
			grid-template-areas: 'cover release score';
			align-items: start;
			column-gap: 12px;
			min-height: 0;
			padding: 12px;
			border: 0;
			border-radius: 20px;
			background: var(--input);
		}

		.row .cover {
			grid-area: cover;
			--size: 64px !important;

			border-radius: 12px;
		}

		.release {
			grid-area: release;
		}

		.score {
			grid-area: score;
		}

		.number {
			font-size: 20px;
			letter-spacing: -0.04em;
		}

		.percent {
			font-size: 11px;
		}

		.bar,
		.why,
		.go,
		.keys {
			display: none;
		}

		.title {
			font-size: 16px;
		}

		.artist {
			display: none;
		}

		.facts {
			font-size: 12px;
		}

		.narrow-chips {
			display: block;
			margin-top: 8px;
		}

		.narrow-chips .chip {
			height: 22px;
			padding: 0 8px;
			font-size: 10px;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dot {
			animation: none;
		}
	}
</style>
