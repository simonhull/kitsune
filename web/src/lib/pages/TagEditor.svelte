<!--
	The one tag editor, over the page that opened it: an album, some songs, or one. The songs on the
	left, every one ticked to start with; on the right what the ticked songs share (Tags, Shuffle,
	Matches), or one song's every tag; the dock at the foot. A phone has Songs, Tags and More as tabs.
-->
<script lang="ts">
	import { onDestroy, onMount, tick, untrack } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import Cover from '#lib/cover/Cover.svelte';
	import { sharedFields, shownOf } from '#lib/library/edit.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { matchInEditor } from '#lib/matching/hooks.ts';
	import { matchSheet } from '#lib/matching/sheet.svelte.ts';
	import EditorSongs from './EditorSongs.svelte';
	import SharedField from './SharedField.svelte';
	import ShuffleTab from './ShuffleTab.svelte';
	import SongTags from './SongTags.svelte';
	import TagBar from './TagBar.svelte';
	import type { EditSession, Written } from './tagEditor.svelte.ts';

	interface Props {
		session: EditSession;
		/** Drops the edit, as Back does. */
		leave: () => void;
		/** Every file and rule saved: the host closes the editor and says so. */
		done: (written: Extract<Written, { kind: 'done' }>) => void;
	}

	let { session: given, leave, done }: Props = $props();
	/** One editor for this component's life: the host keys it, and a field left as it goes reads it. */
	const session = untrack(() => given);

	const phone = new MediaQuery('(max-width: 640px)', false);

	type Tab = 'tags' | 'shuffle' | 'matches';
	type PhoneTab = 'songs' | 'tags' | 'more';

	// Someone who can't change tags starts where they can change something.
	let tab = $state<Tab>(untrack(() => session.canTag) ? 'tags' : 'shuffle');
	let phoneTab = $state<PhoneTab>(untrack(() => session.canTag) ? 'tags' : 'more');
	let finding = $state(false);
	let find = $state('');
	let replace = $state('');
	let head: HTMLElement | undefined = $state();
	/** What had the focus before the editor took it, to give it back. */
	let opener: HTMLElement | null = null;

	const albumId = $derived(session.albumIds.at(0));
	const entry = $derived(albumId === undefined ? undefined : library.entry(albumId));
	const total = $derived(session.tracks.length);
	/** What the head counts: known before the files are read. */
	const count = $derived(
		session.songCount > 0 ? session.songCount : (entry?.trackCount ?? session.songCount)
	);
	const ticked = $derived(session.tickedIds.length);
	const openTrack = $derived(session.open === null ? undefined : session.track(session.open));
	const match = $derived(matchInEditor(session));
	const tabs = $derived<{ id: Tab; label: string }[]>([
		{ id: 'tags', label: 'Tags' },
		{ id: 'shuffle', label: 'Shuffle' },
		...(match === undefined ? [] : [{ id: 'matches' as const, label: 'Matches' }])
	]);
	const shownTab = $derived(tabs.some(({ id }) => id === tab) ? tab : 'tags');
	const albumMixed = $derived(
		session.shape === 'selection' && shownOf(session.tickedTracks, session.draft, 'album').mixed
	);
	const plural = (count: number, one: string): string =>
		`${String(count)} ${one}${count === 1 ? '' : 's'}`;
	const eyebrow = $derived(
		session.shape === 'album'
			? ['EDITING TAGS', entry?.artist, entry?.year?.toString(), plural(count, 'FILE')]
					.filter((part) => part !== undefined && part !== '')
					.join(' · ')
					.toUpperCase()
			: `EDITING · ${plural(count, 'SONG')} FROM ${plural(session.albumIds.length, 'ALBUM')} · FROM YOUR SELECTION`.toUpperCase()
	);
	const phoneEyebrow = $derived(
		session.shape === 'album' ? `EDITING TAGS · ${plural(count, 'FILE')}`.toUpperCase() : eyebrow
	);
	const heading = $derived(
		session.shape === 'album' ? (entry?.title ?? '') : plural(count, 'song')
	);
	const sharedLine = $derived(
		ticked === 0
			? 'Tick a song to edit it.'
			: `${ticked === total ? 'Editing every song.' : `Editing ${String(ticked)} of ${String(total)}.`} What they share is filled in; leave "Mixed" alone and each keeps its own.`
	);
	/** Why the Tags tab can't edit, when it can't: the Shuffle tab still can. */
	const noTags = $derived(
		library.user?.role === 'admin'
			? "Tag writes are off on this server, so the files can't be changed from here. The Shuffle tab still works."
			: 'Only an administrator can change tags. Your shuffle rules are yours: set them on the Shuffle tab.'
	);

	// A match fills the fields: they're where it shows.
	$effect(() => {
		if (session.matched !== null) {
			untrack(() => {
				tab = 'tags';
				phoneTab = 'tags';
			});
		}
	});

	onMount(() => {
		opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		head?.focus({ preventScroll: true });
	});

	onDestroy(() => {
		if (opener?.isConnected === true) {
			opener.focus({ preventScroll: true });
		}
	});

	/** Opens a song to every tag, or goes back to them all; a phone shows it on the Tags tab. */
	function expand(id: string): void {
		session.expand(session.open === id ? null : id);
		phoneTab = 'tags';
	}

	async function closeSong(): Promise<void> {
		const was = session.open;
		session.expand(null);
		await tick();
		if (was !== null && !phone.current) {
			document.getElementById(`more-${was}`)?.focus();
		}
	}

	/** Brings a field with a problem into view, wherever it is, and gives it the focus. */
	async function reveal(field: string): Promise<void> {
		const song = session.tracks.find((track) => field.endsWith(`-${track.id}`));
		if (song === undefined) {
			session.expand(null);
			tab = 'tags';
			phoneTab = 'tags';
		} else if (field.startsWith('title-')) {
			phoneTab = 'songs';
		} else {
			session.expand(song.id);
			phoneTab = 'tags';
		}
		await tick();
		document.getElementById(field)?.focus();
	}

	async function write(): Promise<void> {
		const written = await session.write();
		if (written.kind === 'done') {
			done(written);
		} else if (written.kind === 'problem') {
			await reveal(written.field);
		}
	}

	function onkeydown(event: KeyboardEvent): void {
		if (
			event.key === 'Escape' &&
			!event.defaultPrevented &&
			matchSheet.current === null &&
			!session.saving
		) {
			event.preventDefault();
			leave();
		}
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

<svelte:window {onkeydown} />

{#snippet tagsTab()}
	{#if !session.canTag}
		<p class="line">{noTags}</p>
	{:else}
		<p class="line">{sharedLine}</p>
		{#if ticked > 0}
			<div class="grid" onfocusout={left}>
				{#each sharedFields as spec (spec.key)}
					<SharedField {spec} {session} />
				{/each}
			</div>
			{#if albumMixed}
				<p class="note">
					Album is mixed: giving these songs one album name moves them into that album, and the
					Library follows.
				</p>
			{/if}
			<p class="mono">QUICK JOBS</p>
			<div class="jobs">
				<button
					type="button"
					onclick={() => {
						session.numberFromOne();
					}}>Number from 1</button
				>
				<button
					type="button"
					onclick={() => {
						session.titlesFromFiles();
					}}>Titles from file names</button
				>
				<button
					type="button"
					onclick={() => {
						session.titleCase();
					}}>Title Case</button
				>
				<button type="button" aria-expanded={finding} onclick={() => (finding = !finding)}
					>Find and replace</button
				>
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
	{/if}
{/snippet}

{#snippet shuffleTab()}
	<p class="mono">IN YOUR SHUFFLE · NOT A TAG, NEVER WRITTEN TO A FILE</p>
	{#if ticked === 0}
		<p class="line">Tick a song to edit it.</p>
	{:else}
		<ShuffleTab {session} />
	{/if}
{/snippet}

{#snippet matchesTab()}
	{#if match !== undefined}
		<p class="line">
			Look this album up on MusicBrainz. Kitsune proposes what the tags would become; you tick what
			to take, and it lands in the fields like any other change, before anything is written.
		</p>
		<button type="button" class="find-matches" onclick={match}>Find matches</button>
		<p class="mono">ONLY FOR ONE ALBUM AT A TIME</p>
	{/if}
{/snippet}

<div class="layer" class:island={library.playing !== null} role="region" aria-label="Editing songs">
	<div class="editor">
		<header class="head">
			{#if session.shape === 'album' && albumId !== undefined}
				<span class="art"><Cover {albumId} {entry} place="small" box={96} /></span>
			{/if}
			<div class="words">
				<p class="eyebrow" tabindex="-1" bind:this={head}>
					<span class="wide-words">{eyebrow}</span><span class="phone-words">{phoneEyebrow}</span>
				</p>
				<h1>{heading}<span class="dot">.</span></h1>
			</div>
		</header>

		<div class="phone-tabs" role="tablist" aria-label="Songs or tags">
			<button
				type="button"
				role="tab"
				aria-selected={phoneTab === 'songs'}
				onclick={() => (phoneTab = 'songs')}>Songs · {ticked} of {total}</button
			>
			<button
				type="button"
				role="tab"
				aria-selected={phoneTab === 'tags'}
				onclick={() => (phoneTab = 'tags')}>Tags</button
			>
			<button
				type="button"
				role="tab"
				aria-selected={phoneTab === 'more'}
				onclick={() => (phoneTab = 'more')}>More</button
			>
		</div>

		<div class="columns" data-phone-tab={phoneTab}>
			<section class="songs" aria-label="Songs in this edit">
				<EditorSongs {session} {expand} />
			</section>

			<section
				class="side"
				aria-label={openTrack === undefined
					? 'Edit the ticked songs'
					: `Every tag of ${openTrack.tags.fields.title?.at(0) ?? openTrack.tags.file}`}
			>
				{#if phone.current && phoneTab === 'more'}
					<div class="stack">
						{@render shuffleTab()}
						{#if match !== undefined}
							<div class="more-matches">{@render matchesTab()}</div>
						{/if}
					</div>
				{:else if openTrack !== undefined}
					{#key openTrack.id}
						<SongTags track={openTrack} {session} close={() => void closeSong()} />
					{/key}
				{:else if phone.current}
					<div class="stack">{@render tagsTab()}</div>
				{:else}
					<div class="stack">
						<div class="tabs" role="tablist" aria-label="What to edit">
							{#each tabs as { id, label } (id)}
								<button
									type="button"
									role="tab"
									id={`edit-tab-${id}`}
									aria-selected={shownTab === id}
									aria-controls="edit-panel"
									onclick={() => (tab = id)}>{label}</button
								>
							{/each}
						</div>
						<div
							id="edit-panel"
							class="stack"
							role="tabpanel"
							aria-labelledby={`edit-tab-${shownTab}`}
						>
							{#if shownTab === 'shuffle'}
								{@render shuffleTab()}
							{:else if shownTab === 'matches'}
								{@render matchesTab()}
							{:else}
								{@render tagsTab()}
							{/if}
						</div>
					</div>
				{/if}
			</section>
		</div>
	</div>

	<TagBar {session} {leave} write={() => void write()} />
</div>

<style>
	/* Over the page, which keeps its place beneath: the nav stays, and the island under the dock. */
	.layer {
		--dock-bottom: 24px;

		position: fixed;
		inset: 0;
		z-index: 2;
		overflow: hidden;
		background: var(--ground);
		/* Backwards only: a translate left on the layer would hold the fixed dock in it. */
		animation: rise var(--morph) var(--curve-morph) backwards;
	}

	.layer.island {
		--dock-bottom: 108px;
	}

	@keyframes rise {
		from {
			opacity: 0;
			translate: 0 16px;
		}
	}

	/*
	 * The window's height, as the board has it: the head, then the two columns down to the dock, each
	 * scrolling on its own, so the fields stay beside however long a list of songs.
	 */
	.editor {
		container-type: inline-size;
		display: flex;
		flex-direction: column;
		height: 100%;
		padding: 100px 32px calc(var(--dock-bottom) + 80px);
	}

	.head {
		display: flex;
		align-items: flex-end;
		gap: 22px;
		padding: 0 32px;
	}

	.art {
		position: relative;
		flex: none;
		width: 96px;
		height: 96px;
		border-radius: 18px;
		overflow: hidden;
		box-shadow: 0 0 0 1px var(--edge-card);
	}

	.words {
		display: flex;
		flex-direction: column;
		gap: 8px;
		min-width: 0;
	}

	.eyebrow,
	.mono {
		font-family: var(--font-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.eyebrow {
		font-size: var(--type-mono);
		outline: none;
	}

	.phone-words {
		display: none;
	}

	h1 {
		overflow: hidden;
		padding-bottom: 0.09em;
		margin-bottom: -0.09em;
		font-size: 80px;
		font-weight: 900;
		letter-spacing: -0.06em;
		line-height: 0.9;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.dot {
		color: var(--fox-text);
	}

	/* The board's 744 and 608 at 1440, 1520 and 952 at the 2560 the step zooms from. */
	.columns {
		flex: 1;
		display: grid;
		grid-template-columns: minmax(0, 1fr) clamp(340px, 30.71cqw + 185px, 952px);
		gap: 24px;
		min-height: 0;
		margin-top: 36px;
	}

	.songs,
	.side {
		min-width: 0;
		min-height: 0;
		overflow-y: auto;
		border-radius: var(--radius-28);
		scrollbar-width: thin;
	}

	.songs {
		padding: 10px 18px 16px;
		background: var(--card);
	}

	.side {
		container-type: inline-size;
		padding: 20px 24px 24px;
		background: var(--card-raised);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--fox-line) 50%, transparent);
	}

	.stack {
		display: flex;
		flex-direction: column;
		gap: 14px;
	}

	.tabs,
	.phone-tabs {
		display: flex;
		gap: 6px;
		padding: 4px;
		border-radius: var(--radius-pill);
		background: var(--input);
		border: 1px solid var(--edge-card);
	}

	.tabs {
		width: fit-content;
	}

	.tabs button,
	.phone-tabs button {
		height: 40px;
		padding: 0 16px;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text-secondary);
		font-size: 13px;
		font-weight: 700;
	}

	.tabs button[aria-selected='true'],
	.phone-tabs button[aria-selected='true'] {
		background: var(--fox);
		color: var(--fox-ink);
	}

	.phone-tabs {
		display: none;
	}

	.line,
	.note {
		font-size: 14px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.note {
		font-size: 13px;
		color: var(--text-meta);
	}

	.mono {
		margin-top: 4px;
		font-size: 11px;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 8px 20px;
	}

	.jobs {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	.jobs button,
	.find .go {
		height: var(--target);
		padding: 0 16px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-field);
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
		height: 32px;
		padding: 0;
		border: 0;
		outline: none;
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: var(--type-body);
		font-weight: 600;
	}

	.find .go,
	.find-matches {
		background: var(--fox);
		border-color: transparent;
		color: var(--fox-ink);
		font-weight: 700;
	}

	.find .go:disabled {
		opacity: 0.5;
	}

	.find-matches {
		align-self: flex-start;
		height: var(--target);
		padding: 0 18px;
		border: 0;
		border-radius: var(--radius-pill);
		font-size: 14px;
	}

	/* From 2560 the editor takes the step up, so a 4K screen reads like the 1440 board, wider. */
	@media (min-width: 2560px) {
		.editor {
			zoom: 1.5;
		}

		.layer :global(.dock) {
			scale: 1.5;
			transform-origin: bottom center;
		}
	}

	/* A phone: Songs, Tags and More as tabs, the fields in one column. */
	@media (max-width: 640px) {
		.layer {
			overflow-y: auto;
		}

		.editor {
			display: block;
			height: auto;
			padding: 88px 16px calc(var(--dock-bottom) + 100px);
		}

		.head {
			padding: 0;
		}

		.art,
		.wide-words {
			display: none;
		}

		.phone-words {
			display: inline;
		}

		.words {
			gap: 6px;
		}

		h1 {
			font-size: 40px;
			line-height: 0.95;
		}

		.phone-tabs {
			display: flex;
			margin: 14px 0;
		}

		.phone-tabs button {
			flex: 1 1 auto;
			padding: 0 6px;
			white-space: nowrap;
		}

		.columns {
			display: block;
			margin-top: 0;
		}

		.side {
			overflow: visible;
			padding: 0;
			background: none;
			box-shadow: none;
		}

		.songs {
			overflow: visible;
			padding: 4px 12px 12px;
		}

		.columns[data-phone-tab='songs'] .side,
		.columns:not([data-phone-tab='songs']) .songs {
			display: none;
		}

		.stack {
			gap: 10px;
		}

		.grid {
			grid-template-columns: minmax(0, 1fr);
			gap: 6px;
		}

		.grid :global(.field) {
			grid-column: span 1;
		}

		.more-matches {
			display: flex;
			flex-direction: column;
			gap: 10px;
			margin-top: 8px;
			padding-top: 16px;
			border-top: 1px solid var(--edge-card);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.layer {
			animation: none;
		}
	}
</style>
