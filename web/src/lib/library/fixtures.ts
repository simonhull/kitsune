import { fnv1a } from '#lib/cover/cover.ts';
import {
	AlbumNotFoundError,
	LoginError,
	type Album,
	type AlbumEntry,
	type AlbumPage,
	type AlbumSort,
	type LibraryFacts,
	type LibrarySource,
	type Track,
	type User
} from './source';

// The slugs are chosen so FNV-1a lands each album on the cover its board shows.
function board(id: string, title: string, artist: string): AlbumEntry {
	return { id, title, artist, trackCount: 8, compilation: false };
}

const glasshouse: AlbumEntry = {
	...board('glasshouse-66', 'Glasshouse', 'Low Tide Choir'),
	trackCount: 10
};
const nightFerry = board('night-ferry-94', 'Night Ferry', 'The Halden Set');
const orbitals = board('orbitals-14', 'Orbitals', 'Nadia Brandt');
const quietMachines = board('quiet-machines-24', 'Quiet Machines', 'Ilse Varga');
const harbourLights = board('harbour-lights-6', 'Harbour Lights', 'Ana Lindqvist');
const paperSuns = board('paper-suns-16', 'Paper Suns', 'Odile Marsh');
const redSignal = board('red-signal-109', 'Red Signal', 'Vanta Park');
const longSummer = board('long-summer-7', 'Long Summer', 'Kaito Ferreira');
const tidewater = board('tidewater-12', 'Tidewater', 'Mere Coast');
const afterHours = board('after-hours-19', 'After Hours', 'Juno Adeyemi');
// Not on Home: a deep link here has no card to return to.
const tinLantern = board('tin-lantern-78', 'Tin Lantern', 'Sorrel Vane');

function track(
	album: AlbumEntry,
	trackNo: number,
	title: string,
	seconds: number,
	gain: number | null,
	guest?: string
): Track {
	return {
		id: `${album.id}-${String(trackNo)}`,
		title,
		artists: guest === undefined ? [album.artist] : [album.artist, guest],
		discNo: 1,
		trackNo,
		durationMs: seconds * 1000,
		format: 'FLAC',
		sampleRate: 96_000,
		bitDepth: 24,
		...(gain === null ? {} : { replayGain: { trackGain: gain } })
	};
}

const glasshouseTracks: Track[] = [
	track(glasshouse, 1, 'Breakwater', 238, -6.2),
	track(glasshouse, 2, 'Salt in the Lungs', 271, -6.2, 'Ines Duarte'),
	track(glasshouse, 3, 'Greenhouse Effect', 192, -6.2),
	track(glasshouse, 4, 'Small Weather', 306, -6.2),
	track(glasshouse, 5, 'Condensation', 167, -6.2),
	track(glasshouse, 6, 'Panes', 260, -6.2),
	track(glasshouse, 7, 'Low Tide', 362, null),
	track(glasshouse, 8, 'Ribbons', 213, -6.2),
	track(glasshouse, 9, 'Harbor Wall', 224, -6.2),
	track(glasshouse, 10, 'Still Water', 236, -6.2)
];

const firstWords = ['Slow', 'Paper', 'Winter', 'Hollow', 'Amber', 'Distant', 'Quiet', 'Broken'];
const lastWords = ['Light', 'Rooms', 'Signal', 'Rivers', 'Hours', 'Static', 'Gardens', 'Echo'];

/** Draws a stable track list from the album's ID. */
function tracksFor(album: AlbumEntry): Track[] {
	const seed = fnv1a(album.id);
	return Array.from({ length: 8 }, (_, index) => {
		const first = firstWords.at((seed + index) % firstWords.length) ?? 'Slow';
		const last = lastWords.at((seed + index * 3) % lastWords.length) ?? 'Light';
		return track(album, index + 1, `${first} ${last}`, 150 + ((seed >>> index) % 210), -5.4);
	});
}

function albumFrom(entry: AlbumEntry, tracks: Track[], year: number): Album {
	return {
		...entry,
		year,
		tracks,
		durationMs: tracks.reduce((sum, { durationMs }) => sum + durationMs, 0),
		formats: ['FLAC'],
		addedAt: '2026-10-12T18:00:00Z'
	};
}

const albums = new Map<string, Album>(
	[
		albumFrom(glasshouse, glasshouseTracks, 2024),
		...[
			nightFerry,
			orbitals,
			quietMachines,
			harbourLights,
			paperSuns,
			redSignal,
			longSummer,
			tidewater,
			afterHours,
			tinLantern
		].map((entry) => albumFrom(entry, tracksFor(entry), 2019 + (fnv1a(entry.id) % 7)))
	].map((album) => [album.id, album])
);

const newestFirst = [glasshouse, paperSuns, redSignal, longSummer, tidewater, afterHours];

const byName = [...albums.values()]
	.filter((album) => album.id !== tinLantern.id)
	.toSorted((a, b) => a.title.localeCompare(b.title));

const user: User = {
	id: 1,
	name: 'Simon',
	role: 'admin',
	createdAt: '2026-01-01T00:00:00Z'
};

const facts: LibraryFacts = {
	albums: albums.size,
	tracks: 82,
	durationMs: 21_480_000,
	roots: 2,
	issues: 3
};

export interface FixtureOptions {
	/** Delay every response by this many milliseconds, as a real server would. */
	latencyMs?: number;
}

/** Builds a {@link LibrarySource} from the boards' fake library. */
export function createFixtureSource(options: FixtureOptions = {}): LibrarySource {
	const { latencyMs = 0 } = options;

	async function respond<T>(answer: () => T): Promise<T> {
		if (latencyMs > 0) {
			await new Promise((resolve) => setTimeout(resolve, latencyMs));
		}
		return answer();
	}

	return {
		me: () => respond(() => user),
		login: (name) =>
			respond(() => {
				if (name === '') {
					throw new LoginError({ kind: 'credentials' });
				}
				return user;
			}),
		facts: () => respond(() => facts),
		ready: () => respond(() => true),
		albumsPage: (sort, _cursor, limit) =>
			respond(() => ({ albums: (sort === 'added' ? newestFirst : byName).slice(0, limit) })),
		album: (id) =>
			respond(() => {
				const album = albums.get(id);
				if (album === undefined) {
					throw new AlbumNotFoundError(id);
				}
				return album;
			})
	};
}

/** Albums per flush on the real library's first scan, in order. */
export const burstSizes = [86, 67, 29, 4, 48, 75, 56, 56, 69, 54, 89, 62, 99, 49, 65, 19];
const firstScanDelay = 1.2;
const scanArtists = ['Mere Coast', 'Ilse Varga', 'The Halden Set', 'Vanta Park', 'Sorrel Vane'];

function scanEntry(index: number): AlbumEntry {
	const id = `scan-${String(index)}`;
	const seed = fnv1a(id);
	const first = firstWords.at(seed % firstWords.length) ?? 'Slow';
	const last = lastWords.at((seed >>> 8) % lastWords.length) ?? 'Light';
	const artist = scanArtists.at((seed >>> 16) % scanArtists.length) ?? 'Mere Coast';
	return {
		id,
		title: `${first} ${last} ${String(index)}`,
		artist,
		trackCount: 8,
		compilation: false
	};
}

/** When each burst lands, in milliseconds from the start: uneven, as flushes are, and never after the end. */
export function burstTimes(scanMs: number, gapMs: number): number[] {
	const times: number[] = [];
	let at = Math.min(gapMs * firstScanDelay, scanMs * 0.8);
	while (at < scanMs) {
		times.push(at);
		// From a ninth of the mean to nearly one and a half, like the real gaps of 3 to 39 s.
		at += gapMs * (0.11 + (fnv1a(String(times.length)) % 1000) * 0.00178);
	}
	return times;
}

export interface ScanOptions extends FixtureOptions {
	scanMs: number;
	gapMs: number;
	/** When the scan began, on the clock `now` reads. */
	startedAt: number;
	now?: () => number;
}

/**
 * A first scan on a timer, through the same contract as a real one: bursts sized like the real
 * library's, growing counts, and not ready until the end. It's all a function of the clock, so a
 * reload picks up where the scan had got to.
 */
export function createScanFixtureSource({
	scanMs,
	gapMs,
	startedAt,
	now = Date.now,
	latencyMs = 0
}: ScanOptions): LibrarySource {
	const times = burstTimes(scanMs, gapMs);
	const base = createFixtureSource({ latencyMs });

	function found(): number {
		const elapsed = now() - startedAt;
		return times
			.filter((at) => at <= elapsed)
			.reduce((sum, _, index) => sum + (burstSizes.at(index % burstSizes.length) ?? 0), 0);
	}

	function entries(sort: AlbumSort): AlbumEntry[] {
		const all = Array.from({ length: found() }, (_, index) => scanEntry(index));
		return sort === 'added'
			? all.toReversed()
			: all.toSorted((a, b) => a.title.localeCompare(b.title));
	}

	function page(sort: AlbumSort, cursor: string | undefined, limit: number): AlbumPage {
		const all = entries(sort);
		const start = Number(cursor ?? 0);
		const end = start + limit;
		return {
			albums: all.slice(start, end),
			...(end < all.length ? { nextCursor: String(end) } : {})
		};
	}

	async function respond<T>(answer: () => T): Promise<T> {
		await base.ready();
		return answer();
	}

	return {
		...base,
		facts: () =>
			respond(() => {
				const albums = found();
				return {
					albums,
					tracks: albums * 8,
					durationMs: albums * 8 * 240_000,
					roots: 1,
					issues: 0
				};
			}),
		ready: () => respond(() => now() - startedAt >= scanMs),
		albumsPage: (sort, cursor, limit = 60) => respond(() => page(sort, cursor, limit)),
		album: (id) =>
			respond(() => {
				const index = Number(id.slice('scan-'.length));
				if (!id.startsWith('scan-') || !Number.isInteger(index) || index >= found()) {
					throw new AlbumNotFoundError(id);
				}
				const entry = scanEntry(index);
				return albumFrom(entry, tracksFor(entry), 2019 + (fnv1a(id) % 7));
			})
	};
}

/** Finds when this scan began, or starts it, so that a reload resumes it. A new tab starts again. */
export function scanStart(key: string, now: number): number {
	try {
		const held = Number(sessionStorage.getItem(key));
		if (held > 0) {
			return held;
		}
		sessionStorage.setItem(key, String(now));
	} catch {
		// Storage is off: a reload restarts the scan, which is all that's lost.
	}
	return now;
}
