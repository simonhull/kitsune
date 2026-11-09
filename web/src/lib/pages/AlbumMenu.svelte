<!--
	An album's own ⋯: Play, Play next, Add to queue and Shuffle, then Edit tags…, Find matches… and Go to artist. One
	menu, from the album page's hero and from any album card.
-->
<script lang="ts">
	import { library } from '#lib/library/library.svelte.ts';
	import { albumActions, albumMenu } from './albumMenu.svelte.ts';
	import Floating from './Floating.svelte';

	const open = $derived(albumMenu.current);
	const album = $derived(open === null ? undefined : library.album(open.entry.id));
	const actions = $derived(open === null ? null : albumActions(open.entry.id, album));
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
	</Floating>
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
	.item:focus-visible {
		background: var(--hover);
	}

	.quiet {
		color: var(--text-secondary);
	}

	.rule {
		height: 1px;
		margin: 8px 4px;
		background: var(--edge);
	}
</style>
