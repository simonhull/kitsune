<!--
	One playlist: its mosaic, what it holds, its name as a heading that edits in place (saved on blur
	or Enter, put back by Esc), Play, Shuffle and ⋯ (Rename, Delete with a confirm in place), and its
	songs in its order, which a grip moves.

	A smart playlist's page has its rules read back as a sentence under the name, Edit rules beside
	Shuffle, and its songs in the order the rules give, with nothing to drag or take out. Edit rules
	opens the rules editor in place of the head and the songs, with a history entry of its own: Back
	and Esc leave it, and its draft goes with it.

	A file playlist's page reads like a smart one's: the name is a plain heading, since renaming is the
	file's, and under it the FROM line names the file and when it was read, with a pill that opens the
	lines that found no song. Copy to a playlist makes it the person's own. Export .m3u, on any
	playlist's ⋯, opens in place of the buttons.
-->
<script lang="ts">
	import { layInFront, SurfaceSlot } from '#lib/morph/surface.ts';
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import Floating from '#lib/pages/Floating.svelte';
	import { morph } from '#lib/morph/registry.ts';
	import Mosaic from '#lib/pages/Mosaic.svelte';
	import PlaylistSongs from '#lib/pages/PlaylistSongs.svelte';
	import { headLine, phoneHeadLine } from '#lib/pages/playlistWords.ts';
	import { editorHistory } from '#lib/pages/editorHistory.svelte.ts';
	import SmartEditor from '#lib/pages/SmartEditor.svelte';
	import SmartStamp from '#lib/pages/SmartStamp.svelte';
	import FileTag from '#lib/pages/FileTag.svelte';
	import MissingLines from '#lib/pages/MissingLines.svelte';
	import PlaylistExport from '#lib/pages/PlaylistExport.svelte';
	import {
		fileHeadLine,
		fileMenuNote,
		filePhoneHeadLine,
		notFoundPill,
		readWords
	} from '#lib/pages/fileWords.ts';
	import { LivePreview, SmartDraft } from '#lib/pages/smartRules.svelte.ts';
	import { phoneSentence, sentence, sentenceLead } from '#lib/library/smartlist.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import { shuffles } from '#lib/shuffle/session.svelte.ts';
	import { playlistSource } from '#lib/shuffle/sources.ts';
	import type { PageProps } from './$types';

	let { params }: PageProps = $props();

	const id = $derived(params.id);
	const load = $derived(library.playlists.detail(id));
	const playlist = $derived(load.kind === 'ready' ? load.value : undefined);
	/** What the list knows, so the first frame has its name and mosaic before the songs come. */
	const known = $derived(playlist ?? library.playlists.summary(id));

	let windowWidth = $state(1280);
	const phone = $derived(windowWidth < 700);
	const smart = $derived(known?.smart === true);
	/** Where a file playlist comes from; undefined for any other. */
	const file = $derived(known?.file);
	const lost = $derived(file?.notFound ?? 0);
	/** Whether the lines that found no song are open, under the head. */
	let missingOpen = $state(false);
	/** Whether Export is open, in place of the buttons. */
	let exporting = $state(false);
	/** Delete's confirm, a surface in place of the buttons. */
	const deleteSurface = new SurfaceSlot();
	const now = Date.now();
	const rules = $derived(known?.rules);
	/** A Playlist rule's playlist, by name: undefined when it's gone. */
	const nameOf = (other: string): string | undefined => library.playlists.summary(other)?.name;
	const random = $derived(rules?.order === 'random' || rules?.limit?.by === 'random');

	/** The rules editor's draft, while it's open. */
	let editing = $state<SmartDraft | null>(null);
	let saving = $state(false);
	const preview = new LivePreview((request, signal) => library.playlists.preview(request, signal));
	editorHistory(
		() => (editing === null ? null : 'smart-rules'),
		() => {
			editing = null;
		}
	);
	const others = $derived(
		library.playlists.all
			.filter((p) => p.id !== id && !p.id.startsWith('pending:'))
			.map((p) => ({ id: p.id, name: p.name }))
	);
	const savedIds = $derived(
		playlist === undefined ? null : new Set(playlist.entries.map(({ track }) => track.id))
	);

	function editRules(): void {
		if (known?.rules !== undefined) {
			editing = new SmartDraft(known.rules, known.name);
		}
	}

	async function saveRules(): Promise<void> {
		const draft = editing;
		if (draft === null) {
			return;
		}
		saving = true;
		const name = draft.name.trim();
		try {
			await library.playlists.saveRules(
				id,
				draft.rules,
				name === '' || name === known?.name ? undefined : name
			);
			editing = null;
		} catch {
			// The draft stays open, so nothing typed is lost; the dock's Save can be pressed again.
		} finally {
			saving = false;
		}
	}

	async function copy(): Promise<void> {
		const made = await library.playlists.copy(id).catch(() => undefined);
		if (made !== undefined) {
			void goto(resolve('/playlists/[id]', { id: made.id }));
		}
	}

	$effect(() => {
		const wanted = id;
		untrack(() => void library.playlists.loadDetail(wanted));
	});

	/** The name as typed, while the heading has the focus; null shows the name it has. */
	let typing = $state<string | null>(null);
	let nameField: HTMLInputElement | undefined = $state();
	const name = $derived(typing ?? known?.name ?? '');

	function saveName(): void {
		const typed = typing;
		typing = null;
		const trimmed = typed?.trim() ?? '';
		if (known === undefined || trimmed === '' || trimmed === known.name) {
			return;
		}
		void library.playlists.rename(id, trimmed).catch(() => undefined);
	}

	let menu = $state<{ x: number; y: number } | null>(null);
	let confirming = $state(false);

	async function remove(): Promise<void> {
		confirming = false;
		void goto(resolve('/playlists'));
		await library.playlists.remove(id).catch(() => undefined);
	}

	/** Plays from an entry, through the playlist in its order: its album is here, or one fetch. */
	let latestTap = 0;
	function playFrom(entryId: string): void {
		if (playlist === undefined) {
			return;
		}
		const at = playlist.entries.findIndex((entry) => entry.id === entryId);
		const [first, ...rest] = playlist.entries.slice(Math.max(0, at));
		if (first === undefined) {
			return;
		}
		latestTap += 1;
		const tap = latestTap;
		const later = rest.map(({ track, albumId }) => ({ trackId: track.id, albumId }));
		const start = (): void => {
			const album = library.album(first.albumId);
			const track = album?.tracks.find((each) => each.id === first.track.id);
			if (album !== undefined && track !== undefined) {
				player.playList({ album, track }, later);
			}
		};
		if (library.album(first.albumId) !== undefined) {
			start();
			return;
		}
		void library.loadAlbum(first.albumId).then(() => {
			if (tap === latestTap) {
				start();
			}
		});
	}

	const source = $derived(
		playlist === undefined
			? undefined
			: playlistSource(id, playlist.name, () => {
					const now = library.playlists.detail(id);
					return now.kind === 'ready' ? now.value.entries : [];
				})
	);
</script>

<svelte:window bind:innerWidth={windowWidth} />

<svelte:head>
	<title>{known?.name ?? 'Playlist'} · Kitsune</title>
</svelte:head>

<div class="page-frame playlist" class:editing={editing !== null}>
	{#if editing !== null}
		<SmartEditor
			draft={editing}
			{preview}
			requestOf={(asked) => ({ rules: asked, id })}
			mosaic={known?.mosaic ?? []}
			savedCount={playlist?.songCount}
			{savedIds}
			playlists={others}
			{saving}
			onleave={() => {
				editing = null;
			}}
			onsave={() => void saveRules()}
			buds
		/>
	{:else if load.kind === 'missing'}
		<p class="note" role="status" data-morph-part="words" data-morph-key="missing">
			There's no such playlist.
			<a href={resolve('/playlists')}>All playlists</a>
		</p>
	{:else}
		<div class="hero" class:exporting>
			<span
				class="mosaic"
				data-morph-part="art"
				data-morph-key="mosaic"
				use:morph={{ id: `mosaic:${id}`, slot: 'hero', kind: 'cover' }}
			>
				<Mosaic
					albums={known?.mosaic ?? []}
					size={phone ? 112 : 200}
					radius={phone ? 'var(--radius-16)' : 'var(--radius-24)'}
				/>
				{#if smart}<SmartStamp
						size={phone ? 'phone' : 'page'}
					/>{:else if file !== undefined}<FileTag
						format={file.format}
						size={phone ? 'phone' : 'page'}
					/>{/if}
			</span>
			<div class="words" data-morph-part="words" data-morph-key="words">
				<span class="facts"
					>{known === undefined
						? 'PLAYLIST'
						: file !== undefined
							? phone
								? filePhoneHeadLine(known)
								: fileHeadLine(known)
							: phone
								? phoneHeadLine(known)
								: headLine(known)}</span
				>
				{#if file !== undefined}
					<h1 class="name plain" use:morph={{ id: `listname:${id}`, slot: 'hero', kind: 'title' }}>
						{known?.name}
					</h1>
					<div class="from">
						<span class="from-words"
							>FROM <span class="path">{file.path}</span
							>{#if !phone}{` · ${readWords(file.readAt, now)}`}{/if}</span
						>
						{#if lost > 0}
							<button
								type="button"
								class={['pill', missingOpen && 'open']}
								aria-expanded={missingOpen}
								onclick={() => {
									missingOpen = !missingOpen;
								}}
								>{notFoundPill(lost, phone)}<span aria-hidden="true">{missingOpen ? '▴' : '›'}</span
								></button
							>
						{/if}
					</div>
				{:else}
					<input
						class="name"
						use:morph={{ id: `listname:${id}`, slot: 'hero', kind: 'title' }}
						bind:this={nameField}
						value={name}
						aria-label="Playlist name"
						maxlength="200"
						autocomplete="off"
						spellcheck="false"
						oninput={(event) => {
							typing = event.currentTarget.value;
						}}
						onfocus={() => {
							typing ??= known?.name ?? '';
						}}
						onblur={saveName}
						onkeydown={(event) => {
							if (event.key === 'Enter') {
								event.preventDefault();
								nameField?.blur();
							} else if (event.key === 'Escape') {
								event.preventDefault();
								event.stopPropagation();
								typing = null;
								nameField?.blur();
							}
						}}
					/>
				{/if}
				{#if smart && rules !== undefined}
					{#if phone}
						<button type="button" class="sentence-phone" data-buds onclick={editRules}
							><span class="lead">{rules.match === 'all' ? 'ALL OF ' : 'ANY OF '}</span
							>{phoneSentence(rules, nameOf)} · <span class="edit-words">Edit rules ›</span></button
						>
					{:else}
						<p class="sentence" aria-label="Its rules">
							<span class="lead">{sentenceLead(rules)}</span>
							{#each sentence(rules, nameOf) as part, i (i)}
								{#if part.join !== undefined}<span class="joiner">{part.join}</span>{/if}
								<span class="part" class:group={part.group}>{part.text}</span>
							{/each}
						</p>
					{/if}
				{/if}
				{#if exporting && known !== undefined}
					<div class="export-slot">
						<PlaylistExport
							playlist={known}
							{phone}
							onleave={() => {
								exporting = false;
							}}
						/>
					</div>
				{:else if confirming}
					<span
						class="confirm"
						role="group"
						aria-label="Delete the playlist"
						{@attach deleteSurface.attach}
					>
						<span class="confirm-words">Delete {known?.name}? Its songs stay in your library.</span>
						<span class="buttons">
							<button
								type="button"
								class="ghost"
								onclick={() => {
									confirming = false;
								}}>Keep it</button
							>
							<button type="button" class="delete" onclick={() => void remove()}
								>Delete playlist</button
							>
						</span>
					</span>
				{:else}
					<span class="buttons" {@attach layInFront}>
						<button
							type="button"
							class="play"
							disabled={playlist === undefined || playlist.entries.length === 0}
							onclick={() => {
								const first = playlist?.entries.at(0);
								if (first !== undefined) {
									playFrom(first.id);
								}
							}}><Icon name="play" size={14} />Play</button
						>
						<button
							type="button"
							class="shuffle"
							disabled={source === undefined || playlist?.entries.length === 0}
							onpointerenter={() => {
								if (source !== undefined) {
									void shuffles.prime(source);
								}
							}}
							onclick={() => {
								if (source !== undefined) {
									shuffles.start(source);
								}
							}}>Shuffle</button
						>
						{#if smart && !phone}
							<button type="button" class="edit" data-buds onclick={editRules}
								><Icon name="rules" size={15} />Edit rules</button
							>
						{/if}
						{#if file !== undefined && !phone}
							<button type="button" class="edit" onclick={() => void copy()}
								><svg
									width="15"
									height="15"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
									stroke-linecap="round"
									stroke-linejoin="round"
									aria-hidden="true"
									><rect x="8" y="8" width="12" height="12" rx="2"></rect><path
										d="M16 8V5a1 1 0 0 0-1-1H5a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3"
									></path></svg
								>Copy to a playlist</button
							>
						{/if}
						<button
							type="button"
							class="more"
							aria-label="More for this playlist"
							aria-haspopup="menu"
							onclick={(event) => {
								const box = event.currentTarget.getBoundingClientRect();
								menu = { x: box.left, y: box.bottom + 8 };
							}}
							><svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"
								><circle cx="5" cy="12" r="1.8"></circle><circle cx="12" cy="12" r="1.8"
								></circle><circle cx="19" cy="12" r="1.8"></circle></svg
							></button
						>
						{#if !phone}
							<span class="grow"></span>
							<a class="all" href={resolve('/playlists')}>All playlists</a>
						{/if}
					</span>
				{/if}
			</div>
		</div>

		{#if missingOpen && file !== undefined}
			<div data-morph-part="panel" data-morph-key="missing-lines">
				<MissingLines
					lines={playlist?.missing ?? []}
					pastLimit={file.pastLimit}
					found={known?.songCount ?? 0}
					{phone}
					onhide={() => {
						missingOpen = false;
					}}
				/>
			</div>
		{/if}
		{#if playlist !== undefined}
			<PlaylistSongs {playlist} onplay={playFrom} smartRules={rules} file={file !== undefined} />
		{:else if load.kind === 'unavailable'}
			<p class="note" role="status" data-morph-part="panel" data-morph-key="unavailable">
				Its songs couldn't be loaded.
				<button type="button" onclick={() => void library.playlists.loadDetail(id, true)}
					>Try again</button
				>
			</p>
		{/if}
	{/if}
</div>

{#if menu !== null}
	<Floating
		x={menu.x}
		y={menu.y}
		label={known?.name ?? 'Playlist'}
		width={file === undefined ? 240 : 280}
		onclose={() => {
			menu = null;
		}}
	>
		{#if file !== undefined}
			<button
				type="button"
				role="menuitem"
				class="item"
				data-menu-item
				onclick={() => {
					menu = null;
					void copy();
				}}>Copy to a playlist <span class="hint">MAKE IT YOURS</span></button
			>
			<button
				type="button"
				role="menuitem"
				class="item"
				data-menu-item
				onclick={() => {
					menu = null;
					confirming = false;
					missingOpen = false;
					exporting = true;
				}}>Export .m3u</button
			>
			{#if lost > 0}
				<button
					type="button"
					role="menuitem"
					class="item"
					data-menu-item
					onclick={() => {
						menu = null;
						missingOpen = true;
					}}>Show what’s not found <span class="hint">{lost}</span></button
				>
			{/if}
			<p class="menu-note">{fileMenuNote}</p>
		{:else}
			<button
				type="button"
				role="menuitem"
				class="item"
				data-menu-item
				onclick={() => {
					menu = null;
					requestAnimationFrame(() => {
						nameField?.focus();
						nameField?.select();
					});
				}}>Rename</button
			>
			{#if smart}
				<button
					type="button"
					role="menuitem"
					class="item"
					data-menu-item
					onclick={() => {
						menu = null;
						editRules();
					}}>Edit rules</button
				>
				<button
					type="button"
					role="menuitem"
					class="item"
					data-menu-item
					onclick={() => {
						menu = null;
						void copy();
					}}>Copy to a playlist</button
				>
				{#if random}
					<button
						type="button"
						role="menuitem"
						class="item"
						data-menu-item
						onclick={() => {
							menu = null;
							void library.playlists.reseed(id).catch(() => undefined);
						}}>Pick again</button
					>
				{/if}
			{/if}
			<button
				type="button"
				role="menuitem"
				class="item"
				data-menu-item
				onclick={() => {
					menu = null;
					confirming = false;
					exporting = true;
				}}>Export .m3u</button
			>
			<div class="rule"></div>
			<button
				type="button"
				role="menuitem"
				class="item danger"
				data-menu-item
				onclick={() => {
					menu = null;
					confirming = true;
				}}><Icon name="trash" size={16} />Delete playlist</button
			>
		{/if}
	</Floating>
{/if}

<style>
	.playlist {
		--frame-top: 100px;

		display: flex;
		flex-direction: column;
		gap: calc(22px * var(--step));
	}

	.hero {
		display: flex;
		align-items: flex-end;
		gap: calc(28px * var(--step));
	}

	.mosaic {
		position: relative;
		flex: none;
		border-radius: var(--radius-24);
		box-shadow:
			var(--shadow-float),
			0 0 0 1px var(--edge);
	}

	.words {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.facts {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.name {
		width: 100%;
		box-sizing: border-box;
		padding: 0;
		border: 0;
		border-bottom: 2px solid transparent;
		background: transparent;
		outline: none;
		color: var(--text);
		font: 900 clamp(40px, 6vw, calc(76px * var(--step))) / 1 var(--font-sans);
		letter-spacing: var(--tracking-display);
		text-overflow: ellipsis;
	}

	.name:hover {
		border-bottom-color: var(--edge-strong);
	}

	.name:focus {
		border-bottom-color: var(--fox-line);
	}

	.buttons {
		display: flex;
		align-items: center;
		gap: 10px;
		margin-top: 6px;
	}

	.grow {
		flex: 1;
	}

	.buttons button,
	.all {
		height: var(--target);
		box-sizing: border-box;
		display: inline-flex;
		align-items: center;
		gap: 8px;
		border: 0;
		border-radius: var(--radius-pill);
		font: 600 calc(15px * var(--step)) var(--font-sans);
		color: var(--text);
		text-decoration: none;
	}

	.play {
		padding: 0 20px;
		background: var(--fox);
		color: var(--fox-ink) !important;
		font-weight: 700 !important;
	}

	.shuffle {
		padding: 0 18px;
		background: none;
		box-shadow: inset 0 0 0 1px var(--edge-strong);
	}

	.shuffle:hover {
		background: var(--hover);
	}

	.edit {
		padding: 0 18px;
		background: none;
		box-shadow: inset 0 0 0 1px var(--edge-strong);
	}

	.edit:hover {
		background: var(--hover);
	}

	.sentence {
		margin-top: 2px;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px 8px;
		font-size: 14px;
		color: var(--text-secondary);
	}

	.lead,
	.joiner {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.1em;
		color: var(--text-meta);
	}

	.joiner {
		font-size: 10px;
		letter-spacing: 0.12em;
	}

	.part {
		display: inline-flex;
		align-items: center;
		height: 28px;
		padding: 0 11px;
		border-radius: var(--radius-pill);
		background: var(--hover);
		border: 1px solid transparent;
		white-space: nowrap;
	}

	.part.group {
		border: 1px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
	}

	.sentence-phone {
		grid-column: 1 / -1;
		grid-row: 3;
		margin: 8px 0 0;
		padding: 10px 12px;
		border: 0;
		border-radius: var(--radius-16);
		background: var(--hover);
		color: var(--text-secondary);
		font: 400 13px / 1.5 var(--font-sans);
		text-align: left;
	}

	.sentence-phone .lead {
		font-size: 10px;
	}

	.edit-words {
		color: var(--fox-text);
		font-weight: 700;
	}

	.more {
		width: var(--target);
		justify-content: center;
		background: none;
		color: var(--text-meta) !important;
	}

	.more:hover {
		background: var(--hover);
	}

	.all {
		height: 40px;
		padding: 0 14px;
		background: var(--hover);
		font-size: 13px;
	}

	button:disabled {
		opacity: 0.5;
	}

	.confirm {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px 16px;
		margin-top: 6px;
	}

	.confirm-words {
		font-size: 15px;
		color: var(--text-secondary);
	}

	.ghost {
		padding: 0 16px;
		background: var(--hover);
	}

	.delete {
		padding: 0 18px;
		background: var(--danger);
		color: var(--ground) !important;
		font-weight: 700 !important;
	}

	.note {
		color: var(--text-secondary);
	}

	.note a,
	.note button {
		margin-left: 8px;
		border: 0;
		background: none;
		color: var(--fox-text);
		font: inherit;
		font-weight: 700;
	}

	.item {
		height: 40px;
		padding: 0 12px;
		display: flex;
		align-items: center;
		gap: 10px;
		border: 0;
		border-radius: var(--radius-12);
		background: transparent;
		color: var(--text);
		font: 600 14px var(--font-sans);
		text-align: left;
	}

	.item:hover:not(:disabled),
	.item:focus-visible {
		background: var(--hover);
	}

	.item:disabled {
		opacity: 0.6;
	}

	.hero.exporting {
		align-items: flex-start;
	}

	.hint {
		margin-left: auto;
		white-space: nowrap;
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.menu-note {
		margin: 4px 6px 2px;
		padding: 8px 6px 4px;
		border-top: 1px solid var(--edge);
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.06em;
		line-height: 1.5;
		color: var(--text-meta);
	}

	.plain {
		overflow: hidden;
		white-space: nowrap;
		line-height: 0.95;
	}

	.plain:hover {
		border-bottom-color: transparent;
	}

	.from {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 12px;
		font-family: var(--font-mono);
		font-size: calc(12px * var(--step));
		letter-spacing: 0.04em;
		color: var(--text-meta);
	}

	.from-words {
		min-width: 0;
	}

	.path {
		color: var(--text);
	}

	.pill {
		height: calc(28px * var(--step));
		padding: 0 calc(12px * var(--step));
		display: inline-flex;
		align-items: center;
		gap: 6px;
		border: 1.5px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--fox-text);
		font: 700 calc(11px * var(--step)) var(--font-mono);
		letter-spacing: 0.08em;
		transition: background-color 160ms var(--curve-morph);
	}

	.pill:hover,
	.pill.open {
		background: color-mix(in srgb, var(--fox) 12%, transparent);
	}

	.danger {
		color: var(--danger);
	}

	.rule {
		height: 1px;
		margin: 8px 4px;
		background: var(--edge);
	}

	@media (max-width: 699px) {
		.playlist {
			--frame-top: 84px;
		}

		/* The phone board: the mosaic beside the name, and the buttons under both. */
		.hero {
			display: grid;
			grid-template-columns: 112px minmax(0, 1fr);
			align-items: end;
			gap: 6px 14px;
		}

		.hero .mosaic {
			grid-row: 1 / 3;
		}

		.words {
			display: contents;
		}

		.facts {
			grid-column: 2;
			grid-row: 1;
			font-size: 10px;
		}

		.name {
			grid-column: 2;
			grid-row: 2;
			font-size: 36px;
			letter-spacing: -0.05em;
		}

		.name.plain {
			font-size: 32px;
			line-height: 1;
			white-space: normal;
		}

		/* One line: the path, cut short, and the pill. */
		.from {
			grid-column: 1 / -1;
			grid-row: 3;
			flex-wrap: nowrap;
			margin-top: 6px;
			font-size: 10px;
		}

		.from-words {
			flex: 1;
			overflow: hidden;
			text-overflow: ellipsis;
			white-space: nowrap;
		}

		.pill {
			flex: none;
			height: 32px;
			padding: 0 10px;
			font-size: 10px;
			letter-spacing: 0.06em;
		}

		.buttons,
		.confirm,
		.export-slot {
			grid-column: 1 / -1;
			grid-row: 4;
			margin-top: 12px;
		}

		.play,
		.shuffle {
			flex: 1;
			justify-content: center;
		}
	}
</style>
