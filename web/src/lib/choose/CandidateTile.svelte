<!--
	One candidate in the picker: its preview at the kind's shape, pressed to pin it; the chosen one
	ringed off register (the fox ring, the misprint behind it) and, when pinned, stamped. Its two mono
	caption lines say where it came from, its size, and what it is now.
-->
<script lang="ts">
	import Picture from '#lib/art/Picture.svelte';
	import type { PictureKind } from '#lib/art/art.ts';
	import type { ArtistImageKind, ImageCandidate } from './types.ts';
	import { captions } from './words.ts';

	interface Props {
		candidate: ImageCandidate;
		kind: ArtistImageKind;
		phone: boolean;
		/** Its pin is on its way. */
		busy: boolean;
		/** Something else is on its way: it waits. */
		waiting: boolean;
		onpin: () => void;
	}

	let { candidate, kind, phone, busy, waiting, onpin }: Props = $props();

	const lines = $derived(captions(candidate, new Date(), phone));
	/** The ladder its preview is drawn from: a banner's is a backdrop's. */
	const pictureKind: PictureKind = $derived(kind === 'banner' ? 'background' : kind);
	const label = $derived(
		[
			lines[0].map((part) => part.text).join(''),
			candidate.chosen ? lines[1].map((part) => part.text).join('') : 'Pin it'
		].join(' · ')
	);
</script>

<div class={['tile', kind]} class:chosen={candidate.chosen} data-candidate={candidate.id}>
	<button
		type="button"
		class="frame"
		data-menu-item
		aria-pressed={candidate.chosen}
		aria-label={label}
		disabled={waiting && !busy}
		onclick={onpin}
	>
		{#if candidate.preview !== undefined}
			<Picture
				image={candidate.preview}
				kind={pictureKind}
				box={phone ? 170 : 270}
				fit={kind === 'logo' ? 'contain' : 'cover'}
				ground={kind !== 'logo'}
			/>
		{:else}
			<span class="none">NO PREVIEW</span>
		{/if}
		{#if candidate.chosen && candidate.pinned}
			<span class="stamp" aria-hidden="true">PINNED</span>
		{/if}
		{#if candidate.chosen}<span class="ring" aria-hidden="true"></span>{/if}
	</button>
	<span class="caption">
		{#each lines[0] as part, at (at)}<span>{part.text}</span>{/each}
	</span>
	<span class="caption second" class:fox={busy || lines[1].some((part) => part.fox === true)}>
		{#if busy}
			PINNING…
		{:else}
			{#each lines[1] as part, at (at)}
				{#if part.href === undefined}
					<span>{part.text}</span>
				{:else}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- The licence's own page, not a route. -->
					<a href={part.href} target="_blank" rel="noreferrer noopener" data-menu-item
						>{part.text}</a
					>
				{/if}
			{/each}
		{/if}
	</span>
</div>

<style>
	.tile {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}

	.frame {
		position: relative;
		display: block;
		width: 100%;
		aspect-ratio: 1;
		padding: 0;
		border: 0;
		border-radius: var(--radius-16);
		background: var(--card-raised);
		box-shadow: inset 0 0 0 1px var(--edge-card);
		cursor: pointer;
	}

	.background .frame {
		aspect-ratio: 16 / 9;
	}

	.banner .frame {
		aspect-ratio: 1000 / 185;
	}

	/* A logo stands on the checker ground, so its transparency shows as what it is. */
	.logo .frame {
		aspect-ratio: 2.4;
		background: repeating-conic-gradient(var(--card-raised) 0 25%, var(--hover) 0 50%) 0 0 / 16px
			16px;
	}

	.frame :global(.picture) {
		position: absolute;
		inset: 0;
		border-radius: inherit;
		overflow: hidden;
	}

	.logo .frame :global(.picture) {
		inset: 10%;
		border-radius: 0;
	}

	.frame:disabled {
		cursor: progress;
	}

	.frame:hover:not(:disabled) {
		box-shadow: inset 0 0 0 1px var(--edge-hover);
	}

	.frame:focus-visible {
		outline: 2px solid var(--fox-line);
		outline-offset: 4px;
	}

	/* The chosen one, ringed off register: the fox ring, its misprint behind it. */
	.ring {
		position: absolute;
		inset: -5px;
		border-radius: calc(var(--radius-16) + 4px);
		box-shadow:
			inset 0 0 0 2px var(--fox-line),
			3px 3px 0 0 color-mix(in srgb, var(--misprint) 55%, transparent);
		pointer-events: none;
	}

	.stamp {
		position: absolute;
		left: 10px;
		top: 10px;
		padding: 5px 9px;
		background: var(--fox);
		color: var(--fox-ink);
		font: 700 10.5px var(--font-mono);
		letter-spacing: 0.08em;
	}

	.none {
		position: absolute;
		inset: 0;
		display: grid;
		place-items: center;
		font: 500 10.5px var(--font-mono);
		letter-spacing: 0.08em;
		color: var(--text-label);
	}

	.caption {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font: 500 10.5px var(--font-mono);
		letter-spacing: 0.06em;
		color: var(--text-secondary);
	}

	.caption:first-of-type {
		margin-top: 5px;
	}

	.second {
		min-height: 1.3em;
		color: var(--text-label);
	}

	.second.fox {
		color: var(--fox-text);
	}

	.second a {
		color: inherit;
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.second a:hover {
		color: var(--fox-text);
	}
</style>
