import { fnv1a } from '#lib/cover/cover.ts';
import {
	AlbumNotFoundError,
	type Album,
	type AlbumEntry,
	type Home,
	type LibrarySource,
	type NowPlaying,
	type Track
} from './source';

// The slugs are chosen so FNV-1a lands each album on the cover its board shows.
const glasshouse: AlbumEntry = {
	id: 'glasshouse-66',
	title: 'Glasshouse',
	artist: 'Low Tide Choir'
};
const nightFerry: AlbumEntry = {
	id: 'night-ferry-94',
	title: 'Night Ferry',
	artist: 'The Halden Set'
};
const orbitals: AlbumEntry = { id: 'orbitals-14', title: 'Orbitals', artist: 'Nadia Brandt' };
const quietMachines: AlbumEntry = {
	id: 'quiet-machines-24',
	title: 'Quiet Machines',
	artist: 'Ilse Varga'
};
const harbourLights: AlbumEntry = {
	id: 'harbour-lights-6',
	title: 'Harbour Lights',
	artist: 'Ana Lindqvist'
};
const paperSuns: AlbumEntry = { id: 'paper-suns-16', title: 'Paper Suns', artist: 'Odile Marsh' };
const redSignal: AlbumEntry = { id: 'red-signal-109', title: 'Red Signal', artist: 'Vanta Park' };
const longSummer: AlbumEntry = {
	id: 'long-summer-7',
	title: 'Long Summer',
	artist: 'Kaito Ferreira'
};
const tidewater: AlbumEntry = { id: 'tidewater-12', title: 'Tidewater', artist: 'Mere Coast' };
const afterHours: AlbumEntry = {
	id: 'after-hours-19',
	title: 'After Hours',
	artist: 'Juno Adeyemi'
};
// Not on Home: a deep link here has no card to return to.
const tinLantern: AlbumEntry = {
	id: 'tin-lantern-78',
	title: 'Tin Lantern',
	artist: 'Sorrel Vane'
};

const glasshouseTracks: Track[] = [
	{ number: 1, title: 'Breakwater', seconds: 238, replayGain: -6.2 },
	{
		number: 2,
		title: 'Salt in the Lungs',
		featuring: 'Ines Duarte',
		seconds: 271,
		replayGain: -6.2
	},
	{ number: 3, title: 'Greenhouse Effect', seconds: 192, replayGain: -6.2 },
	{ number: 4, title: 'Small Weather', seconds: 306, replayGain: -6.2 },
	{ number: 5, title: 'Condensation', seconds: 167, replayGain: -6.2 },
	{ number: 6, title: 'Panes', seconds: 260, replayGain: -6.2 },
	{ number: 7, title: 'Low Tide', seconds: 362, replayGain: null },
	{ number: 8, title: 'Ribbons', seconds: 213, replayGain: -6.2 },
	{ number: 9, title: 'Harbor Wall', seconds: 224, replayGain: -6.2 },
	{ number: 10, title: 'Still Water', seconds: 236, replayGain: -6.2 }
];

const firstWords = ['Slow', 'Paper', 'Winter', 'Hollow', 'Amber', 'Distant', 'Quiet', 'Broken'];
const lastWords = ['Light', 'Rooms', 'Signal', 'Rivers', 'Hours', 'Static', 'Gardens', 'Echo'];

/** Draws a stable track list from the album's ID. */
function tracksFor(id: string): Track[] {
	const seed = fnv1a(id);
	return Array.from({ length: 8 }, (_, index) => {
		const first = firstWords.at((seed + index) % firstWords.length) ?? 'Slow';
		const last = lastWords.at((seed + index * 3) % lastWords.length) ?? 'Light';
		return {
			number: index + 1,
			title: `${first} ${last}`,
			seconds: 150 + ((seed >>> index) % 210),
			replayGain: -5.4
		};
	});
}

function albumFrom(entry: AlbumEntry, tracks: Track[], year: number): Album {
	const seconds = tracks.reduce((sum, track) => sum + track.seconds, 0);
	return {
		...entry,
		year,
		minutes: Math.round(seconds / 60),
		tracks,
		format: 'FLAC 24/96',
		path: `/srv/music/${entry.artist}/${String(year)} - ${entry.title}/`,
		added: '12 Oct 2026'
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
		].map((entry) => albumFrom(entry, tracksFor(entry.id), 2019 + (fnv1a(entry.id) % 7)))
	].map((album) => [album.id, album])
);

const home: Home = {
	greeting: 'Good evening, Simon',
	summary: '1,284 albums · 2 roots watched · in sync',
	continueListening: {
		album: glasshouse,
		trackNumber: 2,
		trackCount: 10,
		positionSeconds: 102
	},
	health: { issues: 9, detail: '6 missing ReplayGain · 3 duplicates' },
	week: { listened: '11h 40m', detail: '212 plays · top: Low Tide Choir' },
	recentlyAdded: [paperSuns, redSignal, longSummer, tidewater, afterHours],
	onRepeat: [glasshouse, nightFerry, orbitals, quietMachines, harbourLights]
};

function nowPlayingFor(albumId: string): NowPlaying {
	const album = albums.get(albumId);
	if (album === undefined) {
		throw new AlbumNotFoundError(albumId);
	}
	const index = album.id === glasshouse.id ? 1 : 0;
	const track = album.tracks.at(index);
	if (track === undefined) {
		throw new RangeError(`${album.title} has no track ${String(index + 1)}`);
	}
	const next = album.tracks.slice(index + 1, index + 3);
	return {
		album: { id: album.id, title: album.title, artist: album.artist },
		track,
		trackCount: album.tracks.length,
		positionSeconds: 102,
		format: album.format,
		upNext: [
			...next.map((upcoming) => ({ title: upcoming.title, albumId: album.id })),
			{ title: tinLantern.title, albumId: tinLantern.id }
		],
		moreCount: 5
	};
}

export interface FixtureOptions {
	/** Delay every response by this many milliseconds, as a real server would. */
	latencyMs?: number;
	/** Play this album instead of Glasshouse. */
	playingAlbumId?: string;
}

/** Builds a {@link LibrarySource} from the boards' fake library. */
export function createFixtureSource(options: FixtureOptions = {}): LibrarySource {
	const { latencyMs = 0, playingAlbumId = glasshouse.id } = options;

	async function respond<T>(answer: () => T): Promise<T> {
		if (latencyMs > 0) {
			await new Promise((resolve) => setTimeout(resolve, latencyMs));
		}
		return answer();
	}

	return {
		home: () => respond(() => home),
		album: (id) =>
			respond(() => {
				const album = albums.get(id);
				if (album === undefined) {
					throw new AlbumNotFoundError(id);
				}
				return album;
			}),
		nowPlaying: () => respond(() => nowPlayingFor(playingAlbumId))
	};
}
