<script lang="ts">
	import { getContext, onDestroy, onMount, tick, untrack } from 'svelte';
	import { beforeNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { emptyLine, scanLine } from '#lib/library/home.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { AlbumEntry, LibraryFacts } from '#lib/library/source.ts';
	import {
		countFades,
		crossfade,
		deckToHome,
		drop,
		type Choreography
	} from '#lib/morph/choreography.ts';
	import { morphsKey } from '#lib/morph/engine.ts';
	import { Layer } from '#lib/morph/engines/layer.ts';
	import type { DealtCard, Departure, Morphs } from '#lib/morph/engines/spring.ts';
	import { dealTargets, reducedMotion } from '#lib/morph/journey.ts';
	import { morph, type MorphEntry } from '#lib/morph/registry.ts';
	import { holdSettled } from '#lib/morph/settled.ts';
	import { restOf, shrinkingDelays, spinOf } from './deck.ts';

	interface Props {
		facts: LibraryFacts;
	}

	/** How long a riffle's first step is, and a deal's. Each one after is shorter. */
	const riffleStepMs = 40;
	const dealStepMs = 30;
	/** How long a riffle's last card gets to come to rest, if frames have stopped coming. */
	const riffleSettlesMs = 320;

	let { facts }: Props = $props();

	const morphs = getContext<Morphs>(morphsKey);

	/** The pile as it stood when a press began: what's under a finger mustn't change before the click. */
	let held = $state.raw<AlbumEntry[] | null>(null);

	const pile = $derived(held ?? library.pile);
	const top = $derived(pile.at(-1));

	// Coming back to the pile shows it as it stands: only what arrives from here on drops.
	const alreadyHere = new Set(untrack(() => library.pile.map(({ id }) => id)));

	let root: HTMLElement | undefined = $state();
	/** Where the deck stood, once it's out of the flow and Home has taken its place. */
	let dealtFrom = $state<number | null>(null);
	let landing: Layer[] = [];
	let frame = 0;
	let lastFrame = 0;
	/** Set once a navigation has taken over: whatever the deal was about to do, it mustn't. */
	let overtaken = false;
	let onLanded: (() => void) | null = null;

	/** Each card moves on its own Channels, so one still landing carries on under the next. */
	function drops(element: HTMLElement, id: string): void {
		if (alreadyHere.has(id)) {
			return;
		}
		landing.push(new Layer(element, reducedMotion() ? crossfade : drop(spinOf(id))));
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
		if (frame === 0) {
			onLanded?.();
		}
	}

	function pause(ms: number): Promise<void> {
		return new Promise((resolve) => {
			setTimeout(resolve, ms);
		});
	}

	function allLanded(): Promise<void> {
		if (landing.length === 0) {
			return Promise.resolve();
		}
		const landed = new Promise<void>((resolve) => {
			onLanded = resolve;
		});
		return Promise.race([landed, pause(riffleSettlesMs)]);
	}

	function albumOf({ id }: MorphEntry): string {
		return id.slice('cover:'.length);
	}

	/** Holds Home back until the deal: mounted and measurable, but not yet to be seen. */
	function holdBack(main: Element, island: boolean): void {
		const units = [...main.querySelectorAll('[data-morph-part]')].filter(
			(unit) => unit instanceof HTMLElement && unit.dataset['morphPart'] !== 'intro'
		);
		const glass = island
			? document.querySelectorAll('[data-island-glass],[data-island-content]')
			: [];
		for (const element of [...units, ...glass]) {
			element.setAttribute('data-morph-hidden', '');
		}
	}

	/** Cut short before the deal began: whatever was held back is simply there. */
	function letHomeShow(): void {
		for (const element of document.querySelectorAll('[data-morph-hidden]')) {
			element.removeAttribute('data-morph-hidden');
		}
	}

	/** Lands on the pile whichever targets aren't on it, in reverse, so the first ends up on top. */
	async function riffle(targets: readonly MorphEntry[]): Promise<void> {
		const onPile = new Set(library.pile.map(({ id }) => id));
		const missing = targets
			.flatMap((target) => library.entry(albumOf(target)) ?? [])
			.filter(({ id }) => !onPile.has(id))
			.toReversed();
		const cards = await library.cardsFor(missing);
		const keep = new Set(targets.map(albumOf));
		const delays = shrinkingDelays(
			cards.map(({ id }) => id),
			riffleStepMs
		);
		for (const [index, card] of cards.entries()) {
			if (overtaken) {
				return;
			}
			library.riffle(card, keep);
			await pause((delays.at(index + 1) ?? 0) - (delays.at(index) ?? 0));
		}
		await tick();
		await allLanded();
	}

	/** Works out who flies where and what goes down with the pile, from the pile as it now stands. */
	function plan(targets: readonly MorphEntry[]): { cards: DealtCard[]; departures: Departure[] } {
		const cardsOnPile = [...(root?.querySelectorAll<HTMLElement>('[data-deck-card]') ?? [])];
		const delays = shrinkingDelays(targets.map(albumOf), dealStepMs);
		const cards = targets.flatMap((target, index) => {
			const element = cardsOnPile.find(({ dataset }) => dataset['deckCard'] === albumOf(target));
			return element === undefined
				? []
				: [
						{
							source: { id: target.id, slot: 'deck', kind: 'cover', element } as const,
							target,
							delay: delays.at(index) ?? 0
						}
					];
		});
		const dealt = new Set(cards.map(({ source }) => source.element));
		const last = cards.at(-1)?.delay ?? 0;
		const rest = cardsOnPile.filter((element) => !dealt.has(element));
		const lines = [...(root?.querySelectorAll<HTMLElement>('[data-deck-line]') ?? [])];
		return {
			cards,
			departures: [
				...rest.map((element) => ({
					element: element.parentElement ?? element,
					part: { ...deckToHome.leaving, delay: last }
				})),
				...lines.map((element) => ({ element, part: countFades }))
			]
		};
	}

	/**
	 * Plays the end of the scan: the deck steps out of the flow, Home mounts under it held back, and
	 * what Home shows is dealt to it. Missed moments get no deal: a hidden tab gets Home, and reduced
	 * motion gets the blend.
	 */
	async function deal(open: () => void): Promise<void> {
		if (document.hidden) {
			open();
			return;
		}
		if (reducedMotion()) {
			await morphs.blend(async () => {
				open();
				library.leaveDeck();
				await tick();
			});
			return;
		}
		const main = root?.closest('main');
		if (root === undefined || main === null || main === undefined) {
			open();
			return;
		}
		const release = holdSettled();
		overtaken = false;
		const islandForms = library.playing === null;
		dealtFrom = root.offsetTop;
		open();
		await tick();
		holdBack(main, islandForms);
		main.inert = true;
		try {
			const targets = [...dealTargets().values()];
			await riffle(targets);
			main.inert = false;
			if (overtaken) {
				letHomeShow();
				return;
			}
			const choreography: Choreography = islandForms
				? deckToHome
				: { ...deckToHome, islandForms: undefined };
			const { cards, departures } = plan(targets);
			library.dealing = 'dealt';
			await morphs.deal(choreography, cards, departures);
		} finally {
			main.inert = false;
			release();
		}
	}

	onMount(() => {
		library.dealer = deal;
		return () => {
			library.dealer = null;
		};
	});

	beforeNavigate(() => {
		overtaken = true;
	});

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

<!-- Once dealing it's no longer a unit of the page: Home is arriving, and this is what it's made of. -->
<section
	class="deck"
	class:dealing={dealtFrom !== null}
	aria-label="First scan"
	data-morph-part={dealtFrom === null ? 'card' : undefined}
	data-morph-key="deck"
	inert={dealtFrom !== null}
	style:top={dealtFrom === null ? undefined : `${String(dealtFrom)}px`}
	bind:this={root}
>
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
						data-deck-card={card.id}
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
		<p class="count" role="status" data-deck-line>{scanLine(facts)}</p>
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

	/* Home takes its place in the flow, and the pile stays exactly where it stood, on top of it. */
	.dealing {
		position: absolute;
		left: 0;
		right: 0;
		z-index: 1;
		pointer-events: none;
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
