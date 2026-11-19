<!--
	The match sheet, over whatever page opened it. It buds from Find matches… with the releases (M4),
	and becomes the review of the one picked by reshaping in place, the page staying underneath; Back is
	the same box reshaped again, never a slide, and it keeps its height. A press outside or Esc closes it
	from either step, and it gives way where it stands. At 3840 it is one step up: releases and review
	side by side.
-->
<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { reshapeOn } from '#lib/morph/reshapeOn.svelte.ts';
	import { SurfaceSlot } from '#lib/morph/surface.ts';
	import MatchList from './MatchList.svelte';
	import MatchReview from './MatchReview.svelte';
	import { matchSheet } from './sheet.svelte.ts';

	const opening = $derived(matchSheet.current);
	const step = $derived(matchSheet.step);
	const review = $derived(matchSheet.review);

	let narrow = $state(false);
	let huge = $state(false);
	let sheet: HTMLElement | undefined = $state();
	/** The sheet has been tall: it stays so, so going back never plays the growth in reverse. */
	let grown = $state(false);
	let opener: Element | null = null;

	const surface = new SurfaceSlot({ key: 'match-sheet' });
	reshapeOn(
		surface,
		() => opening,
		() =>
			`${step}|${review.kind}|${review.kind === 'ready' ? review.value.candidate.id : ''}|${String(grown)}`
	);

	// No scrim: a press outside closes it, as a menu.
	$effect(() => {
		if (opening === null) {
			return;
		}
		function outside(event: PointerEvent): void {
			const target = event.target;
			if (
				sheet !== undefined &&
				target instanceof Element &&
				!sheet.contains(target) &&
				target.closest('[data-surface], [role="menu"], [role="dialog"]') === null
			) {
				close();
			}
		}
		addEventListener('pointerdown', outside, true);
		return () => {
			removeEventListener('pointerdown', outside, true);
		};
	});

	onMount(() => {
		const phone = matchMedia('(max-width: 720px)');
		const big = matchMedia('(min-width: 2560px)');
		const read = (): void => {
			narrow = phone.matches;
			huge = big.matches;
		};
		read();
		phone.addEventListener('change', read);
		big.addEventListener('change', read);
		return () => {
			phone.removeEventListener('change', read);
			big.removeEventListener('change', read);
		};
	});

	$effect(() => {
		if (opening === null) {
			grown = false;
			return;
		}
		if (step === 'review') {
			grown = true;
		}
	});

	$effect(() => {
		if (opening !== null) {
			opener ??= document.activeElement;
			void tick().then(() => sheet?.focus());
		}
	});

	function close(): void {
		matchSheet.close();
		const back = opener;
		opener = null;
		if (back instanceof HTMLElement) {
			back.focus();
		}
	}

	function onkeydown(event: KeyboardEvent): void {
		if (event.key === 'Escape' && opening !== null) {
			event.preventDefault();
			close();
		}
	}
</script>

<svelte:window {onkeydown} />

{#if opening !== null}
	<div
		class="sheet"
		class:tall={grown || step === 'review' || huge}
		class:narrow
		class:huge
		role="dialog"
		aria-modal="true"
		aria-labelledby={step === 'review' && !huge ? 'review-heading' : 'match-heading'}
		tabindex="-1"
		bind:this={sheet}
		{@attach surface.attach}
	>
		{#if narrow}<span class="handle" aria-hidden="true"></span>{/if}
		{#if huge}
			<div class="split">
				<MatchList beside {close} />
				<div class="beside">
					{#if step === 'review' && review.kind === 'ready'}
						{#key review.value.candidate.id}
							<div class="slide grow"><MatchReview held={review.value} {narrow} beside /></div>
						{/key}
					{:else if step === 'review' && review.kind === 'failed'}
						<p class="trouble" role="alert">
							That release can’t be read just now. Pick it again in a moment.
						</p>
					{:else if step === 'review'}
						<p class="asking" role="status">
							<span class="dot" aria-hidden="true"></span>Asking MusicBrainz…
						</p>
					{:else}
						<p class="empty">Pick a release; its review opens here.</p>
					{/if}
				</div>
			</div>
		{:else if step === 'list'}
			<div class="slide"><MatchList {close} /></div>
		{:else if review.kind === 'ready'}
			<div class="slide grow"><MatchReview held={review.value} {narrow} /></div>
		{:else if review.kind === 'failed'}
			<div class="slide">
				<p class="trouble" role="alert">
					{review.failure.kind === 'gone'
						? 'The album or that release is gone.'
						: 'That release can’t be read just now. Go back and pick it again in a moment.'}
				</p>
				<button type="button" class="ghost" onclick={() => matchSheet.back()}
					>Back to the releases</button
				>
			</div>
		{:else}
			<p class="asking" role="status">
				<span class="dot" aria-hidden="true"></span>Asking MusicBrainz…
			</p>
		{/if}
	</div>
{/if}

<style>
	/*
	 * The sheet is always the review's height. With the releases it sits lower, by --drop, and the
	 * list keeps to what shows; growing into the review is the drop going to nothing: a transform,
	 * never a change of height.
	 */
	.sheet {
		--full: calc(100dvh - 96px);
		--drop: calc(var(--full) - min(760px, 100dvh - 120px));

		position: fixed;
		left: var(--gutter);
		right: var(--gutter);
		bottom: 0;
		z-index: 5;
		height: var(--full);
		box-sizing: border-box;
		display: flex;
		flex-direction: column;
		padding: 28px 32px 0;
		border-radius: var(--radius-32) var(--radius-32) 0 0;
		background: var(--card);
		border: 1px solid var(--edge);
		border-bottom: 0;
		box-shadow: var(--shadow-float);
		outline: none;
		translate: 0 var(--drop);
	}

	/* What shows of the list, while the sheet sits low. */
	.sheet > .slide,
	.sheet > .asking {
		max-height: calc(100% - var(--drop));
	}

	/* The review: the same sheet, risen. */
	.sheet.tall {
		--drop: 0px;
	}

	.slide {
		flex: 1;
		min-height: 0;
		display: flex;
		flex-direction: column;
	}

	.slide > :global(*) {
		flex: 1;
		min-height: 0;
	}

	.asking,
	.empty {
		margin: 40px 0;
		display: flex;
		align-items: center;
		gap: 10px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--fox);
		animation: pulse 1.2s ease-in-out infinite;
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.25;
		}

		50% {
			opacity: 1;
		}
	}

	.trouble {
		margin: 32px 0 16px;
		font-size: 15px;
		color: var(--text-secondary);
	}

	.ghost {
		align-self: flex-start;
		height: var(--target);
		padding: 0 18px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: transparent;
		color: inherit;
		font: inherit;
		font-size: 14px;
		font-weight: 600;
		cursor: pointer;
	}

	/* One step up: releases and review side by side, every size a step larger. */
	/* Zoomed: lengths it takes from the screen are divided back out. */
	.sheet.huge {
		--full: calc((100dvh - 124px) / var(--step));

		left: calc(var(--gutter) / var(--step));
		right: calc(var(--gutter) / var(--step));
		padding: 36px 40px 0;
		border-radius: 40px 40px 0 0;
		zoom: var(--step);
	}

	.split {
		flex: 1;
		min-height: 0;
		display: grid;
		grid-template-columns: 800px minmax(0, 1fr);
		column-gap: 40px;
	}

	.split > :global(*) {
		min-height: 0;
		overflow-y: auto;
	}

	.beside {
		display: flex;
		flex-direction: column;
	}

	/* A phone: a full sheet, below the album's name. */
	.sheet.narrow {
		--full: calc(100dvh - 12px);
		--drop: 116px;

		left: 0;
		right: 0;
		padding: 10px 16px 0;
		border-radius: var(--radius-28) var(--radius-28) 0 0;
		border-left: 0;
		border-right: 0;
		overflow-y: auto;
	}

	.sheet.narrow.tall {
		--drop: 0px;
	}

	.handle {
		flex: none;
		width: 40px;
		height: 5px;
		margin: 0 auto 14px;
		border-radius: 3px;
		background: var(--edge-strong);
	}

	@media (max-width: 1100px) and (min-width: 721px) {
		.sheet {
			padding: 24px 24px 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dot {
			animation: none;
		}
	}
</style>
