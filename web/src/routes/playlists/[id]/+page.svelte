<!--
	One playlist: its mosaic, what it holds, its name as a heading that edits in place (saved on blur
	or Enter, put back by Esc), Play, Shuffle and ⋯ (Rename, Delete with a confirm in place), and its
	songs in its order, which a grip moves.
-->
<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { library } from '#lib/library/library.svelte.ts';
	import Floating from '#lib/pages/Floating.svelte';
	import Mosaic from '#lib/pages/Mosaic.svelte';
	import PlaylistSongs from '#lib/pages/PlaylistSongs.svelte';
	import { headLine, phoneHeadLine } from '#lib/pages/playlistWords.ts';
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

<div class="page-frame playlist">
	{#if load.kind === 'missing'}
		<p class="note" role="status">
			There's no such playlist.
			<a href={resolve('/playlists')}>All playlists</a>
		</p>
	{:else}
		<div class="hero">
			<span class="mosaic">
				<Mosaic
					albums={known?.mosaic ?? []}
					size={phone ? 112 : 200}
					radius={phone ? 'var(--radius-16)' : 'var(--radius-24)'}
				/>
			</span>
			<div class="words">
				<span class="facts"
					>{known === undefined ? 'PLAYLIST' : phone ? phoneHeadLine(known) : headLine(known)}</span
				>
				<input
					class="name"
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
				{#if confirming}
					<span class="confirm" role="group" aria-label="Delete the playlist">
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
					<span class="buttons">
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

		{#if playlist !== undefined}
			<PlaylistSongs {playlist} onplay={playFrom} />
		{:else if load.kind === 'unavailable'}
			<p class="note" role="status">
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
		width={240}
		onclose={() => {
			menu = null;
		}}
	>
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
		<button type="button" role="menuitem" class="item quiet" data-menu-item disabled
			>Export .m3u <span class="soon">SOON</span></button
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

	.quiet {
		color: var(--text-secondary);
	}

	.soon {
		margin-left: auto;
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-meta);
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

		.buttons,
		.confirm {
			grid-column: 1 / -1;
			grid-row: 3;
			margin-top: 12px;
		}

		.play,
		.shuffle {
			flex: 1;
			justify-content: center;
		}
	}
</style>
