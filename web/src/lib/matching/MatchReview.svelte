<!--
	The review step of the match sheet, boards B, F and G: every field that would change, current
	against proposed, ticked field by field and song by song; the pairing, which a person can change;
	the cover beside the album's; and the IDs, always written with Apply.
-->
<script lang="ts">
	import Cover from '#lib/cover/Cover.svelte';
	import { library } from '#lib/library/library.svelte.ts';
	import Floating from '#lib/pages/Floating.svelte';
	import { clock } from '#lib/library/format.ts';
	import {
		fieldRows,
		kindStates,
		pairOptions,
		repair,
		setChange,
		setField,
		setFields,
		setKind,
		setSong,
		songKinds,
		songRows,
		summaryOf,
		type Change,
		type FieldRow,
		type SongRow
	} from './review.ts';
	import { matchSheet, type Held } from './sheet.svelte.ts';
	import Tick from './Tick.svelte';
	import { factsLine, labelOf, reviewLine } from './words.ts';

	interface Props {
		held: Held;
		/** A phone: stacked, with switches. */
		narrow: boolean;
		/** One step up, beside the list: no way back is needed. */
		beside?: boolean;
	}

	let { held, narrow, beside = false }: Props = $props();

	const choices = $derived(matchSheet.choices);
	const fields = $derived(choices === null ? { rows: [], agreeing: [] } : fieldRows(held, choices));
	const songs = $derived(choices === null ? [] : songRows(held, choices));
	const kinds = $derived(kindStates(songs));
	const summary = $derived(choices === null ? null : summaryOf(held, choices));
	const release = $derived(held.candidate.release);
	const albumId = $derived(matchSheet.current?.albumId ?? '');
	const entry = $derived(library.entry(albumId));
	const releases = $derived(
		matchSheet.list.kind === 'ready' ? matchSheet.list.value.candidates.length : 0
	);
	const paired = $derived(songs.filter((s) => s.release !== null && !s.suspect).length);
	const fieldsOn = $derived(fields.rows.filter((r) => r.on).length);
	const discs = $derived(new Set(songs.map((s) => s.disc)).size > 1);
	let showAll = $state(false);
	/** The song whose pair is being chosen, and where its menu opens. */
	let pairing = $state<{ id: string; x: number; y: number } | null>(null);

	function update(next: typeof choices): void {
		matchSheet.choices = next;
	}

	/** A length in milliseconds, as m:ss. */
	const length = (ms: number): string => clock(Math.round(ms / 1000));
	const plural = (n: number, one: string): string => `${String(n)} ${one}${n === 1 ? '' : 's'}`;

	const headline = $derived.by(() => {
		if (summary === null) {
			return '';
		}
		const parts = [
			...(summary.fields > 0 ? [plural(summary.fields, 'album field')] : []),
			...(summary.songChanges > 0 ? [plural(summary.songChanges, 'song change')] : [])
		];
		const what = parts.length === 0 ? 'MusicBrainz IDs' : `${parts.join(', ')} and MusicBrainz IDs`;
		return summary.files === 0
			? 'Nothing ticked yet'
			: `${what}, to ${summary.files === summary.of ? plural(summary.files, 'file') : `${String(summary.files)} of ${plural(summary.of, 'file')}`}`;
	});
	/** From the tag editor, Apply fills its fields and writes nothing. */
	const filling = $derived(matchSheet.current?.fill !== undefined);
	const footnote = $derived.by(() => {
		if (summary === null) {
			return '';
		}
		const waiting =
			summary.waiting === 0
				? ''
				: `${plural(summary.waiting, 'song')} ${summary.waiting === 1 ? 'stays' : 'stay'} as ${summary.waiting === 1 ? 'it is' : 'they are'}. `;
		const split = summary.splits
			? 'They keep the album name they have, so they’d show as an album of their own. '
			: '';
		return filling
			? `${waiting}${split}Fills the editor’s fields: nothing is written until you write them.`
			: `${waiting}${split}Writes through the tag editor, so the Library follows.`;
	});

	const two = (n: number): string => String(n).padStart(2, '0');
	const releasePlace = (s: SongRow): string =>
		s.release === null ? '—' : `${String(s.release.disc)}·${two(s.release.position)}`;
	const gapOf = (s: SongRow): string => {
		const theirs = s.release?.lengthMs;
		if (theirs === undefined) {
			return '';
		}
		const seconds = Math.round((theirs - s.lengthMs) / 1000);
		const sign = seconds < 0 ? '−' : '+';
		return `${sign}${length(Math.abs(seconds) * 1000)}`;
	};
	const lengths = (s: SongRow): string =>
		s.release?.lengthMs === undefined
			? length(s.lengthMs)
			: `${length(s.lengthMs)} · ${length(s.release.lengthMs)}`;

	function change(s: SongRow, kind: string): Change | undefined {
		return s.changes.find((c) => c.kind === kind);
	}

	/** The rows a phone shows: those that change or look wrong, unless asked for all. */
	const shown = $derived(
		narrow && !showAll
			? songs.filter((s) => s.changes.length > 0 || s.suspect || s.release === null)
			: songs
	);

	function rowState(row: FieldRow): 'all' | 'none' {
		return row.on ? 'all' : 'none';
	}

	function openPairs(event: MouseEvent, id: string): void {
		const box = (event.currentTarget as HTMLElement).getBoundingClientRect();
		pairing = { id, x: box.right - 380, y: box.bottom + 6 };
	}

	const pairingSong = $derived(
		pairing === null ? undefined : songs.find((s) => s.id === pairing?.id)
	);
</script>

{#snippet value(values: string[] | null, empty: string)}
	{#if values === null}<i>several</i>{:else if values.length === 0}<i>{empty}</i
		>{:else}{values.join(', ')}{/if}
{/snippet}

{#snippet diff(c: Change, s: SongRow)}
	<button
		type="button"
		class="diff"
		class:on={c.on && s.on}
		aria-pressed={c.on && s.on}
		title={c.on ? 'Keep yours' : 'Take MusicBrainz’s'}
		onclick={() => {
			if (choices !== null) {
				update(setChange(choices, s.id, c.kind, !c.on));
			}
		}}
	>
		<span class="now">{c.now || '—'}</span><span class="arrow">→</span><span class="next"
			>{c.next}</span
		>
	</button>
{/snippet}

<div class="review" class:narrow class:beside>
	<header>
		{#if !beside}
			<button type="button" class="ghost back" onclick={() => matchSheet.back()}>
				<svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true"
					><path d="M15 5l-7 7 7 7" /></svg
				>
				{releases}
				{releases === 1 ? 'release' : 'releases'}
			</button>
		{/if}
		<span class="cover">
			{#if held.candidate.cover !== undefined}<img
					src={held.candidate.cover}
					alt=""
					width="56"
					height="56"
				/>{/if}
		</span>
		<div class="title">
			<p class="eyebrow">REVIEW · {reviewLine(held.candidate)}</p>
			<div class="line">
				<h2 id="review-heading">{release.title}</h2>
				<span class="facts"
					>{[release.artist, labelOf(release).label, factsLine(release)]
						.filter((s) => s !== '')
						.join(' · ')}</span
				>
			</div>
		</div>
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- MusicBrainz's own page, not a route. -->
		<a class="page" href={held.candidate.page} target="_blank" rel="noopener noreferrer"
			>Open on MusicBrainz ↗</a
		>
	</header>

	{#if choices !== null && summary !== null}
		<div class="body">
			<div class="side">
				<section class="well" aria-label="Album fields">
					<div class="well-head">
						{#if !narrow}
							<Tick
								state={fieldsOn === 0 ? 'none' : fieldsOn === fields.rows.length ? 'all' : 'some'}
								label="Every album field"
								disabled={fields.rows.length === 0}
								toggle={(on) => {
									update(setFields(held, choices, on));
								}}
							/>
						{/if}
						<span class="grow"
							>ALBUM · {fieldsOn} OF {plural(fields.rows.length, 'CHANGE').toUpperCase()}</span
						>
						<span>{narrow ? 'USE' : 'NOW → PROPOSED'}</span>
					</div>
					{#each fields.rows as row (row.key)}
						<div class="field" class:on={row.on}>
							{#if !narrow}
								<Tick
									state={rowState(row)}
									label={`Use the proposed ${row.label.toLowerCase()}`}
									toggle={(on) => {
										update(setField(choices, row.key, on));
									}}
								/>
							{/if}
							<span class="field-words">
								<span class="label">{row.label.toUpperCase()}</span>
								<span class="change">
									<span class="now">{@render value(row.now, 'empty')}</span>
									{#if !narrow}<span class="arrow">→</span>{/if}
									<span class="next">{row.next.join(', ')}</span>
								</span>
								{#if row.note !== undefined}<span class="note">{row.note}</span>{/if}
							</span>
							{#if narrow}
								<Tick
									switch
									state={rowState(row)}
									label={`Use the proposed ${row.label.toLowerCase()}`}
									toggle={(on) => {
										update(setField(choices, row.key, on));
									}}
								/>
							{/if}
						</div>
					{/each}
					{#if fields.agreeing.length > 0}
						<p class="agree">
							{fields.agreeing.join(', ').replace(/, ([^,]*)$/, ' and $1')}
							{fields.agreeing.length === 1 ? 'already agrees' : 'already agree'}.
						</p>
					{/if}
				</section>

				<section class="well covers" aria-label="Cover">
					<div>
						<span class="art"><Cover {albumId} {entry} place="card" box={200} /></span>
						<p class="caption">Now</p>
					</div>
					<div>
						<span class="art theirs">
							{#if held.candidate.cover !== undefined}<img
									src={held.candidate.cover}
									alt="The release’s cover"
								/>{/if}
						</span>
						<p class="caption">Cover Art Archive</p>
						<p class="mono">TO COMPARE · KITSUNE DOESN’T WRITE COVERS YET</p>
					</div>
				</section>

				<section class="ids mono" aria-label="MusicBrainz IDs">
					<p class="ids-head">ALSO WRITTEN, ALWAYS: MUSICBRAINZ IDS</p>
					<p>release {release.id}</p>
					{#if release.groupId !== undefined}<p>release group {release.groupId}</p>{/if}
					<p>artist and album artist, where they line up</p>
					<p>
						recording and track · {summary.files === summary.of
							? `all ${String(summary.of)} songs`
							: `${plural(summary.files, 'song')}, not the ${String(summary.waiting)} left as they are`}
					</p>
				</section>
			</div>

			<section class="well songs" aria-label="Songs and their pairs">
				<div class="songs-head">
					<span class="mono">SONGS · {paired} OF {songs.length} PAIRED</span>
					{#if !narrow}
						{#each songKinds as { kind, name } (kind)}
							{#if kinds[kind].total > 0}
								<span class="pill">
									<Tick
										state={kinds[kind].state}
										size={14}
										label={`Use the proposed ${name.toLowerCase()}`}
										toggle={(on) => {
											update(setKind(held, choices, kind, on));
										}}
									/>
									{name}<span class="count"
										>{kinds[kind].state === 'some'
											? `${String(kinds[kind].on)} of ${String(kinds[kind].total)}`
											: kinds[kind].total}</span
									>
								</span>
							{/if}
						{/each}
						<span class="hint">Re-pair a song from its ⋯</span>
					{/if}
				</div>
				{#if !narrow}
					<div class="cols mono" aria-hidden="true">
						<span></span><span>PAIR</span><span>TITLE</span><span>ARTIST</span><span class="right"
							>LENGTHS</span
						><span></span>
					</div>
				{/if}
				{#each shown as s, index (s.id)}
					{#if discs && !narrow && (index === 0 || shown[index - 1]?.disc !== s.disc)}
						{@const name =
							s.release === null
								? undefined
								: held.review.tracks.find((t) => t.disc === s.disc)?.proposed.discSubtitle?.at(0)}
						<div class="disc">
							<span>Disc {s.disc}</span>{#if name !== undefined}<span class="sub">{name}</span>{/if}
						</div>
					{/if}
					{@const title = change(s, 'title')}
					{@const artists = change(s, 'artists')}
					{@const numbers = change(s, 'numbers')}
					<div class="song" class:suspect={s.suspect} class:off={!s.on}>
						{#if !narrow}
							<Tick
								state={s.on ? 'all' : 'none'}
								label={`Write ${s.title}`}
								disabled={s.release === null}
								toggle={(on) => {
									update(setSong(choices, s.id, on));
								}}
							/>
							<span class="mono pair">{s.position} → {releasePlace(s)}</span>
						{:else}
							<span class="mono pair">{s.position}</span>
						{/if}
						<span class="cell title">
							{#if title !== undefined}{@render diff(title, s)}{:else}<span class="plain"
									>{s.title}</span
								>{/if}
							{#if narrow && s.suspect}<span class="flag"
									>PAIRED WITH {s.release?.title.toUpperCase()} · {gapOf(s)}</span
								>{/if}
							{#if narrow && s.release === null}<span class="flag">NO PAIR · STAYS AS IT IS</span
								>{/if}
						</span>
						{#if !narrow}
							<span class="cell artist">
								{#if artists !== undefined}{@render diff(artists, s)}{:else}<span class="plain"
										>{s.artists}</span
									>{/if}
								{#if numbers !== undefined}<span class="numbers">{@render diff(numbers, s)}</span
									>{/if}
							</span>
							<span class="mono right lengths" class:warn={s.suspect}>{lengths(s)}</span>
						{/if}
						<span class="end">
							{#if !narrow && s.suspect}<span class="flag">{gapOf(s)} · WRONG PAIR?</span>{/if}
							{#if !narrow && s.release === null}<span class="flag">NO PAIR</span>{/if}
							{#if narrow && (s.suspect || s.release === null)}
								<button
									type="button"
									class="ghost repair"
									onclick={(event) => openPairs(event, s.id)}>Re-pair</button
								>
							{:else if narrow}
								<Tick
									switch
									state={s.on ? 'all' : 'none'}
									label={`Write ${s.title}`}
									toggle={(on) => {
										update(setSong(choices, s.id, on));
									}}
								/>
							{:else}
								<button
									type="button"
									class="more"
									class:open={pairing?.id === s.id}
									aria-label={`Pair ${s.title} with another song`}
									onclick={(event) => openPairs(event, s.id)}
									><svg width="14" height="14" viewBox="0 0 24 24" aria-hidden="true"
										><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle
											cx="19"
											cy="12"
											r="1.8"
										/></svg
									></button
								>
							{/if}
						</span>
					</div>
				{/each}
				{#if narrow && shown.length < songs.length}
					<p class="agree">
						The other {songs.length - shown.length} songs agree; only their IDs and ISRCs are written.
						<button type="button" class="link" onclick={() => (showAll = true)}
							>Show all {songs.length}</button
						>
					</p>
				{/if}
			</section>
		</div>

		<div class="bar">
			{#if matchSheet.report !== null}
				<div class="report" role="alert">
					<p>{matchSheet.report.message}</p>
					{#if matchSheet.report.detail !== undefined}<p class="detail">
							{matchSheet.report.detail}
						</p>{/if}
				</div>
			{:else}
				<span class="summary" aria-live="polite"
					><span class="what">{headline}</span><span class="foot">{footnote}</span></span
				>
			{/if}
			{#if !beside && !narrow}<button type="button" class="ghost" onclick={() => matchSheet.back()}
					>Back</button
				>{/if}
			<button
				type="button"
				class="apply"
				disabled={summary.files === 0 || matchSheet.applying || matchSheet.report?.retry === false}
				onclick={() => void matchSheet.apply()}
			>
				{#if matchSheet.applying}Writing…{:else if filling}Fill {plural(
						summary.files,
						'song'
					)}{:else}Apply to {plural(summary.files, 'file')}{/if}
			</button>
		</div>
	{:else}
		<p class="asking" role="status">
			<span class="dot" aria-hidden="true"></span>Reading the files…
		</p>
	{/if}
</div>

{#if pairing !== null && pairingSong !== undefined && choices !== null}
	<Floating
		x={pairing.x}
		y={pairing.y}
		label={`Pair ${pairingSong.title} with`}
		width={380}
		onclose={() => (pairing = null)}
	>
		<p class="menu-head">
			PAIR “{pairingSong.title.toUpperCase()}” {length(pairingSong.lengthMs)} WITH
		</p>
		{#each pairOptions(held, pairingSong.id).slice(0, 5) as option (option.index)}
			{@const agrees = option.gapMs !== null && Math.abs(option.gapMs) <= 3000}
			<button
				type="button"
				role="menuitem"
				data-menu-item
				class="option"
				class:current={pairingSong.pair === option.index}
				onclick={() => {
					update(repair(held, choices, pairingSong.id, option.index));
					pairing = null;
				}}
			>
				<span class="mono">{String(option.track.disc)}·{two(option.track.position)}</span>
				<span class="option-title">{option.track.title}</span>
				<span class="mono">
					{option.track.lengthMs === undefined ? '' : length(option.track.lengthMs)}
					{agrees
						? '· AGREES'
						: option.gapMs === null
							? ''
							: `· ${option.gapMs < 0 ? '−' : '+'}${length(Math.abs(option.gapMs))}`}
				</span>
			</button>
		{/each}
		<div class="rule"></div>
		<button
			type="button"
			role="menuitem"
			data-menu-item
			class="option quiet"
			onclick={() => {
				update(repair(held, choices, pairingSong.id, null));
				pairing = null;
			}}>Leave this song as it is</button
		>
		<p class="menu-note">
			Pairing it with a song another holds swaps the two, track numbers too, so the files follow the
			release.
		</p>
	</Floating>
{/if}

<style>
	.review {
		container: review / inline-size;
		display: flex;
		flex-direction: column;
		min-height: 0;
		height: 100%;
	}

	button {
		font: inherit;
		color: inherit;
		cursor: pointer;
	}

	header {
		display: flex;
		align-items: center;
		gap: 20px;
	}

	.ghost {
		height: var(--target);
		padding: 0 18px;
		display: inline-flex;
		align-items: center;
		gap: 6px;
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

	.back {
		padding-left: 12px;
	}

	.back svg {
		fill: none;
		stroke: currentColor;
		stroke-width: 2;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	header .cover {
		width: 56px;
		height: 56px;
		flex: none;
		overflow: hidden;
		border-radius: var(--radius-12);
		background: var(--fill);
	}

	header .cover img,
	.art img {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.title {
		flex: 1;
		min-width: 0;
	}

	.eyebrow,
	.mono {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.line {
		margin-top: 6px;
		display: flex;
		align-items: baseline;
		gap: 14px;
		min-width: 0;
	}

	h2 {
		flex: none;
		max-width: 60%;
		overflow: hidden;
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
		line-height: 1;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.facts {
		overflow: hidden;
		font-size: 15px;
		color: var(--text-secondary);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.page {
		font-size: 14px;
		font-weight: 600;
		color: var(--fox-text);
		text-decoration: none;
		white-space: nowrap;
	}

	.body {
		flex: 1;
		min-height: 0;
		margin-top: 24px;
		display: grid;
		grid-template-columns: 520px minmax(0, 1fr);
		column-gap: 32px;
		align-items: start;
		overflow-y: auto;
		padding-bottom: 24px;
	}

	.side {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.well {
		padding: 14px 20px 8px;
		border-radius: var(--radius-24);
		background: var(--input);
	}

	.well-head {
		display: flex;
		align-items: center;
		gap: 12px;
		height: 36px;
		border-bottom: 1px solid var(--edge-strong);
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.grow {
		flex: 1;
	}

	.field {
		display: grid;
		grid-template-columns: 16px minmax(0, 1fr);
		column-gap: 12px;
		align-items: start;
		padding: 11px 0;
		border-bottom: 1px solid var(--edge-card);
	}

	.field :global(.tick) {
		margin-top: 2px;
	}

	.field-words {
		min-width: 0;
	}

	.label {
		display: block;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.change {
		display: flex;
		align-items: baseline;
		gap: 10px;
		margin-top: 4px;
		font-size: 15px;
		white-space: nowrap;
	}

	.change .now,
	.diff .now {
		font-weight: 600;
		color: var(--text);
	}

	.change .next,
	.diff .next {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		color: var(--text-meta);
	}

	.on .change .now,
	.diff.on .now {
		font-weight: 400;
		color: var(--text-meta);
		text-decoration: line-through;
	}

	.on .change .next,
	.diff.on .next {
		font-weight: 600;
		color: var(--text);
	}

	.arrow {
		color: var(--text-meta);
	}

	i {
		font-style: italic;
		font-weight: 400;
		text-decoration: none;
	}

	.note {
		display: block;
		margin-top: 4px;
		font-size: 12px;
		color: var(--text-meta);
	}

	.agree {
		padding: 10px 0 6px;
		font-size: 12px;
		color: var(--text-meta);
	}

	.covers {
		display: flex;
		gap: 24px;
		padding: 16px 20px;
	}

	.covers > div {
		width: 200px;
	}

	.art {
		position: relative;
		display: block;
		width: 100%;
		aspect-ratio: 1;
		overflow: hidden;
		border-radius: var(--radius-16);
		background: var(--fill);
	}

	.art.theirs {
		box-shadow: 0 0 0 2px var(--edge-strong);
	}

	.caption {
		margin-top: 8px;
		font-size: 13px;
		font-weight: 600;
	}

	.covers .mono {
		font-size: 11px;
		letter-spacing: 0.04em;
	}

	.ids {
		padding: 4px 4px 0;
		font-size: 11px;
		line-height: 1.75;
		letter-spacing: 0;
		color: var(--text-label);
		overflow-wrap: anywhere;
	}

	.ids-head {
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.songs-head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		min-height: 48px;
	}

	.songs-head .mono {
		margin-right: 6px;
		font-size: 11px;
	}

	.pill {
		height: 34px;
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 0 12px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		font-size: 13px;
		font-weight: 600;
	}

	.count {
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 400;
		color: var(--text-meta);
	}

	.hint {
		margin-left: auto;
		font-size: 12px;
		color: var(--text-meta);
	}

	.cols,
	.song {
		display: grid;
		grid-template-columns: 16px 100px minmax(0, 1.15fr) minmax(0, 0.85fr) 104px 176px;
		align-items: center;
		column-gap: 12px;
		padding: 0 10px;
	}

	.cols {
		height: 32px;
		border-bottom: 1px solid var(--edge-strong);
		font-size: 11px;
	}

	.right {
		text-align: right;
	}

	.disc {
		display: flex;
		align-items: flex-end;
		gap: 12px;
		height: 40px;
		padding: 0 10px 6px;
		box-sizing: border-box;
		border-bottom: 1px solid var(--edge-strong);
		font-size: 17px;
		font-weight: 800;
		letter-spacing: var(--tracking-title);
	}

	.disc .sub {
		font-size: 13px;
		font-weight: 400;
		letter-spacing: 0;
		color: var(--text-secondary);
	}

	.song {
		position: relative;
		min-height: 38px;
		box-sizing: border-box;
		border-bottom: 1px solid var(--edge-card);
	}

	.song.suspect {
		border-radius: 10px;
		background: color-mix(in srgb, var(--fox) 12%, transparent);
		box-shadow: inset 0 0 0 1px var(--fox-line);
	}

	.pair {
		font-size: 12px;
		letter-spacing: 0;
	}

	.cell {
		min-width: 0;
		display: flex;
		align-items: baseline;
		gap: 12px;
		font-size: 14px;
		white-space: nowrap;
	}

	.cell.artist {
		font-size: 13px;
		color: var(--text-secondary);
	}

	.plain {
		overflow: hidden;
		text-overflow: ellipsis;
		color: var(--text-secondary);
	}

	.diff {
		min-width: 0;
		display: inline-flex;
		align-items: baseline;
		gap: 8px;
		padding: 0;
		border: 0;
		background: transparent;
		text-align: left;
	}

	.diff .now {
		flex: 0 1 auto;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.song.off .diff .now {
		font-weight: 600;
		color: var(--text);
		text-decoration: none;
	}

	.song.off .diff .next {
		font-weight: 400;
		color: var(--text-meta);
	}

	.numbers {
		font-family: var(--font-mono);
		font-size: 12px;
	}

	.lengths {
		font-size: 12px;
		letter-spacing: 0;
	}

	.lengths.warn,
	.flag {
		color: var(--fox-text);
	}

	.end {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 6px;
	}

	.flag {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.04em;
		white-space: nowrap;
	}

	.more {
		width: 28px;
		height: 28px;
		display: grid;
		place-items: center;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		opacity: 0.4;
	}

	.song:hover .more,
	.song.suspect .more,
	.more:focus-visible,
	.more.open {
		opacity: 1;
	}

	.more.open {
		background: var(--fill);
	}

	.more svg {
		fill: currentColor;
	}

	.bar {
		flex: none;
		display: flex;
		align-items: center;
		gap: 12px;
		min-height: 84px;
		margin: 0 -32px;
		padding: 12px 32px;
		box-sizing: border-box;
		border-top: 1px solid var(--edge);
		background: var(--card);
	}

	.summary,
	.report {
		flex: 1;
		min-width: 0;
	}

	.what {
		display: block;
		font-size: 15px;
		font-weight: 700;
	}

	.foot,
	.detail {
		display: block;
		margin-top: 3px;
		font-size: 13px;
		color: var(--text-meta);
	}

	.report p:first-child {
		font-size: 15px;
		font-weight: 700;
		color: var(--problem);
	}

	.apply {
		height: 48px;
		padding: 0 26px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font-size: 14px;
		font-weight: 700;
		white-space: nowrap;
	}

	.apply:disabled {
		opacity: 0.45;
		cursor: default;
	}

	.asking {
		margin: 40px 0;
		display: flex;
		align-items: center;
		gap: 10px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--fox);
		animation: pulse 1.2s ease-in-out infinite;
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.25;
		}

		50% {
			opacity: 1;
		}
	}

	.menu-head {
		padding: 6px 12px 8px;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.option {
		width: 100%;
		min-height: 44px;
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 0 12px;
		border: 0;
		border-radius: var(--radius-12);
		background: transparent;
		font-size: 14px;
		font-weight: 600;
		text-align: left;
	}

	.option:hover,
	.option:focus-visible {
		background: var(--fill);
		outline: none;
	}

	.option.current {
		box-shadow: inset 0 0 0 1px var(--edge-strong);
	}

	.option-title {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.option .mono {
		font-size: 11px;
		letter-spacing: 0.02em;
	}

	.quiet {
		color: var(--text-secondary);
	}

	.rule {
		height: 1px;
		margin: 6px 4px;
		background: var(--edge);
	}

	.menu-note {
		padding: 4px 12px 6px;
		font-size: 12px;
		line-height: 1.45;
		color: var(--text-meta);
	}

	.link {
		padding: 0;
		border: 0;
		background: none;
		font-weight: 600;
		color: var(--fox-text);
	}

	/* Narrower desktops: the songs go under the album. */
	@container review (max-width: 1240px) {
		.body {
			grid-template-columns: minmax(0, 1fr);
			row-gap: 16px;
		}

		.side {
			display: grid;
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
			align-items: start;
			gap: 16px;
		}

		.ids {
			grid-column: 1 / -1;
		}

		.covers > div {
			width: min(200px, 45%);
		}

		.cols,
		.song {
			grid-template-columns: 16px 90px minmax(0, 1.2fr) minmax(0, 0.8fr) 104px 150px;
		}
	}

	/* One step up: fields, songs, then cover and IDs, side by side. */
	.beside .body {
		grid-template-columns: 620px minmax(0, 1fr);
	}

	/* A phone: stacked, with switches, and only the songs that change. */
	/* Back on its own row, then the cover and the release. */
	.narrow header {
		display: grid;
		grid-template-columns: 56px minmax(0, 1fr);
		gap: 16px 12px;
	}

	.narrow .back {
		grid-column: 1 / -1;
		justify-self: start;
	}

	.narrow .page {
		display: none;
	}

	.narrow h2 {
		max-width: none;
		font-size: 28px;
		font-weight: 900;
	}

	.narrow .line {
		flex-direction: column;
		gap: 4px;
	}

	.narrow .facts {
		font-size: 12px;
		white-space: normal;
	}

	.narrow .body {
		display: flex;
		flex-direction: column;
		align-items: stretch;
		gap: 22px;
	}

	.narrow .side {
		display: contents;
	}

	.narrow .well {
		padding: 0;
		background: transparent;
	}

	.narrow .well-head {
		height: auto;
		border: 0;
	}

	.narrow .field {
		margin-top: 6px;
		grid-template-columns: minmax(0, 1fr) auto;
		align-items: center;
		padding: 12px 14px;
		border: 0;
		border-radius: var(--radius-16);
		background: var(--input);
	}

	.narrow .change {
		flex-direction: column;
		gap: 1px;
		white-space: normal;
	}

	.narrow .change .now {
		font-size: 13px;
	}

	.narrow .change .next {
		font-size: 16px;
	}

	.narrow .songs {
		order: 2;
	}

	.narrow .covers {
		order: 3;
		padding: 0;
		gap: 12px;
	}

	.narrow .covers > div {
		width: 120px;
	}

	.narrow .ids {
		order: 4;
		font-size: 10px;
	}

	.narrow .song {
		grid-template-columns: 34px minmax(0, 1fr) auto;
		min-height: 60px;
		padding: 8px 10px;
	}

	.narrow .song.suspect {
		border-radius: 14px;
	}

	.narrow .cell.title {
		flex-direction: column;
		gap: 2px;
		white-space: normal;
	}

	.narrow .cell.title .diff {
		flex-direction: column;
		gap: 0;
	}

	.narrow .diff .arrow {
		display: none;
	}

	.narrow .diff .now {
		font-size: 12px;
	}

	.narrow .diff .next,
	.narrow .plain {
		font-size: 15px;
		font-weight: 600;
		color: var(--text);
	}

	.narrow .flag {
		font-size: 10px;
	}

	.repair {
		height: 36px;
		padding: 0 12px;
		border-color: var(--fox-line);
		font-size: 13px;
	}

	.narrow .bar {
		position: sticky;
		bottom: 0;
		flex-direction: column;
		align-items: stretch;
		margin: 0 -16px;
		padding: 12px 16px calc(24px + env(safe-area-inset-bottom));
		background: var(--glass-solid);
	}

	.narrow .what {
		font-size: 13px;
		font-weight: 400;
		color: var(--text-secondary);
	}

	.narrow .foot {
		display: none;
	}

	.narrow .apply {
		width: 100%;
		height: 52px;
		font-size: 15px;
	}

	@media (prefers-reduced-motion: reduce) {
		.dot {
			animation: none;
		}
	}
</style>
