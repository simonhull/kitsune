import { fnv1a } from '#lib/cover/cover.ts';
import {
	AlbumNotFoundError,
	ArtistNotFoundError,
	LoginError,
	type Album,
	type AlbumEntry,
	type AlbumPage,
	type AlbumSort,
	type ArtistDetail,
	type ArtistEntry,
	type ArtistLink,
	type LibraryFacts,
	type LibrarySource,
	type SearchResults,
	type Track,
	type TrackHit,
	type User
} from './source';

// The slugs are chosen so FNV-1a lands each album on the cover its board shows.
/** A few genres in turn, enough to fill the chips and to filter by. */
const boardGenres = [
	['Electronic', 'Ambient'],
	['Rock'],
	['Pop', 'Rock'],
	['Electronic'],
	['Metal', 'Rock'],
	['Soundtrack'],
	['Pop']
];

function board(id: string, title: string, artist: string): AlbumEntry {
	const genres = boardGenres.at(fnv1a(id) % boardGenres.length) ?? [];
	return {
		id,
		title,
		artist,
		trackCount: 8,
		compilation: false,
		addedAt: '2026-10-12T18:00:00Z',
		displayGenres: genres
	};
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
// Owned by nobody: its name stays plain text, and its artists appear on it.
const seaSampler: AlbumEntry = {
	...board('sea-sampler-31', 'Sea Sampler', 'Various Artists'),
	trackCount: 3,
	compilation: true
};

function artistLink(name: string): ArtistLink {
	return { id: `artist-${name.toLowerCase().replaceAll(' ', '-')}`, name };
}

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

const seaSamplerTracks: Track[] = [
	{ ...track(seaSampler, 1, 'Shoreline', 214, -7.1), artists: ['Ilse Varga'] },
	{ ...track(seaSampler, 2, 'Undertow', 248, -7.1), artists: ['Mere Coast'] },
	{ ...track(seaSampler, 3, 'Foghorn', 193, -7.1), artists: ['Ines Duarte'] }
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
		artists: entry.compilation ? [] : [artistLink(entry.artist)],
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
		albumFrom(seaSampler, seaSamplerTracks, 2023),
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

const artistNames = [...albums.values()].flatMap((album) =>
	album.tracks.flatMap((entry) => entry.artists)
);

const artistLinks = [...new Set(artistNames)]
	.map(artistLink)
	.toSorted((a, b) => a.name.localeCompare(b.name));

function owns({ artists }: Album, { id }: ArtistLink): boolean {
	return artists.some((owner) => owner.id === id);
}

function isOn({ tracks }: Album, { name }: ArtistLink): boolean {
	return tracks.some(({ artists }) => artists.includes(name));
}

function entryOf({
	id,
	title,
	artist,
	year,
	trackCount,
	compilation,
	addedAt,
	displayGenres
}: Album): AlbumEntry {
	return {
		id,
		title,
		artist,
		...(year === undefined ? {} : { year }),
		trackCount,
		compilation,
		addedAt,
		...(displayGenres === undefined ? {} : { displayGenres })
	};
}

function detailOf(link: ArtistLink): ArtistDetail {
	const everything = [...albums.values()];
	const albumsOwned = everything.filter((album) => owns(album, link));
	return {
		...link,
		albumCount: albumsOwned.length,
		albums: albumsOwned.map(entryOf),
		appearsOn: everything.filter((album) => !owns(album, link) && isOn(album, link)).map(entryOf)
	};
}

const owners: ArtistEntry[] = artistLinks
	.map(detailOf)
	.filter(({ albumCount }) => albumCount > 0)
	.map(({ id, name, albumCount }) => ({ id, name, albumCount }));

function hit(album: Album, entry: Track): TrackHit {
	const { id, title, artists, durationMs } = entry;
	return { id, title, artists, durationMs, albumId: album.id, album: album.title };
}

const trackHits = [...albums.values()].flatMap((album) =>
	album.tracks.map((entry) => hit(album, entry))
);

/** Finds what the real thing would, without the typos: the states are what fixtures are for. */
function find(query: string): SearchResults {
	const needle = query.trim().toLowerCase();
	const matches = (name: string): boolean => needle !== '' && name.toLowerCase().includes(needle);
	return {
		artists: artistLinks.filter(({ name }) => matches(name)).slice(0, 10),
		albums: [...albums.values()].filter(({ title }) => matches(title)).slice(0, 10),
		tracks: trackHits.filter(({ title }) => matches(title)).slice(0, 20)
	};
}

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
			}),
		artists: () => respond(() => owners),
		artist: (id) =>
			respond(() => {
				const link = artistLinks.find((artist) => artist.id === id);
				if (link === undefined) {
					throw new ArtistNotFoundError(id);
				}
				return detailOf(link);
			}),
		search: (query, signal) =>
			respond(() => {
				signal?.throwIfAborted();
				return find(query);
			}),
		// Never fetched: the fixtures play on a silent clock.
		streamUrl: (trackId) => `fixture:${trackId}`
	};
}

/** The real library: however fast a scan is set to run, this is what it finds. */
const scanLibrary = 927;
/** Seconds between flushes at the scanner's 50-track batch. */
export const defaultScanGap = 2.7;
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

/** When each burst lands, in milliseconds from the start: a little uneven, and never after the end. */
export function burstTimes(scanMs: number, gapMs: number): number[] {
	const times: number[] = [];
	let at = Math.min(gapMs * firstScanDelay, scanMs * 0.8);
	while (at < scanMs) {
		times.push(at);
		at += gapMs * (0.7 + (fnv1a(String(times.length)) % 1000) * 0.0006);
	}
	return times;
}

/**
 * How many albums this burst finds: a share of the library by how much of the scan one gap is, so
 * 2.7 s in a 412 s scan is the real thing, about 6 at a time, and the same library in 30 s is dense.
 */
export function burstSize(index: number, scanMs: number, gapMs: number): number {
	const even = (scanLibrary * gapMs) / scanMs;
	const spread = 0.2 * ((fnv1a(`burst-${String(index)}`) % 2001) / 1000 - 1);
	return Math.max(1, Math.round(even * (1 + spread)));
}

export interface ScanOptions extends FixtureOptions {
	scanMs: number;
	gapMs: number;
	/** When the scan began, on the clock `now` reads. */
	startedAt: number;
	now?: () => number;
}

/**
 * A first scan on a timer, through the same contract as a real one: small frequent bursts, growing
 * counts, and not ready until the end. It's all a function of the clock, so a
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
			.reduce((sum, _, index) => sum + burstSize(index, scanMs, gapMs), 0);
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
