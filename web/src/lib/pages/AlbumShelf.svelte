<script lang="ts">
	import type { AlbumEntry } from '#lib/library/source.ts';
	import AlbumCard from './AlbumCard.svelte';

	interface Props {
		albums: readonly AlbumEntry[];
	}

	let { albums }: Props = $props();
</script>

<div class="shelf">
	{#each albums as album (album.id)}
		<AlbumCard {album} />
	{/each}
</div>

<style>
	.shelf {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 24px;
	}

	@media (max-width: 720px) {
		.shelf {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 20px 16px;
		}
	}
</style>
