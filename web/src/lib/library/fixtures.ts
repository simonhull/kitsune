import { fnv1a } from '#lib/cover/cover.ts';
import {
	AlbumNotFoundError,
	LoginError,
	type Album,
	type AlbumEntry,
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
