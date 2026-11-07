<script lang="ts">
	import { filledStars, maxStars, starsLabel, type AlbumStars } from '#lib/library/albumStars.ts';

	interface Props {
		/** Null for an album with nothing rated: five empty stars. */
		stars: AlbumStars | null;
		/** Empty stars are outlines, as in the Library's facts, or dimmed solid ones, as on a stage. */
		empty?: 'outline' | 'dim';
	}

	let { stars, empty = 'outline' }: Props = $props();

	const filled = $derived(stars === null ? 0 : filledStars(stars.mean));
	const label = $derived(stars === null ? 'No songs rated' : starsLabel(stars));
	const blank = $derived(empty === 'outline' ? '☆' : '★');
</script>

<!-- Read-only: an album's stars are worked out from its songs, and can't be set. -->
<span class="shown" role="img" aria-label={label}
	>{'★'.repeat(filled)}<span class="empty">{blank.repeat(maxStars - filled)}</span></span
>

<style>
	.shown {
		letter-spacing: 0.1em;
		white-space: nowrap;
	}

	.empty {
		color: var(--star-empty, inherit);
	}
</style>
