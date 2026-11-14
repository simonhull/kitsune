<!--
	Add to playlist: the person's playlists, most recently added to first, each saying whether it has
	the songs already, a field to find one past eight, and New playlist…, which names one in place and
	puts the songs in it. Beside a menu's item it's a submenu; above the selection bar a popover; on a
	phone a sheet from the bottom, since a phone's menu has no room beside it.
-->
<script lang="ts">
	import { tick, untrack } from 'svelte';
	import { library } from '#lib/library/library.svelte.ts';
	import { distinct } from '#lib/library/playlistEdits.ts';
	import type { PlaylistSummary } from '#lib/library/source.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import Floating from './Floating.svelte';
	import Mosaic from './Mosaic.svelte';
	import { playlistActions } from './playlistActions.svelte.ts';
	import {
		byLastAdded,
		filterFrom,
		holdsWords,
		matching,
		newPlaylistName,
		sheetLine
	} from './playlistWords.ts';

	interface Props {
		/** The songs to add, in order. Empty while they're not known yet: `resolveTracks` finds them. */
		trackIds: readonly string[];
		/** Finds the songs when they're not known yet, as an album that hasn't loaded. */
		resolveTracks?: (() => Promise<readonly string[]>) | undefined;
		/** How many songs, for the heading, before they're known. */
		count: number;
		mode: 'submenu' | 'popover' | 'sheet';
		/** Where it opens: beside the item, or (`popover`) the bottom-left corner over the button. */
		x?: number;
		y?: number;
		/** The mono line over the list, as "ADD 3 SONGS TO". None in a submenu. */
		heading?: string | undefined;
		/** It closed: `done` when something was added, and whatever opened it closes too. */
		onclose: (done: boolean, refocus: boolean) => void;
	}

	let { trackIds, resolveTracks, count, mode, x = 0, y = 0, heading, onclose }: Props = $props();

	let typed = $state('');
	let naming = $state(false);
	let draft = $state('');
	let nameField: HTMLInputElement | undefined = $state();
	let sheet: HTMLElement | undefined = $state();

	// A sheet takes the focus as it opens, so Esc is its own.
	$effect(() => {
		sheet?.focus();
	});

	const songs = $derived(trackIds.length > 0 ? distinct(trackIds) : count);
	const all = $derived(byLastAdded(library.playlists.all));
	const shown = $derived(matching(all, typed));
	const filtering = $derived(all.length > filterFrom);

	$effect(() => {
		const ids = trackIds;
		untrack(() => void library.playlists.askHolds(ids));
	});

	/** Moves an element to the body, out of whatever layer the page drew it in. */
	function onBody(node: HTMLElement): { destroy(): void } {
		document.body.append(node);
		return {
			destroy() {
				node.remove();
			}
		};
	}

	// Where and how it opened, kept as it was: closing takes away what opened it.
	const at = untrack(() => ({ mode, x, y }));

	if (library.playlists.list.kind !== 'ready') {
		void library.playlists.load();
	}

	function holdsOf(p: PlaylistSummary): number | undefined {
		return library.playlists.holds(p.id, trackIds);
	}

	/**
	 * The songs, read before closing: closing takes away what opened it, and its props with it. Ones
	 * not known yet are found after.
	 */
	function songsNow(): Promise<readonly string[]> {
		const known = trackIds;
		const find = resolveTracks;
		return known.length > 0 || find === undefined ? Promise.resolve(known) : find();
	}

	function choose(p: PlaylistSummary): void {
		const songs = songsNow();
		onclose(true, true);
		void songs.then((ids) => {
			if (ids.length > 0) {
				playlistActions.add(p.id, ids);
			}
		});
	}

	async function startNaming(): Promise<void> {
		naming = true;
		draft = '';
		await tick();
		nameField?.focus();
	}

	function make(): void {
		const name = draft;
		const songs = songsNow();
		onclose(true, true);
		void songs.then((ids) => playlistActions.create(name, ids));
	}

	function nameKeys(event: KeyboardEvent): void {
		if (event.key === 'Enter') {
			event.preventDefault();
			event.stopPropagation();
			make();
		} else if (event.key === 'Escape') {
			event.preventDefault();
			event.stopPropagation();
			naming = false;
		}
		// The arrows and Tab move the caret, not the menu's focus.
		if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Tab'].includes(event.key)) {
			event.stopPropagation();
		}
	}

	/** On a phone a sheet has no Esc key of its own: the scrim and Esc both close it. */
	function sheetKeys(event: KeyboardEvent): void {
		if (event.key === 'Escape') {
			event.preventDefault();
			event.stopPropagation();
			onclose(false, true);
		}
	}
</script>

{#snippet list(big: boolean)}
	{#if heading !== undefined}
		<p class="heading">{heading}</p>
	{/if}
	{#if filtering}
		<label class="find" class:big>
			<Icon name="search" size={14} />
			<input
				type="search"
				placeholder="Find a playlist"
				aria-label="Find a playlist"
				data-menu-item
				bind:value={typed}
			/>
		</label>
	{/if}
	{#if naming}
		<div class="naming" class:big>
			<span class="name-it">NAME IT</span>
			<input
				bind:this={nameField}
				bind:value={draft}
				placeholder={newPlaylistName}
				aria-label="Playlist name"
				maxlength="200"
				onkeydown={nameKeys}
			/>
			<span class="name-acts">
				<button type="button" class="make" onclick={make}>Make it</button>
				<button
					type="button"
					class="leave"
					onclick={() => {
						naming = false;
					}}>Leave it</button
				>
			</span>
		</div>
	{:else}
		<button
			type="button"
			role="menuitem"
			class="item new"
			class:big
			data-menu-item
			onclick={() => void startNaming()}
		>
			{#if big}<span class="dashed"><Icon name="plus" size={16} /></span>{:else}<Icon
					name="plus"
					size={14}
				/>{/if}New playlist…
		</button>
	{/if}
	{#each shown as p (p.id)}
		{@const holds = holdsOf(p)}
		{@const words = holdsWords(holds, songs)}
		{@const full = holds !== undefined && holds > 0 && holds >= songs}
		<button
			type="button"
			role="menuitem"
			class="item playlist"
			class:big
			class:full
			data-menu-item
			data-playlist={p.id}
			disabled={p.id.startsWith('pending:')}
			onclick={() => {
				choose(p);
			}}
		>
			<Mosaic albums={p.mosaic} size={big ? 40 : 30} radius={big ? '10px' : '7px'} />
			<span class="names">
				<span class="pname">{p.name}</span>
				{#if big}
					<span class="sub">{sheetLine(p, holds, songs)}</span>
				{:else if words !== null}
					<span class="sub">{words}</span>
				{/if}
			</span>
			{#if full && !big}<span class="tick"><Icon name="tick" size={16} /></span>{/if}
		</button>
	{:else}
		{#if typed.trim() !== ''}
			<p class="none">No playlist is called that</p>
		{/if}
	{/each}
{/snippet}

{#if at.mode === 'sheet'}
	<!-- The scrim is a finger's way out; Esc is a keyboard's. Both live on the body, over the page's
	own layers, the toasts and the island. -->
	<div
		use:onBody
		class="scrim"
		data-submenu
		aria-hidden="true"
		onclick={() => {
			onclose(false, false);
		}}
	></div>
	<div
		use:onBody
		class="sheet"
		data-submenu
		role="dialog"
		aria-label="Add to playlist"
		tabindex="-1"
		bind:this={sheet}
		onkeydown={sheetKeys}
	>
		<span class="grab" aria-hidden="true"></span>
		{@render list(true)}
	</div>
{:else}
	<Floating
		x={at.x}
		y={at.y}
		label="Add to playlist"
		width={300}
		submenu={at.mode === 'submenu'}
		above={at.mode === 'popover'}
		onclose={(refocus) => {
			onclose(false, refocus);
		}}
	>
		{@render list(false)}
	</Floating>
{/if}

<style>
	.heading {
		padding: 8px 12px 6px;
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.find {
		height: 40px;
		margin-bottom: 4px;
		padding: 0 12px;
		display: flex;
		align-items: center;
		gap: 8px;
		border-radius: var(--radius-12);
		background: var(--control-wash);
		color: var(--text-meta);
	}

	.find.big {
		height: 44px;
		margin: 12px 0 8px;
		border-radius: 14px;
	}

	.find input,
	.naming input {
		flex: 1;
		min-width: 0;
		border: 0;
		background: transparent;
		outline: none;
		color: var(--text);
		font: 500 14px var(--font-sans);
	}

	.find.big input {
		font-size: 15px;
	}

	.find input::-webkit-search-cancel-button {
		display: none;
	}

	.item {
		flex: none;
		width: 100%;
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

	.item:hover,
	.item:focus-visible {
		background: var(--hover);
	}

	.item:disabled {
		opacity: 0.5;
	}

	.new {
		color: var(--fox-text);
		font-weight: 700;
	}

	.playlist {
		height: 48px;
	}

	.full {
		background: color-mix(in srgb, var(--fox) 10%, transparent);
	}

	.full:hover,
	.full:focus-visible {
		background: color-mix(in srgb, var(--fox) 16%, transparent);
	}

	.names {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 1px;
	}

	.pname {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.sub {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 400;
		color: var(--text-meta);
	}

	.tick {
		display: grid;
		color: var(--fox-text);
	}

	.naming {
		margin: 2px 0 6px;
		padding: 10px 12px 12px;
		display: flex;
		flex-direction: column;
		gap: 10px;
		border-radius: var(--radius-12);
		box-shadow: inset 0 0 0 2px var(--fox-line);
	}

	.name-it {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.naming input {
		height: 36px;
		border-bottom: 2px solid var(--fox-line);
		font-size: 17px;
		font-weight: 700;
	}

	.naming input::placeholder {
		font-style: italic;
		color: var(--text-meta);
	}

	.name-acts {
		display: flex;
		gap: 8px;
	}

	.name-acts button {
		height: 36px;
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
		padding: 0 12px;
		background: transparent;
		color: var(--text-secondary);
		font-weight: 600;
	}

	.none {
		padding: 10px 12px;
		font-size: 13px;
		color: var(--text-meta);
	}

	.scrim {
		position: fixed;
		inset: 0;
		z-index: 6;
		background: color-mix(in srgb, var(--ground) 60%, transparent);
		animation: fade var(--morph) var(--curve-morph) both;
	}

	@keyframes fade {
		from {
			opacity: 0;
		}
	}

	.sheet {
		position: fixed;
		z-index: 7;
		left: 0;
		right: 0;
		bottom: 0;
		max-height: calc(100dvh - 96px);
		overflow-y: auto;
		box-sizing: border-box;
		padding: 10px 16px calc(28px + env(safe-area-inset-bottom));
		display: flex;
		flex-direction: column;
		border-radius: var(--radius-28) var(--radius-28) 0 0;
		background: var(--card);
		border-top: 1px solid var(--edge-raised);
		animation: rise var(--morph) var(--curve-settle) both;
	}

	.sheet:focus {
		outline: none;
	}

	@keyframes rise {
		from {
			transform: translateY(40%);
			opacity: 0;
		}
	}

	.grab {
		display: block;
		width: 40px;
		height: 4px;
		margin: 0 auto 14px;
		border-radius: 2px;
		background: var(--edge-strong);
	}

	.sheet .heading {
		padding: 0;
	}

	.big.item {
		height: 52px;
		padding: 0 4px;
		gap: 12px;
		font-size: 15px;
	}

	.big.playlist {
		height: 56px;
	}

	.dashed {
		display: grid;
		place-items: center;
		width: 40px;
		height: 40px;
		box-sizing: border-box;
		border-radius: 10px;
		border: 2px dashed color-mix(in srgb, var(--fox) 60%, transparent);
	}

	.naming.big {
		margin-top: 6px;
	}

	@media (prefers-reduced-motion: reduce) {
		.sheet,
		.scrim {
			animation-duration: 120ms;
		}
	}
</style>
