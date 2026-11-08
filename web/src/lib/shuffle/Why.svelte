<!--
	Why a song in Up next came up: a quiet line, and the factors on a press. Part of "why this song",
	which why.ts describes, and which nothing else depends on.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import type { UpNextItem } from './session.svelte.ts';
	import { whyFactors, whyLine } from './why.ts';

	interface Props {
		item: UpNextItem;
	}

	let { item }: Props = $props();

	const why = $derived(item.why);
	const line = $derived(why === undefined ? '' : whyLine(why, item.titles.length));
	const title = $derived(item.titles.join(' + '));

	/** Clear of the screen's edges by this much. */
	const margin = 12;

	let open = $state(false);
	let trigger: HTMLElement | undefined = $state();
	let panel: HTMLElement | undefined = $state();
	let left = $state(0);
	let top = $state(0);
	/** A phone's: it rises from the bottom, so nothing places it. */
	let sheet = $state(false);

	function show(event: MouseEvent & { currentTarget: HTMLElement }): void {
		trigger = event.currentTarget;
		sheet = matchMedia('(max-width: 720px)').matches;
		const width = innerWidth >= 2560 ? 560 : 420;
		const box = trigger.getBoundingClientRect();
		left = Math.max(margin, Math.min(box.right - width, innerWidth - width - margin));
		top = box.bottom + 8;
		open = true;
	}

	function close(refocus: boolean): void {
		open = false;
		if (refocus) {
			trigger?.focus();
		}
	}

	// Kept on screen once its height is known: above the row when there's no room below.
	$effect(() => {
		if (!open || sheet || panel === undefined || trigger === undefined) {
			return;
		}
		const { height } = panel.getBoundingClientRect();
		if (top + height > innerHeight - margin) {
			top = Math.max(margin, trigger.getBoundingClientRect().top - height - 8);
		}
	});

	/** The panel takes the focus as it opens, so Escape closes it from the keyboard. */
	function focusOnOpen(node: HTMLElement): void {
		node.focus();
	}

	onMount(() => {
		function outside(event: PointerEvent): void {
			if (open && event.target instanceof Node && panel?.contains(event.target) !== true) {
				close(false);
			}
		}
		addEventListener('pointerdown', outside, true);
		return () => {
			removeEventListener('pointerdown', outside, true);
		};
	});
</script>

{#if why !== undefined}
	<span class="why">
		<button
			type="button"
			class="why-open"
			class:open
			aria-label={`Why ${title} came up: ${line}`}
			aria-expanded={open}
			onclick={show}><span class="who">{item.artist} · </span>{line}</button
		>
		<button
			type="button"
			class="why-more"
			aria-label={`Why ${title} came up`}
			aria-expanded={open}
			onclick={show}
			><svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true"
				><circle cx="5" cy="12" r="1.8"></circle><circle cx="12" cy="12" r="1.8"></circle><circle
					cx="19"
					cy="12"
					r="1.8"
				></circle></svg
			></button
		>
		{#if open}
			<div
				class="pop"
				role="dialog"
				aria-label={`Why ${title} came up`}
				tabindex="-1"
				bind:this={panel}
				class:sheet
				style:left={sheet ? undefined : `${String(left)}px`}
				style:top={sheet ? undefined : `${String(top)}px`}
				use:focusOnOpen
				onkeydown={(event) => {
					if (event.key === 'Escape') {
						event.stopPropagation();
						close(true);
					}
				}}
			>
				<p class="heading">WHY {title.toUpperCase()} CAME UP</p>
				{#each whyFactors(why) as f (f.name)}
					<div class="factor">
						<span>{f.name}</span>
						<span class="track"
							><span class="fill" class:lifts={f.lifts} style:width={`${String(f.bar * 100)}%`}
							></span></span
						>
						<span class="times">{f.times}</span>
					</div>
				{/each}
				{#if !why.memory}
					<p class="note">Skips and how much you hear don't count: you turned that off.</p>
				{/if}
				<p class="note">
					Then spaced: no {item.artist} for the next four songs, no {item.album.title} for eight.
				</p>
			</div>
		{/if}
	</span>
{/if}

<style>
	.why {
		display: contents;
	}

	.why-open {
		flex: none;
		max-width: 300px;
		min-height: 44px;
		padding: 0;
		border: 0;
		background: none;
		font-family: var(--font-mono);
		font-size: 11px;
		line-height: 1.5;
		letter-spacing: 0.04em;
		text-align: right;
		text-transform: uppercase;
		color: var(--text-meta);
		cursor: pointer;
	}

	.why-open:hover,
	.why-open.open {
		color: var(--fox-text);
	}

	.who,
	.why-more {
		display: none;
	}

	.why-more {
		width: 44px;
		height: 44px;
		place-items: center;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text-meta);
	}

	circle {
		fill: currentcolor;
	}

	.pop {
		position: fixed;
		z-index: 6;
		width: 420px;
		box-sizing: border-box;
		padding: 24px;
		display: flex;
		flex-direction: column;
		gap: 12px;
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid var(--edge-raised);
		box-shadow: var(--shadow-float);
		text-align: left;
	}

	.pop:focus {
		outline: none;
	}

	.heading {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.factor {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 140px 52px;
		align-items: center;
		gap: 12px;
		font-size: 14px;
	}

	.track {
		height: 6px;
		border-radius: var(--radius-pill);
		background: var(--edge-raised);
		overflow: hidden;
	}

	.fill {
		display: block;
		height: 100%;
		border-radius: var(--radius-pill);
		background: var(--text-meta);
	}

	.fill.lifts {
		background: var(--fox);
	}

	.times {
		font-family: var(--font-mono);
		font-size: 12px;
		text-align: right;
		color: var(--text-secondary);
	}

	.note {
		margin-top: 4px;
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	/* A phone: the line goes under the title with the artist, ⋯ opens the factors from the bottom. */
	@media (max-width: 720px) {
		.why-open {
			grid-column: 2;
			grid-row: 2;
			max-width: none;
			min-height: 0;
			overflow: hidden;
			white-space: nowrap;
			text-overflow: ellipsis;
			text-align: left;
			text-transform: none;
			pointer-events: none;
		}

		.who {
			display: inline;
		}

		.why-more {
			grid-column: 3;
			grid-row: 1 / span 2;
			display: grid;
		}

		.pop.sheet {
			left: 8px;
			right: 8px;
			bottom: 8px;
			width: auto;
		}

		.factor {
			grid-template-columns: minmax(0, 1fr) 96px 44px;
		}
	}

	@media (min-width: 2560px) {
		.why-open {
			max-width: 440px;
			font-size: 15px;
		}

		.pop {
			width: 560px;
			padding: 32px;
			gap: 16px;
		}

		.heading {
			font-size: 15px;
		}

		.factor {
			grid-template-columns: minmax(0, 1fr) 190px 72px;
			font-size: 18px;
		}

		.times {
			font-size: 16px;
		}

		.note {
			font-size: 17px;
		}
	}
</style>
