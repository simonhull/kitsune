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

/**
 * The rung for a cover drawn `pixels` wide on the screen, density included: the smallest that
 * covers it. A source with nothing more to give stops at 512, as the ladder does.
 */
export function rungForBox({ width, height }: Pick<Artwork, 'width' | 'height'>, pixels: number) {
	if (pixels <= 128) {
		return 128;
	}
	return pixels > 512 && Math.max(width, height) > 512 ? 1024 : 512;
}

/** The best rung decoded so far, per cover: a morph shows this at once, and never waits. */
const decoded = new Map<string, number>();
const fading = new Set<string>();
const preloading = new Map<string, Promise<boolean>>();
/**
 * The covers decoded last, held on to: an image nothing refers to can leave the memory cache before
 * its card mounts, and then that card has to wait for it. It must outnumber what's decoded ahead.
 */
const justDecoded: HTMLImageElement[] = [];
const keepDecoded = 16;

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

/**
 * Starts a rung's bytes moving ahead of the click that needs them. Resolves once they're decoded,
 * and says whether they were. Whoever waits for this is what the cover first appears through, so it
 * counts as first sight: a cover that decoded here must not fade in again when it mounts.
 */
export function preload(artwork: Artwork | undefined, place: CoverPlace): Promise<boolean> {
	if (artwork === undefined) {
		return Promise.resolve(false);
	}
	const { hash } = artwork;
	const size = rungFor(artwork, place);
	const key = `${hash}-${String(size)}`;
	if ((decoded.get(hash) ?? 0) >= size) {
		return Promise.resolve(true);
	}
	const running = preloading.get(key);
	if (running !== undefined) {
		return running;
	}
	const image = new Image();
	image.fetchPriority = 'high';
	image.src = artworkUrl(hash, size);
	const done = image
		.decode()
		.then(() => {
			markDecoded(hash, size);
			firstSight(hash);
			justDecoded.push(image);
			justDecoded.splice(0, justDecoded.length - keepDecoded);
			return true;
		})
		.catch(() => false)
		.finally(() => preloading.delete(key));
	preloading.set(key, done);
	return done;
}
