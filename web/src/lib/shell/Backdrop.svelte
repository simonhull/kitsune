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
		<div class="album" data-morph-stage style:background={palette.stage}>
			<Wash {albumId} variant="hero" />
			<div class="fade"></div>
		</div>
		<div
			class="bloom"
			data-morph-bloom=""
			style:--glow={palette.glow}
			style:--field={palette.field}
		></div>
		<div
			class="poster"
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
			rgb(10 9 8 / 0%) 30%,
			rgb(10 9 8 / 55%) 70%,
			var(--ground) 100%
		);
	}

	/* Light breaking out of the cover: soft by construction, so it can only scale and fade. */
	.bloom {
		--bloom-size: 100vmax;

		position: fixed;
		left: calc(50% - var(--bloom-size) / 2);
		top: calc(50% - var(--bloom-size) / 2);
		width: var(--bloom-size);
		height: var(--bloom-size);
		border-radius: 50%;
		background: radial-gradient(
			closest-side,
			var(--glow) 0%,
			color-mix(in srgb, var(--field) 80%, transparent) 42%,
			transparent 100%
		);
		opacity: 0;
		visibility: hidden;
	}

	:global(.bloom[data-morph-bloom='open']) {
		opacity: 1;
		visibility: visible;
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
