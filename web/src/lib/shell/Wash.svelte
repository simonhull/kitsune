<script lang="ts">
	import { coverFor } from '#lib/cover/cover.ts';

	interface Props {
		albumId: string;
		/** `tile` is Home's card; `hero` is the album page. */
		variant: 'tile' | 'hero';
	}

	let { albumId, variant }: Props = $props();

	const palette = $derived(coverFor(albumId).palette);
</script>

<!-- The album's cover, melted: the same palette, blurred past recognition. -->
<div
	class="wash {variant}"
	aria-hidden="true"
	style:--field={palette.field}
	style:--glow={palette.glow}
	style:--band={palette.band}
>
	<div class="field"></div>
	<div class="glow"></div>
	<div class="band"></div>
</div>

<style>
	.wash {
		position: absolute;
	}

	.wash div {
		position: absolute;
	}

	.field {
		inset: 0;
		background: var(--field);
	}

	.glow {
		border-radius: 50%;
		background: var(--glow);
	}

	.band {
		left: 0;
		right: 0;
		bottom: 0;
		background: var(--band);
	}

	.tile {
		inset: -20%;
		filter: blur(70px) saturate(1.4);
		opacity: 0.85;
	}

	.tile .glow {
		left: 40%;
		top: 0;
		width: 60%;
		height: 70%;
	}

	.tile .band {
		top: 65%;
	}

	.hero {
		inset: -10%;
		filter: blur(36px) saturate(1.3);
		transform: scale(1.05);
	}

	.hero .glow {
		left: 30%;
		top: -10%;
		width: 70%;
		height: 95%;
	}

	.hero .band {
		top: 62%;
	}
</style>
