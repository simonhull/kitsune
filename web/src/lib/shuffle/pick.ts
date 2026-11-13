import type { LibraryIndex, ShuffleMark } from '#lib/library/source.ts';

/** A song as the first pick sees it: where it is, the person's stars, and their rules on it. */
export interface Candidate {
	id: string;
	albumId: string;
	/** ½ to 5 in halves, or absent. */
	stars?: number;
	mark?: ShuffleMark;
}

/** What shuffle picks: one song, or a run of combined songs, in the order they play. */
export interface Unit {
	songs: readonly Candidate[];
}

/** The rating curve at "how much ratings matter" 1, as the server's engine has it. */
const starCurve = [0.2, 0.6, 1, 1.6, 2.4] as const;
/** What an unrated song weighs as: the middle of the scale. */
const unratedStars = 3;

/**
 * The units of the songs in scope: a song on its own, or the whole run it's combined in, from its
 * first song, wherever the run's songs are. `everyone` finds a run's songs outside the scope. A song
 * excluded from shuffle never comes up on its own; a run plays it, unless every song in it is
 * excluded.
 */
export function unitsOf(
	inScope: Iterable<Candidate>,
	everyone: Iterable<Candidate> = inScope
): Unit[] {
	const runs = new Map<string, Candidate[]>();
	for (const song of everyone) {
		const run = song.mark?.run;
		if (run !== undefined) {
			runs.set(run, [...(runs.get(run) ?? []), song]);
		}
	}
	const units: Unit[] = [];
	const taken = new Set<string>();
	for (const song of inScope) {
		const run = song.mark?.run;
		if (run === undefined) {
			if (song.mark?.excluded !== true) {
				units.push({ songs: [song] });
			}
			continue;
		}
		if (taken.has(run)) {
			continue;
		}
		taken.add(run);
		const songs = [...(runs.get(run) ?? [song])].sort(
			(a, b) => (a.mark?.position ?? 0) - (b.mark?.position ?? 0)
		);
		if (songs.some(({ mark }) => mark?.excluded !== true)) {
			units.push({ songs });
		}
	}
	return units;
}

/**
 * A unit's weight from its rating alone: what is known here, without the server's history. The mean of its rated songs, unrated counting as 3, on the server's curve, at `ratings`.
 */
export function weightOf({ songs }: Unit, ratings: number): number {
	const rated = songs.filter(({ stars }) => stars !== undefined && stars >= 0.5 && stars <= 5);
	const mean =
		rated.length === 0
			? unratedStars
			: rated.reduce((sum, { stars }) => sum + (stars ?? 0), 0) / rated.length;
	// Under 1★, only ½★ songs: the first step, 1★ to 2★, goes on down, as on the server.
	const low = Math.min(Math.max(1, Math.floor(mean)), starCurve.length);
	const at = (stars: number): number => Math.log(starCurve[stars - 1] ?? 1);
	const curve =
		low >= starCurve.length ? at(low) : at(low) + (mean - low) * (at(low + 1) - at(low));
	return Math.exp(ratings * curve);
}

/** Draws a unit, each in proportion to its weight. None for no units. */
export function pickUnit(
	units: readonly Unit[],
	ratings: number,
	random: () => number = Math.random
): Unit | undefined {
	const weights = units.map((unit) => weightOf(unit, ratings));
	let target = random() * weights.reduce((sum, weight) => sum + weight, 0);
	for (const [index, weight] of weights.entries()) {
		target -= weight;
		if (target < 0) {
			return units[index];
		}
	}
	return units.at(-1);
}

/** Says whether a song of the index is in scope: what a Library filter keeps, or anything. */
export type InScope = (
	artist: LibraryIndex['artists'][number],
	album: LibraryIndex['artists'][number]['albums'][number],
	track: LibraryIndex['artists'][number]['albums'][number]['tracks'][number]
) => boolean;

/** What the index holds, as candidates: those `keep` keeps, and everyone, for the runs. */
export function candidatesOf(
	index: LibraryIndex,
	marksOf: (trackId: string) => { stars?: number },
	keep: InScope = () => true
): { inScope: Candidate[]; everyone: Candidate[] } {
	const inScope: Candidate[] = [];
	const everyone: Candidate[] = [];
	for (const artist of index.artists) {
		for (const album of artist.albums) {
			for (const track of album.tracks) {
				const candidate: Candidate = {
					id: track.id,
					albumId: album.id,
					...marksOf(track.id),
					mark: track.shuffle
				};
				everyone.push(candidate);
				if (keep(artist, album, track)) {
					inScope.push(candidate);
				}
			}
		}
	}
	return { inScope, everyone };
}
