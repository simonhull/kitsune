<script lang="ts">
	import type { AlbumPalette } from '#lib/cover/palette.ts';

	interface Props {
		palette: AlbumPalette;
	}

	let { palette }: Props = $props();
</script>

<!-- Light breaking out of the cover: soft by construction, so it only ever scales and fades. -->
<div
	class="bloom"
	data-morph-bloom=""
	aria-hidden="true"
	style:--glow={palette.glow}
	style:--field={palette.field}
></div>

<style>
	.bloom {
		--bloom-size: 100vmax;

		position: fixed;
		left: calc(50% - var(--bloom-size) / 2);
		top: calc(50% - var(--bloom-size) / 2);
		z-index: 2;
		width: var(--bloom-size);
		height: var(--bloom-size);
		border-radius: 50%;
		background: radial-gradient(
			closest-side,
			var(--glow) 0%,
			var(--field) 34%,
			color-mix(in srgb, var(--field) 55%, transparent) 62%,
			transparent 100%
		);
		opacity: 0;
		visibility: hidden;
		pointer-events: none;
	}

	:global(.bloom[data-morph-bloom='open']) {
		opacity: 1;
		visibility: visible;
	}
</style>
