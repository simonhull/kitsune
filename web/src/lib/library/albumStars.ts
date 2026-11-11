import type { Counted } from './heartStars.ts';
import { maxStars } from './stars.ts';

/** An album's stars, worked out from its songs: never set, never stored, so never stale. */
export interface AlbumStars {
	/** The mean of the rated songs, to one place. */
	mean: number;
	/** How many songs are rated, counting those a heart rates. */
	rated: number;
	/** How many of those count by their heart alone. */
	byHeart: number;
	/** How many songs there are. */
	of: number;
}

/**
 * Works out an album's stars from its songs' counted stars (their own, else their heart's), or null
 * when none is rated. An unrated song counts for nothing, not for zero: it hasn't been judged.
 */
export function albumStars(
	tracks: readonly { id: string }[],
	marks: { counted(trackId: string): Counted | null }
): AlbumStars | null {
	let sum = 0;
	let rated = 0;
	let byHeart = 0;
	for (const { id } of tracks) {
		const rating = marks.counted(id);
		if (rating !== null) {
			sum += rating.stars;
			rated += 1;
			byHeart += rating.byHeart ? 1 : 0;
		}
	}
	// Tenths as integers, so a mean like 3.25 rounds up as it reads, not as binary has it.
	return rated === 0
		? null
		: { mean: Math.round((sum * 10) / rated) / 10, rated, byHeart, of: tracks.length };
}

/** What the stars say, for someone who can't see them. */
export function starsLabel({ mean, rated, byHeart }: AlbumStars): string {
	const from = `Averages ${String(mean)} of ${String(maxStars)} from ${String(rated)} rated ${rated === 1 ? 'song' : 'songs'}`;
	return byHeart === 0 ? from : `${from}, ${String(byHeart)} by heart`;
}
