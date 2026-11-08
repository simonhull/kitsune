<script lang="ts">
	import { resolve } from '$app/paths';
	import { goto } from '$app/navigation';
	import { library } from '#lib/library/library.svelte.ts';
	import { heartPath } from '#lib/shell/Icon.svelte';
	import FileInfo from './FileInfo.svelte';
	import Floating from './Floating.svelte';
	import type { MenuTarget, SongActions } from './songActions.ts';

	interface Props {
		x: number;
		y: number;
		target: MenuTarget;
		actions: SongActions;
		onclose: (refocus: boolean) => void;
	}

	let { x, y, target, actions, onclose }: Props = $props();

	let info = $state(false);

	const ids = $derived(target.kind === 'song' ? [target.trackId] : target.trackIds);
	const hearted = $derived(ids.every((id) => library.marks.hearted('tracks', id)));
	/** The stars a song has; for several, the stars they all share, if they do. */
	const stars = $derived.by(() => {
		const all = new Set(ids.map((id) => library.marks.stars(id)));
		return all.size === 1 ? ([...all][0] ?? 0) : 0;
	});

	function then(action: () => void): () => void {
		return () => {
			action();
			onclose(true);
		};
	}

	function rate(star: number): void {
		actions.rate(stars === star ? null : star);
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
		{#if target.kind === 'picked'}
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
		<div class="rating" role="group" aria-label="Rating">
			<span class="rating-name">Rating</span>
			{#each [1, 2, 3, 4, 5] as star (star)}
				<button
					type="button"
					class="star"
					class:on={star <= stars}
					aria-label={`${String(star)} of 5`}
					aria-pressed={star <= stars}
					data-menu-item
					onclick={() => {
						rate(star);
					}}>★</button
				>
			{/each}
		</div>
		{#if actions.editTags !== undefined}
			<button
				type="button"
				role="menuitem"
				class="item"
				data-menu-item
				onclick={then(actions.editTags)}>Edit tags…</button
			>
		{/if}
		{#if target.kind === 'song'}
			{@const { albumId, artistId } = target}
			<div class="rule"></div>
			<button
				type="button"
				role="menuitem"
				class="item quiet"
				data-menu-item
				onclick={then(() => void goto(resolve('/album/[id]', { id: albumId })))}>Go to album</button
			>
			{#if artistId !== undefined}
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
		height: 40px;
		padding: 0 6px 0 12px;
		display: flex;
		align-items: center;
		font-size: 14px;
		font-weight: 600;
	}

	.rating-name {
		flex: 1;
	}

	.star {
		width: 24px;
		height: 36px;
		padding: 0;
		border: 0;
		border-radius: 8px;
		background: transparent;
		font-size: 16px;
		color: var(--star-off);
	}

	.star.on {
		color: var(--star);
	}

	.star:hover,
	.star:has(~ .star:hover) {
		color: color-mix(in srgb, var(--star) 75%, transparent);
	}
</style>
