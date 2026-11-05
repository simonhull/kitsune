<script lang="ts">
	import { clock } from '#lib/library/format.ts';
	import { morph } from '#lib/morph/registry.ts';

	/** How far an arrow key moves, in seconds. */
	const keyStep = 5;

	interface Props {
		position: number;
		duration: number;
		/** Whether there's anything to seek in yet: what's only cued has no sound to move. */
		live: boolean;
		seek: (seconds: number) => void;
	}

	let { position, duration, live, seek }: Props = $props();

	/** Where a drag has got to: nothing seeks until it's let go. */
	let preview = $state<number | null>(null);
	const shown = $derived(preview ?? position);
	const percent = $derived(`${String(duration > 0 ? (shown / duration) * 100 : 0)}%`);

	function at({ clientX, currentTarget }: PointerEvent): number {
		if (!(currentTarget instanceof HTMLElement)) {
			return 0;
		}
		const { left, width } = currentTarget.getBoundingClientRect();
		return Math.min(Math.max((clientX - left) / width, 0), 1) * duration;
	}

	function press(event: PointerEvent): void {
		if (live && event.button === 0 && event.currentTarget instanceof HTMLElement) {
			event.currentTarget.setPointerCapture(event.pointerId);
			preview = at(event);
		}
	}

	function drag(event: PointerEvent): void {
		if (preview !== null) {
			preview = at(event);
		}
	}

	function release(event: PointerEvent): void {
		if (preview !== null) {
			seek(at(event));
			preview = null;
		}
	}

	function cancel(): void {
		preview = null;
	}

	function key(event: KeyboardEvent): void {
		const step = {
			ArrowLeft: -keyStep,
			ArrowDown: -keyStep,
			ArrowRight: keyStep,
			ArrowUp: keyStep
		}[event.key];
		if (live && step !== undefined) {
			event.preventDefault();
			seek(position + step);
		}
	}
</script>

<div class="scrub">
	<span class="time">{clock(shown)}</span>
	<div
		class="track"
		role="slider"
		tabindex="0"
		aria-label="Seek"
		aria-valuemin={0}
		aria-valuemax={Math.round(duration)}
		aria-valuenow={Math.round(shown)}
		aria-valuetext={`${clock(shown)} of ${clock(duration)}`}
		aria-disabled={!live}
		onpointerdown={press}
		onpointermove={drag}
		onpointerup={release}
		onpointercancel={cancel}
		onkeydown={key}
	>
		<div
			class="fill"
			style:width={percent}
			use:morph={{ id: 'progress', slot: 'poster', kind: 'bar' }}
		></div>
	</div>
	<span class="time">{clock(duration)}</span>
</div>

<style>
	.scrub {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		gap: 14px;
	}

	.time {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		color: var(--wash);
		white-space: nowrap;
	}

	.track {
		position: relative;
		flex: 1;
		height: 8px;
		border-radius: 4px;
		background: rgb(255 255 255 / 16%);
		cursor: pointer;
		touch-action: none;
	}

	/* The bar is 8px; the target isn't. */
	.track::before {
		content: '';
		position: absolute;
		inset: -18px 0;
	}

	.track[aria-disabled='true'] {
		cursor: default;
	}

	.fill {
		position: absolute;
		inset: 0 auto 0 0;
		border-radius: 4px;
		background: var(--fox);
	}

	@media (max-width: 640px) {
		.scrub {
			order: 3;
			flex-basis: 100%;
		}
	}
</style>
