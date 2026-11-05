<script lang="ts">
	import { artworkUrl } from '#lib/library/api.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { AlbumEntry, Artwork } from '#lib/library/source.ts';
	import GeneratedCover from './GeneratedCover.svelte';
	import { bestDecoded, firstSight, markDecoded, rungFor, type CoverPlace } from './rungs.ts';

	interface Props {
		albumId: string;
		/** Undefined on a cold deep link: nothing is known, so nothing is guessed. */
		entry: Pick<AlbumEntry, 'artwork'> | undefined;
		place: CoverPlace;
		lazy?: boolean;
	}

	let { albumId, entry, place, lazy = false }: Props = $props();

	const artwork = $derived(entry?.artwork);
	const wanted = $derived(artwork === undefined ? 0 : rungFor(artwork, place));
	const url = $derived(artwork === undefined ? '' : artworkUrl(artwork.hash, wanted));

	/** What's already decoded shows at once: a page's first frame never waits for bytes. */
	function bestOf({ hash }: Artwork): number {
		return bestDecoded(hash) ?? 0;
	}

	const best = $derived(artwork === undefined ? 0 : bestOf(artwork));

	let readyUrl = $state('');
	let brokenUrl = $state('');
	let fresh = $state(false);

	const ready = $derived(best === wanted || readyUrl === url);
	const broken = $derived(brokenUrl === url);

	function loaded({ currentTarget }: Event): void {
		const shown = artwork;
		if (shown === undefined || !(currentTarget instanceof HTMLImageElement)) {
			return;
		}
		const src = url;
		const size = wanted;
		currentTarget
			.decode()
			.then(() => {
				markDecoded(shown.hash, size);
				fresh = firstSight(shown.hash);
				readyUrl = src;
			})
			.catch(() => undefined);
	}

	function failed(): void {
		brokenUrl = url;
		// A cover can only fail for one of two reasons here, and this finds out if it's the session.
		library.probe();
	}
</script>

{#if entry === undefined}
	<span class="cover neutral" aria-hidden="true"></span>
{:else if artwork === undefined}
	<GeneratedCover {albumId} />
{:else}
	<span class="cover" aria-hidden="true" data-cover-want={url} style:background={artwork.ground}>
		{#if !broken}
			{#if best !== 0 && best !== wanted}
				<img src={artworkUrl(artwork.hash, best)} alt="" decoding="sync" draggable="false" />
			{/if}
			<!-- `loading` before `src`, or the fetch has started by the time it's read. -->
			<img
				loading={lazy ? 'lazy' : 'eager'}
				class="want"
				class:ready
				class:fresh
				data-cover-pending={ready ? undefined : ''}
				src={url}
				alt=""
				decoding={ready ? 'sync' : 'async'}
				draggable="false"
				onload={loaded}
				onerror={failed}
				onanimationend={() => {
					fresh = false;
				}}
			/>
		{/if}
	</span>
{/if}

<style>
	.cover {
		position: absolute;
		inset: 0;
		display: block;
		overflow: hidden;
	}

	.neutral {
		background: var(--card);
	}

	img {
		position: absolute;
		inset: 0;
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
		user-select: none;
		-webkit-user-drag: none;
	}

	.want {
		opacity: 0;
	}

	.want.ready {
		opacity: 1;
	}

	/* Added on first decode, so no clone of this markup can ever replay it. */
	.want.fresh {
		animation: cover-in var(--morph-fast) var(--curve-morph);
	}

	@keyframes cover-in {
		from {
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.want.fresh {
			animation-duration: 120ms;
		}
	}
</style>
