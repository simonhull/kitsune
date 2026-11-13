<!--
	A choice of one to five whole stars, as radios: the You page's card, what its hearts become, and
	the Last.fm love line. Unlike a song's rating (StarsInput) it is never empty, and a press of the
	star chosen keeps it. The arrows move it, as a radio group's do.
-->
<script lang="ts">
	import { maxStars } from '#lib/library/stars.ts';

	interface Props {
		/** 1 to 5. A half, which only the API sets, lights the stars under it and checks none. */
		value: number;
		/** What it chooses, for a screen reader: "Stars for the 271 songs". */
		label: string;
		/** The card's 28 px stars in 44 px targets, or a setting line's 20 px in 32 × 44. */
		size: 'card' | 'line';
		onchange: (stars: number) => void;
	}

	let { value, label, size, onchange }: Props = $props();

	const places = Array.from({ length: maxStars }, (_, at) => at + 1);

	function onkeydown(event: KeyboardEvent): void {
		const by = { ArrowRight: 1, ArrowUp: 1, ArrowLeft: -1, ArrowDown: -1 }[event.key];
		if (by === undefined) {
			return;
		}
		event.preventDefault();
		const next = Math.min(maxStars, Math.max(1, Math.round(value) + by));
		onchange(next);
		const group = event.currentTarget instanceof HTMLElement ? event.currentTarget : null;
		group?.querySelector<HTMLElement>(`[data-star='${String(next)}']`)?.focus();
	}
</script>

<span class={['pick', size]} role="radiogroup" aria-label={label} tabindex="-1" {onkeydown}>
	{#each places as place (place)}
		<button
			type="button"
			role="radio"
			aria-checked={value === place}
			aria-label={`${String(place)} ${place === 1 ? 'star' : 'stars'}`}
			tabindex={value === place || (place === 1 && !Number.isInteger(value)) ? 0 : -1}
			data-star={place}
			class={{ full: value >= place }}
			onclick={() => {
				onchange(place);
			}}><span aria-hidden="true">★</span></button
		>
	{/each}
</span>

<style>
	.pick {
		display: inline-flex;
		align-items: center;
	}

	.pick:focus {
		outline: none;
	}

	button {
		--c: var(--star);

		display: grid;
		place-items: center;
		padding: 0;
		border: 0;
		border-radius: var(--radius-12);
		background: none;
		color: var(--star-off);
		line-height: 1;
		cursor: pointer;
	}

	button:focus-visible {
		outline: 2px solid var(--fox-line);
		outline-offset: -2px;
	}

	.card button {
		width: var(--target);
		height: var(--target);
		font-size: 28px;
	}

	.line button {
		width: 32px;
		height: var(--target);
		font-size: 20px;
	}

	.full {
		color: var(--c);
	}

	/* What a press would choose: the stars up to the one under the pointer, half lit. */
	@media (hover: hover) {
		.pick:hover button {
			color: var(--star-off);
		}

		.pick:hover button:has(~ button:hover),
		.pick button:hover {
			color: color-mix(in srgb, var(--star) 70%, transparent);
		}
	}
</style>
