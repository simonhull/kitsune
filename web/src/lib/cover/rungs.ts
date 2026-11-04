import { artworkUrl } from '#lib/library/api.ts';
import type { Artwork } from '#lib/library/source.ts';

/** Where a cover sits, which decides how much of it is worth fetching. */
export type CoverPlace = 'small' | 'card' | 'stage';

/** A source this small has nothing more to give at 1024: the ladder never upscales. */
const stageNeeds = 640;

/** The rendition a place wants. Cards never exceed 512; only a stage earns 1024. */
export function rungFor({ width, height }: Pick<Artwork, 'width' | 'height'>, place: CoverPlace) {
	if (place === 'small') {
		return 128;
	}
	return place === 'stage' && Math.max(width, height) > stageNeeds ? 1024 : 512;
}

/** The best rung decoded so far, per cover: a morph shows this at once, and never waits. */
const decoded = new Map<string, number>();
const fading = new Set<string>();
const preloading = new Set<string>();

export function bestDecoded(hash: string): number | undefined {
	return decoded.get(hash);
}

export function markDecoded(hash: string, size: number): void {
	decoded.set(hash, Math.max(size, decoded.get(hash) ?? 0));
}

/** Reports whether this cover is new to the session: each fades in once, ever. */
export function firstSight(hash: string): boolean {
	if (fading.has(hash)) {
		return false;
	}
	fading.add(hash);
	return true;
}

/** Starts a rung's bytes moving ahead of the click that needs them. */
export function preload(artwork: Artwork | undefined, place: CoverPlace): void {
	if (artwork === undefined) {
		return;
	}
	const { hash } = artwork;
	const size = rungFor(artwork, place);
	const key = `${hash}-${String(size)}`;
	if ((decoded.get(hash) ?? 0) >= size || preloading.has(key)) {
		return;
	}
	preloading.add(key);
	const image = new Image();
	image.fetchPriority = 'high';
	image.src = artworkUrl(hash, size);
	image
		.decode()
		.then(() => {
			markDecoded(hash, size);
		})
		.catch(() => undefined)
		.finally(() => preloading.delete(key));
}
