import { changedSettings, foldersFixture, settingsFixture } from './settingsFixtures.ts';
import { fnv1a } from '#lib/cover/cover.ts';
import { letterOf } from '#lib/pages/libraryRows.ts';
import { normalizeCode } from '#lib/pages/passwords.ts';
import { discsOf } from './format.ts';
import { partOf } from './indexPatch.ts';
import { createHealthFixture, type HealthFixture } from './healthFixtures.ts';
import {
	AlbumNotFoundError,
	ArtistNotFoundError,
	EditError,
	LoginError,
	PasswordError,
	ResetError,
	ServerError,
	SetupError,
	UnauthorizedError,
	type Album,
	type AlbumEdit,
	type AlbumEntry,
	type AlbumPage,
	type AlbumSort,
	type AlbumTags,
	type ArtistDetail,
	type ListedArtist,
	type ArtistLink,
	type IndexField,
	type LastfmStatus,
	type LibraryAlbum,
	type LibraryArtist,
	type LibraryIndex,
	type LibrarySource,
	type LibraryTrack,
	type MarkKind,
	type Me,
	type PrefKey,
	type Prefs,
	type RawTag,
	type ScanStatus,
	type ShuffleMode,
	type ShufflePage,
	type ShufflePick,
	type ShuffleScope,
	type ShuffleWhy,
	type SearchResults,
	type SetupStatus,
	type ShuffleMark,
	type TagEdit,
	type TagField,
	type Track,
	type TrackHit,
	type TrackTags
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

/** The genres every fixture track is tagged with, as the tag editor's boards show them. */
const boardTags = ['Ambient', 'Post-rock'];

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
		...(gain === null ? {} : { replayGain: { trackGain: gain, source: 'tags' as const } }),
		trackTotal: album.trackCount,
		discTotal: 1,
		genres: boardTags,
		etag: `${album.id}-${String(trackNo)}:0`,
		hearted: false
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
		tracks: tracks.map((track) => ({ ...track, year })),
		discs: discsOf(tracks),
		durationMs: tracks.reduce((sum, { durationMs }) => sum + durationMs, 0),
		formats: ['FLAC'],
		addedAt: '2026-10-12T18:00:00Z',
		genres: boardTags,
		etag: `${entry.id}:0`,
		hearted: false
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

/** Named by a title, a length and who it's by, if that's not the album's own artist. */
type Song = [title: string, seconds: number, by?: string];

function extraAlbum(entry: AlbumEntry, year: number | undefined, songs: Song[], disc = 1): Album {
	const tracks = songs.map(([title, seconds, by], index) => ({
		...track(entry, index + 1, title, seconds, null),
		discNo: disc,
		...(by === undefined ? {} : { artists: [by] })
	}));
	return { ...albumFrom({ ...entry, trackCount: tracks.length }, tracks, year ?? 0), year };
}

/** Two discs, with track numbers that start again on the second. */
function twoDiscs(entry: AlbumEntry, year: number, one: Song[], two: Song[]): Album {
	const first = extraAlbum(entry, year, one);
	const second = extraAlbum(entry, year, two, 2).tracks.map((each) => ({
		...each,
		id: `${each.id}-2`
	}));
	const tracks = [...first.tracks, ...second];
	return { ...first, tracks, trackCount: tracks.length, discs: discsOf(tracks) };
}

const longTitle =
	'A Collection of Very Nearly Everything We Ever Recorded in the Basement of the Old Fire Hall';
const longArtist = 'The Extraordinarily Long-Winded Philharmonic Orchestra of Upper Saskatchewan';

/** Only the Library lists these: Home's fixtures are counted, and stay as their boards have them. */
const listOnly = new Map<string, Album>(
	[
		extraAlbum(board('tidewrack-41', 'Tidewrack', 'Low Tide Choir'), 2018, [
			['Kelp Line', 251],
			['What the Water Kept', 320],
			['Gulls', 178],
			['Neap', 220],
			['Spring Tide', 284],
			['Driftwood Hymn', 372]
		]),
		extraAlbum(board('undated-52', 'Undated', 'Low Tide Choir'), undefined, [['Loose Ends', 200]]),
		twoDiscs(
			board('long-way-round-63', 'Long Way Round', 'Mere Coast'),
			2020,
			[
				['Outbound', 240],
				['Crossing', 300],
				['Landfall', 280]
			],
			[
				['Homebound', 260],
				['Shipping Forecast', 190, 'Ilse Varga']
			]
		),
		extraAlbum(board('shared-shore-74', 'Shared Shore', 'Ana Lindqvist, Ilse Varga'), 2022, [
			['Two Tides', 200, 'Ana Lindqvist, Ilse Varga'],
			['Their Harbor', 215, 'Ana Lindqvist, Ilse Varga']
		]),
		extraAlbum(board('rooms-85', longTitle, longArtist), 2016, [
			['An Unreasonably Long Song Title That Has Nowhere Left to Go on This Row at All', 900],
			['Short', 60, 'Somebody Else With Rather a Long Name of Their Own, Actually']
		]),
		extraAlbum(board('zero-96', '8 Hours', '808 Harbour'), 2017, [['Signal One', 200]]),
		extraAlbum(board('elan-97', 'Vital', 'Élan Quartet'), 2015, [['Accent', 200]])
	].map((album) => [album.id, album])
);

/** Where an album files: as `GET /library/index` has it, the first of its artists, or their name. */
function ownerOf({ artist, compilation }: Album): Pick<LibraryArtist, 'name' | 'sortName'> {
	const name = artist.split(', ').at(0) ?? artist;
	return {
		name,
		sortName: !compilation && name.startsWith('The ') ? `${name.slice(4)}, The` : name
	};
}

function listed({ tracks, year, discs, ...album }: Album): LibraryAlbum {
	const { id, title, artist, compilation } = album;
	// As the server sends them: only when they say what the songs don't.
	const telling = discs.length > 1 || discs.some(({ name }) => name !== undefined);
	return {
		id,
		title,
		artist,
		compilation,
		hearted: false,
		...(year === undefined ? {} : { year }),
		...(telling ? { discs } : {}),
		tracks: tracks.map(({ id: track, title: name, durationMs, artists, discNo, trackNo }) => ({
			id: track,
			title: name,
			durationMs,
			artists,
			hearted: false,
			...(discNo === undefined ? {} : { discNo }),
			...(trackNo === undefined ? {} : { trackNo })
		}))
	};
}

/** When the fixtures' plays happened, counted back from: a fixed moment, so lines don't drift. */
export const fixtureNow = Date.parse('2026-11-07T12:00:00Z');
const dayMs = 86_400_000;
const composersOf = ['I. Marrow', 'M. Vey', 'O. Strand'];

/** What `fields` adds to a fixture track: drawn from its ID, so it's the same every time. */
function fieldsOf(
	album: Album,
	track: Track,
	fields: ReadonlySet<IndexField>
): Partial<LibraryTrack> {
	const seed = fnv1a(track.id);
	const plays = seed % 7 === 0 ? 0 : seed % 53;
	const lossy = album.id === 'tidewrack-41';
	const out: Partial<LibraryTrack> = {};
	if (fields.has('genre') && album.displayGenres !== undefined && album.displayGenres.length > 0) {
		out.displayGenres = album.displayGenres;
	}
	if (fields.has('albumArtist') && !album.compilation) {
		out.albumArtists = [album.artist];
	}
	if (fields.has('year') && album.year !== undefined && album.year !== 0) {
		out.year = album.year;
	}
	if (fields.has('composer')) {
		out.composers = [composersOf.at(seed % composersOf.length) ?? ''];
	}
	if (fields.has('format')) {
		Object.assign(
			out,
			lossy
				? { format: 'MP3', sampleRate: 44_100 }
				: { format: 'FLAC', sampleRate: 44_100, bitDepth: 16 }
		);
	}
	if (fields.has('format') || fields.has('bitrate')) {
		out.bitrate = lossy ? 320 : 800 + (seed % 300);
	}
	if (fields.has('added')) {
		out.addedAt = album.addedAt;
	}
	// Most files carry their gain; one in seven doesn't, so "no RG" has something to show, and
	// one in five more was measured by Kitsune.
	if (fields.has('gain') && seed % 7 !== 0) {
		out.trackGain = -((seed % 110) / 10);
		out.trackGainSource = seed % 5 === 0 ? 'measured' : 'tags';
	}
	if (fields.has('plays') && plays > 0) {
		out.plays = plays;
	}
	if (fields.has('lastPlayed') && plays > 0) {
		out.lastPlayedAt = new Date(
			fixtureNow - (seed % 90) * dayMs - (seed % 1000) * 60_000
		).toISOString();
	}
	return out;
}

function indexFixture(): LibraryIndex {
	const groups = new Map<string, LibraryArtist>();
	for (const album of [...albums.values(), ...listOnly.values()]) {
		const owner = ownerOf(album);
		const group = groups.get(owner.name) ?? {
			...owner,
			...(album.compilation ? {} : { id: artistLink(owner.name).id }),
			hearted: false,
			albums: []
		};
		group.albums.push(listed(album));
		groups.set(owner.name, group);
	}
	const files = ({ sortName }: LibraryArtist): string => `${letterOf(sortName)}${sortName}`;
	const artists = [...groups.values()].toSorted((a, b) => {
		const [one, two] = [files(a), files(b)];
		return one.startsWith('#') === two.startsWith('#')
			? one.localeCompare(two)
			: one < two
				? -1
				: 1;
	});
	for (const artist of artists) {
		artist.albums.sort((a, b) => (b.year ?? 0) - (a.year ?? 0));
	}
	return { artists };
}

/**
 * Board A's marks, which every fixture source opens with: hearts of every kind, and Glasshouse
 * rated 6 of its 10 songs to a 3.8. A few more songs elsewhere give 4 and up something to find.
 */
const boardHearts: readonly string[] = [
	'artists/artist-low-tide-choir',
	'albums/glasshouse-66',
	'tracks/glasshouse-66-1',
	'tracks/glasshouse-66-5',
	'tracks/glasshouse-66-9',
	'tracks/tidewrack-41-2',
	'tracks/night-ferry-94-3',
	'albums/long-way-round-63'
];
const boardStars: readonly (readonly [string, number])[] = [
	['glasshouse-66-1', 5],
	['glasshouse-66-2', 3],
	['glasshouse-66-5', 4],
	['glasshouse-66-6', 2],
	['glasshouse-66-7', 5],
	['glasshouse-66-9', 4],
	['tidewrack-41-1', 4],
	['tidewrack-41-4', 2],
	['orbitals-14-2', 5],
	['sea-sampler-31-2', 4]
];

const newestFirst = [glasshouse, paperSuns, redSignal, longSummer, tidewater, afterHours];

/** An album as a list has it: no tracks, and no heart, which a list never says. */
function listEntry(album: Album): AlbumEntry {
	const entry: Partial<Album> = { ...album };
	delete entry.hearted;
	delete entry.tracks;
	return entry as AlbumEntry;
}

const byName = [...albums.values()]
	.filter((album) => album.id !== tinLantern.id)
	.toSorted((a, b) => a.title.localeCompare(b.title))
	.map(listEntry);

const user: Me = {
	id: 1,
	name: 'Simon',
	role: 'admin',
	createdAt: '2026-01-01T00:00:00Z',
	canEditTags: true
};

/** The boards' counts, the real library's: the fake one is only a window onto it. */
const counts = { albums: 927, tracks: 7586, durationMs: 1_677_087_013, roots: 2 };

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
	const songs = artistIndexOf(indexFixture(), link).artists.flatMap((artist) =>
		artist.albums.flatMap((album) => album.tracks)
	);
	return {
		...link,
		albumCount: albumsOwned.length,
		albums: albumsOwned.map(entryOf),
		appearsOn: everything.filter((album) => !owns(album, link) && isOn(album, link)).map(entryOf),
		songCount: songs.length
	};
}

/**
 * As `GET /library/index?artist=` has it: the songs an artist is on, under one heading, the albums
 * it owns first and then those it appears on, each newest first and with just its songs.
 */
function artistIndexOf(index: LibraryIndex, link: ArtistLink): LibraryIndex {
	const all = index.artists.flatMap((artist) => artist.albums);
	const owned = (album: LibraryAlbum): boolean => {
		const full = albums.get(album.id) ?? listOnly.get(album.id);
		return full !== undefined && owns(full, link);
	};
	const newest = (a: LibraryAlbum, b: LibraryAlbum): number => (b.year ?? 0) - (a.year ?? 0);
	const appears = all
		.filter((album) => !owned(album))
		.map((album) => ({
			...album,
			tracks: album.tracks.filter(({ artists }) => artists.includes(link.name))
		}))
		.filter(({ tracks }) => tracks.length > 0);
	const heading = index.artists.find(({ id }) => id === link.id);
	return {
		artists: [
			{
				id: link.id,
				name: link.name,
				sortName: heading?.sortName ?? link.name,
				hearted: heading?.hearted ?? false,
				albums: [...all.filter(owned).toSorted(newest), ...appears.toSorted(newest)]
			}
		]
	};
}

const owners: ListedArtist[] = artistLinks
	.map(detailOf)
	.filter(({ albumCount }) => albumCount > 0)
	.map(({ id, name, albumCount, albums: owned }) => ({
		id,
		name,
		albumCount,
		addedAt: owned.map(({ addedAt = '' }) => addedAt).reduce((a, b) => (a > b ? a : b), ''),
		plays: 0
	}));

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

/** Moves an ETag on, as a rewrite of the file would. */
function nextEtag(etag: string): string {
	const [name = '', count = '0'] = etag.split(':');
	return `${name}:${String(Number(count) + 1)}`;
}

/** What a tag editor sees of a track that's never been edited, in the boards' words. */
function tagsOf(album: Album, track: Track, index: number): TrackTags {
	const number = (value: number | undefined): string[] =>
		value === undefined ? [] : [String(value)];
	return {
		id: track.id,
		file: `${String(index + 1).padStart(2, '0')}-${track.title.toLowerCase().replaceAll(' ', '-')}.flac`,
		fields: {
			title: [track.title],
			album: [album.title],
			artists: track.artists,
			...(album.compilation ? { compilation: ['1'] } : { albumArtists: [album.artist] }),
			track: number(track.trackNo),
			trackTotal: number(album.trackCount),
			disc: number(track.discNo),
			discTotal: ['1'],
			date: [`${String(album.year)}-03-12`],
			genres: track.genres
		},
		tags: []
	};
}

/** The tags no field covers. The last few are read-only, each for its own reason. */
function otherTags({ id, replayGain }: Track): RawTag[] {
	return [
		{ key: 'ENCODED-BY', values: ['[as tagged]'] },
		{ key: 'MOOD', values: ['Slow', 'Tidal'] },
		{ key: 'MUSICBRAINZ_TRACKID', values: [`fixture-${id}`] },
		...(replayGain?.trackGain === undefined
			? []
			: [
					{
						key: 'REPLAYGAIN_TRACK_GAIN',
						values: [`${String(replayGain.trackGain)} dB`],
						readOnly: true
					}
				]),
		{ key: 'ACOUSTID_FINGERPRINT', values: ['AQADtMmSJMmS'], readOnly: true },
		{ key: 'NOTES', values: [`${'A value too long to show whole. '.repeat(8)}…`], readOnly: true }
	];
}

/** Credits and such for the board's own track, and timestamped lyrics for the one after it. */
const boardTrackTags: Record<string, Partial<TrackTags>> = {
	'glasshouse-66-2': {
		fields: {
			artists: ['Low Tide Choir', 'Ines Duarte'],
			composers: ['Mara Quill'],
			label: ['Northlight'],
			catalogNumber: ['NL-014'],
			originalDate: ['2019'],
			language: ['eng'],
			bpm: ['92'],
			key: ['D min'],
			sortArtist: ['Low Tide Choir'],
			lyrics: ['[Your lyrics stay as they are in the file.]']
		}
	},
	'glasshouse-66-3': {
		fields: { lyrics: ['[00:01.00] Glass over water\n[00:04.20] Light in the seams'] },
		readOnly: ['lyrics']
	}
};

function allTagsOf(album: Album): AlbumTags {
	return {
		etag: album.etag,
		tracks: album.tracks.map((track, index) => {
			const base = tagsOf(album, track, index);
			const extra = boardTrackTags[track.id] ?? {};
			return {
				...base,
				...extra,
				fields: { ...base.fields, ...extra.fields },
				tags: otherTags(track)
			};
		})
	};
}

/** Does to one file's tags what `POST /albums/{id}/edits` would. */
function applied(track: TrackTags, edit: TagEdit): TrackTags {
	const fields = { ...track.fields, ...edit.set };
	for (const field of edit.clear ?? []) {
		fields[field] = [];
	}
	const changes = new Map(edit.tags?.map(({ key, values }) => [key, values]));
	const tags = track.tags
		.map((tag) => ({ ...tag, values: changes.get(tag.key) ?? tag.values }))
		.filter(({ values }) => values.length > 0);
	return { ...track, fields, tags };
}

/** Lays an edit of every track and of one track over each other, as the server does. */
function layered(all: TagEdit | undefined, own: TagEdit | undefined): TagEdit {
	const named = (edit: TagEdit | undefined): TagField[] => [
		...(Object.keys(edit?.set ?? {}) as TagField[]),
		...(edit?.clear ?? [])
	];
	const mine = named(own);
	const set = Object.entries(all?.set ?? {}).filter(([field]) => !mine.includes(field as TagField));
	return {
		set: { ...Object.fromEntries(set), ...own?.set },
		clear: [...(all?.clear ?? []).filter((field) => !mine.includes(field)), ...(own?.clear ?? [])],
		tags: own?.tags ?? []
	};
}

function first(track: TrackTags, field: TagField): string | undefined {
	return track.fields[field]?.at(0);
}

/** A number as the library reads one. */
function numbered(track: TrackTags, field: TagField): { value?: number } {
	const value = first(track, field);
	return value === undefined ? {} : { value: Number(value) };
}

/** The library's view of tags just written. */
function reindexed(album: Album, tags: AlbumTags, touched: readonly string[]): Album {
	const held = new Map(album.tracks.map((track) => [track.id, track]));
	const tracks = tags.tracks.flatMap((tagged) => {
		const was = held.get(tagged.id);
		if (was === undefined) {
			return [];
		}
		const rest = { ...was };
		delete rest.trackNo;
		delete rest.discNo;
		delete rest.discSubtitle;
		delete rest.year;
		const trackNo = numbered(tagged, 'track').value;
		const discNo = numbered(tagged, 'disc').value;
		const discSubtitle = first(tagged, 'discSubtitle');
		const year = Number(first(tagged, 'date')?.slice(0, 4) ?? 0);
		return [
			{
				...rest,
				title: first(tagged, 'title') ?? was.title,
				genres: tagged.fields.genres ?? [],
				...(trackNo === undefined ? {} : { trackNo }),
				...(discNo === undefined ? {} : { discNo }),
				...(discSubtitle === undefined ? {} : { discSubtitle }),
				...(year === 0 ? {} : { year }),
				etag: touched.includes(tagged.id) ? nextEtag(was.etag) : was.etag
			}
		];
	});
	const ordered = tracks.toSorted(
		(a, b) => (a.discNo ?? 1) - (b.discNo ?? 1) || (a.trackNo ?? Infinity) - (b.trackNo ?? Infinity)
	);
	const [{ year } = album] = ordered;
	const rest = { ...album };
	delete rest.year;
	return {
		...rest,
		...(year === undefined ? {} : { year }),
		tracks: ordered,
		discs: discsOf(ordered),
		genres: [...new Set(ordered.flatMap(({ genres }) => genres))],
		etag: nextEtag(album.etag)
	};
}

/** Each state the Last.fm boards drew. */
export type LastfmFixture = 'unset' | 'none' | 'connected' | 'waiting' | 'reconnect';

const lastfmFixtures: readonly LastfmFixture[] = [
	'unset',
	'none',
	'connected',
	'waiting',
	'reconnect'
];

/** Reads a Last.fm fixture from the URL: anything else is a server with no key yet set. */
export function lastfmFixture(value: string | null): LastfmFixture {
	return lastfmFixtures.find((name) => name === value) ?? 'none';
}

/** The plays board A counts. */
const boardPlays = 3412;

/** What the states board shows for each state, as of `now`. */
export function lastfmStatusOf(fixture: LastfmFixture, now = Date.now()): LastfmStatus {
	const off = { scrobble: true, nowPlaying: true, loves: true, state: 'ok', waiting: 0 } as const;
	switch (fixture) {
		case 'unset':
			return { ...off, configured: false, connected: false };
		case 'none':
			return { ...off, configured: true, connected: false };
		case 'connected':
			return {
				...off,
				configured: true,
				connected: true,
				name: 'simonhull',
				lastSentAt: new Date(now - 2 * 60_000).toISOString()
			};
		case 'waiting':
			return {
				...off,
				configured: true,
				connected: true,
				name: 'simonhull',
				state: 'waiting',
				waiting: 3,
				lastSentAt: new Date(now - 40 * 60_000).toISOString(),
				retryAt: new Date(now + 4 * 60_000).toISOString()
			};
		case 'reconnect':
			return {
				...off,
				configured: true,
				connected: true,
				name: 'simonhull',
				state: 'reconnect',
				waiting: 12,
				lastSentAt: new Date(now - 3 * 86_400_000).toISOString()
			};
	}
}

/**
 * A server on its first run: `needed` opened from its network, `code` from outside it, `done` once
 * an account exists. Each starts signed out.
 */
export type SetupFixture = 'needed' | 'code' | 'done';

/** Reads a setup fixture from the URL: anything else is a server set up long ago, signed in. */
export function setupFixture(value: string | null): SetupFixture | undefined {
	return value === 'needed' || value === 'code' || value === 'done' ? value : undefined;
}

/** The code the fixtures' server logged, in the shape a real one has. */
export const fixtureSetupCode = 'k7m2-q9xd-4tbn-r8ve';

/** Wrong codes the server takes before it makes the page wait, as its login limiter does. */
const freeWrongCodes = 6;

/** How the server compares a code: case, dashes and spaces don't count. */
function plainCode(code: string): string {
	return code.toLowerCase().replaceAll(/[\s-]/g, '');
}

/** The fixtures' one password: the You page's current password, and what's typed to sign in. */
export const fixturePassword = 'correct horse';

/** The reset code the fixtures' server logged. */
export const fixtureResetCode = 'q2rv-8hx4-m0tn-j7cd';

/** The People board's accounts, as minutes since each was last seen. */
const boardAccounts: readonly [
	name: string,
	role: 'admin' | 'user',
	minutesAgo: number | null,
	sessions: number
][] = [
	['anna', 'user', 125, 2],
	['dad', 'user', null, 0],
	['marcus', 'user', 26 * 60, 1]
];

/** Each way an edit can go wrong, for the states the boards never drew. */
export type FixtureEditFault = 'busy' | 'stale' | 'drift' | 'refused' | 'partial';

export interface FixtureOptions {
	/** Delay every response by this many milliseconds, as a real server would. */
	latencyMs?: number;
	/** The state the Health page opens on. */
	health?: HealthFixture;
	role?: Me['role'];
	/** Fail the first edit this way. A drift never clears: only a scan would help. */
	editFault?: FixtureEditFault | undefined;
	/** How Last.fm stands. */
	lastfm?: LastfmFixture;
	/** A first run, signed out. Absent means a server long set up, signed in. */
	setup?: SetupFixture | undefined;
	/** The preferences the person has already saved. */
	prefs?: Partial<Prefs>;
}

/** Builds a {@link LibrarySource} from the boards' fake library. */
/** Why a fixture song came up: its marks, and nothing learned. */
function neutralWhy(rating: number, hearted: boolean): ShuffleWhy {
	return {
		stars: rating,
		rating: 1,
		hearted,
		heart: 1,
		played: false,
		sinceDays: 0,
		recency: 1.2,
		memory: true,
		earlySkips: 0,
		skipMemory: 1,
		starts: 0,
		skips: 0,
		skipRate: 1,
		listens: 0,
		heard: 0,
		listen: 1,
		flow: 1
	};
}

export function createFixtureSource(options: FixtureOptions = {}): LibrarySource {
	const { latencyMs = 0, health = 'idle', role = 'admin' } = options;
	const signedIn: Me = { ...user, role, canEditTags: role === 'admin' };
	const scans = createHealthFixture(health, role);
	let fault = options.editFault;
	// Each source has its own: an edit is for this tab, until a reload.
	const held = new Map(albums);
	const heldTags = new Map<string, AlbumTags>();
	// So are the marks, like the server's are each person's.
	const hearts = new Set(boardHearts);
	const stars = new Map(boardStars);
	// And the shuffle rules: runs by ID, and the songs left out.
	const runs = new Map<string, string[]>();
	const excluded = new Set<string>();
	let runsMade = 0;
	/** Each shuffle's whole order, worked out at the start: the fixtures have no listening to learn from. */
	const shuffles = new Map<string, { mode: ShuffleMode; picks: ShufflePick[] }>();

	/** The songs a scope holds, with their albums. */
	function inScope(scope: ShuffleScope): { album: Album; track: Track }[] {
		const all = [...held.values()].flatMap((album) =>
			album.tracks.map((track) => ({ album, track }))
		);
		switch (scope.kind) {
			case 'album':
				return all.filter(({ album }) => album.id === scope.id);
			case 'artist':
				return all.filter(({ album }) => album.artists.some(({ id }) => id === scope.id));
			case 'tracks':
				return all.filter(({ track }) => scope.tracks.includes(track.id));
			case 'hearted':
				return all.filter(({ track }) => hearts.has(`tracks/${track.id}`));
			case 'rated':
				return all.filter(({ track }) => (stars.get(track.id) ?? 0) >= 4);
			case 'library':
				return all;
		}
	}

	/** A page of a fixture shuffle, from `from`. */
	function shufflePage(id: string, from: number): ShufflePage {
		const shuffle = shuffles.get(id);
		if (shuffle === undefined) {
			throw new ServerError(404);
		}
		const picks = shuffle.picks.slice(from, from + 50);
		const to = from + picks.length;
		const done = shuffle.mode === 'finite' && to >= shuffle.picks.length;
		return { id, mode: shuffle.mode, from, to, songs: shuffle.picks.length, done, picks };
	}
	let lastfm = lastfmStatusOf(options.lastfm ?? 'none');
	let settings = settingsFixture(lastfm.configured);
	const prefs = new Map<PrefKey, unknown>(
		Object.entries(options.prefs ?? {}) as [PrefKey, unknown][]
	);
	let hasAccounts = options.setup === undefined || options.setup === 'done';
	let signedOut = options.setup !== undefined;
	let wrongCodes = 0;
	let resetAsked = false;

	function whoIsSignedIn(): Me {
		if (signedOut) {
			throw new UnauthorizedError();
		}
		return signedIn;
	}

	/** Checks a first account as the server would, and makes it. */
	function makeFirst(name: string, password: string, code: string | undefined): Me {
		if (hasAccounts) {
			throw new SetupError({ kind: 'done' });
		}
		if (options.setup === 'code') {
			if (wrongCodes >= freeWrongCodes) {
				throw new SetupError({ kind: 'throttled', retryAfterSeconds: 60 });
			}
			if (code === undefined || plainCode(code) !== plainCode(fixtureSetupCode)) {
				wrongCodes += code === undefined ? 0 : 1;
				throw new SetupError({ kind: 'code' });
			}
		}
		if (name.length === 0 || name.length > 64 || name.trim() !== name) {
			throw new SetupError({ kind: 'name' });
		}
		if ([...password].length < 8 || password.length > 1024) {
			throw new SetupError({ kind: 'password' });
		}
		hasAccounts = true;
		signedOut = false;
		return signedIn;
	}

	/** A track as the marks have it: a stale rating goes, as the server leaves one out. */
	function markedTrack<T extends Track | LibraryTrack>(track: T): T {
		const fresh = { ...track, hearted: hearts.has(`tracks/${track.id}`) };
		delete fresh.stars;
		const rated = stars.get(track.id);
		return rated === undefined ? fresh : { ...fresh, stars: rated };
	}

	function marked(album: Album): Album {
		return {
			...album,
			hearted: hearts.has(`albums/${album.id}`),
			tracks: album.tracks.map(markedTrack)
		};
	}

	function shuffleMarkOf(trackId: string): ShuffleMark | undefined {
		const run = [...runs].find(([, tracks]) => tracks.includes(trackId));
		const mark: ShuffleMark = {
			...(run === undefined
				? {}
				: { run: run[0], position: run[1].indexOf(trackId) + 1, length: run[1].length }),
			...(excluded.has(trackId) ? { excluded: true } : {})
		};
		return Object.keys(mark).length === 0 ? undefined : mark;
	}

	function markedIndex(fields: readonly IndexField[] = []): LibraryIndex {
		const asked = new Set(fields);
		const extra = (album: LibraryAlbum, track: LibraryTrack): LibraryTrack => {
			const full = held.get(album.id) ?? listOnly.get(album.id);
			const source = full?.tracks.find(({ id }) => id === track.id);
			const shown =
				full === undefined || source === undefined || asked.size === 0
					? track
					: { ...track, ...fieldsOf(full, source, asked) };
			const mark = asked.has('shuffle') ? shuffleMarkOf(track.id) : undefined;
			return mark === undefined ? shown : { ...shown, shuffle: mark };
		};
		return {
			artists: indexFixture().artists.map((artist) => ({
				...artist,
				hearted: artist.id !== undefined && hearts.has(`artists/${artist.id}`),
				albums: artist.albums.map((album) => ({
					...album,
					hearted: hearts.has(`albums/${album.id}`),
					tracks: album.tracks.map((track) => markedTrack(extra(album, track)))
				}))
			}))
		};
	}

	function tagsIn(album: Album): AlbumTags {
		const tags = heldTags.get(album.id) ?? allTagsOf(album);
		heldTags.set(album.id, tags);
		return tags;
	}

	/** Writes `edit` to the album's tracks, stopping before `upTo` of them when told. */
	function write(album: Album, edit: AlbumEdit, upTo = Infinity): Album {
		const tags = tagsIn(album);
		const own = new Map(edit.tracks?.map(({ id, ...change }) => [id, change]));
		const named = tags.tracks.filter(({ id }) => edit.all !== undefined || own.has(id));
		const touched = named.slice(0, upTo).map(({ id }) => id);
		const tracks = tags.tracks.map((track) =>
			touched.includes(track.id) ? applied(track, layered(edit.all, own.get(track.id))) : track
		);
		const after = reindexed(album, { ...tags, tracks }, touched);
		heldTags.set(album.id, { etag: after.etag, tracks });
		held.set(album.id, after);
		return marked(after);
	}

	/** Tags are for administrators, as on the server. */
	/** Settings are an administrator's, as on the server. */
	function adminOnly(): void {
		if (role !== 'admin') {
			throw new ServerError(403);
		}
	}

	function refuseAnyoneElse(): void {
		if (!signedIn.canEditTags) {
			throw new EditError({ kind: 'forbidden' });
		}
	}

	/** Stops an edit the way {@link FixtureOptions.editFault} says, once. */
	function stumble(album: Album, edit: AlbumEdit): void {
		const planned = fault;
		fault = planned === 'drift' ? planned : undefined;
		const ids = album.tracks.map(({ id }) => id);
		switch (planned) {
			case 'busy':
				throw new EditError({ kind: 'busy', retryAfterSeconds: 30 });
			case 'stale':
				write(album, { tracks: [{ id: ids.at(-1) ?? '', set: { title: ['Moved by a scan'] } }] });
				throw new EditError({ kind: 'stale' });
			case 'drift':
				throw new EditError({ kind: 'stale' });
			case 'refused':
				throw new EditError({
					kind: 'refused',
					detail: "year 1066: the file can't take this edit",
					changed: [],
					remaining: ids
				});
			case 'partial': {
				const reached = Math.ceil((edit.tracks?.length ?? ids.length) / 2);
				const named = edit.all === undefined ? (edit.tracks?.map(({ id }) => id) ?? []) : ids;
				write(album, edit, reached);
				throw new EditError({
					kind: 'failed',
					detail: 'internal error',
					changed: named.slice(0, reached),
					remaining: named.slice(reached)
				});
			}
			case undefined:
		}
	}

	async function respond<T>(answer: () => T): Promise<T> {
		if (latencyMs > 0) {
			await new Promise((resolve) => setTimeout(resolve, latencyMs));
		}
		return answer();
	}

	return {
		me: () => respond(whoIsSignedIn),
		sessionUser: () => respond(whoIsSignedIn),
		reportPlay: () => respond(() => undefined),
		reportListen: () => respond(() => undefined),
		setHeart: (kind: MarkKind, id: string, on: boolean) =>
			respond(() => {
				const key = `${kind}/${id}`;
				if (on) {
					hearts.add(key);
				} else {
					hearts.delete(key);
				}
			}),
		setStars: (trackId, rating) =>
			respond(() => {
				if (rating === null) {
					stars.delete(trackId);
				} else {
					stars.set(trackId, rating);
				}
			}),
		login: (name) =>
			respond(() => {
				if (name === '' || !hasAccounts) {
					throw new LoginError({ kind: 'credentials' });
				}
				signedOut = false;
				return signedIn;
			}),
		setupStatus: () =>
			respond((): SetupStatus => ({
				needed: !hasAccounts,
				codeRequired: !hasAccounts && options.setup === 'code'
			})),
		setup: (name, password, code) => respond(() => makeFirst(name, password, code)),
		requestReset: () =>
			respond(() => {
				resetAsked = true;
			}),
		confirmReset: (_name, code, password) =>
			respond(() => {
				if (!resetAsked || normalizeCode(code) !== normalizeCode(fixtureResetCode)) {
					throw new ResetError({ kind: 'code' });
				}
				if (password.length > 1024) {
					throw new ResetError({ kind: 'password' });
				}
				resetAsked = false;
				signedOut = false;
				return signedIn;
			}),
		changePassword: (current, password) =>
			respond(() => {
				whoIsSignedIn();
				if (current !== fixturePassword) {
					throw new PasswordError({ kind: 'current' });
				}
				if ([...password].length < 8 || password.length > 1024) {
					throw new PasswordError({ kind: 'password' });
				}
			}),
		accounts: () =>
			respond(() => {
				adminOnly();
				const now = Date.now();
				return [
					{ user: signedIn, lastSeen: new Date(now).toISOString(), sessions: 3 },
					...boardAccounts.map(([name, accountRole, minutesAgo, sessions], index) => ({
						user: { id: index + 2, name, role: accountRole, createdAt: '2026-02-01T00:00:00Z' },
						...(minutesAgo === null
							? {}
							: { lastSeen: new Date(now - minutesAgo * 60_000).toISOString() }),
						sessions
					}))
				].toSorted((a, b) => a.user.name.localeCompare(b.user.name));
			}),
		setPassword: (userId, password) =>
			respond(() => {
				adminOnly();
				if (userId === signedIn.id) {
					throw new ServerError(403);
				}
				if ([...password].length < 8 || password.length > 1024) {
					throw new PasswordError({ kind: 'password' });
				}
			}),
		logout: () => respond(() => undefined),
		listening: () => respond(() => ({ plays: boardPlays, hearts: hearts.size, rated: stars.size })),
		lastfm: () => respond(() => lastfm),
		connectLastfm: () =>
			respond(() => {
				if (!lastfm.configured) {
					throw new ServerError(409);
				}
				// There's no last.fm to go to: the fixture says yes at once.
				lastfm = lastfmStatusOf('connected');
				return 'fixture:connected';
			}),
		setLastfm: (patch) =>
			respond(() => {
				if (!lastfm.connected) {
					throw new ServerError(404);
				}
				lastfm = { ...lastfm, ...patch };
				return lastfm;
			}),
		disconnectLastfm: () =>
			respond(() => {
				lastfm = lastfmStatusOf(lastfm.configured ? 'none' : 'unset');
			}),
		nowPlaying: () => respond(() => undefined),
		facts: () => respond(() => ({ ...counts, ...scans.facts() })),
		libraryIndex: (fields) => respond(() => markedIndex(fields)),
		libraryIndexOf: (albums, fields) => respond(() => partOf(markedIndex(fields), albums)),
		artistSongs: (id, fields) =>
			respond(() => {
				const link = artistLinks.find((artist) => artist.id === id);
				if (link === undefined) {
					throw new ArtistNotFoundError(id);
				}
				return artistIndexOf(markedIndex(fields), link);
			}),
		pref: <K extends PrefKey>(key: K) => respond(() => (prefs.get(key) ?? null) as Prefs[K] | null),
		setPref: (key, value) =>
			respond(() => {
				prefs.set(key, structuredClone(value));
			}),
		ready: () => respond(() => true),
		albumsPage: (sort, _cursor, limit) =>
			respond(() => ({ albums: (sort === 'added' ? newestFirst : byName).slice(0, limit) })),
		album: (id) =>
			respond(() => {
				const album = held.get(id) ?? listOnly.get(id);
				if (album === undefined) {
					throw new AlbumNotFoundError(id);
				}
				return marked(album);
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
		albumTags: (id) =>
			respond(() => {
				refuseAnyoneElse();
				const album = held.get(id);
				if (album === undefined) {
					throw new EditError({ kind: 'gone' });
				}
				return tagsIn(album);
			}),
		editAlbum: (id, etag, edit) =>
			respond(() => {
				refuseAnyoneElse();
				const album = held.get(id);
				if (album === undefined) {
					throw new EditError({ kind: 'gone' });
				}
				if (album.etag !== etag) {
					throw new EditError({ kind: 'stale' });
				}
				stumble(album, edit);
				return write(album, edit);
			}),
		// The fixtures never move a song: each album takes its own songs' part of the edit.
		editTracks: (edit) =>
			respond(() => {
				refuseAnyoneElse();
				const albums = edit.albums.map(({ id, etag }) => {
					const album = held.get(id);
					if (album === undefined) {
						throw new EditError({ kind: 'gone' });
					}
					if (album.etag !== etag) {
						throw new EditError({ kind: 'stale', album: id });
					}
					return album;
				});
				const placed = edit.tracks.map(({ id }) => {
					const album = albums.find(({ tracks }) => tracks.some((track) => track.id === id));
					if (album === undefined) {
						throw new EditError({ kind: 'refused', detail: '', changed: [], remaining: [id] });
					}
					return { id, albumId: album.id };
				});
				for (const album of albums) {
					const own = edit.tracks.filter(({ id }) => album.tracks.some((track) => track.id === id));
					write(album, {
						tracks: own.map((track) => ({ ...track, ...layered(edit.all, track) }))
					});
				}
				return { tracks: placed };
			}),
		shuffleRules: (trackIds) =>
			respond(() => ({
				runs: [...runs]
					.filter(([, tracks]) => tracks.some((id) => trackIds.includes(id)))
					.map(([id, tracks]) => ({ id, tracks: [...tracks] })),
				excluded: trackIds.filter((id) => excluded.has(id))
			})),
		startShuffle: (start) =>
			respond(() => {
				const songs = inScope(start.scope).filter(({ track }) => !excluded.has(track.id));
				const order = songs
					.map((song) => ({ song, key: Math.random() }))
					.sort((a, b) => a.key - b.key)
					.map(({ song }) => song);
				const lead = order.findIndex(({ track }) => track.id === start.first);
				if (lead > 0) {
					order.unshift(...order.splice(lead, 1));
				}
				const picks = order.map(({ album, track }) => ({
					songs: [
						{
							id: track.id,
							title: track.title,
							artists: track.artists,
							albumId: album.id,
							album: album.title,
							durationMs: track.durationMs,
							artwork: album.artwork
						}
					],
					why: neutralWhy(stars.get(track.id) ?? 0, hearts.has(`tracks/${track.id}`))
				}));
				const id = `shuffle-${String(shuffles.size + 1)}`;
				shuffles.set(id, { mode: start.mode, picks });
				return shufflePage(id, 0);
			}),
		nextShuffle: (id, after) => respond(() => shufflePage(id, after)),
		setRun: (trackIds) =>
			respond(() => {
				const first = [...runs].find(([, tracks]) => tracks.includes(trackIds[0] ?? ''));
				for (const [id, tracks] of runs) {
					const left = tracks.filter((track) => !trackIds.includes(track));
					if (left.length < 2) {
						runs.delete(id);
					} else {
						runs.set(id, left);
					}
				}
				runsMade += 1;
				const id =
					first !== undefined && !runs.has(first[0]) ? first[0] : `run-${String(runsMade)}`;
				runs.set(id, [...trackIds]);
				return { id, tracks: [...trackIds] };
			}),
		deleteRun: (id) =>
			respond(() => {
				if (!runs.delete(id)) {
					throw new ServerError(404);
				}
			}),
		setExcluded: (trackIds, on) =>
			respond(() => {
				for (const id of trackIds) {
					if (on) {
						excluded.add(id);
					} else {
						excluded.delete(id);
					}
				}
			}),
		trackAlbum: (trackId) =>
			respond(() => {
				const album = [...held.values()].find(({ tracks }) =>
					tracks.some(({ id }) => id === trackId)
				);
				if (album === undefined) {
					throw new EditError({ kind: 'gone' });
				}
				return album.id;
			}),
		// Never fetched: the fixtures play on a silent clock.
		streamUrl: (trackId) => `fixture:${trackId}`,
		startScan: (mode) => respond(() => scans.start(mode)),
		issues: () => respond(() => scans.issues()),
		settings: () =>
			respond(() => {
				adminOnly();
				return structuredClone(settings);
			}),
		folders: (path) =>
			respond(() => {
				adminOnly();
				const listing = foldersFixture(path, settings.settings['library.roots'].value);
				if (listing === null) {
					throw new ServerError(404);
				}
				return listing;
			}),
		changeSettings: (change) =>
			respond(() => {
				adminOnly();
				settings = changedSettings(settings, change);
				const key = settings.settings['lastfm.api_key'].value;
				const configured = key !== '' && settings.settings['lastfm.shared_secret'].set;
				if (configured !== lastfm.configured) {
					lastfm = lastfmStatusOf(configured ? 'none' : 'unset');
				}
				return structuredClone(settings);
			})
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

	function statusAt(read: number): ScanStatus {
		return now() - startedAt >= scanMs
			? { state: 'idle', read: 0, total: 0 }
			: {
					state: 'running',
					mode: 'quick',
					read,
					total: scanLibrary * 8,
					startedAt: new Date(startedAt).toISOString()
				};
	}

	async function respond<T>(answer: () => T): Promise<T> {
		await base.ready();
		return answer();
	}

	/** The real thing has the library locked for the whole of a scan. */
	const busy = (): Promise<never> =>
		respond(() => {
			throw new EditError({ kind: 'busy', retryAfterSeconds: 30 });
		});

	return {
		...base,
		editAlbum: busy,
		facts: () =>
			respond(() => {
				const albums = found();
				return {
					albums,
					tracks: albums * 8,
					durationMs: albums * 8 * 240_000,
					roots: 1,
					issues: 0,
					issueCounts: { unreadable: 0, corrupted: 0, unsupported: 0, warnings: 0 },
					scan: statusAt(albums * 8)
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
