<script lang="ts">
	import { untrack } from 'svelte';
	import Cover from '#lib/cover/Cover.svelte';
	import { bulkFields, effective, shownOf } from '#lib/library/bulk.ts';
	import { clock } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import BulkField from './BulkField.svelte';
	import ShuffleTab from './ShuffleTab.svelte';
	import type { BulkSession } from './bulkEdits.svelte.ts';

	interface Props {
		session: BulkSession;
	}

	let { session }: Props = $props();

	/** Why the Tags tab can't edit, when it can't: the Shuffle tab still can. */
	const noTags = $derived(
		library.user?.role === 'admin'
			? "Tag writes are off on this server, so the files can't be changed from here. The Shuffle tab still works."
			: 'Only an administrator can change tags. Your shuffle rules are yours: set them on the Shuffle tab.'
	);

	const selection = $derived(session.layout === 'selection');
	const picked = $derived(session.pickedTracks.length);
	const albumCount = $derived(session.albumIds.length);
	const groups = $derived(
		session.albumIds.flatMap((albumId) => {
			const tracks = session.tracks.filter((track) => track.albumId === albumId);
			return tracks.length === 0 ? [] : [{ albumId, tracks }];
		})
	);
	const albumMixed = $derived(
		selection && shownOf(session.pickedTracks, session.draft, 'album').mixed
	);
	const plural = (count: number, one: string): string =>
		`${String(count)} ${one}${count === 1 ? '' : 's'}`;

	// Someone who can't change tags starts where they can change something.
	let tab = $state<'tags' | 'shuffle'>(untrack(() => session.canTag) ? 'tags' : 'shuffle');
	let finding = $state(false);
	let find = $state('');
	let replace = $state('');

	$effect(() => {
		for (const id of session.albumIds) {
			void library.loadAlbum(id);
		}
	});

	function lengthOf(albumId: string, trackId: string): string {
		const track = library.album(albumId)?.tracks.find(({ id }) => id === trackId);
		return track === undefined ? '' : clock(track.durationMs / 1000);
	}

	function numberOf(albumId: string, trackId: string, index: number): string {
		const track = session.tracks.find(({ id }) => id === trackId);
		const [own] = track === undefined ? [] : effective(track, session.draft, 'track');
		return (own ?? String(index + 1)).padStart(2, '0');
	}

	function titleOf(trackId: string): string {
		const track = session.tracks.find(({ id }) => id === trackId);
		return track === undefined
			? ''
			: (effective(track, session.draft, 'title').at(0) ?? track.tags.file);
	}

	function metaOf(albumId: string, count: number): string {
		const album = library.album(albumId) ?? library.entry(albumId);
		const total = album?.trackCount ?? count;
		return [
			album?.artist,
			album?.year === undefined ? undefined : String(album.year),
			`${String(count)} OF ${String(total)}`
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

	function replaceAll(): void {
		session.findAndReplace(find, replace);
		finding = false;
		find = '';
		replace = '';
	}
</script>

{#if selection}
	<header class="head">
		<p class="eyebrow">
			EDITING · {plural(picked, 'SONG').toUpperCase()} FROM {plural(
				albumCount,
				'ALBUM'
			).toUpperCase()} · FROM YOUR SELECTION
		</p>
		<h1>{plural(picked, 'song')}</h1>
	</header>
{/if}

<div class="bulk" class:selection>
	<section class="songs" aria-label={selection ? 'The selected songs' : 'Tracks'}>
		{#if session.albums === null}
			<p class="note">Reading the files…</p>
		{:else if selection}
			{#each groups as { albumId, tracks } (albumId)}
				{@const album = library.album(albumId) ?? library.entry(albumId)}
				<div class="album">
					<span class="cover"><Cover {albumId} entry={album} place="small" box={56} /></span>
					<span>
						<span class="album-title">{album?.title ?? ''}</span>
						<span class="album-meta">{metaOf(albumId, tracks.length)}</span>
					</span>
				</div>
				<ul>
					{#each tracks as track, index (track.id)}
						{@const title = titleOf(track.id)}
						<li>
							<label class="row" class:on={session.picked.has(track.id)}>
								<input
									type="checkbox"
									aria-label={`Keep ${title} in this edit`}
									checked={session.picked.has(track.id)}
									onchange={() => {
										session.toggle(track.id);
									}}
								/>
								<span class="no">{numberOf(albumId, track.id, index)}</span>
								<span class="title">{title}</span>
								<span class="len">{lengthOf(albumId, track.id)}</span>
							</label>
						</li>
					{/each}
				</ul>
			{/each}
			<p class="note">
				Untick a song to leave it out of this edit. To add songs, go back and select them in the
				Library.
			</p>
		{:else}
			<label class="pick-all">
				<input
					type="checkbox"
					aria-label="Pick every track"
					checked={picked === session.tracks.length && picked > 0}
					indeterminate={picked > 0 && picked < session.tracks.length}
					onchange={() => {
						session.toggleAll();
					}}
				/>PICK ALL
			</label>
			<ul>
				{#each session.tracks as track, index (track.id)}
					{@const title = titleOf(track.id)}
					<li>
						<label class="row tall" class:on={session.picked.has(track.id)}>
							<input
								type="checkbox"
								aria-label={`Pick ${title}`}
								checked={session.picked.has(track.id)}
								onchange={() => {
									session.toggle(track.id);
								}}
							/>
							<span class="no">{numberOf(track.albumId, track.id, index)}</span>
							<span class="title">{title}</span>
							<span class="len">{lengthOf(track.albumId, track.id)}</span>
						</label>
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<section
		class="edit"
		aria-label={selection ? 'Edit the selected songs' : 'Tags for the picked tracks'}
	>
		{#if !selection}
			<h2>{plural(picked, 'track')}</h2>
		{/if}
		<div class="tabs" role="tablist" aria-label="What to edit">
			<button
				type="button"
				role="tab"
				id="bulk-tab-tags"
				aria-selected={tab === 'tags'}
				aria-controls="bulk-panel"
				onclick={() => (tab = 'tags')}>Tags</button
			>
			<button
				type="button"
				role="tab"
				id="bulk-tab-shuffle"
				aria-selected={tab === 'shuffle'}
				aria-controls="bulk-panel"
				onclick={() => (tab = 'shuffle')}>Shuffle</button
			>
		</div>
		<div id="bulk-panel" role="tabpanel" aria-labelledby={`bulk-tab-${tab}`}>
			{#if tab === 'shuffle'}
				<ShuffleTab {session} />
			{:else if !session.canTag}
				<p class="hint">{noTags}</p>
			{:else if picked === 0}
				<p class="hint">Pick a song to edit its tags.</p>
			{:else}
				<p class="hint">
					What they share is filled in. Leave "Mixed" alone and each {selection ? 'song' : 'track'} keeps
					its own.
				</p>
				<div class="grid" onfocusout={left}>
					{#each bulkFields as spec (spec.key)}
						<BulkField {spec} {session} />
					{/each}
				</div>
				{#if albumMixed}
					<p class="move">
						Album is mixed: giving these songs one album name moves them into that album, and the
						Library follows.
					</p>
				{/if}
				<p class="jobs-label">QUICK JOBS</p>
				<div class="jobs">
					{#if !selection}
						<button
							type="button"
							onclick={() => {
								session.numberFromOne();
							}}>Number from 1</button
						>
					{/if}
					{#if selection}
						<button
							type="button"
							onclick={() => {
								session.titleCase();
							}}>Title Case</button
						>
						<button type="button" aria-expanded={finding} onclick={() => (finding = !finding)}
							>Find and replace</button
						>
					{/if}
					<button
						type="button"
						onclick={() => {
							session.titlesFromFiles();
						}}>Titles from file names</button
					>
					{#if !selection}
						<button
							type="button"
							onclick={() => {
								session.titleCase();
							}}>Title Case</button
						>
						<button type="button" aria-expanded={finding} onclick={() => (finding = !finding)}
							>Find and replace</button
						>
					{/if}
				</div>
				{#if finding}
					<form
						class="find"
						onsubmit={(event) => {
							event.preventDefault();
							replaceAll();
						}}
					>
						<label>
							<span>Find in titles</span>
							<!-- svelte-ignore a11y_autofocus -->
							<input type="text" autocomplete="off" autofocus bind:value={find} />
						</label>
						<label>
							<span>Replace with</span>
							<input type="text" autocomplete="off" bind:value={replace} />
						</label>
						<button type="submit" class="go" disabled={find === ''}>Replace</button>
					</form>
				{/if}
			{/if}
		</div>
	</section>
</div>

<style>
	.head {
		padding: 40px 64px 0;
	}

	.eyebrow,
	.jobs-label,
	.pick-all,
	.album-meta,
	.no,
	.len {
		font-family: var(--font-mono);
		letter-spacing: 0.08em;
	}

	.eyebrow {
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	h1 {
		margin: 10px 0 0;
		font-size: clamp(56px, 6.2vw, 88px);
		font-weight: 900;
		letter-spacing: -0.06em;
		line-height: 0.9;
	}

	.bulk {
		display: grid;
		grid-template-columns: minmax(0, 700fr) minmax(0, 652fr);
		gap: 24px;
		padding: 0 32px;
	}

	.bulk.selection {
		margin-top: 40px;
	}

	.songs,
	.edit {
		min-width: 0;
		border-radius: var(--radius-28);
		align-self: start;
	}

	.songs {
		padding: 12px 20px 16px;
		background: var(--card);
	}

	.selection .songs {
		padding-top: 16px;
	}

	.edit {
		padding: 20px 24px 24px;
		background: var(--card-raised);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--fox-line) 50%, transparent);
	}

	ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.pick-all {
		display: flex;
		align-items: center;
		gap: 14px;
		height: 48px;
		border-bottom: 1px solid var(--edge-strong);
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	input[type='checkbox'] {
		flex: none;
		width: 20px;
		height: 20px;
		margin: 0;
		accent-color: var(--fox);
	}

	.row {
		display: flex;
		align-items: center;
		gap: 14px;
		height: 56px;
		padding-left: 4px;
		border-bottom: 1px solid var(--edge-card);
		cursor: pointer;
	}

	.row.tall {
		height: 60px;
		padding-left: 0;
	}

	.row.on {
		background: color-mix(in srgb, var(--fox) 8%, transparent);
	}

	.no {
		width: 24px;
		flex: none;
		font-size: 13px;
		letter-spacing: 0;
		color: var(--text-meta);
	}

	.title {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		font-size: var(--type-body);
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.len {
		flex: none;
		padding-right: 4px;
		font-size: var(--type-mono);
		letter-spacing: 0;
		color: var(--text-meta);
	}

	.album {
		display: flex;
		align-items: center;
		gap: 14px;
		padding: 14px 0 10px;
	}

	.cover {
		position: relative;
		display: block;
		flex: none;
		width: 56px;
		height: 56px;
		border-radius: var(--radius-12);
		overflow: hidden;
	}

	.album-title {
		display: block;
		font-size: 17px;
		font-weight: 700;
	}

	.album-meta {
		display: block;
		font-size: var(--type-mono);
		letter-spacing: 0;
		color: var(--text-meta);
	}

	.note,
	.move {
		margin: 18px 0 0;
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	.move {
		margin-top: 14px;
	}

	h2 {
		margin: 0 0 14px;
		font-size: 24px;
		font-weight: 800;
		letter-spacing: -0.03em;
	}

	.tabs {
		display: flex;
		gap: 6px;
		width: fit-content;
		padding: 4px;
		border-radius: var(--radius-pill);
		background: var(--fill);
		border: 1px solid var(--edge-card);
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

	.tabs button[aria-selected='true'] {
		background: var(--fox);
		color: var(--on-fox-ink);
		font-weight: 700;
	}

	.hint {
		margin: 4px 0 18px;
		font-size: 14px;
		color: var(--text-secondary);
	}

	.tabs + #bulk-panel .hint {
		margin-top: 16px;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 10px 20px;
	}

	.jobs-label {
		margin: 22px 0 0;
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	.jobs {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		margin-top: 8px;
	}

	.jobs button,
	.find .go {
		height: var(--target);
		padding: 0 16px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: transparent;
		font-size: 14px;
		font-weight: 600;
	}

	.jobs button[aria-expanded='true'] {
		border-color: var(--fox-line);
	}

	.find {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 12px 16px;
		margin-top: 14px;
	}

	.find label {
		flex: 1 1 180px;
		min-width: 0;
		border-bottom: 1px solid var(--edge-field);
	}

	.find label:focus-within {
		border-bottom-color: var(--text);
	}

	.find span {
		display: block;
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	.find input {
		width: 100%;
		height: 34px;
		padding: 0;
		border: 0;
		outline: none;
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: var(--type-body);
		font-weight: 600;
	}

	.find .go {
		background: var(--fox);
		border-color: transparent;
		color: var(--on-fox-ink);
		font-weight: 700;
	}

	.find .go:disabled {
		opacity: 0.5;
	}

	/* From 2560 the editor takes the page's step up, so a 4K screen reads like 1920. */
	@media (min-width: 2560px) {
		.head,
		.bulk {
			zoom: 1.5;
		}
	}

	/* A phone stacks the songs over the fields. */
	@media (max-width: 700px) {
		.head {
			padding: 24px 16px 0;
		}

		.bulk {
			grid-template-columns: minmax(0, 1fr);
			gap: 16px;
			padding: 0 12px;
		}

		.bulk.selection {
			margin-top: 24px;
		}

		.edit {
			padding: 18px 16px 20px;
		}
	}

	@media (max-width: 480px) {
		.grid {
			grid-template-columns: minmax(0, 1fr);
		}

		.grid :global(.field) {
			grid-column: span 1;
		}
	}
</style>
