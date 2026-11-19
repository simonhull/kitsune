<script lang="ts">
	import { resolve } from '$app/paths';
	import { goto } from '$app/navigation';
	import { library } from '#lib/library/library.svelte.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import StarsInput from '#lib/shell/StarsInput.svelte';
	import { pickerBeside, type PickerPlace } from './pickerPlace.ts';
	import FileInfo from './FileInfo.svelte';
	import Floating from './Floating.svelte';
	import PlaylistList from './PlaylistList.svelte';
	import PlaylistPicker from './PlaylistPicker.svelte';
	import { addHeading } from './playlistWords.ts';
	import { rowMenuItems, type MenuTarget, type SongActions } from './songActions.ts';

	interface Props {
		x: number;
		y: number;
		target: MenuTarget;
		actions: SongActions;
		/** The page it's on, whose own link it leaves out: no Go to album on the album. */
		here?: 'album' | 'artist' | undefined;
		/** Takes the songs out of the queue, in Up next. */
		remove?: (() => void) | undefined;
		/** Takes the songs out of the playlist, on a playlist's page: Delete does it there. */
		takeOut?: (() => void) | undefined;
		onclose: (refocus: boolean) => void;
	}

	let { x, y, target, actions, here, remove, takeOut, onclose }: Props = $props();

	let info = $state(false);
	/** Add to playlist, open beside its item, or as a sheet on a phone. */
	let picking = $state<PickerPlace | null>(null);
	let addItem: HTMLButtonElement | undefined = $state();
	const items = $derived(
		new Set(
			rowMenuItems(target, {
				here,
				editable: actions.editTags !== undefined,
				removable: remove !== undefined,
				trashable: actions.trash !== undefined,
				takeOut: takeOut !== undefined
			})
		)
	);

	/** Opens Add to playlist beside its item, on the side there's room. */
	function pick(item: HTMLElement): void {
		picking = pickerBeside(item.getBoundingClientRect(), innerWidth);
	}

	const ids = $derived(target.kind === 'song' ? [target.trackId] : target.trackIds);
	/** On a phone the menu itself becomes Add to playlist's list, in place. */
	const listing = $derived(picking?.mode === 'phone');
	const face = $derived(listing ? 'list' : info ? 'info' : 'menu');
	/** A song's stars; for several, the stars they all share, if they do. */
	const stars = $derived.by(() => {
		const all = new Set(ids.map((id) => library.marks.stars(id)));
		return all.size === 1 ? ([...all][0] ?? null) : null;
	});

	function then(action: () => void): () => void {
		return () => {
			action();
			onclose(true);
		};
	}
</script>

<Floating
	{x}
	{y}
	label={target.kind === 'song' ? target.title : `${String(target.count)} songs`}
	width={info ? 360 : 300}
	{face}
	full={listing}
	name="row-menu"
	{onclose}
>
	{#if listing}
		<PlaylistList
			big
			trackIds={ids}
			count={ids.length}
			heading={addHeading(ids.length)}
			onclose={(done, refocus) => {
				picking = null;
				if (done) {
					onclose(true);
				} else if (refocus) {
					addItem?.focus();
				}
			}}
		/>
	{:else}
		<div class="head">
			<p class="name">
				{target.kind === 'song' ? target.title : `${target.count.toLocaleString('en-US')} songs`}
			</p>
			<p class="line">{target.line}</p>
		</div>
		{#if info && target.kind === 'song'}
			<FileInfo albumId={target.albumId} trackId={target.trackId} />
		{:else}
			{#if items.has('play')}
				<button
					type="button"
					role="menuitem"
					class="item"
					data-menu-item
					onclick={then(actions.play)}>Play</button
				>
			{/if}
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
			{#if items.has('addToPlaylist')}
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
						if (event.key === 'ArrowRight' && innerWidth >= 700) {
							event.preventDefault();
							pick(event.currentTarget);
						}
					}}
					><span class="grow">Add to playlist</span><Icon name="chevron-right" size={14} /></button
				>
			{/if}
			<div class="rule"></div>
			<div class="rating">
				<span class="rating-name" aria-hidden="true">Rating</span>
				<!-- ↑ and ↓ stay the menu's, to go between its items; ← and → move the rating. -->
				<StarsInput
					size="menu"
					value={stars}
					label={target.kind === 'song' ? `Rating of ${target.title}` : 'Rating of all of them'}
					vertical={false}
					data-menu-item
					onchange={(next) => {
						actions.rate(next);
					}}
				/>
			</div>
			{#if items.has('editTags') && actions.editTags !== undefined}
				<button
					type="button"
					role="menuitem"
					class="item"
					data-menu-item
					onclick={then(actions.editTags)}>Edit tags…</button
				>
			{/if}
			{#if items.has('remove') && remove !== undefined}
				<button type="button" role="menuitem" class="item" data-menu-item onclick={then(remove)}
					>Remove from queue</button
				>
			{/if}
			{#if target.kind === 'song'}
				{@const { albumId, artistId } = target}
				<div class="rule"></div>
				{#if items.has('goToAlbum')}
					<button
						type="button"
						role="menuitem"
						class="item quiet"
						data-menu-item
						onclick={then(() => void goto(resolve('/album/[id]', { id: albumId })))}
						>Go to album</button
					>
				{/if}
				{#if items.has('goToArtist') && artistId !== undefined}
					<button
						type="button"
						role="menuitem"
						class="item quiet"
						data-menu-item
						onclick={then(() => void goto(resolve('/artists/[id]', { id: artistId })))}
						>Go to artist</button
					>
				{/if}
				<button
					type="button"
					role="menuitem"
					class="item quiet"
					data-menu-item
					onclick={() => {
						info = true;
					}}>File info</button
				>
			{/if}
			{#if items.has('takeOut') || items.has('trash')}
				<div class="rule"></div>
			{/if}
			{#if items.has('takeOut') && takeOut !== undefined}
				<button
					type="button"
					role="menuitem"
					class="item"
					data-menu-item
					aria-keyshortcuts="Delete"
					onclick={then(takeOut)}
					><span class="grow">Take out of playlist</span><span class="key" aria-hidden="true"
						>DEL</span
					></button
				>
			{/if}
			{#if items.has('trash') && actions.trash !== undefined}
				<button
					type="button"
					role="menuitem"
					class="item danger"
					data-menu-item
					aria-keyshortcuts={takeOut === undefined ? 'Delete' : undefined}
					onclick={then(actions.trash)}
					><Icon name="trash" size={16} /><span class="grow">Move to Trash</span
					>{#if takeOut === undefined}<span class="key" aria-hidden="true">DEL</span>{/if}</button
				>
			{/if}
		{/if}
	{/if}
</Floating>
{#if picking !== null && picking.mode === 'submenu'}
	<PlaylistPicker
		trackIds={ids}
		count={ids.length}
		mode="submenu"
		x={picking.x}
		y={picking.y}
		from={picking.from}
		onclose={(done, refocus) => {
			picking = null;
			if (done) {
				onclose(true);
			} else if (refocus) {
				addItem?.focus();
			}
		}}
	/>
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
		gap: 10px;
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

	.quiet {
		color: var(--text-secondary);
	}

	.danger {
		color: var(--danger);
	}

	.grow {
		flex: 1;
	}

	.key {
		font-family: var(--font-mono);
		font-size: 11px;
		font-weight: 400;
		color: var(--text-meta);
	}

	/* A finger has no Delete key to be told of. */
	@media (pointer: coarse) {
		.key {
			display: none;
		}
	}

	.rule {
		height: 1px;
		margin: 8px 4px;
		background: var(--edge);
	}

	.rating {
		min-height: 40px;
		padding: 0 6px 0 12px;
		display: flex;
		align-items: center;
		font-size: 14px;
		font-weight: 600;
	}

	.rating-name {
		flex: 1;
	}
</style>
