<!--
	The tag editor's left column: the songs, every one ticked to start with. A tick says what the
	right side edits; a title is renamed in its row, whatever is ticked; › opens a song's every tag.
	A selection groups its songs under each album.
-->
<script lang="ts">
	import Cover from '#lib/cover/Cover.svelte';
	import { isChanged, valueOf, wasOf, type EditTrack } from '#lib/library/edit.ts';
	import { clock } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { fieldId } from '#lib/library/tags.ts';
	import type { EditSession } from './tagEditor.svelte.ts';

	interface Props {
		session: EditSession;
		/** Opens a song to every tag, or closes it. */
		expand: (id: string) => void;
	}

	let { session, expand }: Props = $props();

	const total = $derived(session.tracks.length);
	const ticked = $derived(session.tickedIds.length);
	const groups = $derived(
		session.albumIds.flatMap((albumId) => {
			const tracks = session.tracks.filter((track) => track.albumId === albumId);
			return tracks.length === 0 ? [] : [{ albumId, tracks }];
		})
	);
	const selection = $derived(session.shape === 'selection');

	$effect(() => {
		for (const id of session.albumIds) {
			void library.loadAlbum(id);
		}
	});

	function lengthOf(track: EditTrack): string {
		const known = library.album(track.albumId)?.tracks.find(({ id }) => id === track.id);
		return known === undefined ? '' : clock(known.durationMs / 1000);
	}

	function first(track: EditTrack, field: 'title' | 'track' | 'disc'): string {
		return valueOf(track, session.draft, session.ticked, field).at(0) ?? '';
	}

	function metaOf(albumId: string, count: number): string {
		const album = library.album(albumId) ?? library.entry(albumId);
		const of = album?.trackCount ?? count;
		return [
			album?.artist,
			album?.year === undefined ? undefined : String(album.year),
			`${String(count)} OF ${String(of)}`
		]
			.filter((part) => part !== undefined)
			.join(' · ')
			.toUpperCase();
	}

	function left({ target }: FocusEvent): void {
		if (target instanceof HTMLElement && target.id !== '') {
			session.leftField(target.id);
		}
	}
</script>

<div class="head">
	<label class="all">
		<input
			type="checkbox"
			aria-label="Every song"
			checked={ticked === total && total > 0}
			indeterminate={ticked > 0 && ticked < total}
			disabled={total === 0}
			onchange={() => {
				session.toggleAll();
			}}
		/>
		{ticked === total
			? `All ${String(total)} songs`
			: `${String(ticked)} of ${String(total)} songs`}
	</label>
	{#if session.canTag}
		<span class="hint" aria-hidden="true">CLICK A TITLE TO RENAME · › FOR EVERY TAG</span>
	{/if}
</div>

{#if session.tracks.length === 0}
	<p class="note">{session.report === null ? 'Reading the files…' : session.report.message}</p>
{:else}
	<div onfocusout={left}>
		{#each groups as { albumId, tracks } (albumId)}
			{#if selection}
				{@const album = library.album(albumId) ?? library.entry(albumId)}
				<div class="album">
					<span class="cover"><Cover {albumId} entry={album} place="small" box={40} /></span>
					<span class="about">
						<span class="album-title">{album?.title ?? ''}</span>
						<span class="album-meta">{metaOf(albumId, tracks.length)}</span>
					</span>
				</div>
			{/if}
			<ul>
				{#each tracks as track (track.id)}
					{@const title = fieldId(track.id, 'title')}
					{@const name = first(track, 'title') || track.tags.file}
					{@const changed = isChanged(track, session.draft, session.ticked, 'title')}
					{@const problem = session.problem(title)}
					{@const open = session.open === track.id}
					{@const was = wasOf(track, 'title').at(0)}
					<li
						class="row"
						class:open
						class:changed
						class:invalid={session.hasProblem(title)}
						data-edit-track={track.id}
					>
						<input
							type="checkbox"
							aria-label={`Edit ${name}`}
							checked={session.ticked.has(track.id)}
							onchange={() => {
								session.toggle(track.id);
							}}
						/>
						<span class="disc">{first(track, 'disc')}</span>
						<span class="no">{first(track, 'track').padStart(2, '0')}</span>
						{#if session.canTag}
							<input
								id={title}
								class="title"
								type="text"
								autocomplete="off"
								aria-label={`Title of ${was ?? track.tags.file}`}
								aria-invalid={problem !== undefined}
								aria-describedby={problem === undefined ? undefined : `${title}-problem`}
								placeholder="Empty"
								bind:value={
									() => first(track, 'title'),
									(value) => {
										session.setField(track.id, 'title', [value]);
									}
								}
							/>
						{:else}
							<span class="title read">{name}</span>
						{/if}
						<span class="length">{lengthOf(track)}</span>
						{#if session.canTag}
							<button
								type="button"
								class="more"
								id={`more-${track.id}`}
								aria-label={`Every tag of ${name}`}
								aria-pressed={open}
								onclick={() => {
									expand(track.id);
								}}
								><svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true"
									><path d="M9 6l6 6-6 6"></path></svg
								></button
							>
						{/if}
						{#if problem !== undefined}
							<p class="problem" id={`${title}-problem`}>{problem}</p>
						{/if}
					</li>
				{/each}
			</ul>
		{/each}
	</div>
	{#if selection}
		<p class="note">To add songs, go back and select them in the Library.</p>
	{/if}
{/if}

<style>
	.head {
		display: flex;
		align-items: center;
		gap: 14px;
		height: 52px;
		border-bottom: 1px solid var(--edge-card);
	}

	.all {
		display: flex;
		align-items: center;
		gap: 12px;
		font-size: 13px;
		font-weight: 600;
		color: var(--text-secondary);
	}

	.hint,
	.album-meta,
	.disc,
	.no,
	.length {
		font-family: var(--font-mono);
	}

	.hint {
		margin-left: auto;
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	input[type='checkbox'] {
		flex: none;
		width: 20px;
		height: 20px;
		margin: 0;
		accent-color: var(--fox);
	}

	ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.album {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 14px 0 6px;
	}

	.cover {
		position: relative;
		display: block;
		flex: none;
		width: 40px;
		height: 40px;
		border-radius: 10px;
		overflow: hidden;
	}

	.about {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.album-title {
		font-size: 15px;
		font-weight: 700;
	}

	.album-meta {
		font-size: 11px;
		color: var(--text-meta);
	}

	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		column-gap: 12px;
		min-height: 48px;
		padding: 0 4px;
		border-bottom: 1px solid var(--edge-card);
		border-radius: 10px;
	}

	.row.open {
		background: color-mix(in srgb, var(--fox) 8%, transparent);
	}

	.disc {
		flex: none;
		width: 20px;
		font-size: var(--type-mono);
		color: var(--text-meta);
		text-align: right;
	}

	.no {
		flex: none;
		width: 26px;
		font-size: 13px;
	}

	.title {
		flex: 1;
		min-width: 0;
		height: 36px;
		padding: 0;
		border: 0;
		border-bottom: 1px solid var(--edge-field);
		outline: none;
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: var(--type-body);
		font-weight: 600;
		transition: border-color var(--morph-fast) var(--curve-morph);
	}

	.title.read {
		display: flex;
		align-items: center;
		overflow: hidden;
		border-bottom-color: transparent;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.title:focus {
		border-bottom-color: var(--text);
	}

	.title::placeholder {
		font-style: italic;
		font-weight: 400;
		color: var(--text-meta);
	}

	.changed .title {
		border-bottom-color: var(--fox-line);
	}

	.invalid .title {
		border-bottom-color: var(--problem-edge);
	}

	.length {
		flex: none;
		width: 44px;
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		color: var(--text-meta);
		text-align: right;
	}

	.more {
		flex: none;
		display: grid;
		place-items: center;
		width: 40px;
		height: 40px;
		padding: 0;
		border: 0;
		border-radius: 50%;
		background: transparent;
		color: var(--text-meta);
	}

	.more svg {
		fill: none;
		stroke: currentColor;
		stroke-width: 2.2;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	.more:hover {
		color: var(--fox-text);
	}

	.more[aria-pressed='true'] {
		background: var(--fox);
		color: var(--fox-ink);
	}

	.problem {
		flex-basis: 100%;
		padding: 0 0 8px 116px;
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--problem);
	}

	.note {
		margin-top: 16px;
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	/* Whatever takes focus must clear the dock and the island, which float over the page. */
	.row :global(input) {
		scroll-margin: 96px 0 200px;
	}

	@media (max-width: 640px) {
		.hint {
			display: none;
		}

		.row {
			column-gap: 10px;
		}

		/* The title needs the room; the disc is in the song's every tag. */
		.disc {
			display: none;
		}

		.no {
			width: 20px;
		}

		.problem {
			padding-left: 0;
		}
	}
</style>
