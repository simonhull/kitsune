<script lang="ts">
	import type { Mark } from '#lib/library/albumOrder.ts';

	interface Props {
		label: string;
		marks: readonly Mark[];
		here: Mark | undefined;
		onjump: (index: number) => void;
	}

	let { label, marks, here, onjump }: Props = $props();

	let rail: HTMLElement | undefined = $state();

	function go({ index }: Mark): void {
		if (index !== null) {
			onjump(index);
		}
	}

	/** On a phone the rail is one tall target: a finger slides down it, past marks too small to tap. */
	function slide({ clientY }: PointerEvent): void {
		if (rail !== undefined) {
			const { top, height } = rail.getBoundingClientRect();
			const place = Math.floor(((clientY - top) / height) * marks.length);
			const under = Math.min(marks.length - 1, Math.max(0, place));
			const mark =
				marks.slice(under).find(({ index }) => index !== null) ??
				marks.findLast(({ index }) => index !== null);
			if (mark !== undefined) {
				go(mark);
			}
		}
	}

	function onpointerdown(event: PointerEvent): void {
		if (event.pointerType !== 'mouse') {
			rail?.setPointerCapture(event.pointerId);
			slide(event);
		}
	}

	function onpointermove(event: PointerEvent): void {
		if (rail?.hasPointerCapture(event.pointerId) === true) {
			slide(event);
		}
	}
</script>

<nav
	class="rail"
	aria-label={label}
	style:--marks={marks.length}
	bind:this={rail}
	{onpointerdown}
	{onpointermove}
>
	{#each marks as mark (mark.label)}
		<button
			type="button"
			disabled={mark.index === null}
			aria-current={mark === here ? 'true' : undefined}
			onclick={() => {
				go(mark);
			}}>{mark.label}</button
		>
	{/each}
</nav>

<style>
	.rail {
		--room: calc(100dvh - 216px);

		position: fixed;
		z-index: 1;
		top: 104px;
		right: 24px;
		width: var(--target);
		display: flex;
		flex-direction: column;
		align-items: center;
		font-family: var(--font-mono);
		font-size: 13px;
		color: var(--text-meta);
	}

	button {
		width: 100%;
		height: min(25px, calc(var(--room) / var(--marks)));
		padding: 0;
		border: 0;
		background: none;
		font: inherit;
		line-height: 1;
		font-variant-numeric: tabular-nums;
		color: inherit;
	}

	button:hover:enabled {
		color: var(--fox-text);
	}

	button:disabled {
		color: var(--text-label);
		opacity: 0.45;
		cursor: default;
	}

	[aria-current='true'] {
		color: var(--fox-text);
		font-weight: 700;
	}

	/* Narrow enough to leave the covers their room, and wide enough to find with a thumb. */
	@media (max-width: 720px) {
		.rail {
			--room: calc(100dvh - 232px);

			top: 100px;
			right: 0;
			width: 32px;
			font-size: 11px;
			touch-action: none;
		}

		button {
			height: min(22px, calc(var(--room) / var(--marks)));
		}
	}
</style>
