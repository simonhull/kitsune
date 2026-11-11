<script lang="ts">
	import { resolve } from '$app/paths';
	import { goto } from '$app/navigation';
	import { library } from '#lib/library/library.svelte.ts';
	import { heartPath } from '#lib/shell/Icon.svelte';
	import StarsInput from '#lib/shell/StarsInput.svelte';
	import FileInfo from './FileInfo.svelte';
	import Floating from './Floating.svelte';
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
		onclose: (refocus: boolean) => void;
	}

	let { x, y, target, actions, here, remove, onclose }: Props = $props();

	let info = $state(false);
	const items = $derived(
		new Set(
			rowMenuItems(target, {
				here,
				editable: actions.editTags !== undefined,
				removable: remove !== undefined
			})
		)
	);

	const ids = $derived(target.kind === 'song' ? [target.trackId] : target.trackIds);
	const hearted = $derived(ids.every((id) => library.marks.hearted('tracks', id)));
	/** The stars a song has; for several, the stars they all share, if they do. */
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
	{onclose}
>
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
			<button type="button" role="menuitem" class="item" data-menu-item onclick={then(actions.play)}
				>Play</button
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
		<div class="rule"></div>
		<button
			type="button"
			role="menuitemcheckbox"
			aria-checked={hearted}
			class="item with-icon"
			class:on={hearted}
			data-menu-item
			onclick={() => {
				actions.heart();
			}}
			><svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true"
				><path d={heartPath}></path></svg
			>{hearted ? 'Hearted' : 'Heart'}</button
		>
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
	{/if}
</Floating>

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
	.item:focus-visible {
		background: var(--hover);
	}

	.quiet {
		color: var(--text-secondary);
	}

	.with-icon path {
		fill: none;
		stroke: currentColor;
		stroke-width: 2;
	}

	.with-icon.on path {
		fill: var(--fox-text);
		stroke: var(--fox-text);
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
