<!--
	An album's own ⋯: Play, Play next, Add to queue and Shuffle, then Edit tags…, Find matches… and Go to artist. One
	menu, from the album page's hero and from any album card.
-->
<script lang="ts">
	import { library } from '#lib/library/library.svelte.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import { albumActions, albumMenu } from './albumMenu.svelte.ts';
	import Floating from './Floating.svelte';
	import { pickerBeside, type PickerPlace } from './pickerPlace.ts';
	import PlaylistPicker from './PlaylistPicker.svelte';
	import { addHeading } from './playlistWords.ts';

	const open = $derived(albumMenu.current);
	const album = $derived(open === null ? undefined : library.album(open.entry.id));
	const actions = $derived(open === null ? null : albumActions(open.entry, album));
	const line = $derived.by(() => {
		if (open === null) {
			return '';
		}
		const parts = [open.entry.artist.toUpperCase()];
		if (album !== undefined) {
			const minutes = Math.max(1, Math.round(album.durationMs / 60_000));
			parts.push(`${String(album.tracks.length)} ${album.tracks.length === 1 ? 'SONG' : 'SONGS'}`);
			parts.push(`${String(minutes)} MIN`);
		}
		return parts.join(' · ');
	});

	/** Add to playlist, beside its item; the album's songs in album order. */
	let picking = $state<PickerPlace | null>(null);
	let addItem: HTMLButtonElement | undefined = $state();
	const songIds = $derived(album?.tracks.map((track) => track.id) ?? []);

	// A menu opened on another album starts with its list shut.
	$effect(() => {
		void open?.entry.id;
		picking = null;
	});

	function pick(item: HTMLElement): void {
		picking = pickerBeside(item.getBoundingClientRect(), innerWidth);
	}

	function then(action: () => void): () => void {
		return () => {
			action();
			albumMenu.close(true);
		};
	}
</script>

{#if open !== null && actions !== null}
	<Floating
		x={open.x}
		y={open.y}
		label={open.entry.title}
		width={300}
		onclose={(refocus) => {
			albumMenu.close(refocus);
		}}
	>
		<div class="head">
			<p class="name">{open.entry.title}</p>
			<p class="line">{line}</p>
		</div>
		<button type="button" role="menuitem" class="item" data-menu-item onclick={then(actions.play)}
			>Play</button
		>
		<button
			type="button"
			role="menuitem"
			class="item"
			data-menu-item
			onclick={then(actions.playNext)}>Play next</button
		>
		<button
			type="button"
			role="menuitem"
			class="item"
			data-menu-item
			onclick={then(actions.addToQueue)}>Add to queue</button
		>
		<button
			type="button"
			role="menuitem"
			class="item"
			data-menu-item
			onclick={then(actions.shuffle)}>Shuffle</button
		>
		<button
			type="button"
			role="menuitem"
			class="item"
			class:open={picking !== null}
			data-menu-item
			aria-haspopup="menu"
			aria-expanded={picking !== null}
			bind:this={addItem}
			onclick={(event) => {
				pick(event.currentTarget);
			}}
			onkeydown={(event) => {
				if (event.key === 'ArrowRight') {
					event.preventDefault();
					pick(event.currentTarget);
				}
			}}><span class="grow">Add to playlist</span><Icon name="chevron-right" size={14} /></button
		>
		{#if actions.editTags !== undefined}
			<div class="rule"></div>
			<button
				type="button"
				role="menuitem"
				class="item"
				data-menu-item
				onclick={then(actions.editTags)}>Edit tags…</button
			>
			{#if actions.findMatches !== undefined}
				<button
					type="button"
					role="menuitem"
					class="item"
					data-menu-item
					onclick={then(actions.findMatches)}>Find matches…</button
				>
			{/if}
		{/if}
		{#if actions.organize !== undefined}
			{#if actions.editTags === undefined}<div class="rule"></div>{/if}
			<button
				type="button"
				role="menuitem"
				class="item organize"
				data-menu-item
				onclick={then(actions.organize)}
				><span class="organize-name">Organize files…</span><span class="organize-note"
					>PREVIEWS THESE {String(album?.tracks.length ?? open.entry.trackCount)} · MOVES NOTHING YET</span
				></button
			>
		{/if}
		{#if actions.goToArtist !== undefined}
			<div class="rule"></div>
			<button
				type="button"
				role="menuitem"
				class="item quiet"
				data-menu-item
				onclick={then(actions.goToArtist)}>Go to artist</button
			>
		{/if}
		{#if actions.trash !== undefined}
			<div class="rule"></div>
			<button
				type="button"
				role="menuitem"
				class="item danger"
				data-menu-item
				onclick={then(actions.trash)}><Icon name="trash" size={16} />Move album to Trash</button
			>
		{/if}
	</Floating>
	{#if picking !== null}
		{@const id = open.entry.id}
		<PlaylistPicker
			trackIds={songIds}
			resolveTracks={async () => {
				await library.loadAlbum(id);
				return library.album(id)?.tracks.map((track) => track.id) ?? [];
			}}
			count={album?.tracks.length ?? open.entry.trackCount}
			mode={picking.mode}
			x={picking.x}
			y={picking.y}
			heading={picking.mode === 'sheet'
				? addHeading(album?.tracks.length ?? open.entry.trackCount)
				: undefined}
			onclose={(done, refocus) => {
				picking = null;
				if (done) {
					albumMenu.close(true);
				} else if (refocus) {
					addItem?.focus();
				}
			}}
		/>
	{/if}
{/if}

<style>
	.head {
		padding: 8px 12px 10px;
	}

	.name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 15px;
		font-weight: 700;
	}

	.line {
		margin-top: 2px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	.item {
		height: 40px;
		padding: 0 12px;
		display: flex;
		align-items: center;
		border: 0;
		border-radius: var(--radius-12);
		background: transparent;
		color: var(--text);
		font: 600 14px var(--font-sans);
		text-align: left;
	}

	.item:hover,
	.item:focus-visible,
	.item.open {
		background: var(--hover);
	}

	.grow {
		flex: 1;
	}

	.organize {
		height: auto;
		flex-direction: column;
		align-items: flex-start;
		gap: 2px;
		padding: 9px 12px;
	}

	.organize-note {
		font: 400 10.5px var(--font-mono);
		letter-spacing: 0.04em;
		color: var(--fox-text);
	}

	.quiet {
		color: var(--text-secondary);
	}

	.danger {
		gap: 10px;
		color: var(--danger);
	}

	.danger:hover,
	.danger:focus-visible {
		background: var(--danger-wash);
	}

	.rule {
		height: 1px;
		margin: 8px 4px;
		background: var(--edge);
	}
</style>
