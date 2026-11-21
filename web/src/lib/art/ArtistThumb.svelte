<!--
	An artist's thumb: round, because they are people (covers stay square, as records). With no
	picture it is the typeset initial on the artist's own colour, never a grey silhouette; one not yet
	known is a plain ground, and its picture develops in place when it comes.
-->
<script lang="ts">
	import type { Picture as PictureData } from '#lib/library/source.ts';
	import { morph, type MorphSlot } from '#lib/morph/registry.ts';
	import { initialColors, initialOf } from './art.ts';
	import Picture from './Picture.svelte';

	interface Props {
		artistId: string;
		name: string;
		/** Null: it has none, so the initial. Undefined: nothing says yet. */
		thumb: PictureData | null | undefined;
		/** Its diameter in CSS pixels at the first size step. */
		size: number;
		/** Where it travels from or to, as the artist's face: absent where it never does. */
		slot?: MorphSlot | undefined;
		lazy?: boolean;
	}

	let { artistId, name, thumb, size, slot, lazy = false }: Props = $props();

	const own = $derived(initialColors(artistId));
	const params = $derived({ id: `face:${artistId}`, slot: slot ?? 'row', kind: 'cover' as const });
</script>

{#snippet face()}
	{#if thumb === null}
		<span class="initial" style:background={own.ground} style:color={own.ink}
			>{initialOf(name)}</span
		>
	{:else if thumb !== undefined}
		<Picture image={thumb} kind="thumb" box={size * 1.25} {lazy} />
	{/if}
{/snippet}

{#if slot === undefined}
	<span class="thumb" aria-hidden="true" style:--size={`${String(size)}px`}>{@render face()}</span>
{:else}
	<span class="thumb" aria-hidden="true" style:--size={`${String(size)}px`} use:morph={params}
		>{@render face()}</span
	>
{/if}

<style>
	.thumb {
		position: relative;
		flex: none;
		display: inline-block;
		width: calc(var(--size) * var(--step));
		height: calc(var(--size) * var(--step));
		border-radius: 50%;
		overflow: hidden;
		background: var(--card-raised);
		box-shadow: inset 0 0 0 1px var(--cover-edge);
		vertical-align: middle;
	}

	.initial {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: calc(var(--size) * var(--step) * 0.52);
		font-weight: 900;
		letter-spacing: -0.04em;
		line-height: 1;
	}
</style>
