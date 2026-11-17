<!--
	The Playlists page: the person's playlists as cards, each a mosaic of its first albums with its
	name and size, and a dashed card that turns into a name field in place to make a new one. A smart
	playlist's card wears the SMART stamp; after New playlist, a fox-dashed card opens the rules
	editor on a fresh draft, which makes nothing until it's saved.

	Below a hairline, From files: the playlists read from .m3u files in the library, which everyone
	on the server sees, each with its dog-ear and paper tag.
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import { morph } from '#lib/morph/registry.ts';
	import Mosaic from '#lib/pages/Mosaic.svelte';
	import SmartStamp from '#lib/pages/SmartStamp.svelte';
	import FileTag from '#lib/pages/FileTag.svelte';
	import { countWithFiles, fileCardLine } from '#lib/pages/fileWords.ts';
	import type { PlaylistSummary } from '#lib/library/source.ts';
	import { playlistActions } from '#lib/pages/playlistActions.svelte.ts';
	import { cardLine, newPlaylistName } from '#lib/pages/playlistWords.ts';

	const lists = $derived(library.playlists.list);
	const all = $derived(library.playlists.all);
	const own = $derived(all.filter(({ source }) => source !== 'file'));
	const files = $derived(all.filter(({ source }) => source === 'file'));
	const smartCount = $derived(all.filter(({ smart }) => smart).length);

	let naming = $state(false);
	let draft = $state('');
	let field: HTMLInputElement | undefined = $state();

	$effect(() => {
		void library.playlists.load();
	});

	$effect(() => {
		if (naming) {
			field?.focus();
		}
	});

	function make(): void {
		const typed = draft;
		naming = false;
		draft = '';
		void playlistActions.create(typed, []);
	}

	/** Opening a card starts reading its songs a moment early: on the pointer, or the focus. */
	function early(id: string): void {
		if (!id.startsWith('pending:')) {
			void library.playlists.loadDetail(id);
		}
	}
</script>

<svelte:head>
	<title>Playlists · Kitsune</title>
</svelte:head>

<div class="page-frame playlists">
	<div class="intro" data-morph-part="intro" data-morph-key="intro">
		<h1>Playlists<span class="dot" aria-hidden="true">.</span></h1>
		{#if lists.kind === 'ready'}
			<p class="facts">{countWithFiles(all.length, smartCount, files.length)}</p>
		{/if}
	</div>

	{#if lists.kind === 'unavailable'}
		<p class="note" role="status" data-morph-part="intro" data-morph-key="note">
			Your playlists couldn't be loaded.
			<button type="button" onclick={() => void library.playlists.load()}>Try again</button>
		</p>
	{:else}
		<div class="grid">
			{#each own as p (p.id)}
				{@render card(p)}
			{/each}
			<div class="new" data-morph-part="card" data-morph-key="new">
				{#if naming}
					<form
						class="naming"
						onsubmit={(event) => {
							event.preventDefault();
							make();
						}}
					>
						<span class="naming-in">
							<label class="label" for="new-playlist-name">NAME IT</label>
							<input
								id="new-playlist-name"
								bind:this={field}
								bind:value={draft}
								placeholder={newPlaylistName}
								maxlength="200"
								autocomplete="off"
								onkeydown={(event) => {
									if (event.key === 'Escape') {
										event.preventDefault();
										naming = false;
										draft = '';
									}
								}}
							/>
							<span class="buttons">
								<button type="submit" class="make">Make it</button>
								<button
									type="button"
									class="leave"
									onclick={() => {
										naming = false;
										draft = '';
									}}>Leave it</button
								>
							</span>
						</span>
					</form>
				{:else}
					<button
						type="button"
						class="dashed"
						onclick={() => {
							naming = true;
						}}
						><span class="dashed-in"><Icon name="plus" size={28} /><span>New playlist</span></span
						></button
					>
				{/if}
			</div>
			<a
				class="dashed smart"
				href={resolve('/playlists/smart')}
				data-morph-part="card"
				data-morph-key="new-smart"
			>
				<span class="dashed-in"
					><Icon name="rules" size={28} /><span>New smart playlist</span><span class="sub"
						>RULES CHOOSE THE SONGS</span
					></span
				>
			</a>
		</div>
		{#if files.length > 0}
			<section class="files" aria-labelledby="from-files">
				<div class="files-head" data-morph-part="intro" data-morph-key="files-head">
					<h2 id="from-files">From files</h2>
					<span class="files-note"
						>.M3U IN THE LIBRARY · EVERYONE ON THIS SERVER SEES THESE · EDIT THE FILE TO CHANGE ONE</span
					>
				</div>
				<div class="grid">
					{#each files as p (p.id)}
						{@render card(p)}
					{/each}
				</div>
			</section>
		{/if}
	{/if}
</div>

{#snippet card(p: PlaylistSummary)}
	{@const pending = p.id.startsWith('pending:')}
	{@const filed = p.source === 'file' ? fileCardLine(p) : null}
	<a
		class="card"
		data-morph-part="card"
		data-morph-key={`playlist:${p.id}`}
		href={pending ? undefined : resolve('/playlists/[id]', { id: p.id })}
		aria-disabled={pending ? 'true' : undefined}
		onpointerenter={() => {
			early(p.id);
		}}
		onfocus={() => {
			early(p.id);
		}}
	>
		<span class="cover" use:morph={{ id: `mosaic:${p.id}`, slot: 'card', kind: 'cover' }}
			><Mosaic albums={p.mosaic} radius="var(--radius-24)" />{#if p.smart}<SmartStamp
				/>{:else if p.file !== undefined}<FileTag format={p.file.format} />{/if}</span
		>
		<span class="name" use:morph={{ id: `listname:${p.id}`, slot: 'card', kind: 'title' }}
			>{p.name}</span
		>
		{#if filed === null}
			<span class="line">{cardLine(p)}</span>
		{:else}
			<span class="line">{filed[0]}<span class="lost">{filed[1]}</span></span>
		{/if}
	</a>
{/snippet}

<style>
	.playlists {
		--frame-top: 106px;

		display: flex;
		flex-direction: column;
		gap: calc(32px * var(--step));
	}

	.intro {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	h1 {
		font-size: clamp(44px, 7vw, calc(72px * var(--step)));
		font-weight: 900;
		letter-spacing: var(--tracking-display);
		line-height: 1;
	}

	.dot {
		color: var(--fox);
	}

	.facts,
	.line,
	.label {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.facts {
		margin-left: 4px;
	}

	.note {
		color: var(--text-secondary);
	}

	.note button {
		margin-left: 8px;
		border: 0;
		background: none;
		color: var(--fox-text);
		font: inherit;
		font-weight: 700;
		text-decoration: underline;
	}

	/* The album grid's steps, with bigger cards: 4 at 1440, 7 at 4K. */
	.grid {
		display: grid;
		grid-template-columns: repeat(var(--columns, 4), minmax(0, 1fr));
		gap: calc(28px * var(--step));
	}

	@media (max-width: 699px) {
		.grid {
			--columns: 2;

			gap: 20px 16px;
		}
	}

	@media (min-width: 700px) and (max-width: 1099px) {
		.grid {
			--columns: 3;
		}
	}

	@media (min-width: 1680px) {
		.grid {
			--columns: 5;
		}
	}

	@media (min-width: 2560px) {
		.grid {
			--columns: 6;
		}
	}

	@media (min-width: 3200px) {
		.grid {
			--columns: 7;
		}
	}

	.card {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 10px;
		color: inherit;
		text-decoration: none;
	}

	.files {
		display: flex;
		flex-direction: column;
		gap: calc(16px * var(--step));
		padding-top: calc(18px * var(--step));
		border-top: 1px solid var(--edge);
	}

	.files-head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 6px 14px;
	}

	h2 {
		font-size: calc(22px * var(--step));
		font-weight: 800;
		letter-spacing: -0.03em;
	}

	.files-note {
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.lost {
		color: var(--fox-text);
	}

	.card[aria-disabled='true'] {
		opacity: 0.6;
	}

	.cover {
		position: relative;
		display: block;
		border-radius: var(--radius-24);
		box-shadow: 0 0 0 1px var(--edge);
		transition: transform 220ms var(--curve-settle);
	}

	.card:hover .cover {
		transform: translateY(-3px);
	}

	.card:focus-visible {
		outline: none;
	}

	.card:focus-visible .cover {
		box-shadow:
			0 0 0 1px var(--edge),
			0 0 0 3px var(--fox-line);
	}

	.name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: var(--type-title);
		font-weight: 700;
	}

	.new {
		min-width: 0;
	}

	.dashed,
	.naming {
		width: 100%;
		aspect-ratio: 1;
		box-sizing: border-box;
		display: grid;
		place-items: center;
		border-radius: var(--radius-24);
	}

	.dashed {
		border: 2px dashed var(--edge-strong);
		background: none;
		color: var(--text-secondary);
		font: 700 calc(15px * var(--step)) var(--font-sans);
		transition:
			border-color 160ms var(--curve-morph),
			color 160ms var(--curve-morph);
	}

	.dashed:hover,
	.dashed:focus-visible {
		outline: none;
		border-color: var(--fox-line);
		color: var(--text);
	}

	.smart {
		border-color: color-mix(in srgb, var(--fox-line) 55%, transparent);
		color: var(--fox-text);
		text-decoration: none;
	}

	.smart:hover,
	.smart:focus-visible {
		color: var(--fox-text);
	}

	.sub {
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 400;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.dashed-in {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 10px;
	}

	.naming {
		padding: calc(24px * var(--step));
		border: 2px solid var(--fox-line);
		background: var(--card);
	}

	.naming-in {
		width: 100%;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.label {
		font-size: 11px;
	}

	input {
		width: 100%;
		height: var(--target);
		box-sizing: border-box;
		padding: 0;
		border: 0;
		border-bottom: 2px solid var(--fox-line);
		background: transparent;
		outline: none;
		color: var(--text);
		font: 700 calc(20px * var(--step)) var(--font-sans);
	}

	input::placeholder {
		font-style: italic;
		color: var(--text-meta);
	}

	.buttons {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	.make,
	.leave {
		height: 40px;
		border: 0;
		border-radius: var(--radius-pill);
		font: 700 14px var(--font-sans);
	}

	.make {
		padding: 0 16px;
		background: var(--fox);
		color: var(--fox-ink);
	}

	.leave {
		padding: 0 14px;
		background: none;
		color: var(--text-secondary);
		font-weight: 600;
	}

	@media (prefers-reduced-motion: reduce) {
		.cover {
			transition: none;
		}
	}
</style>
