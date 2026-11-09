<!--
	A box the review ticks, drawn as the boards draw it over a real checkbox: fox when on, a dash when
	some of what it stands for is on. On a phone it is a switch.
-->
<script lang="ts">
	import type { PillState } from './review.ts';

	interface Props {
		state: PillState;
		label: string;
		/** A switch, as on a phone, in place of a box. */
		switch?: boolean;
		size?: number;
		disabled?: boolean;
		toggle: (on: boolean) => void;
	}

	let {
		state,
		label,
		switch: asSwitch = false,
		size = 16,
		disabled = false,
		toggle
	}: Props = $props();
</script>

<span
	class="tick"
	class:switch={asSwitch}
	class:on={state !== 'none'}
	style:--size={`${String(size)}px`}
>
	<input
		type="checkbox"
		role={asSwitch ? 'switch' : undefined}
		aria-label={label}
		checked={state === 'all'}
		indeterminate={state === 'some'}
		{disabled}
		onchange={(event) => {
			toggle(state !== 'all' && event.currentTarget.checked);
		}}
	/>
	{#if asSwitch}
		<span class="track" aria-hidden="true"><span class="knob"></span></span>
	{:else}
		<span class="box" aria-hidden="true">
			<svg width="10" height="10" viewBox="0 0 24 24">
				{#if state === 'all'}<path d="M4 12.5l5 5L20 6.5" />{:else if state === 'some'}<path
						d="M5 12h14"
					/>{/if}
			</svg>
		</span>
	{/if}
</span>

<style>
	.tick {
		position: relative;
		display: inline-grid;
		flex: none;
		place-items: center;
		width: var(--size);
		height: var(--size);
	}

	.switch {
		width: 48px;
		height: 28px;
	}

	input {
		position: absolute;
		inset: -14px;
		z-index: 1;
		width: auto;
		height: auto;
		margin: 0;
		opacity: 0;
		cursor: pointer;
	}

	input:disabled {
		cursor: default;
	}

	.box {
		display: grid;
		place-items: center;
		width: 100%;
		height: 100%;
		box-sizing: border-box;
		border-radius: 4px;
		border: 1.5px solid var(--text-label);
		transition:
			background-color var(--morph-fast) var(--curve-morph),
			border-color var(--morph-fast) var(--curve-morph);
	}

	.on .box {
		border-color: var(--fox);
		background: var(--fox);
	}

	svg {
		width: 62%;
		height: 62%;
		fill: none;
		stroke: var(--fox-ink);
		stroke-width: 3.4;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	.track {
		position: relative;
		width: 100%;
		height: 100%;
		border-radius: var(--radius-pill);
		background: var(--edge-strong);
		transition: background-color var(--morph-fast) var(--curve-morph);
	}

	.knob {
		position: absolute;
		top: 3px;
		left: 3px;
		width: 22px;
		height: 22px;
		border-radius: 50%;
		background: var(--text-secondary);
		transition:
			translate var(--morph-fast) var(--curve-settle),
			background-color var(--morph-fast) var(--curve-morph);
	}

	.on .track {
		background: var(--fox);
	}

	.on .knob {
		translate: 20px 0;
		background: var(--fox-ink);
	}

	input:focus-visible + .box,
	input:focus-visible + .track {
		outline: 2px solid var(--fox-line);
		outline-offset: 2px;
	}

	@media (prefers-reduced-motion: reduce) {
		.box,
		.track,
		.knob {
			transition: none;
		}
	}
</style>
