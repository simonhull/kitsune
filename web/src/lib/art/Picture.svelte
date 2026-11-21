<!--
	A picture drawn into its box at true size: an artist's thumb, backdrop or logo, a disc, a back
	cover. Its ground is laid first; the picture develops in place once decoded (a print coming up: no
	colour, hard contrast, dark, to itself), and never holds anything up. A new picture for the same
	box, from an `artists` event, develops over the one it replaces, which goes once it has.
-->
<script lang="ts">
	import type { Picture } from '#lib/library/source.ts';
	import { decodedPictures, pictureRung, pictureUrl, type PictureKind } from './art.ts';

	interface Props {
		image: Picture;
		kind: PictureKind;
		/** How many CSS pixels its longest side is drawn at, which picks the rendition. */
		box: number;
		fit?: 'cover' | 'contain';
		/** Where a cropped or letterboxed picture sits in its box. */
		position?: string;
		/** Lay the ground colour under it: off for a logo or a disc, which have alpha. */
		ground?: boolean;
		lazy?: boolean;
	}

	let {
		image,
		kind,
		box,
		fit = 'cover',
		position = 'center',
		ground = !image.alpha,
		lazy = false
	}: Props = $props();

	const density = typeof devicePixelRatio === 'undefined' ? 1 : devicePixelRatio;
	const url = $derived(pictureUrl(image.hash, pictureRung(kind, box * density)));

	/** The URL whose picture has decoded, and whether it did so in front of someone. */
	let readyUrl = $state('');
	let fresh = $state(false);
	/** The picture being replaced, kept under the new one until that has developed. */
	let previous = $state('');
	let shownUrl = '';

	const ready = $derived(readyUrl === url || decodedPictures.has(url));

	$effect.pre(() => {
		const next = url;
		if (shownUrl !== next) {
			if (shownUrl !== '' && decodedPictures.has(shownUrl)) {
				previous = shownUrl;
			}
			fresh = false;
		}
		shownUrl = next;
	});

	function loaded({ currentTarget }: Event): void {
		if (!(currentTarget instanceof HTMLImageElement)) {
			return;
		}
		const src = url;
		currentTarget
			.decode()
			.then(() => {
				fresh = !decodedPictures.has(src);
				decodedPictures.add(src);
				readyUrl = src;
				if (!fresh) {
					previous = '';
				}
			})
			.catch(() => undefined);
	}
</script>

<span
	class="picture"
	class:ground
	aria-hidden="true"
	data-picture={image.hash}
	data-picture-ready={ready ? '' : undefined}
	style:background={ground ? image.ground : undefined}
	style:--fit={fit}
	style:--position={position}
>
	{#if previous !== '' && previous !== url}
		<img src={previous} alt="" decoding="sync" draggable="false" />
	{/if}
	{#key url}
		<img
			loading={lazy ? 'lazy' : 'eager'}
			class="want"
			class:ready
			class:fresh
			src={url}
			alt=""
			decoding={ready ? 'sync' : 'async'}
			draggable="false"
			onload={loaded}
			onanimationend={() => {
				fresh = false;
				previous = '';
			}}
		/>
	{/key}
</span>

<style>
	.picture {
		position: absolute;
		inset: 0;
		display: block;
		overflow: hidden;
		border-radius: inherit;
	}

	img {
		position: absolute;
		inset: 0;
		display: block;
		width: 100%;
		height: 100%;
		object-fit: var(--fit);
		object-position: var(--position);
		user-select: none;
		-webkit-user-drag: none;
	}

	.want {
		opacity: 0;
	}

	.want.ready {
		opacity: 1;
	}

	/* Added on first decode, and gone when it ends: no picture of this markup ever replays it. */
	.want.fresh {
		animation: develop var(--morph-slow) var(--curve-morph);
	}

	@keyframes develop {
		from {
			opacity: 0;
			filter: saturate(0) contrast(1.6) brightness(0.5);
		}
	}

	@keyframes blend-in {
		from {
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.want.fresh {
			animation: blend-in 120ms linear;
		}
	}
</style>
