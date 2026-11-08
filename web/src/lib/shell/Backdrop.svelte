<script lang="ts">
	import type { AlbumPalette } from '#lib/cover/palette.ts';
	import Wash from './Wash.svelte';

	interface Props {
		/** The album whose colors fill the stage; it stays set after the stage clears. */
		albumId: string | null;
		/** Null when no album has been on stage yet. */
		palette: AlbumPalette | null;
		/** Whether the album is known yet: a cold deep link has to wait for its colors. */
		known: boolean;
		mode: 'clear' | 'album' | 'poster';
	}

	let { albumId, palette, known, mode }: Props = $props();

	// The one color change no morph carries: a cold deep link's stage, once its entry arrives.
	let waitedFor: string | null = null;
	const cold = $derived.by(() => {
		if (!known) {
			waitedFor = albumId;
		}
		return waitedFor === albumId;
	});
</script>

{#if albumId !== null && palette !== null}
	<div class="backdrop" data-mode={known ? mode : 'clear'} aria-hidden="true">
		<div class="album" class:cold data-morph-stage="album" style:background={palette.stage}>
			<Wash {palette} variant="hero" />
			<div class="fade"></div>
		</div>
		<div
			class="poster"
			class:cold
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
		transition: height var(--morph) var(--curve-morph);
	}

	.fade {
		position: absolute;
		inset: 0;
		background: linear-gradient(
			180deg,
			color-mix(in srgb, var(--ground) 0%, transparent) 14%,
			color-mix(in srgb, var(--ground) 34%, transparent) 48%,
			color-mix(in srgb, var(--ground) 58%, transparent) 70%,
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

	[data-mode='album'] .album.cold,
	[data-mode='poster'] .poster.cold {
		animation: stage-in var(--morph) var(--curve-morph);
	}

	@keyframes stage-in {
		from {
			opacity: 0;
		}
	}
</style>
