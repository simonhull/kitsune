<!--
	The covers of the albums a group's copies are on, overlapping, each a little turned: the board's
	stack. An album with no cover shows its initials.
-->
<script lang="ts">
	import Cover from '#lib/cover/Cover.svelte';
	import type { DuplicateAlbum } from './types.ts';

	interface Props {
		albums: readonly DuplicateAlbum[];
		/** The side of one cover, in CSS pixels at the base step. */
		size?: number;
	}

	let { albums, size = 40 }: Props = $props();

	/** At most three show: past that, the stack says nothing more. */
	const shown = $derived(albums.slice(0, 3));

	function initials(title: string): string {
		return title
			.split(/\s+/)
			.filter((word) => /^\p{L}/u.test(word))
			.slice(0, 3)
			.map((word) => word[0]?.toUpperCase() ?? '')
			.join('');
	}
</script>

<span
	class="stack"
	style:--size={`${String(size)}px`}
	style:--count={shown.length}
	aria-hidden="true"
>
	{#each shown as album, i (album.id)}
		<span class="cover" style:--i={i}>
			{#if album.artwork !== undefined}
				<Cover albumId={album.id} entry={{ artwork: album.artwork }} place="small" box={size} />
			{:else}
				<span class="initials mono">{initials(album.title)}</span>
			{/if}
		</span>
	{/each}
</span>

<style>
	.mono {
		font-family: var(--font-mono);
	}

	.stack {
		position: relative;
		flex: none;
		width: calc((var(--size) + (var(--count) - 1) * var(--size) * 0.4) * var(--step));
		height: calc(var(--size) * 1.08 * var(--step));
	}

	.cover {
		position: absolute;
		left: calc(var(--i) * var(--size) * 0.4 * var(--step));
		top: calc(var(--i) * 2px);
		width: calc(var(--size) * var(--step));
		height: calc(var(--size) * var(--step));
		overflow: hidden;
		border-radius: calc(var(--size) * 0.17 * var(--step));
		background: var(--card-raised);
		border: 2px solid var(--ground);
		transform: rotate(calc(-4deg + var(--i) * 7deg));
	}

	.cover :global(img) {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.initials {
		display: grid;
		place-items: center;
		width: 100%;
		height: 100%;
		font-size: calc(var(--size) * 0.22 * var(--step));
		font-weight: 700;
		color: var(--text-meta);
	}
</style>
