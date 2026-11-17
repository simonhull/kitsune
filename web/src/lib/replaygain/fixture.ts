/**
 * The ReplayGain routes on the boards' library, worked out in the page so e2e needs no server. The
 * owner's albums with no ReplayGain from plan 070's boards are here as counts, beside the fixture
 * library's own measured album (Tin Lantern), whose songs really change: a run writes them, and
 * their gain reads as tagged; Undo makes them measured again. A run goes album by album on a clock,
 * telling its listeners as the server's events would, and leaves the board's few songs as they were.
 */

import type { Album } from '#lib/library/source.ts';
import {
	ReplayGainError,
	type GainAlbum,
	type GainLeft,
	type GainLeftReason,
	type GainRun,
	type GainRunAlbum,
	type GainRunDetail,
	type ReplayGainEvent,
	type ReplayGainSource,
	type ReplayGainStatus
} from './types.ts';

/** Where the fixtures start. */
export type GainFixture =
	/** Songs to write, as board A. */
	| 'ready'
	/** The same, with the loudness pass still measuring 212 more. */
	| 'measuring'
	/** Every song carries its own ReplayGain, and there are no runs: the card isn't drawn. */
	| 'none'
	/** Every song was written by a run that can still be undone: the quiet card. */
	| 'written';

/** The `?gain=` a fixtures page opens with. */
export function gainFixture(value: string | null): GainFixture {
	return value === 'measuring' || value === 'none' || value === 'written' ? value : 'ready';
}

/** A song the board's run leaves as it was: its number on the album, title, path and why. */
type Leaves = readonly (readonly [number, string, string, GainLeftReason])[];

/** An album of the boards with no ReplayGain: only its count is known, and what a run leaves. */
interface BoardAlbum {
	album: GainAlbum;
	leaves: Leaves;
}

const boardAlbum = (
	id: string,
	title: string,
	artist: string,
	year: number,
	format: string,
	songs: number,
	leaves: Leaves = []
): BoardAlbum => ({ album: { id, title, artist, year, formats: [format], songs }, leaves });

/** The board's albums, in the order a run takes them. */
const boardAlbums: readonly BoardAlbum[] = [
	boardAlbum('gain-sky-earth', 'The Sky, The Earth & All Between', 'Architects', 2025, 'FLAC', 12),
	boardAlbum('gain-calling-all-dawns', 'Calling All Dawns', 'Christopher Tin', 2009, 'MP3', 12),
	boardAlbum(
		'gain-frieren',
		'Frieren: Beyond Journey’s End (Original Soundtrack)',
		'Evan Call',
		2024,
		'FLAC',
		70
	),
	boardAlbum(
		'gain-rayman-2',
		'Rayman 2 – The Complete Soundtrack',
		'Compilations',
		1999,
		'FLAC',
		167,
		[
			[
				150,
				'(Prototype) Opening the Sanctuary',
				'Compilations/Rayman 2/150 - (Prototype) Opening the Sanctuary.flac',
				'unwritable'
			]
		]
	),
	boardAlbum(
		'gain-rayman-3',
		'Rayman 3 – The Complete Soundtrack',
		'Compilations',
		2003,
		'FLAC',
		292,
		[
			[41, 'Hoodlum Hideout', 'Compilations/Rayman 3/041 - Hoodlum Hideout.flac', 'changed'],
			[42, 'The Swamps', 'Compilations/Rayman 3/042 - The Swamps.flac', 'changed']
		]
	),
	boardAlbum('gain-deltarune', 'DELTARUNE Chapter 1 OST', 'Toby Fox', 2018, 'FLAC', 40, [
		[
			12,
			'Field of Hopes and Dreams',
			'Toby Fox/DELTARUNE/12 Field of Hopes and Dreams.flac',
			'trashed'
		]
	]),
	boardAlbum('gain-persona5', 'PERSONA5 ORIGINAL SOUNDTRACK', 'Shoji Meguro', 2016, 'FLAC', 110),
	boardAlbum('gain-undertale', 'UNDERTALE Soundtrack', 'Toby Fox', 2015, 'FLAC', 101),
	boardAlbum('gain-gates', 'Gates of Ascension', 'John E Carter', 2019, 'MP3', 14, [
		[
			3,
			'Distant Lands',
			'John Carter/Gates of Ascension/JohnECarter-GatesofAscension-03-DistantLands.mp3',
			'changed'
		]
	])
];

/** Songs still waiting for the loudness pass, under `measuring`. */
const measuringWaiting = 212;

/** An album a run takes: what it writes and what it leaves. */
interface Planned {
	album: GainAlbum;
	/** The library's own songs it writes, by ID; none for a board album. */
	tracks: string[];
	/** How many it writes. */
	writes: number;
	left: GainLeft[];
}

/** A run going in the page, or one that went. */
interface Going {
	run: GainRun;
	albums: Planned[];
	/** Albums finished. */
	at: number;
	timer: ReturnType<typeof setTimeout> | undefined;
	/** The run this one undoes. */
	undoing?: Going;
}

export interface GainFixtureOptions {
	start: GainFixture;
	/** The library's albums as they are now: their measured songs are what a run writes. */
	albums: () => readonly Album[];
	/** A run wrote these songs of a library album (`true`), or an undo took it out (`false`). */
	wrote: (albumId: string, trackIds: readonly string[], written: boolean) => void;
	/** May write files: an administrator, with tag writes on. */
	writes: () => boolean;
	/** How long each album of a run takes. */
	albumMs?: number;
	respond: <T>(answer: () => T) => Promise<T>;
}

/** The ReplayGain routes on the fixture library. */
export function createReplayGainFixture(options: GainFixtureOptions): ReplayGainSource {
	const albumMs = options.albumMs ?? 450;
	const runs: Going[] = [];
	const listeners = new Set<(event: ReplayGainEvent) => void>();
	const changes = new Set<(albums: string[]) => void>();
	/** The board albums' songs not yet written, by album. */
	const pending = new Map(
		boardAlbums.map(({ album }) => [
			album.id,
			options.start === 'ready' || options.start === 'measuring' ? album.songs : 0
		])
	);
	let made = 0;

	/** The library's own albums with measured songs, as a run would find them now. */
	function libraryAlbums(): Planned[] {
		return options.albums().flatMap((album): Planned[] => {
			const measured = album.tracks.filter((track) => track.replayGain?.source === 'measured');
			if (measured.length === 0) {
				return [];
			}
			const formats = [...new Set(measured.map((track) => track.format.toUpperCase()))];
			return [
				{
					album: {
						id: album.id,
						title: album.title,
						artist: album.artist,
						...(album.year === undefined ? {} : { year: album.year }),
						...(album.artwork === undefined ? {} : { artwork: album.artwork }),
						formats,
						songs: measured.length
					},
					tracks: measured.map((track) => track.id),
					writes: measured.length,
					left: []
				}
			];
		});
	}

	/** Every album with something to write now, in the order a run takes them. */
	function writable(): Planned[] {
		const board = boardAlbums.flatMap(({ album, leaves }): Planned[] => {
			const songs = pending.get(album.id) ?? 0;
			if (songs === 0) {
				return [];
			}
			// The songs it leaves are the last still waiting: a run that wrote the rest left them.
			const left = leaves.slice(0, songs).map(([no, title, path, reason]): GainLeft => ({
				track: `${album.id}-${String(no)}`,
				title,
				path,
				root: 0,
				reason
			}));
			return [{ album: { ...album, songs }, tracks: [], writes: songs - left.length, left }];
		});
		return [...board, ...libraryAlbums()];
	}

	function status(): ReplayGainStatus {
		const albums = writable();
		const formats = new Map<string, number>();
		for (const { album } of albums) {
			for (const format of album.formats) {
				formats.set(format, (formats.get(format) ?? 0) + album.songs);
			}
		}
		const going = runs.find((each) => each.run.state === 'running');
		return {
			songs: albums.reduce((sum, { album }) => sum + album.songs, 0),
			albums: albums.length,
			formats: [...formats.entries()].sort((a, b) => b[1] - a[1]).map(([format]) => format),
			biggest: albums
				.map(({ album }) => album)
				.sort((a, b) => b.songs - a.songs)
				.slice(0, 4),
			waiting: options.start === 'measuring' ? measuringWaiting : 0,
			unmeasurable: 0,
			...(going === undefined ? {} : { run: structuredClone(going.run) }),
			runs: runs.slice(0, 10).map((each) => structuredClone(each.run))
		};
	}

	function tell(going: Going): void {
		const { run } = going;
		const event: ReplayGainEvent = {
			run: run.id,
			state: run.state,
			done: run.done,
			total: run.total,
			albumsDone: going.at,
			...(run.album === undefined ? {} : { album: run.album })
		};
		for (const heard of listeners) {
			heard(event);
		}
	}

	function finish(going: Going, state: GainRun['state']): void {
		const { run } = going;
		run.state = state;
		run.finishedAt = new Date().toISOString();
		run.albumsDone = going.at;
		delete run.album;
		delete run.stopping;
		run.undoable = run.kind === 'run' && run.written > 0;
		if (going.undoing !== undefined) {
			going.undoing.run.state = 'undone';
			going.undoing.run.undoneBy = run.id;
		}
		tell(going);
	}

	/** Writes one album, or takes it out for an undo. */
	function land(going: Going, planned: Planned): void {
		const undo = going.run.kind === 'undo';
		const { album } = planned;
		if (planned.tracks.length > 0) {
			options.wrote(album.id, planned.tracks, !undo);
			for (const heard of changes) {
				heard([album.id]);
			}
		} else {
			const before = pending.get(album.id) ?? 0;
			pending.set(album.id, undo ? before + planned.writes : before - planned.writes);
		}
		going.run.done += planned.writes + planned.left.length;
		going.run.written += planned.writes;
		going.run.left += planned.left.length;
	}

	function step(going: Going): void {
		const planned = going.albums[going.at];
		if (planned === undefined) {
			finish(going, 'done');
			return;
		}
		land(going, planned);
		going.at += 1;
		const next = going.albums[going.at];
		if (next === undefined) {
			finish(going, 'done');
			return;
		}
		if (going.run.stopping === true) {
			finish(going, 'stopped');
			return;
		}
		going.run.album = next.album.id;
		going.run.albumsDone = going.at;
		tell(going);
		going.timer = setTimeout(() => {
			step(going);
		}, albumMs);
	}

	function newRun(kind: GainRun['kind'], albums: readonly Planned[]): GainRun {
		made += 1;
		return {
			id: `gain-run-${String(made)}`,
			kind,
			state: 'running',
			startedAt: new Date().toISOString(),
			by: 'simon',
			total: albums.reduce((sum, a) => sum + a.writes + a.left.length, 0),
			done: 0,
			written: 0,
			left: 0,
			failed: 0,
			albums: albums.length,
			albumsDone: 0,
			undoable: false
		};
	}

	function begin(going: Going): GainRun {
		runs.unshift(going);
		const head = going.albums[0];
		if (head !== undefined) {
			going.run.album = head.album.id;
		}
		going.timer = setTimeout(() => {
			step(going);
		}, albumMs);
		queueMicrotask(() => {
			tell(going);
		});
		return structuredClone(going.run);
	}

	function canWrite(): void {
		if (!options.writes()) {
			throw new ReplayGainError(409, 'tag writes are off, so nothing on disk changes');
		}
		if (runs.some((going) => going.run.state === 'running')) {
			throw new ReplayGainError(409, 'a run holds the library');
		}
	}

	function goingOf(id: string): Going {
		const going = runs.find((each) => each.run.id === id);
		if (going === undefined) {
			throw new ReplayGainError(404, 'no such run');
		}
		return going;
	}

	function detailOf(going: Going): GainRunDetail {
		const running = going.run.state === 'running';
		const albums = going.albums.map((planned, index): GainRunAlbum => {
			const reached = index < going.at;
			return {
				...structuredClone(planned.album),
				state: reached
					? planned.writes > 0
						? 'written'
						: 'left'
					: running && index === going.at
						? 'writing'
						: 'waiting',
				written: reached ? planned.writes : 0,
				left: reached ? planned.left.length : 0
			};
		});
		const left = going.albums.slice(0, going.at).flatMap((planned) => planned.left);
		return { run: structuredClone(going.run), albums, left: structuredClone(left) };
	}

	// A run that wrote everything, long enough ago to be in the list and still undoable.
	if (options.start === 'written') {
		const albums = boardAlbums.map(({ album }): Planned => ({
			album,
			tracks: [],
			writes: album.songs,
			left: []
		}));
		const run = newRun('run', albums);
		const finishedAt = Date.now() - 3 * 60_000;
		Object.assign(run, {
			state: 'done',
			startedAt: new Date(finishedAt - 98_000).toISOString(),
			finishedAt: new Date(finishedAt).toISOString(),
			done: run.total,
			written: run.total,
			albumsDone: albums.length,
			undoable: true
		});
		runs.push({ run, albums, at: albums.length, timer: undefined });
	}

	return {
		status: () => options.respond(status),

		start: (albums) =>
			options.respond(() => {
				canWrite();
				const named = albums === undefined ? null : new Set(albums);
				const planned = writable().filter(({ album }) => named === null || named.has(album.id));
				if (planned.length === 0) {
					throw new ReplayGainError(422, 'nothing on those albums can be written now');
				}
				return begin({ run: newRun('run', planned), albums: planned, at: 0, timer: undefined });
			}),

		run: (id) => options.respond(() => detailOf(goingOf(id))),

		stop: (id) =>
			options.respond(() => {
				const going = goingOf(id);
				if (going.run.state !== 'running') {
					throw new ReplayGainError(409, 'the run isn’t running');
				}
				going.run.stopping = true;
				return structuredClone(going.run);
			}),

		undo: (id) =>
			options.respond(() => {
				const of = goingOf(id);
				if (!of.run.undoable) {
					throw new ReplayGainError(409, 'nothing of this run can be taken out');
				}
				canWrite();
				const albums = of.albums
					.slice(0, of.at)
					.filter((planned) => planned.writes > 0)
					.map((planned): Planned => ({ ...planned, left: [] }));
				const run = newRun('undo', albums);
				run.undoOf = of.run.id;
				of.run.undoable = false;
				return begin({ run, albums, at: 0, timer: undefined, undoing: of });
			}),

		listen(heard) {
			listeners.add(heard);
			return () => {
				listeners.delete(heard);
			};
		},

		changed(heard) {
			changes.add(heard);
			return () => {
				changes.delete(heard);
			};
		}
	};
}
