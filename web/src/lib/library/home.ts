import type { AlbumEntry, AlbumPlays, History, IssueCounts, LibraryFacts } from './source.ts';

export function count(value: number): string {
	return value.toLocaleString('en-US');
}

export function plural(value: number, one: string, many: string): string {
	return `${count(value)} ${value === 1 ? one : many}`;
}

/** The time of day and the user's name, as the server knows it. */
export function greeting(hour: number, name: string): string {
	if (hour >= 5 && hour < 12) {
		return `Good morning, ${name}`;
	}
	return hour >= 12 && hour < 18 ? `Good afternoon, ${name}` : `Good evening, ${name}`;
}

const minuteMs = 60_000;
const hourMs = 60 * minuteMs;

/** How long the library plays for, in whole hours, or minutes for one shorter than an hour. */
export function listeningHours(durationMs: number): string {
	return durationMs < hourMs
		? plural(Math.round(durationMs / minuteMs), 'minute', 'minutes')
		: plural(Math.round(durationMs / hourMs), 'hour', 'hours');
}

/**
 * The line beside the greeting, in parts: albums, songs, and how long they play for. A phone leaves
 * the last out.
 */
export function summary({ albums, tracks, durationMs }: LibraryFacts): [string, string, string] {
	return [
		plural(albums, 'album', 'albums'),
		plural(tracks, 'song', 'songs'),
		listeningHours(durationMs)
	];
}

/** The line under the deck: only counts, because a scan has no total to measure against. */
export function scanLine({ albums, tracks }: LibraryFacts): string {
	return `${plural(albums, 'album', 'albums')} · ${plural(tracks, 'track', 'tracks')}`;
}

/** Why Home has nothing to show. Each is a different problem with a different answer. */
export function emptyLine({ roots }: LibraryFacts, scanning: boolean): string {
	if (roots === 0) {
		return 'No music folders are set up. Add one under roots in server.toml, then restart.';
	}
	return scanning
		? 'The first scan is running. Albums will appear here as it finds them.'
		: 'The scan found no music in your folders.';
}

/** The noun phrase the Health number means. "Yet" is a promise about a format, never a fault. */
export function issuesNoun({ unreadable, corrupted, unsupported }: IssueCounts): string {
	const issues = unreadable + corrupted + unsupported;
	const noun = issues === 1 ? 'file' : 'files';
	return unsupported === issues ? `${noun} Kitsune can't read yet` : `${noun} Kitsune can't read`;
}

/** What the Health page will find. It must agree with the number, at 0, 1 and N. */
export function healthNote(issues: number, counts: IssueCounts): string {
	return issues === 0 ? 'Every file was read' : `${count(issues)} ${issuesNoun(counts)}`;
}

/** The note on Shuffle Everything: the songs, and the artists once they're known. */
export function everythingNote(tracks: number, artists: number | null): string {
	const songs = plural(tracks, 'song', 'songs');
	return artists === null ? songs : `${songs} · ${plural(artists, 'artist', 'artists')}`;
}

/** What On repeat says under a title, in place of the artist. */
export function playsLine(plays: number): string {
	return plural(plays, 'play', 'plays');
}

/** Under the Just added tile's artist: the year, and when it came, such as `2013 · added Nov 9`. */
export function addedLine({ year, addedAt }: Pick<AlbumEntry, 'year' | 'addedAt'>): string {
	const added =
		addedAt === undefined
			? undefined
			: `added ${new Date(addedAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}`;
	return [year === undefined || year === 0 ? undefined : String(year), added]
		.filter((part) => part !== undefined)
		.join(' · ');
}

/** Home below the start row: each list is drawn only when it holds an album. */
export interface HomeRows {
	/** The newest album, in the Continue card's place while the player holds nothing. */
	feature: AlbumEntry | undefined;
	played: AlbumEntry[];
	onRepeat: AlbumPlays[];
	added: AlbumEntry[];
}

/**
 * Lays out Home from what's here: the newest albums, the history (null before it's known) and the
 * album the player holds. The held album is the Continue card's, so Recently played leaves it out;
 * holding nothing, the newest album takes the card, so Just added leaves that one out instead.
 */
export function homeRows(
	recent: readonly AlbumEntry[],
	history: History | null,
	held: string | null
): HomeRows {
	const feature = held === null ? recent.at(0) : undefined;
	return {
		feature,
		played: (history?.recent ?? []).filter(({ id }) => id !== held),
		onRepeat: history?.onRepeat ?? [],
		added: feature === undefined ? [...recent] : recent.slice(1)
	};
}
