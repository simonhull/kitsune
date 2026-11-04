<script lang="ts">
	import { coverFor } from './cover';

	interface Props {
		albumId: string;
	}

	let { albumId }: Props = $props();

	const cover = $derived(coverFor(albumId));
</script>

<!-- Every size is a percentage, so the same markup stays crisp at 52 px and at 380 px. -->
<span
	class="cover {cover.motif}"
	aria-hidden="true"
	style:--base={cover.palette.base}
	style:--primary={cover.palette.primary}
	style:--secondary={cover.palette.secondary}
>
	<span class="a"></span>
	<span class="b"></span>
</span>

<style>
	.cover {
		position: absolute;
		inset: 0;
		display: block;
		overflow: hidden;
		background: var(--base);
	}

	.a,
	.b {
		position: absolute;
		display: block;
	}

	.orb-horizon .a {
		inset: 14%;
		border-radius: 50%;
		background: var(--primary);
	}

	.orb-horizon .b {
		inset: 56% 0 0;
		background: var(--secondary);
	}

	.split-block .a {
		inset: 0 50% 0 0;
		background: var(--primary);
	}

	.split-block .b {
		inset: 30%;
		background: var(--secondary);
	}

	.hill-moon .a {
		left: -20%;
		top: 40%;
		width: 140%;
		height: 140%;
		border-radius: 50%;
		background: var(--primary);
	}

	.hill-moon .b {
		left: 62%;
		top: 16%;
		width: 18%;
		height: 18%;
		border-radius: 50%;
		background: var(--secondary);
	}

	/* The ring is a disc with the ground laid back over it: a percentage can't size a border. */
	.ring-dot .a {
		inset: 20%;
		border-radius: 50%;
		background: var(--primary);
	}

	.ring-dot .a::after {
		content: '';
		position: absolute;
		inset: 9%;
		border-radius: 50%;
		background: var(--base);
	}

	.ring-dot .b {
		inset: 44%;
		border-radius: 50%;
		background: var(--primary);
	}

	.diamond .a {
		inset: 18%;
		background: var(--primary);
		transform: rotate(45deg);
	}

	.diamond .b {
		display: none;
	}
</style>
