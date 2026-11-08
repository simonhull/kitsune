<script lang="ts">
	import { maxStars } from '#lib/library/albumStars.ts';
	import { library } from '#lib/library/library.svelte.ts';

	interface Props {
		trackId: string;
		/** The song's title, for the labels: "Rate Breakwater 4 of 5". */
		title: string;
	}

	let { trackId, title }: Props = $props();

	const stars = $derived(library.marks.stars(trackId));
	const five = Array.from({ length: maxStars }, (_, index) => index + 1);
</script>

<!-- Pressing the star a song already has clears it. -->
<div class={['stars', { rated: stars !== null }]} role="group" aria-label={`Rate ${title}`}>
	{#each five as star (star)}
		{@const on = stars !== null && star <= stars}
		<button
			type="button"
			class:on
			aria-label={`Rate ${title} ${String(star)} of ${String(maxStars)}`}
			aria-pressed={on}
			onclick={() => {
				library.marks.press(trackId, star);
			}}>★</button
		>
	{/each}
</div>

<style>
	/* Where it stands sets --star-on, and reaches it as :global(.stars) to hide it. */
	.stars {
		flex: none;
		display: flex;
	}

	button {
		width: 24px;
		height: var(--target);
		padding: 0;
		border: 0;
		background: transparent;
		font-size: 15px;
		line-height: 1;
		color: var(--star-empty);
	}

	.on {
		color: var(--star-on, var(--accent));
	}
</style>
