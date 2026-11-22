<!--
	An artist's stage, behind the page as an album's is (the shell's Backdrop holds it). On the
	artist's page it is the hero's: the backdrop full-bleed under the name, its ground laid first,
	the logo a mark at the top right printed off register; with no backdrop, the stage of the album
	they play most. Under 431 px the square thumb is the stage. On Now playing it is the playing
	artist's backdrop under the cover's light, and going to the artist it stays and reshapes into the
	hero's, while the light gives way off it.
-->
<script lang="ts">
	import type { AlbumPalette } from '#lib/cover/palette.ts';
	import type { ArtistBackdrop } from './stage.svelte.ts';
	import Wash from '#lib/shell/Wash.svelte';
	import { phoneStagePicture, pictureRung, pictureUrl } from './art.ts';
	import Picture from './Picture.svelte';

	interface Props {
		stage: ArtistBackdrop;
		/** The cover's light over the backdrop, on Now playing. */
		light: AlbumPalette | null;
	}

	let { stage, light }: Props = $props();

	/**
	 * Comes up on its own when it mounts, as an artist read after the journey landed does: a stage
	 * that simply appeared would be a cut. A picture of it (held, with no stage attribute) never
	 * replays this.
	 */
	let arriving = $state(true);

	const images = $derived(stage.images);
	const fallback = $derived(stage.fallback);
	const place = $derived(stage.place);

	let width = $state(typeof innerWidth === 'undefined' ? 1440 : innerWidth);
	const backdrop = $derived(images.background);
	const square = $derived(phoneStagePicture(images));
	const logo = $derived(place === 'hero' ? images.logo : undefined);
	const density = typeof devicePixelRatio === 'undefined' ? 1 : devicePixelRatio;
	const logoUrl = $derived(
		logo === undefined ? '' : pictureUrl(logo.hash, pictureRung('logo', 520 * density))
	);
</script>

<svelte:window bind:innerWidth={width} />

<div
	class="stage"
	class:arriving
	onanimationend={(event) => {
		if (event.target === event.currentTarget) {
			arriving = false;
		}
	}}
	data-morph-stage="artist"
	data-look={stage.id}
	data-place={place}
	data-wide={stage.wide ? '' : undefined}
	data-square={stage.square ? '' : undefined}
	data-backdrop={backdrop === undefined ? undefined : ''}
	style:--wide-ground={backdrop?.ground ?? fallback?.stage}
	style:--square-ground={square?.ground ?? fallback?.stage}
	style:--light-field={light?.field}
	style:--light-band={light?.band}
	style:--light-stage={light?.stage}
>
	<div class="wide">
		{#if backdrop !== undefined}
			<Picture image={backdrop} kind="background" box={width} />
		{:else if fallback !== null}
			<Wash palette={fallback} variant="hero" />
		{/if}
	</div>
	<div class="square">
		{#if square !== undefined}
			<Picture image={square} kind={square === backdrop ? 'background' : 'thumb'} box={width} />
		{:else if fallback !== null}
			<Wash palette={fallback} variant="hero" />
		{/if}
	</div>
	<div class="grain"></div>
	<div class="fade"></div>
	<div class="light"></div>
	{#if logo !== undefined}
		<!-- A mark, not the headline: the typeset name stays that. Printed off register. -->
		<span class="logo" style:--logo={`url("${logoUrl}")`}>
			<span class="misprint"></span>
			<Picture image={logo} kind="logo" box={520} fit="contain" position="right center" />
		</span>
	{/if}
</div>

<style>
	.stage {
		position: absolute;
		inset: 0 0 auto;
		height: var(--artist-stage);
		overflow: hidden;
		background: var(--wide-ground, var(--ground));
		transition: height var(--morph-slow) var(--curve-settle);
	}

	.stage[data-morph-stage].arriving {
		animation: stage-in var(--morph) var(--curve-morph);
	}

	@keyframes stage-in {
		from {
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.stage[data-morph-stage].arriving {
			animation-duration: 120ms;
		}

		.stage {
			transition: none;
		}
	}

	/* Now playing's: the whole moment, under the cover's light. */
	[data-place='poster'] {
		position: fixed;
		height: 100dvh;
	}

	.wide,
	.square {
		position: absolute;
		inset: 0;
	}

	.square {
		display: none;
	}

	/* Nothing to show at this width: no stage, as for an artist with no art. */
	.stage:not([data-wide]) {
		display: none;
	}

	/* The grain of a print: it fades out before the stage's foot, so it never draws a line there. */
	.grain {
		position: absolute;
		inset: 0;
		opacity: 0.5;
		mix-blend-mode: overlay;
		background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='160'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix type='saturate' values='0'/%3E%3C/filter%3E%3Crect width='160' height='160' filter='url(%23n)'/%3E%3C/svg%3E");
		mask-image: linear-gradient(to bottom, black 45%, transparent 96%);
	}

	/* Into the page's ground at the foot, and darker behind the words on the left. */
	.fade {
		position: absolute;
		inset: 0;
		background:
			linear-gradient(
				to bottom,
				color-mix(in srgb, var(--ground) 0%, transparent) 16%,
				color-mix(in srgb, var(--ground) 50%, transparent) 58%,
				var(--ground) 100%
			),
			linear-gradient(
				to right,
				color-mix(in srgb, var(--ground) 55%, transparent),
				color-mix(in srgb, var(--ground) 0%, transparent) 58%
			);
		transition: opacity var(--morph) var(--curve-morph);
	}

	/* The cover's light, as Now playing's own stage has it, laid over the backdrop. */
	.light {
		position: absolute;
		inset: 0;
		opacity: 0;
		background: radial-gradient(
			80% 70% at 50% 42%,
			color-mix(in srgb, var(--light-field) 52%, transparent) 0%,
			color-mix(in srgb, var(--light-band) 72%, transparent) 55%,
			color-mix(in srgb, var(--light-stage) 92%, transparent) 100%
		);
		transition: opacity var(--morph) var(--curve-morph);
	}

	[data-place='poster'] .light {
		opacity: 1;
	}

	[data-place='poster'] .fade {
		opacity: 0;
	}

	.logo {
		position: absolute;
		top: calc(116px * var(--step));
		right: var(--gutter);
		width: calc(520px * var(--step));
		height: calc(80px * var(--step));
	}

	/* The misprint: a teal copy of the mark, 2.5 px behind it, the focus ring's language. */
	.misprint {
		position: absolute;
		inset: 0;
		translate: 2.5px 2.5px;
		opacity: 0.55;
		background: var(--misprint);
		mask: var(--logo) right center / contain no-repeat;
	}

	@media (max-width: 430px) {
		.stage {
			display: block;
			background: var(--square-ground, var(--ground));
		}

		.stage:not([data-square]) {
			display: none;
		}

		.wide {
			display: none;
		}

		.square {
			display: block;
		}

		.fade {
			background: linear-gradient(
				to bottom,
				color-mix(in srgb, var(--ground) 35%, transparent) 0%,
				color-mix(in srgb, var(--ground) 0%, transparent) 22%,
				color-mix(in srgb, var(--ground) 0%, transparent) 44%,
				color-mix(in srgb, var(--ground) 70%, transparent) 74%,
				var(--ground) 100%
			);
		}

		.logo {
			top: 86px;
			right: 16px;
			width: 190px;
			height: 34px;
		}

		.misprint {
			translate: 1.5px 1.5px;
		}
	}
</style>
