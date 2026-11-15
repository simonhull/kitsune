<!--
	A rating rule's value: five stars, in halves. A press on a star sets it whole; a second press on
	the same star takes off half.
-->
<script lang="ts">
	import { starsWord } from '#lib/library/smartlist.ts';

	interface Props {
		halves: number;
		/** A phone's 44px targets. */
		phone?: boolean;
		onpick: (halves: number) => void;
	}

	let { halves, phone = false, onpick }: Props = $props();

	const path = 'M12 2.8l2.8 5.8 6.3.9-4.6 4.4 1.1 6.3L12 17.2l-5.6 3 1.1-6.3L2.9 9.5l6.3-.9z';
</script>

<span class="stars" class:phone role="group" aria-label={`Rating ${starsWord(halves)}`}>
	{#each [1, 2, 3, 4, 5] as n (n)}
		{@const fill = halves >= n * 2 ? 1 : halves === n * 2 - 1 ? 0.5 : 0}
		<button
			type="button"
			class="star"
			aria-label={`${String(n)} ${n === 1 ? 'star' : 'stars'}, press again for a half less`}
			aria-pressed={fill > 0}
			onclick={() => {
				onpick(halves === n * 2 ? n * 2 - 1 : n * 2);
			}}
		>
			<svg class="empty" viewBox="0 0 24 24" aria-hidden="true"><path d={path}></path></svg>
			<span class="full" style:--fill={String(fill)}>
				<svg viewBox="0 0 24 24" aria-hidden="true"><path d={path}></path></svg>
			</span>
		</button>
	{/each}
	{#if !phone}
		<span class="word">{starsWord(halves)}</span>
	{/if}
</span>

<style>
	.stars {
		display: inline-flex;
		align-items: center;
	}

	.star {
		position: relative;
		width: calc(34px * var(--step));
		height: 40px;
		padding: 0;
		border: 0;
		background: none;
	}

	.phone .star {
		width: 44px;
		height: 44px;
	}

	.star:focus-visible {
		outline: none;
		box-shadow: 0 0 0 2px var(--fox-line);
		border-radius: var(--radius-12);
	}

	svg {
		width: calc(24px * var(--step));
		height: calc(24px * var(--step));
		fill: currentColor;
	}

	.phone svg {
		width: 26px;
		height: 26px;
	}

	.empty,
	.full {
		position: absolute;
		left: 50%;
		top: 50%;
		transform: translate(-50%, -50%);
	}

	.empty {
		color: var(--edge-strong);
	}

	.full {
		display: block;
		overflow: hidden;
		height: calc(24px * var(--step));
		width: calc(var(--fill) * 24px * var(--step));
		transform: translate(calc(-12px * var(--step)), -50%);
		color: var(--fox);
	}

	.phone .full {
		height: 26px;
		width: calc(var(--fill) * 26px);
		transform: translate(-13px, -50%);
	}

	.word {
		margin-left: 6px;
		font-family: var(--font-mono);
		font-size: 12px;
		color: var(--text-meta);
	}
</style>
