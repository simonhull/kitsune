<script lang="ts">
	import { coverFor } from '#lib/cover/cover.ts';
	import Wash from './Wash.svelte';

	interface Props {
		/** The album whose colors fill the stage; it stays set after the stage clears. */
		albumId: string | null;
		mode: 'clear' | 'album' | 'poster';
	}

	let { albumId, mode }: Props = $props();

	const palette = $derived(albumId === null ? null : coverFor(albumId).palette);
</script>

{#if albumId !== null && palette !== null}
	<div class="backdrop" data-mode={mode} aria-hidden="true">
		<div class="album" data-morph-stage="album" style:background={palette.stage}>
			<Wash {albumId} variant="hero" />
			<div class="fade"></div>
		</div>
		<div
			class="poster"
			data-morph-stage="poster"
			style:--field={palette.field}
			style:--band={palette.band}
			style:--stage={palette.stage}
		></div>
	</div>
{/if}

<style>
	.backdrop {
		position: absolute;
		inset: 0 0 auto;
		z-index: 0;
		pointer-events: none;
	}

	.album {
		position: absolute;
		inset: 0 0 auto;
		height: var(--hero-height);
		overflow: hidden;
		opacity: 0;
	}

	.fade {
		position: absolute;
		inset: 0;
		background: linear-gradient(
			180deg,
			rgb(10 9 8 / 0%) 14%,
			rgb(10 9 8 / 34%) 48%,
			rgb(10 9 8 / 58%) 70%,
			var(--ground) 100%
		);
	}

	.poster {
		position: fixed;
		inset: 0;
		opacity: 0;
		background: radial-gradient(
			80% 70% at 50% 42%,
			var(--field) 0%,
			var(--band) 55%,
			var(--stage) 100%
		);
	}

	[data-mode='album'] .album,
	[data-mode='poster'] .poster {
		opacity: 1;
	}
</style>
