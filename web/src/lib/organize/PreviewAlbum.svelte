<!--
	One album in a preview: its cover, who and what, a chip, the folder change once when every song
	leaves one folder for another, then a row per song, the old path struck where it changes and
	the new one in the fox. A conflict's WHY › opens its card under the row. During a run the chip
	says where the album stands instead.
-->
<script lang="ts">
	import Cover from '#lib/cover/Cover.svelte';
	import { tagEditor } from '#lib/pages/tagEditor.svelte.ts';
	import { duplicatesReady, openDuplicates } from './duplicates.ts';
	import type { OrganizeAlbum, OrganizeFile } from './types.ts';
	import {
		albumChip,
		albumLine,
		claimantFacts,
		companionsLine,
		fileChip,
		pathDiff,
		reasonWords
	} from './words.ts';

	/** Songs an album shows before "+ N more". */
	const shown = 3;

	interface Props {
		album: OrganizeAlbum;
		/** During a run: where this album stands. */
		standing?: 'moved' | 'moving' | 'waiting' | undefined;
		/** Picked to move on its own. Absent where nothing can be picked. */
		picked?: boolean | undefined;
		onpick?: (() => void) | undefined;
		phone: boolean;
	}

	let { album, standing, picked, onpick, phone }: Props = $props();

	let open = $state('');
	let all = $state(false);

	const files = $derived(all ? album.files : album.files.slice(0, shown));
	const folderOnce = $derived(album.fromFolder !== undefined && album.toFolder !== undefined);
	const chip = $derived(
		standing === 'moved'
			? 'MOVED ✓'
			: standing === 'moving'
				? 'MOVING'
				: standing === 'waiting'
					? 'WAITING'
					: albumChip(album, album.files)
	);
	const hot = $derived(standing === undefined ? album.kind !== 'inPlace' : standing === 'moving');

	const dirOf = (path: string): string => path.slice(0, Math.max(0, path.lastIndexOf('/')));
	const baseOf = (path: string): string => path.slice(path.lastIndexOf('/') + 1);

	/**
	 * Whether a row shows file names alone: when the folder change is shown once above, or the song
	 * stays in its folder, which the album's line says.
	 */
	function short(file: OrganizeFile): boolean {
		return (
			(folderOnce && !phone) ||
			file.to === undefined ||
			file.kind === 'inPlace' ||
			dirOf(file.from) === dirOf(file.to)
		);
	}

	function editBoth(file: OrganizeFile): void {
		const conflict = file.conflict;
		if (conflict === undefined) {
			return;
		}
		tagEditor.open(
			{
				kind: 'songs',
				songs: [conflict.self, ...conflict.with].map((c) => ({
					id: c.track,
					albumId: c.albumId,
					title: c.title
				}))
			},
			true
		);
	}

	const initials = $derived(
		album.title
			.split(/\s+/)
			.filter((w) => /\p{L}/u.test(w.charAt(0)))
			.slice(0, 2)
			.map((w) => w.charAt(0).toUpperCase())
			.join('')
	);
</script>

<article
	class="album"
	class:conflict={album.kind === 'conflict' && standing === undefined}
	class:phone
	aria-label={album.title}
>
	<div class="head">
		{#if picked !== undefined && onpick !== undefined}
			<input
				type="checkbox"
				class="pick"
				aria-label={`Move only ${album.title}’s songs`}
				checked={picked}
				onchange={onpick}
			/>
		{/if}
		<span class="cover">
			{#if album.artwork !== undefined}
				<Cover
					albumId={album.id}
					entry={{ artwork: album.artwork }}
					place="small"
					box={phone ? 36 : 44}
				/>
			{:else}
				<span class="initials" aria-hidden="true">{initials}</span>
			{/if}
		</span>
		<span class="words">
			<span class="title">{album.title}</span>
			<span class="line mono">{albumLine(album)}</span>
		</span>
		<span class="chip mono" class:hot class:moved={standing === 'moved'}>{chip}</span>
	</div>

	{#if folderOnce && !phone}
		<div class="folder mono">
			<span class="old">{album.fromFolder}/</span><br /><span class="arrow">→ </span><span
				class="new">{album.toFolder}/</span
			>
		</div>
	{/if}

	{#each files as file (file.track)}
		{@const diff = file.to === undefined ? null : pathDiff(file.from, file.to)}
		<div class="file mono" class:phone>
			{#if phone}
				<span class="new-path"
					><span class="arrow">→ </span>{#if file.to === undefined}<span class="reason"
							>{reasonWords[file.reason ?? 'noTags']}</span
						>{:else}{short(file) ? baseOf(file.to) : file.to}{/if}</span
				>
				{#if file.kind !== 'inPlace'}
					<span class="old-path" class:struck={file.to !== undefined}
						>{short(file) ? baseOf(file.from) : file.from}</span
					>
				{/if}
			{:else}
				<span class="old-path"
					>{#if short(file) || diff === null}<span
							class:struck={file.kind === 'rename' || file.kind === 'move'}
							>{baseOf(file.from)}</span
						>{:else}{#each diff.old as part, i (i)}<span class:struck={part.changed}
								>{part.text}</span
							>{/each}{/if}</span
				>
				<span class="new-path" class:quiet={file.kind === 'inPlace'}
					><span class="arrow">→ </span>{#if file.to === undefined}<span class="reason"
							>{reasonWords[file.reason ?? 'noTags']}</span
						>{:else if short(file) || diff === null}<span class:changed={file.kind !== 'inPlace'}
							>{baseOf(file.to)}</span
						>{:else}{#each diff.new as part, i (i)}<span class:changed={part.changed}
								>{part.text}</span
							>{/each}{/if}</span
				>
			{/if}
			{#if file.kind === 'conflict' && standing === undefined}
				<button
					type="button"
					class="why"
					aria-expanded={open === file.track}
					onclick={() => {
						open = open === file.track ? '' : file.track;
					}}>WHY ›</button
				>
			{:else if file.kind === 'skipped'}
				<span class="file-chip">{fileChip[file.kind]}</span>
			{:else if file.caseOnly === true}
				<span class="file-chip">CASE ONLY</span>
			{:else if !phone}
				<span aria-hidden="true"></span>
			{/if}
		</div>
		{#if open === file.track}
			<div class="card" role="region" aria-label="Why it stays">
				{#if file.reason === 'unknownFile'}
					<p class="card-title">
						A file Kitsune doesn’t play is already there: <span class="code"
							>…/{file.unknown ?? ''}</span
						>. The song stays where it is.
					</p>
				{:else if file.conflict !== undefined}
					{@const conflict = file.conflict}
					<p class="card-title">
						{conflict.with.length + 1} songs want <span class="code">{file.to}</span>
					</p>
					<div class="claimants">
						{#each [...conflict.with.filter((c) => c.here), conflict.self, ...conflict.with.filter((c) => !c.here)] as c (c.track)}
							<div class="claimant">
								<span class="claimant-album">{c.album}</span>
								<span class="claimant-path mono">{c.path}</span>
								<span class="claimant-facts mono">{claimantFacts(c)}</span>
							</div>
						{/each}
					</div>
					<div class="card-foot">
						{#if conflict.sameRecording}
							<span class="verdict">They look like the same recording.</span>
							<span class="mono note"
								>{conflict.why === 'digest'
									? 'THE SAME AUDIO · BOTH STAY WHERE THEY ARE'
									: 'SAME ARTIST, TITLE AND LENGTH · BOTH STAY WHERE THEY ARE'}</span
							>
							{#if duplicatesReady}
								<button
									type="button"
									class="fox-button"
									onclick={() => {
										openDuplicates([conflict.self.track, ...conflict.with.map((c) => c.track)]);
									}}>Compare in Duplicates ›</button
								>
							{:else}
								<button
									type="button"
									class="fox-button"
									disabled
									aria-describedby={`soon-${file.track}`}>Compare in Duplicates ›</button
								><span class="soon mono" id={`soon-${file.track}`}>SOON</span>
							{/if}
						{:else}
							<span class="verdict"
								>Two songs, one name. Edit a tag so the template tells them apart.</span
							>
						{/if}
						<button
							type="button"
							class="outline"
							onclick={() => {
								editBoth(file);
							}}>Edit tags</button
						>
					</div>
				{/if}
			</div>
		{/if}
	{/each}
	{#if album.files.length > shown}
		<button
			type="button"
			class="more mono"
			onclick={() => {
				all = !all;
			}}
			>{all
				? 'SHOW FEWER'
				: `+ ${(album.files.length - shown).toLocaleString('en-US')} MORE ${album.files.length - shown === 1 ? 'SONG' : 'SONGS'}`}</button
		>
	{/if}
	{#if album.companions.length > 0}
		<p class="companions mono">BRINGS ALONG {companionsLine(album.companions).slice(2)}</p>
	{/if}
</article>

<style>
	.album {
		display: flex;
		flex-direction: column;
		gap: calc(8px * var(--step));
		padding: calc(14px * var(--step)) calc(18px * var(--step));
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid var(--edge);
		content-visibility: auto;
		contain-intrinsic-size: auto calc(150px * var(--step));
	}

	.album.conflict {
		border: 1.5px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
	}

	.mono {
		font-family: var(--font-mono);
	}

	.head {
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.pick {
		flex: none;
		width: 18px;
		height: 18px;
		accent-color: var(--fox);
	}

	.cover {
		--cover-radius: 9px;

		flex: none;
		width: calc(44px * var(--step));
		height: calc(44px * var(--step));
		border-radius: 9px;
		overflow: hidden;
	}

	.cover :global(img),
	.cover :global(svg) {
		width: 100%;
		height: 100%;
	}

	.initials {
		display: grid;
		place-items: center;
		width: 100%;
		height: 100%;
		background: var(--card-raised);
		color: var(--text-meta);
		font: 700 calc(13px * var(--step)) var(--font-mono);
	}

	.words {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.title {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font-size: calc(16px * var(--step));
		font-weight: 700;
	}

	.line {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font-size: calc(11px * var(--step));
		letter-spacing: 0.04em;
		color: var(--text-meta);
	}

	.chip {
		flex: none;
		padding: 3px 8px;
		border-radius: 6px;
		border: 1.5px dashed var(--edge-strong);
		font-size: calc(10.5px * var(--step));
		font-weight: 700;
		letter-spacing: 0.06em;
		color: var(--text-meta);
		transform: rotate(-1deg);
		white-space: nowrap;
	}

	.chip.hot {
		border-color: color-mix(in srgb, var(--fox-line) 55%, transparent);
		color: var(--fox-text);
	}

	.chip.moved {
		color: var(--text);
	}

	.folder,
	.file,
	.more,
	.companions {
		padding-left: calc(56px * var(--step));
	}

	.folder {
		font-size: calc(12px * var(--step));
		line-height: 1.5;
		word-break: break-all;
	}

	.old {
		color: var(--text-meta);
		text-decoration: line-through;
	}

	.new {
		color: var(--fox-text);
		font-weight: 600;
	}

	.arrow {
		color: var(--fox-text);
	}

	.file {
		display: grid;
		grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr) calc(64px * var(--step));
		column-gap: 14px;
		align-items: baseline;
		font-size: calc(12px * var(--step));
		line-height: 1.5;
	}

	@media (min-width: 2560px) {
		.file {
			grid-template-columns: minmax(0, 1fr) minmax(0, 1.3fr) calc(70px * var(--step));
		}
	}

	.old-path,
	.new-path {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.old-path {
		color: var(--text-meta);
	}

	.struck {
		text-decoration: line-through;
	}

	.new-path {
		color: var(--text);
	}

	.new-path.quiet {
		color: var(--text-meta);
	}

	.changed {
		color: var(--fox-text);
		font-weight: 600;
	}

	.reason {
		color: var(--text-meta);
		letter-spacing: 0.04em;
	}

	.file-chip {
		justify-self: end;
		font-size: calc(10px * var(--step));
		font-weight: 700;
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.why {
		justify-self: end;
		height: calc(26px * var(--step));
		padding: 0 10px;
		border-radius: var(--radius-pill);
		border: 1.5px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
		background: transparent;
		color: var(--fox-text);
		font: 700 calc(10.5px * var(--step)) var(--font-mono);
		cursor: pointer;
	}

	.why[aria-expanded='true'] {
		background: color-mix(in srgb, var(--fox) 12%, transparent);
	}

	.card {
		margin: 6px 0 4px calc(56px * var(--step));
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 16px 18px;
		border-radius: var(--radius-16);
		border: 1.5px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
		background: color-mix(in srgb, var(--fox) 12%, transparent);
	}

	.card-title {
		font-size: calc(16px * var(--step));
		font-weight: 700;
		line-height: 1.4;
	}

	.code {
		font: 600 calc(13px * var(--step)) var(--font-mono);
		word-break: break-all;
	}

	.claimants {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 12px;
	}

	.claimant {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 12px 14px;
		border-radius: var(--radius-12);
		background: var(--card);
		border: 1px solid var(--edge);
	}

	.claimant-album {
		font-size: calc(14px * var(--step));
		font-weight: 700;
	}

	.claimant-path {
		font-size: calc(11px * var(--step));
		line-height: 1.5;
		color: var(--text-meta);
		word-break: break-all;
	}

	.claimant-facts {
		font-size: calc(11px * var(--step));
		color: var(--text-secondary);
	}

	.card-foot {
		display: flex;
		align-items: center;
		gap: 12px;
		flex-wrap: wrap;
	}

	.verdict {
		font-size: calc(14px * var(--step));
		font-weight: 600;
	}

	.note {
		font-size: calc(10.5px * var(--step));
		letter-spacing: 0.04em;
		color: var(--text-meta);
	}

	.fox-button {
		margin-left: auto;
		height: calc(38px * var(--step));
		line-height: calc(38px * var(--step));
		padding: 0 18px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font: 700 calc(14px * var(--step)) var(--font-sans);
		text-decoration: none;
		cursor: pointer;
	}

	.fox-button:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.soon {
		margin-left: -6px;
		padding: 2px 6px;
		border-radius: 5px;
		border: 1.5px dashed var(--edge-strong);
		font-size: 10px;
		font-weight: 700;
		letter-spacing: 0.08em;
		color: var(--text-meta);
		transform: rotate(-2deg);
	}

	.outline {
		height: calc(38px * var(--step));
		padding: 0 16px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font: 600 calc(14px * var(--step)) var(--font-sans);
		cursor: pointer;
	}

	.outline:hover {
		background: var(--hover);
	}

	.more {
		align-self: flex-start;
		border: 0;
		background: none;
		color: var(--text-meta);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.04em;
		cursor: pointer;
	}

	.more:hover {
		color: var(--text);
	}

	.companions {
		font-size: calc(11px * var(--step));
		letter-spacing: 0.04em;
		color: var(--text-meta);
	}

	/* A phone: two lines a song, the new path first, the old struck under it. */
	.album.phone {
		padding: 12px 14px;
		border-radius: var(--radius-16);
	}

	.phone .cover {
		width: 36px;
		height: 36px;
	}

	.phone .title {
		font-size: 15px;
	}

	.phone .line {
		font-size: 10.5px;
	}

	.phone .chip {
		padding: 2px 6px;
		font-size: 9.5px;
	}

	.file.phone {
		display: flex;
		flex-direction: column;
		gap: 1px;
		padding-left: 0;
		font-size: 11.5px;
		line-height: 1.45;
	}

	.file.phone .new-path,
	.file.phone .old-path {
		white-space: normal;
		word-break: break-all;
	}

	.file.phone .why,
	.file.phone .file-chip {
		align-self: flex-start;
	}

	.phone .more,
	.phone .companions {
		padding-left: 0;
	}

	.phone .card {
		margin-left: 0;
	}

	.phone .claimants {
		grid-template-columns: minmax(0, 1fr);
	}
</style>
