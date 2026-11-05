import type { LibraryFacts } from './source.ts';

function count(value: number): string {
	return value.toLocaleString('en-US');
}

function plural(value: number, one: string, many: string): string {
	return `${count(value)} ${value === 1 ? one : many}`;
}

/** The time of day and the user's name, as the server knows it. */
export function greeting(hour: number, name: string): string {
	if (hour >= 5 && hour < 12) {
		return `Good morning, ${name}`;
	}
	return hour >= 12 && hour < 18 ? `Good afternoon, ${name}` : `Good evening, ${name}`;
}

/** The line beside the greeting: only what `/library` reports. */
export function summary({ albums, roots, issues }: LibraryFacts): string {
	return [
		plural(albums, 'album', 'albums'),
		`${plural(roots, 'root', 'roots')} watched`,
		issues === 0 ? 'no issues' : plural(issues, 'issue', 'issues')
	].join(' · ');
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

/** What the Health tile's number means. It must agree with the number, at 0, 1 and N. */
export function healthNote(issues: number): string {
	if (issues === 0) {
		return 'Every file was read';
	}
	return issues === 1 ? '1 file could not be read' : `${count(issues)} files could not be read`;
}

export function trackCount(tracks: number): string {
	return plural(tracks, 'track', 'tracks');
}

const minuteMs = 60_000;
const hourMs = 60 * minuteMs;

/** Total listening time in days and hours, or minutes for a library smaller than an hour. */
export function listeningTime(durationMs: number): string {
	if (durationMs < hourMs) {
		return `${String(Math.round(durationMs / minuteMs))}m`;
	}
	const hours = Math.round(durationMs / hourMs);
	const days = Math.floor(hours / 24);
	return days === 0 ? `${String(hours)}h` : `${count(days)}d ${String(hours - days * 24)}h`;
}
