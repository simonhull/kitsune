<!--
	The match sheet, over whatever page opened it. It rises from the bottom with the releases, and
	grows upward into the review of the one picked, the page staying underneath. Back is a move of its
	own: the list slides in from the left, and the sheet keeps its height. Esc closes from either
	step. At 3840 it is one step up: releases and review side by side.
-->
<script lang="ts">
	import { onMount, tick } from 'svelte';
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
	<!-- The scrim is a pointer's way out; Esc is the keyboard's. -->
	<div class="scrim" aria-hidden="true" onclick={close}></div>
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
			<div class="slide" class:back={matchSheet.motion === 'back'}><MatchList {close} /></div>
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
	.scrim {
		position: fixed;
		inset: 0;
		z-index: 5;
		background: color-mix(in srgb, var(--ground) 60%, transparent);
		animation: fade var(--morph) var(--curve-morph) both;
	}

	@keyframes fade {
		from {
			opacity: 0;
		}
	}

	/*
	 * The sheet is always the review's height. With the releases it sits lower, by --drop, and the
	 * list keeps to what shows; growing into the review is the drop going to nothing: a transform,
	 * never a change of height.
	 */
	.sheet {
		--full: calc(100dvh - 96px);
		--drop: calc(var(--full) - min(760px, 100dvh - 120px));

		position: fixed;
		left: 48px;
		right: 48px;
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
		animation: rise var(--morph) var(--curve-settle) backwards;
		transition: translate var(--morph-slow) var(--curve-settle);
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

	@keyframes rise {
		from {
			transform: translateY(100%);
		}
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

	.slide.grow {
		animation: settle var(--morph-slow) var(--curve-settle) both;
	}

	@keyframes settle {
		from {
			opacity: 0;
			translate: 0 24px;
		}
	}

	/* Back: the list comes in from the left, a move of its own. */
	.slide.back {
		animation: from-left var(--morph) var(--curve-settle) both;
	}

	@keyframes from-left {
		from {
			opacity: 0;
			translate: -48px 0;
		}
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
		--step: 1.25;
		--full: calc((100dvh - 124px) / var(--step));

		left: calc(96px / var(--step));
		right: calc(96px / var(--step));
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
			left: 24px;
			right: 24px;
			padding: 24px 24px 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.sheet {
			transition: none;
		}

		.sheet,
		.scrim,
		.slide.grow,
		.slide.back,
		.dot {
			animation: none;
			transition: none;
		}
	}
</style>
