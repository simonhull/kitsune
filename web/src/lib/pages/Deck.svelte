<script lang="ts">
	import { onDestroy, untrack } from 'svelte';
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { emptyLine, scanLine } from '#lib/library/home.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { AlbumEntry, LibraryFacts } from '#lib/library/source.ts';
	import { crossfade, drop } from '#lib/morph/choreography.ts';
	import { Layer } from '#lib/morph/engines/layer.ts';
	import { reducedMotion } from '#lib/morph/journey.ts';
	import { morph } from '#lib/morph/registry.ts';
	import { restOf } from './deck.ts';

	interface Props {
		facts: LibraryFacts;
	}

	let { facts }: Props = $props();

	/** The pile as it stood when a press began: what's under a finger mustn't change before the click. */
	let held = $state.raw<AlbumEntry[] | null>(null);

	const pile = $derived(held ?? library.pile);
	const top = $derived(pile.at(-1));

	// Coming back to the pile shows it as it stands: only what arrives from here on drops.
	const alreadyHere = new Set(untrack(() => library.pile.map(({ id }) => id)));

	let landing: Layer[] = [];
	let frame = 0;
	let lastFrame = 0;

	/** Each card moves on its own Channels, so one still landing carries on under the next. */
	function drops(element: HTMLElement, id: string): void {
		if (alreadyHere.has(id)) {
			return;
		}
		landing.push(new Layer(element, reducedMotion() ? crossfade : drop));
		if (frame === 0) {
			lastFrame = performance.now();
			frame = requestAnimationFrame(step);
		}
	}

	function step(now: number): void {
		const seconds = Math.min((now - lastFrame) / 1000, 1 / 20);
		lastFrame = now;
		for (const layer of landing) {
			layer.step(seconds);
			layer.write();
		}
		landing = landing.filter((layer) => {
			if (layer.settled) {
				layer.release();
			}
			return !layer.settled;
		});
		frame = landing.length > 0 ? requestAnimationFrame(step) : 0;
	}

	function hold(): void {
		held = library.pile;
		addEventListener('pointerup', letGo, { once: true });
		addEventListener('pointercancel', letGo, { once: true });
	}

	/** After the click, which comes straight after the pointer is released. */
	function letGo(): void {
		removeEventListener('pointerup', letGo);
		removeEventListener('pointercancel', letGo);
		setTimeout(() => {
			held = null;
		});
	}

	onDestroy(() => {
		cancelAnimationFrame(frame);
		removeEventListener('pointerup', letGo);
		removeEventListener('pointercancel', letGo);
	});
</script>

<section class="deck" aria-label="First scan" data-morph-part="card" data-morph-key="deck">
	{#if top !== undefined}
		<a
			class="pile"
			href={resolve('/album/[id]', { id: top.id })}
			aria-label={`${top.title} by ${top.artist}`}
			onpointerdown={hold}
		>
			{#each pile as card (card.id)}
				<span class="slot" use:drops={card.id}>
					<!-- Only the top card is shared: what's under it can't be reached, so it can't travel. -->
					<span
						class="card"
						style:transform={restOf(card.id)}
						use:morph={{
							id: `${card.id === top.id ? 'cover' : 'pile'}:${card.id}`,
							slot: 'deck',
							kind: 'cover'
						}}><Cover albumId={card.id} entry={card} place="card" /></span
					>
				</span>
			{/each}
		</a>
		<p class="count" role="status">{scanLine(facts)}</p>
	{:else}
		<p class="line" role="status">{emptyLine(facts, true)}</p>
	{/if}
	{#if library.down}
		<p class="line" role="status">The server isn't answering. Trying again…</p>
	{/if}
</section>

<style>
	.deck {
		--card-size: 280px;

		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 16px;
		text-align: center;
	}

	/* Room for the tilt: a card turned 9 degrees reaches past its own box. */
	.pile {
		position: relative;
		display: block;
		width: var(--card-size);
		height: var(--card-size);
		margin: 40px 0 32px;
	}

	.slot,
	.card {
		position: absolute;
		inset: 0;
		display: block;
	}

	.card {
		overflow: hidden;
		border-radius: var(--radius-20);
		box-shadow: 0 12px 28px rgb(0 0 0 / 32%);
	}

	.count,
	.line {
		position: relative;
	}

	.count {
		font-family: var(--font-mono);
		font-size: 13px;
		font-variant-numeric: tabular-nums;
		color: var(--text-meta);
	}

	.line {
		max-width: 44ch;
		font-size: var(--type-title);
		line-height: 1.4;
		color: var(--text-secondary);
	}

	@media (max-width: 720px) {
		.deck {
			--card-size: 200px;
		}
	}
</style>
