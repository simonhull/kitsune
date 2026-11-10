/**
 * Writes a generated music library for the perf suite: FLAC files with real tags and folder covers,
 * shaped like a real collection, made from a few ffmpeg templates in seconds.
 *
 * Every track is a copy of a template with its own Vorbis comment. The first two tracks of the
 * albums the suite plays are pink noise, so a stream moves the bytes a real FLAC does; every other
 * track is silence, which FLAC stores in almost nothing, so 50,000 songs fit in about a gigabyte.
 */
import { execFileSync } from 'node:child_process';
import { existsSync, linkSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

/** A library the suite runs on. */
export interface Shape {
	songs: number;
	/** How many albums get pink noise in their first two tracks: the ones the suite plays. */
	loudAlbums: number;
	/** One artist with many songs, for its page: its own albums, and its guest spots. */
	prolific: { albums: number; perAlbum: number; guests: number };
}

/** The prolific artist: last in the Library's order, so nothing else the suite finds moves. */
export const prolificName = 'Zola Tide Choir';

/** An album the suite can play. */
export interface LoudAlbum {
	artist: string;
	title: string;
}

/** What the generator wrote. */
export interface Manifest {
	/** Which way the library was written: one written another way is written again. */
	form: number;
	songs: number;
	albums: number;
	artists: number;
	loud: LoudAlbum[];
	/** The prolific artist's songs: its own and its guest spots. */
	prolific: { name: string; songs: number };
}

/**
 * The library's form: 2 tags every song but the loud albums' with ReplayGain; 3 adds the prolific
 * artist after everything else, from a generator of its own, so the rest is written as it was.
 */
const libraryForm = 3;

const coverCount = 48;
const silentSeconds = 30;
const loudSeconds = 20;

const words = [
	'amber', 'violet', 'harbor', 'ember', 'quiet', 'neon', 'velvet', 'hollow', 'paper', 'static',
	'winter', 'golden', 'satellite', 'river', 'midnight', 'glass', 'orchid', 'thunder', 'cinder',
	'lantern', 'echo', 'meadow', 'iron', 'silver', 'fever', 'ocean', 'cobalt', 'marble', 'feather',
	'signal', 'garden', 'shadow', 'copper', 'canyon', 'radio', 'saffron', 'polar', 'zephyr',
	'juniper', 'kestrel', 'lunar', 'yellow', 'xenon', 'umber', 'tidal', 'wild', 'drift', 'basalt',
	'nova', 'opal'
]; // prettier-ignore
const genres = ['Rock', 'Jazz', 'Electronic', 'Folk', 'Hip-Hop', 'Classical', 'Ambient', 'Soul'];

/** A small seeded generator (mulberry32), so every run writes the same library. */
function random(seed: number): () => number {
	let state = seed;
	return () => {
		state = (state + 0x6d2b79f5) | 0;
		let t = Math.imul(state ^ (state >>> 15), 1 | state);
		t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};
}

function title(next: () => number, count: number): string {
	return Array.from({ length: count }, () => {
		const word = words[Math.floor(next() * words.length)] ?? 'song';
		return word.charAt(0).toUpperCase() + word.slice(1);
	}).join(' ');
}

function ffmpeg(...args: string[]): void {
	execFileSync('ffmpeg', ['-loglevel', 'error', '-y', ...args], { stdio: 'inherit' });
}

/** A FLAC file split at its metadata: what stays, and the audio frames after it. */
interface Template {
	kept: { type: number; body: Buffer }[];
	frames: Buffer;
}

/** Reads a FLAC file and keeps STREAMINFO and SEEKTABLE, dropping every other block. */
function template(path: string): Template {
	const file = readFileSync(path);
	if (file.toString('latin1', 0, 4) !== 'fLaC') {
		throw new Error(`${path} is not FLAC`);
	}
	const kept: Template['kept'] = [];
	let at = 4;
	for (;;) {
		const header = file.readUInt8(at);
		const length = file.readUIntBE(at + 1, 3);
		const type = header & 0x7f;
		if (type === 0 || type === 3) {
			kept.push({ type, body: file.subarray(at + 4, at + 4 + length) });
		}
		at += 4 + length;
		if ((header & 0x80) !== 0) {
			break;
		}
	}
	return { kept, frames: file.subarray(at) };
}

function blockHeader(type: number, length: number, last: boolean): Buffer {
	const header = Buffer.alloc(4);
	header.writeUInt8((last ? 0x80 : 0) | type, 0);
	header.writeUIntBE(length, 1, 3);
	return header;
}

/** A Vorbis comment block's body, as the FLAC spec lays it out: little-endian lengths. */
function vorbisComment(tags: Record<string, string>): Buffer {
	const vendor = Buffer.from('kitsune perf');
	const parts: Buffer[] = [];
	const u32 = (n: number) => {
		const b = Buffer.alloc(4);
		b.writeUInt32LE(n);
		return b;
	};
	parts.push(u32(vendor.length), vendor, u32(Object.keys(tags).length));
	for (const [key, value] of Object.entries(tags)) {
		const field = Buffer.from(`${key}=${value}`);
		parts.push(u32(field.length), field);
	}
	return Buffer.concat(parts);
}

function flacWith({ kept, frames }: Template, tags: Record<string, string>): Buffer {
	const parts: Buffer[] = [Buffer.from('fLaC')];
	for (const { type, body } of kept) {
		parts.push(blockHeader(type, body.length, false), body);
	}
	const comment = vorbisComment(tags);
	parts.push(blockHeader(4, comment.length, true), comment, frames);
	return Buffer.concat(parts);
}

interface PlannedAlbum {
	title: string;
	year: string;
	genre: string;
	songs: string[];
}

interface PlannedArtist {
	name: string;
	albums: PlannedAlbum[];
}

/** Names and shapes everything, from a fixed seed, before a file is written. */
function plan(songs: number): PlannedArtist[] {
	const next = random(42);
	const artists: PlannedArtist[] = [];
	const names = new Set<string>();
	let left = songs;
	while (left > 0) {
		let name = title(next, 1 + Math.floor(next() * 3));
		while (names.has(name)) {
			name = `${name} ${title(next, 1)}`;
		}
		names.add(name);
		const artist: PlannedArtist = { name, albums: [] };
		artists.push(artist);
		const albumCount = 1 + Math.floor(next() * 11);
		const titles = new Set<string>();
		for (let a = 0; a < albumCount && left > 0; a++) {
			let album = title(next, 1 + Math.floor(next() * 4));
			while (titles.has(album)) {
				album = `${album} ${title(next, 1)}`;
			}
			titles.add(album);
			const year = String(1965 + Math.floor(next() * 60));
			const genre = genres[Math.floor(next() * genres.length)] ?? 'Rock';
			const count = Math.min(6 + Math.floor(next() * 9), left);
			left -= count;
			artist.albums.push({
				title: album,
				year,
				genre,
				songs: Array.from({ length: count }, () => title(next, 1 + Math.floor(next() * 4)))
			});
		}
	}
	return artists;
}

/** Case-blind, as the Library sorts artists. */
function compare(a: string, b: string): number {
	const x = a.toLowerCase();
	const y = b.toLowerCase();
	return x < y ? -1 : x > y ? 1 : 0;
}

/** Folder and file names stay plain: the suite never depends on them. */
function safe(name: string): string {
	return name.replace(/[^A-Za-z0-9 ]/g, '');
}

/**
 * Writes the library for `shape` under `root`, unless a finished one is already there, and returns
 * its manifest. `work` holds the templates.
 */
export function writeLibrary(root: string, work: string, shape: Shape): Manifest {
	const manifestPath = join(root, 'manifest.json');
	if (existsSync(manifestPath)) {
		const written = JSON.parse(readFileSync(manifestPath, 'utf8')) as Partial<Manifest>;
		if (written.form === libraryForm) {
			return written as Manifest;
		}
		rmSync(root, { recursive: true, force: true }); // Written another way: start again.
	}
	mkdirSync(work, { recursive: true });
	const silentPath = join(work, 'silent.flac');
	const loudPath = join(work, 'loud.flac');
	if (!existsSync(silentPath)) {
		ffmpeg('-f', 'lavfi', '-i', 'anullsrc=r=44100:cl=stereo', '-t', String(silentSeconds), '-c:a', 'flac', silentPath); // prettier-ignore
	}
	if (!existsSync(loudPath)) {
		ffmpeg('-f', 'lavfi', '-i', 'anoisesrc=r=44100:a=0.3:c=pink:seed=7', '-ac', '2', '-t', String(loudSeconds), '-sample_fmt', 's16', '-c:a', 'flac', loudPath); // prettier-ignore
	}
	const covers = Array.from({ length: coverCount }, (_, i) => {
		const path = join(work, `cover-${String(i)}.jpg`);
		if (!existsSync(path)) {
			ffmpeg('-f', 'lavfi', '-i', `gradients=s=1200x1200:seed=${String(i + 1)}`, '-frames:v', '1', '-q:v', '3', path); // prettier-ignore
		}
		return path;
	});
	const silent = template(silentPath);
	const loud = template(loudPath);

	const artists = plan(shape.songs);
	// The loud albums are those of the first artists in the Library's order, so the suite finds
	// them at the top of the list.
	const loudArtists = new Set<string>();
	let loudCount = 0;
	for (const artist of [...artists].sort((a, b) => compare(a.name, b.name))) {
		if (loudCount >= shape.loudAlbums) {
			break;
		}
		loudArtists.add(artist.name);
		loudCount += artist.albums.length;
	}
	const manifest: Manifest = {
		form: libraryForm,
		songs: 0,
		albums: 0,
		artists: artists.length,
		loud: [],
		prolific: { name: prolificName, songs: 0 }
	};
	for (const artist of artists) {
		const isLoud = loudArtists.has(artist.name);
		for (const album of artist.albums) {
			if (isLoud) {
				manifest.loud.push({ artist: artist.name, title: album.title });
			}
			manifest.albums += 1;
			const dir = join(root, safe(artist.name), safe(album.title));
			mkdirSync(dir, { recursive: true });
			linkSync(covers[manifest.albums % coverCount] ?? '', join(dir, 'cover.jpg'));
			album.songs.forEach((song, index) => {
				const number = index + 1;
				const tags = {
					TITLE: song,
					ARTIST: artist.name,
					ALBUMARTIST: artist.name,
					ALBUM: album.title,
					TRACKNUMBER: String(number),
					DISCNUMBER: '1',
					DATE: album.year,
					GENRE: album.genre
				};
				// The loud albums have no ReplayGain, so the server measures them, as it would a real
				// library's untagged albums; every other song is tagged, so the suite needn't wait for
				// 50,000 files to be measured before it times anything.
				const file = flacWith(
					isLoud && number <= 2 ? loud : silent,
					isLoud
						? tags
						: { ...tags, REPLAYGAIN_TRACK_GAIN: '-6.00 dB', REPLAYGAIN_TRACK_PEAK: '0.5' }
				);
				writeFileSync(join(dir, `${String(number).padStart(2, '0')} ${safe(song)}.flac`), file);
				manifest.songs += 1;
			});
		}
	}
	writeProlific(root, shape.prolific, manifest, { silent, loud });
	writeFileSync(manifestPath, JSON.stringify(manifest));
	return manifest;
}

/**
 * Writes the prolific artist: its own albums, newest first by year, and guest spots on compilations
 * beside a session band. The newest two albums are real noise, every song of them, as they're what
 * its page lists first with nothing played. Every song has ReplayGain, so nothing waits to be
 * measured. Its names come from a generator of their own, so the rest of the library is unchanged.
 */
function writeProlific(
	root: string,
	{ albums, perAlbum, guests }: Shape['prolific'],
	manifest: Manifest,
	templates: { silent: Template; loud: Template }
): void {
	const next = random(55);
	const gain = { REPLAYGAIN_TRACK_GAIN: '-6.00 dB', REPLAYGAIN_TRACK_PEAK: '0.5' };
	const write = (dir: string, number: number, tags: Record<string, string>, noisy: boolean) => {
		writeFileSync(
			join(dir, `${String(number).padStart(2, '0')} ${safe(tags['TITLE'] ?? '')}.flac`),
			flacWith(noisy ? templates.loud : templates.silent, { ...tags, ...gain })
		);
		manifest.songs += 1;
	};
	for (let a = 0; a < albums; a++) {
		const album = `Tide ${title(next, 1)} ${String(a + 1)}`;
		const year = String(2026 - a);
		const genre = genres[Math.floor(next() * genres.length)] ?? 'Rock';
		const dir = join(root, safe(prolificName), safe(album));
		mkdirSync(dir, { recursive: true });
		manifest.albums += 1;
		for (let s = 1; s <= perAlbum; s++) {
			write(
				dir,
				s,
				{
					TITLE: title(next, 1 + Math.floor(next() * 3)),
					ARTIST: prolificName,
					ALBUMARTIST: prolificName,
					ALBUM: album,
					TRACKNUMBER: String(s),
					DISCNUMBER: '1',
					DATE: year,
					GENRE: genre
				},
				a < 2
			);
		}
	}
	for (let g = 0; g < guests; g++) {
		const album = `Harbor Sessions ${String(g + 1)}`;
		const dir = join(root, 'Various Artists', safe(album));
		mkdirSync(dir, { recursive: true });
		manifest.albums += 1;
		const common = {
			ALBUMARTIST: 'Various Artists',
			ALBUM: album,
			COMPILATION: '1',
			DISCNUMBER: '1',
			DATE: String(2020 - (g % 20)),
			GENRE: 'Folk'
		};
		write(
			dir,
			1,
			{ ...common, TITLE: title(next, 2), ARTIST: prolificName, TRACKNUMBER: '1' },
			false
		);
		write(
			dir,
			2,
			{ ...common, TITLE: title(next, 2), ARTIST: 'Zuma Session Band', TRACKNUMBER: '2' },
			false
		);
	}
	manifest.prolific.songs = albums * perAlbum + guests;
	manifest.artists += 2;
}
