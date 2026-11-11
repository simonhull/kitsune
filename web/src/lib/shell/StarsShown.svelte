<script lang="ts">
	import { starsLabel, type AlbumStars } from '#lib/library/albumStars.ts';
	import { maxStars, shownHalves } from '#lib/library/stars.ts';

	interface Props {
		/** Null for an album with nothing rated: five empty stars. */
		stars: AlbumStars | null;
		/** Empty stars are outlines, as in the Library's facts, or dimmed solid ones, as on a stage. */
		empty?: 'outline' | 'dim';
	}

	let { stars, empty = 'outline' }: Props = $props();

	/** The mean, drawn to the nearest half. */
	const shown = $derived(stars === null ? 0 : shownHalves(stars.mean));
	const full = $derived(Math.floor(shown));
	const half = $derived(shown > full);
	const label = $derived(stars === null ? 'No songs rated' : starsLabel(stars));
	const blank = $derived(empty === 'outline' ? '☆' : '★');
</script>

<!-- Read-only: an album's stars are worked out from its songs, and can't be set. -->
<span class="shown" role="img" aria-label={label}
	>{'★'.repeat(full)}{#if half}<span class="half"
			><span class:dim={empty === 'dim'}>{blank}</span><span class="over">★</span></span
		>{/if}<span class:dim={empty === 'dim'}>{blank.repeat(maxStars - full - (half ? 1 : 0))}</span
	></span
>

<style>
	.shown {
		letter-spacing: 0.1em;
		white-space: nowrap;
	}

	.dim {
		color: var(--star-empty);
	}

	/* A half: the empty star, with a filled one over it cut at its middle, in the same ink. */
	.half {
		position: relative;
		display: inline-block;
		letter-spacing: 0;
		margin-right: 0.1em;
	}

	.over {
		position: absolute;
		inset: 0;
		clip-path: inset(0 50% 0 0);
	}
</style>
