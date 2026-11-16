/**
 * The organize routes on the boards' library, worked out in the page with the client's copy of the
 * template language, so e2e and the perf suite's fixtures need no server. The owner's own albums
 * from plan 068's boards are here, with every kind of song a preview shows: in place, renamed, in a
 * new folder, a folder that changes only in case, two songs that want one path, a file Kitsune
 * doesn't know in the way, and one with no tags for the template. A run moves them album by album
 * on a clock, telling its listeners as the server's events would.
 */

import type { Album } from '#lib/library/source.ts';
import {
	defaultCompilations,
	defaultTemplate,
	extensionOf,
	parse,
	pathOf,
	type Song,
	type Template
} from './template.ts';
import {
	OrganizeError,
	type FileKind,
	type OrganizeAlbum,
	type OrganizeClaimant,
	type OrganizeEvent,
	type OrganizeExample,
	type OrganizeFile,
	type OrganizeFilter,
	type OrganizePreview,
	type OrganizeRun,
	type OrganizeRunDetail,
	type OrganizeSource
} from './types.ts';

/** A song on the fixture disk: who it is, where its file is, and what a template reads of it. */
interface DiskSong {
	id: string;
	albumId: string;
	albumTitle: string;
	artist: string;
	path: string;
	durationMs: number;
	addedAt: string;
	plays: number;
	stars: number;
	song: Song;
}

/** An album of the boards: its songs as `[track, title, file name or a whole path]`. */
interface BoardAlbum {
	id: string;
	title: string;
	artist: string;
	/** The album tag, when it isn't the title shown. */
	tag?: string;
	year: number;
	folder: string;
	songs: readonly (readonly [number, string, string])[];
	companions?: readonly string[];
}

const blank: Song = {
	title: '',
	album: '',
	albumArtist: '',
	artists: [],
	genre: '',
	discName: '',
	format: 'flac',
	year: 0,
	originalYear: 0,
	disc: 1,
	discs: 1,
	track: 0,
	discSongs: 0,
	compilation: false
};

/** The owner's albums the boards show, each as it sits on disk now. */
const boardAlbums: readonly BoardAlbum[] = [
	{
		id: 'org-church-music',
		title: 'Church Music',
		artist: 'David Crowder Band',
		year: 2012,
		folder: 'David Crowder Band/2009 - Church Music',
		songs: [
			[1, 'Phos Hilaron (Hail Gladdening Light)', '01 Phos Hilaron (Hail Gladdening Light).flac'],
			[2, 'Alleluia, Sing', '02 Alleluia, Sing.flac'],
			[3, 'The Nearness', '03 The Nearness.flac'],
			[4, 'SMS (Shine)', '04 SMS (Shine).flac'],
			[5, 'How He Loves', '05 How He Loves.flac']
		],
		companions: ['cover.jpg']
	},
	{
		id: 'org-falling-up-2015',
		title: 'Falling Up',
		artist: 'Falling Up',
		year: 2015,
		folder: 'Falling Up/2015 - Falling Up',
		songs: [
			[1, 'Boone Flyer', 'Falling Up - Falling Up - 01 Boone Flyer.flac'],
			[2, 'Flora', 'Falling Up - Falling Up - 02 Flora.flac'],
			[3, 'The Green Rider', 'Falling Up - Falling Up - 03 The Green Rider.flac'],
			[4, 'Aaron Sentinel', 'Falling Up - Falling Up - 04 Aaron Sentinel.flac']
		]
	},
	{
		id: 'org-even-in-arcadia',
		title: 'Even in Arcadia',
		artist: 'Sleep Token',
		year: 2025,
		folder: 'Sleep Token/2025 - Even In Arcadia',
		songs: [
			[1, 'Look to Windward', '01-01 Look To Windward.flac'],
			[2, 'Emergence', '01-02 Emergence.flac'],
			[3, 'Past Self', '01-03 Past Self.flac']
		]
	},
	{
		id: 'org-narnia',
		title: 'The Chronicles of Narnia: The Lion, The Witch and The Wardrobe',
		artist: 'Harry Gregson-Williams',
		year: 2005,
		folder: 'Harry Gregson-Williams',
		songs: [
			[1, 'The Blitz, 1940', '01 The Blitz, 1940.flac'],
			[13, 'Only the Beginning of the Adventure', '13 Only The Beginning of The Advent.flac']
		]
	},
	{
		id: 'org-bellum',
		title: 'Bellum Interruptum',
		artist: 'Mechina',
		year: 2025,
		folder: 'Mechina/2025 - Bellum Interruptum',
		songs: [
			[
				1,
				'The World We Saved 2025',
				'Mechina - Bellum Interruptum - 01 The World We Saved 2025.flac'
			],
			[2, 'Signal Lost', 'Mechina - Bellum Interruptum - 02 Signal Lost.flac']
		]
	},
	{
		id: 'org-edge-of-a-dream',
		title: 'Edge of a Dream',
		artist: 'Kingdom Of Giants',
		year: 2026,
		folder: 'Kingdom Of Giants/2026- Edge of a Dream',
		songs: [
			[1, 'Nightmare', '01 Nightmare.flac'],
			[2, 'Respawn', '02 Respawn.flac']
		],
		companions: ['cover.jpg', 'Edge of a Dream.cue']
	},
	{
		id: 'org-blackstar',
		title: 'Wish Upon a Blackstar',
		artist: 'Celldweller',
		year: 2012,
		folder: 'Celldweller/2012 - Wish Upon a Blackstar',
		songs: [
			[2, 'It Makes No Difference Who We Are', '02 - It Makes No Difference Who We Are.flac'],
			[12, 'The Best It’s Gonna Get', '12 - The Best It’s Gonna Get.flac']
		]
	},
	{
		id: 'org-blackstar-deluxe',
		title: 'Wish Upon a Blackstar (Deluxe)',
		tag: 'Wish Upon a Blackstar',
		artist: 'Celldweller',
		year: 2012,
		folder: 'Celldweller/2012 - Wish Upon a Blackstar (Deluxe)',
		songs: [
			[2, 'It Makes No Difference Who We Are', '02 - It Makes No Difference Who We Are.flac'],
			[12, 'The Best It’s Gonna Get', '12 - The Best It’s Gonna Get.flac']
		]
	},
	{
		id: 'org-frame-canvas',
		title: 'Frame & Canvas',
		artist: 'Braid',
		year: 1998,
		folder: 'Braid/1998 - Frame & Canvas',
		songs: [
			[1, 'The New Nathan Detroits', '01 The New Nathan Detroits.flac'],
			[3, 'Killing a Camera', '03 Killing a Camera.flac']
		]
	},
	{
		id: 'org-voice-memos',
		title: 'Voice Memos',
		artist: 'Falling Up',
		year: 0,
		folder: 'Falling Up/Voice Memos',
		songs: [[0, '', 'memo 0412.flac']]
	},
	{
		id: 'org-fangs',
		title: 'Fangs!',
		artist: 'Falling Up',
		year: 2009,
		folder: 'Falling Up/2009 - Fangs!',
		songs: [
			[1, 'A Colour Eoptian', '01 - A Colour Eoptian.flac'],
			[2, 'Lotus and the Languorous', '02 - Lotus and the Languorous.flac'],
			[3, 'Streams of Woe at Acheron', '03 - Streams of Woe at Acheron.flac'],
			[4, 'Magician Reversed', '04 - Magician Reversed.flac'],
			[5, 'Golden Arrows', '05 - Golden Arrows.flac'],
			[6, 'The King’s Garden', '06 - The King’s Garden.flac']
		]
	},
	{
		id: 'org-crashings',
		title: 'Crashings',
		artist: 'Falling Up',
		year: 2004,
		folder: 'Falling Up/2004 - Crashings',
		songs: [
			[1, 'Bittersweet', '01 - Bittersweet.flac'],
			[2, 'Broken Heart', '02 - Broken Heart.flac']
		]
	}
];

/** Files on the fixture disk that are no song's: one in a song's way. */
const unknownFiles = ['Braid/1998 - Frame & Canvas/03 - Killing a Camera.flac'];

/** The songs to show a template on, by ID: one in place, one renamed, one moved, then more. */
const exampleOrder = [
	'org-fangs-5',
	'org-bellum-1',
	'org-narnia-13',
	'org-even-in-arcadia-2',
	'org-edge-of-a-dream-2',
	'org-falling-up-2015-1'
];

function boardSongs(): DiskSong[] {
	return boardAlbums.flatMap((album) =>
		album.songs.map(([track, title, file], index) => ({
			id: `${album.id}-${String(track)}`,
			albumId: album.id,
			albumTitle: album.title,
			artist: album.artist,
			path: file.includes('/') ? file : `${album.folder}/${file}`,
			durationMs: 200_000 + ((track * 37_000) % 120_000),
			addedAt: album.id.endsWith('deluxe') ? '2025-07-02T18:00:00Z' : '2025-03-10T18:00:00Z',
			plays: (index * 7 + track) % 23,
			stars: album.id.endsWith('deluxe') ? 4 : 0,
			song: {
				...blank,
				title,
				album: album.tag ?? album.title,
				albumArtist: album.artist,
				artists: [album.artist],
				year: album.year,
				track,
				discSongs: album.songs.length
			}
		}))
	);
}

/** The fixture library's own albums: in place, but for Glasshouse, which sits in an old shape. */
function librarySongs(albums: readonly Album[], main: Template, comp: Template): DiskSong[] {
	return albums.flatMap((album) =>
		album.tracks.map((track) => {
			const song: Song = {
				...blank,
				title: track.title,
				album: album.title,
				albumArtist: album.compilation ? 'Various Artists' : album.artist,
				artists: track.artists ?? [album.artist],
				year: album.year ?? 0,
				disc: track.discNo ?? 1,
				discs: album.discs?.length ?? 1,
				track: track.trackNo ?? 0,
				discSongs: album.tracks.filter(({ discNo }) => discNo === track.discNo).length,
				compilation: album.compilation
			};
			const at = pathOf(album.compilation ? comp : main, song, '.flac');
			const shaped = `${album.artist}/${album.title}/${String(track.trackNo ?? 0).padStart(2, '0')} ${track.title}.flac`;
			return {
				id: track.id,
				albumId: album.id,
				albumTitle: album.title,
				artist: album.artist,
				path: album.id === 'glasshouse-66' || at === 'noTags' || at === 'tooLong' ? shaped : at,
				durationMs: track.durationMs,
				addedAt: album.addedAt ?? '2026-10-12T18:00:00Z',
				plays: 3,
				stars: 0,
				song
			};
		})
	);
}

/** Folds a path as a case-insensitive filesystem would. */
const fold = (path: string): string => path.normalize('NFC').toLowerCase();
const dirOf = (path: string): string => path.slice(0, Math.max(0, path.lastIndexOf('/')));
const baseOf = (path: string): string => path.slice(path.lastIndexOf('/') + 1);

/** A planned song. */
interface Planned {
	disk: DiskSong;
	file: OrganizeFile;
}

interface Kept {
	preview: Omit<OrganizePreview, 'albums' | 'nextCursor' | 'filter' | 'matching'>;
	albums: { album: Omit<OrganizeAlbum, 'files' | 'kind'>; songs: Planned[] }[];
}

/** A run going in the page: what it moves, album by album, and where it stands. */
interface Going {
	run: OrganizeRun;
	albums: { id: string; songs: { disk: DiskSong; to: string }[]; companions: number }[];
	at: number;
	timer: ReturnType<typeof setTimeout> | undefined;
	/** What it moved: song ID → where it was. */
	moved: Map<string, string>;
	left: OrganizeRunDetail['left'];
	/** The run this one undoes. */
	undoing?: Going;
}

const pageSize = 200;

export interface OrganizeFixtureOptions {
	/** The library's own albums, as the tag editor leaves them. */
	albums: () => readonly Album[];
	/** The saved templates. */
	templates: () => { template: string; compilations: string };
	/** May move files: an administrator, with tag writes on. */
	writes: () => boolean;
	/** How long each album of a run takes. */
	albumMs?: number;
	respond: <T>(answer: () => T) => Promise<T>;
}

/** The organize routes on the fixture disk. */
export function createOrganizeFixture(options: OrganizeFixtureOptions): OrganizeSource {
	const albumMs = options.albumMs ?? 450;
	/** Where songs moved to, by ID, over the boards' and the library's own. */
	const paths = new Map<string, string>();
	const previews = new Map<string, Kept>();
	const runs: Going[] = [];
	const listeners = new Set<(event: OrganizeEvent) => void>();
	let made = 0;

	function templatesOf(template?: string, compilations?: string): [Template, Template] {
		const saved = options.templates();
		const pick = (field: 'template' | 'compilations', text: string): Template => {
			try {
				return parse(text);
			} catch (error) {
				const { at = 0, reason = '' } = error as { at?: number; reason?: string };
				throw new OrganizeError(422, reason, { field, at });
			}
		};
		return [
			pick('template', template ?? saved.template),
			pick('compilations', compilations ?? saved.compilations)
		];
	}

	/** The shape the library's own albums were filed in, before any template was typed. */
	const shape = parse(defaultTemplate);
	const shapeComp = parse(defaultCompilations);

	/** Every song on the disk, where it is now. */
	function disk(): DiskSong[] {
		return [...boardSongs(), ...librarySongs(options.albums(), shape, shapeComp)].map((song) => ({
			...song,
			path: paths.get(song.id) ?? song.path
		}));
	}

	function claimant(d: DiskSong, here: boolean): OrganizeClaimant {
		return {
			track: d.id,
			title: d.song.title,
			album: d.albumTitle,
			albumId: d.albumId,
			path: d.path,
			root: 0,
			format: 'FLAC',
			bitrate: 1020,
			durationMs: d.durationMs,
			plays: d.plays,
			stars: d.stars,
			addedAt: d.addedAt,
			here
		};
	}

	function plan(songs: DiskSong[], all: DiskSong[], main: Template, comp: Template): Planned[] {
		const occupants = new Map(all.map((d) => [fold(d.path), d]));
		const unknown = new Set(unknownFiles.map(fold));
		const planned = songs.map((d): Planned => {
			const to = pathOf(d.song.compilation ? comp : main, d.song, extensionOf(d.path));
			const file: OrganizeFile = {
				track: d.id,
				title: d.song.title,
				from: d.path,
				kind: 'inPlace'
			};
			if (to === 'noTags' || to === 'tooLong') {
				return { disk: d, file: { ...file, kind: 'skipped', reason: to } };
			}
			file.to = to;
			if (to !== d.path) {
				file.kind = dirOf(to) === dirOf(d.path) ? 'rename' : 'move';
				if (fold(to) === fold(d.path)) {
					file.caseOnly = true;
				}
			}
			return { disk: d, file };
		});
		const wanted = new Map<string, Planned[]>();
		for (const p of planned) {
			if (p.file.kind === 'rename' || p.file.kind === 'move') {
				const key = fold(p.file.to ?? '');
				wanted.set(key, [...(wanted.get(key) ?? []), p]);
			}
		}
		for (const p of planned) {
			if (p.file.kind !== 'rename' && p.file.kind !== 'move') {
				continue;
			}
			const key = fold(p.file.to ?? '');
			const others = (wanted.get(key) ?? []).filter((o) => o !== p).map((o) => o.disk);
			const there = occupants.get(key);
			const blockers = there !== undefined && there.id !== p.disk.id ? [there, ...others] : others;
			if (blockers.length > 0) {
				const same = blockers.some(
					(b) =>
						fold(b.song.title) === fold(p.disk.song.title) &&
						b.artist === p.disk.artist &&
						Math.abs(b.durationMs - p.disk.durationMs) <= 2000
				);
				p.file = {
					...p.file,
					kind: 'conflict',
					reason: 'claimed',
					conflict: {
						self: claimant(p.disk, false),
						with: blockers.map((b) => claimant(b, b === there)),
						sameRecording: same,
						...(same ? { why: 'tags' as const } : {})
					}
				};
			} else if (unknown.has(key)) {
				p.file = {
					...p.file,
					kind: 'conflict',
					reason: 'unknownFile',
					unknown: baseOf(unknownFiles.find((u) => fold(u) === key) ?? '')
				};
			}
		}
		return planned;
	}

	function previewOf(
		scope: { tracks?: readonly string[]; albums?: readonly string[] },
		template?: string,
		compilations?: string
	): Kept {
		const [main, comp] = templatesOf(template, compilations);
		const all = disk();
		let songs = all;
		if (scope.tracks !== undefined) {
			const wanted = new Set(scope.tracks);
			songs = all.filter((d) => wanted.has(d.id));
			if (songs.length !== wanted.size) {
				throw new OrganizeError(404, 'a song named is not in the library');
			}
		} else if (scope.albums !== undefined) {
			const wanted = new Set(scope.albums);
			songs = all.filter((d) => wanted.has(d.albumId));
			if (new Set(songs.map((d) => d.albumId)).size !== wanted.size) {
				throw new OrganizeError(404, 'an album named is not in the library');
			}
		}
		const planned = plan(songs, all, main, comp);
		const byAlbum = new Map<string, Planned[]>();
		for (const p of planned) {
			byAlbum.set(p.disk.albumId, [...(byAlbum.get(p.disk.albumId) ?? []), p]);
		}
		const count = (kind: FileKind): number => planned.filter((p) => p.file.kind === kind).length;
		const folders = new Set(
			planned
				.filter((p) => p.file.kind === 'rename' || p.file.kind === 'move')
				.map((p) => dirOf(p.file.to ?? ''))
		);
		const albumTitles = [...byAlbum.values()].map((list) => list[0]?.disk.albumTitle ?? '');
		const scoped = scope.tracks !== undefined || scope.albums !== undefined;
		made += 1;
		return {
			preview: {
				id: `preview-${String(made)}`,
				template: main.text,
				compilations: comp.text,
				counts: {
					songs: planned.length,
					inPlace: count('inPlace'),
					moves: count('rename') + count('move'),
					renames: count('rename'),
					newFolders: count('move'),
					conflicts: count('conflict'),
					skipped: count('skipped'),
					folders: folders.size
				},
				...(scoped
					? {
							scope: {
								songs: planned.length,
								albums: byAlbum.size,
								...(byAlbum.size === 1 ? { album: albumTitles[0] ?? '' } : {})
							}
						}
					: {})
			},
			albums: [...byAlbum.entries()]
				.sort(([, a], [, b]) => (a[0]?.disk.path ?? '').localeCompare(b[0]?.disk.path ?? ''))
				.map(([id, list]) => {
					const first = list[0]?.disk;
					const board = boardAlbums.find((album) => album.id === id);
					const moving = list.filter((p) => p.file.kind === 'move');
					const companions =
						moving.length === list.length && board?.companions !== undefined
							? board.companions.map((name) => ({
									from: `${board.folder}/${name}`,
									to: `${dirOf(moving[0]?.file.to ?? '')}/${name}`
								}))
							: [];
					return {
						album: {
							id,
							title: first?.albumTitle ?? '',
							artist: first?.artist ?? '',
							root: 0,
							songs: list.length,
							companions
						},
						songs: list
					};
				})
		};
	}

	function pageOf(kept: Kept, filter: OrganizeFilter, q = '', cursor?: string): OrganizePreview {
		const wants = (file: OrganizeFile): boolean =>
			filter === 'all' ? file.kind === 'rename' || file.kind === 'move' : file.kind === filter;
		const needle = q.trim().toLowerCase();
		const finds = (album: Kept['albums'][number]['album'], file: OrganizeFile): boolean =>
			needle === '' ||
			[album.artist, album.title, file.title, file.from, file.to ?? ''].some((s) =>
				s.toLowerCase().includes(needle)
			);
		const matching = kept.albums.flatMap(({ album, songs }) => {
			const files = songs.map((p) => p.file).filter((f) => wants(f) && finds(album, f));
			if (files.length === 0) {
				return [];
			}
			const kinds = new Set(songs.map((p) => p.file.kind));
			const [only] = kinds;
			const froms = new Set(files.map((f) => dirOf(f.from)));
			const tos = new Set(files.map((f) => dirOf(f.to ?? f.from)));
			const [fromFolder] = froms;
			const [toFolder] = tos;
			const folderMove =
				froms.size === 1 &&
				tos.size === 1 &&
				fromFolder !== undefined &&
				toFolder !== undefined &&
				fromFolder !== toFolder &&
				files.every((f) => f.kind === 'move' || f.kind === 'conflict');
			const entry: OrganizeAlbum = {
				...album,
				kind: kinds.size === 1 && only !== undefined ? only : 'mixed',
				files,
				...(folderMove ? { fromFolder, toFolder } : {}),
				...(folderMove && fold(fromFolder) === fold(toFolder) ? { caseOnly: true } : {})
			};
			return [entry];
		});
		const start = Number(cursor ?? 0);
		const end = start + pageSize;
		return {
			...structuredClone(kept.preview),
			filter,
			matching: matching.length,
			albums: matching.slice(start, end),
			...(end < matching.length ? { nextCursor: String(end) } : {})
		};
	}

	function tell(going: Going): void {
		const { run } = going;
		const event: OrganizeEvent = {
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

	function finish(going: Going, state: OrganizeRun['state']): void {
		going.run.state = state;
		going.run.finishedAt = new Date().toISOString();
		delete going.run.album;
		delete going.run.stopping;
		going.run.undoable = going.run.kind !== 'undo' && going.moved.size > 0;
		if (going.undoing !== undefined) {
			going.undoing.run.state = 'undone';
			going.undoing.run.undoneBy = going.run.id;
		}
		tell(going);
	}

	function step(going: Going): void {
		const album = going.albums[going.at];
		if (album === undefined) {
			finish(going, 'done');
			return;
		}
		going.run.album = album.id;
		for (const { disk: d, to } of album.songs) {
			going.moved.set(d.id, d.path);
			paths.set(d.id, to);
			going.run.done += 1;
			going.run.moved += 1;
		}
		going.run.companions += album.companions;
		going.at += 1;
		if (going.run.stopping === true) {
			finish(going, 'stopped');
			return;
		}
		tell(going);
		going.timer = setTimeout(() => {
			step(going);
		}, albumMs);
	}

	function begin(going: Going): OrganizeRun {
		runs.unshift(going);
		const head = going.albums[0];
		if (head !== undefined) {
			going.run.album = head.id;
		}
		going.timer = setTimeout(() => {
			step(going);
		}, albumMs);
		queueMicrotask(() => {
			tell(going);
		});
		return structuredClone(going.run);
	}

	function newRun(kind: OrganizeRun['kind'], template: string): OrganizeRun {
		made += 1;
		return {
			id: `run-${String(made)}`,
			kind,
			state: 'running',
			startedAt: new Date().toISOString(),
			by: 'simon',
			template,
			total: 0,
			done: 0,
			moved: 0,
			folders: 0,
			conflicts: 0,
			left: 0,
			failed: 0,
			companions: 0,
			undoable: false
		};
	}

	const busy = (): boolean => runs.some((going) => going.run.state === 'running');

	function canWrite(): void {
		if (!options.writes()) {
			throw new OrganizeError(409, 'tag writes are off, and with them organizing');
		}
		if (busy()) {
			throw new OrganizeError(409, 'a run holds the library');
		}
	}

	function goingOf(id: string): Going {
		const going = runs.find((each) => each.run.id === id);
		if (going === undefined) {
			throw new OrganizeError(404, 'no such run');
		}
		return going;
	}

	return {
		preview: (request) =>
			options.respond(() => {
				const kept = previewOf(request, request.template, request.compilations);
				previews.set(kept.preview.id, kept);
				return pageOf(kept, 'all');
			}),

		page: (id, { filter, q, cursor }) =>
			options.respond(() => {
				const kept = previews.get(id);
				if (kept === undefined) {
					throw new OrganizeError(404, 'the preview is gone: preview again');
				}
				return pageOf(kept, filter, q, cursor);
			}),

		examples: (count, seed) =>
			options.respond((): OrganizeExample[] => {
				const all = disk();
				const order = exampleOrder.flatMap((id) => all.filter((d) => d.id === id));
				const shift = ((seed % order.length) + order.length) % order.length;
				return Array.from({ length: Math.min(count, order.length) }, (_, i) => {
					const d = order[(i + shift) % order.length] ?? order[0];
					if (d === undefined) {
						throw new OrganizeError(500, 'no songs');
					}
					return {
						track: d.id,
						title: d.song.title,
						artist: d.artist,
						album: d.albumTitle,
						from: d.path,
						root: 0,
						song: structuredClone(d.song)
					};
				});
			}),

		runs: () => options.respond(() => runs.slice(0, 10).map((going) => structuredClone(going.run))),

		run: (id) =>
			options.respond(() => {
				const going = goingOf(id);
				return { run: structuredClone(going.run), left: structuredClone(going.left) };
			}),

		start: ({ previewId, tracks, albums }) =>
			options.respond(() => {
				const kept = previews.get(previewId);
				if (kept === undefined) {
					throw new OrganizeError(404, 'the preview is gone: preview again');
				}
				canWrite();
				const tracksPicked = tracks === undefined ? null : new Set(tracks);
				const albumsPicked = albums === undefined ? null : new Set(albums);
				const run = newRun('run', kept.preview.template);
				const going: Going = {
					run,
					albums: [],
					at: 0,
					timer: undefined,
					moved: new Map(),
					left: []
				};
				for (const { album, songs } of kept.albums) {
					if (albumsPicked !== null && !albumsPicked.has(album.id)) {
						continue;
					}
					const picked = songs.filter((p) => tracksPicked === null || tracksPicked.has(p.disk.id));
					for (const p of picked) {
						if (p.file.kind === 'conflict') {
							run.conflicts += 1;
						} else if (p.file.kind === 'skipped') {
							run.left += 1;
							going.left.push({
								track: p.disk.id,
								title: p.disk.song.title,
								from: p.disk.path,
								root: 0,
								reason: p.file.reason ?? 'noTags'
							});
						}
					}
					const moving = picked.filter((p) => p.file.kind === 'rename' || p.file.kind === 'move');
					if (moving.length > 0) {
						going.albums.push({
							id: album.id,
							songs: moving.map((p) => ({ disk: p.disk, to: p.file.to ?? p.disk.path })),
							companions: album.companions.length
						});
					}
				}
				run.total = going.albums.reduce((sum, a) => sum + a.songs.length, 0);
				run.folders = new Set(going.albums.flatMap((a) => a.songs.map(({ to }) => dirOf(to)))).size;
				previews.delete(previewId);
				return begin(going);
			}),

		stop: (id) =>
			options.respond(() => {
				const going = goingOf(id);
				if (going.run.state !== 'running') {
					throw new OrganizeError(409, 'the run isn’t running');
				}
				going.run.stopping = true;
				return structuredClone(going.run);
			}),

		undo: (id) =>
			options.respond(() => {
				const of = goingOf(id);
				if (!of.run.undoable) {
					throw new OrganizeError(409, 'nothing of this run can go back');
				}
				canWrite();
				const run = newRun('undo', of.run.template);
				run.undoOf = of.run.id;
				const songs = [...of.moved.entries()].map(([trackId, from]) => ({
					id: trackId,
					from
				}));
				const going: Going = {
					run,
					albums: [],
					at: 0,
					timer: undefined,
					moved: new Map(),
					left: []
				};
				for (const album of of.albums.slice(0, of.at)) {
					const back = album.songs
						.filter(({ disk: d }) => songs.some((s) => s.id === d.id))
						.map(({ disk: d }) => ({
							disk: { ...d, path: paths.get(d.id) ?? d.path },
							to: of.moved.get(d.id) ?? d.path
						}));
					if (back.length > 0) {
						going.albums.push({ id: album.id, songs: back, companions: album.companions });
					}
				}
				run.total = songs.length;
				run.folders = new Set(going.albums.flatMap((a) => a.songs.map(({ to }) => dirOf(to)))).size;
				of.run.undoable = false;
				going.undoing = of;
				return begin(going);
			}),

		listen(heard) {
			listeners.add(heard);
			return () => {
				listeners.delete(heard);
			};
		}
	};
}
