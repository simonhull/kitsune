import { maxStars } from './stars.ts';

/** An album's stars, worked out from its songs: never set, never stored, so never stale. */
export interface AlbumStars {
	/** The mean of the rated songs, to one place. */
	mean: number;
	/** How many songs are rated. */
	rated: number;
	/** How many songs there are. */
	of: number;
}

/**
 * Works out an album's stars from its songs' own, or null when none is rated. An unrated song
 * counts for nothing, not for zero: it hasn't been judged.
 */
export function albumStars(
	tracks: readonly { id: string }[],
	marks: { stars(trackId: string): number | null }
): AlbumStars | null {
	let sum = 0;
	let rated = 0;
	for (const { id } of tracks) {
		const stars = marks.stars(id);
		if (stars !== null) {
			sum += stars;
			rated += 1;
		}
	}
	// Tenths as integers, so a mean like 3.25 rounds up as it reads, not as binary has it.
	return rated === 0
		? null
		: { mean: Math.round((sum * 10) / rated) / 10, rated, of: tracks.length };
}

/** What the stars say, for someone who can't see them. */
export function starsLabel({ mean, rated }: AlbumStars): string {
	return `Averages ${String(mean)} of ${String(maxStars)} from ${String(rated)} rated ${rated === 1 ? 'song' : 'songs'}`;
}
