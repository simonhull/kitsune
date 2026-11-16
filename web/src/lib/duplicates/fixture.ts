/**
 * The Duplicates routes on the boards' library (plan 069), worked out in the page so e2e and the
 * perf suite need no server: the owner's groups from board G, each with its copies' facts, the
 * suggested keeper by the server's order of reasons, what keeping each copy moves, and the two
 * Wish Upon a Blackstar folders as an album pair. A merge sends the trashed copies to the fixture
 * Trash; Not duplicates and Bring back are kept for the page's life.
 */

import type { Album, Track, TrashChange } from '#lib/library/source.ts';
import {
	DuplicatesError,
	type Confidence,
	type DuplicateAlbum,
	type DuplicateCopy,
	type DuplicateCounts,
	type DuplicateDetail,
	type DuplicateFilter,
	type DuplicateGroup,
	type DuplicateGroupSummary,
	type DuplicateList,
	type DuplicatePair,
	type DuplicatePairRow,
	type DuplicatePairSummary,
	type DuplicatesSource,
	type KeepPlan,
	type KeepReason,
	type MergePlan
} from './types.ts';

/** The key of the group 068's conflict card links to: Blackstar's The Best It's Gonna Get. */
export const bestItsGonnaGet = 'dup-best-its-gonna-get';

/** A fixture album: its ID, title, artist, year, whether it's an original album, and its folder. */
interface FixtureAlbum {
	id: string;
	title: string;
	artist: string;
	year: number;
	original: boolean;
	folder: string;
	addedAt: string;
}

const albums: readonly FixtureAlbum[] = [
	{
		id: 'dup-liturgy',
		title: 'A Liturgy, a Legacy & a Ragamuffin Band',
		artist: 'Rich Mullins',
		year: 1993,
		original: true,
		folder: 'Rich Mullins/1993 - A Liturgy, a Legacy & a Ragamuffin Band',
		addedAt: '2025-03-21T10:00:00Z'
	},
	{
		id: 'dup-songs',
		title: 'Songs',
		artist: 'Rich Mullins',
		year: 1996,
		original: false,
		folder: 'Rich Mullins/1996 - Songs',
		addedAt: '2025-03-21T10:00:00Z'
	},
	{
		id: 'dup-songs-2',
		title: 'Songs 2',
		artist: 'Rich Mullins',
		year: 1999,
		original: false,
		folder: 'Rich Mullins/1999 - Songs 2',
		addedAt: '2025-04-02T10:00:00Z'
	},
	{
		id: 'dup-no-plan-b',
		title: 'No Plan B',
		artist: 'Manafest',
		year: 2012,
		original: true,
		folder: 'Manafest/2012 - No Plan B',
		addedAt: '2025-05-11T10:00:00Z'
	},
	{
		id: 'dup-the-chase',
		title: 'The Chase',
		artist: 'Manafest',
		year: 2013,
		original: false,
		folder: 'Manafest/2013 - The Chase',
		addedAt: '2025-06-01T10:00:00Z'
	},
	{
		id: 'dup-pride-lands',
		title: 'Rhythm of the Pride Lands',
		artist: 'Lebo M',
		year: 1995,
		original: true,
		folder: 'Lebo M/1995 - Rhythm of the Pride Lands',
		addedAt: '2025-02-02T10:00:00Z'
	},
	{
		id: 'dup-pride-rock',
		title: 'Return to Pride Rock',
		artist: 'Lebo M',
		year: 1998,
		original: false,
		folder: 'Lebo M/1998 - Return to Pride Rock',
		addedAt: '2025-02-03T10:00:00Z'
	},
	{
		id: 'dup-start-again',
		title: 'Start Again',
		artist: 'Nevertel',
		year: 2018,
		original: true,
		folder: 'Nevertel/2018 - Start Again',
		addedAt: '2025-01-09T10:00:00Z'
	},
	{
		id: 'dup-sacrifice',
		title: 'Sacrifice (single)',
		artist: 'Nevertel',
		year: 2017,
		original: true,
		folder: 'Nevertel/2017 - Sacrifice',
		addedAt: '2025-01-10T10:00:00Z'
	},
	{
		id: 'dup-cellout',
		title: 'The Complete Cellout, Volume 01',
		artist: 'Celldweller',
		year: 2010,
		original: true,
		folder: 'Celldweller/2010 - The Complete Cellout, Volume 01',
		addedAt: '2025-03-12T10:00:00Z'
	},
	{
		id: 'dup-voices',
		title: 'Soundtrack for the Voices in My Head, Vol. 01',
		artist: 'Celldweller',
		year: 2008,
		original: false,
		folder: 'Celldweller/2008 - Soundtrack for the Voices in My Head, Vol. 01',
		addedAt: '2025-03-12T10:00:00Z'
	},
	{
		id: 'dup-crashings',
		title: 'Crashings',
		artist: 'Falling Up',
		year: 2004,
		original: true,
		folder: 'Falling Up/2004 - Crashings',
		addedAt: '2024-11-30T10:00:00Z'
	},
	{
		id: 'dup-crashings-mp3',
		title: 'Crashings (Rips)',
		artist: 'Falling Up',
		year: 2004,
		original: true,
		folder: 'Falling Up/Rips/Crashings',
		addedAt: '2023-08-14T10:00:00Z'
	},
	{
		id: 'org-blackstar',
		title: 'Wish Upon a Blackstar',
		artist: 'Celldweller',
		year: 2012,
		original: true,
		folder: 'Celldweller/2012 - Wish Upon a Blackstar',
		addedAt: '2025-03-12T10:00:00Z'
	},
	{
		id: 'org-blackstar-deluxe',
		title: 'Wish Upon a Blackstar (Deluxe)',
		artist: 'Celldweller',
		year: 2012,
		original: true,
		folder: 'Celldweller/2012 - Wish Upon a Blackstar (Deluxe)',
		addedAt: '2025-07-29T10:00:00Z'
	}
];

/** A copy: its album, track number, title, format, bitrate, length, plays, own stars and raters. */
type CopyRow = readonly [
	album: string,
	trackNo: number,
	title: string,
	format: string,
	bitrate: number,
	seconds: number,
	plays: number,
	stars: number,
	raters: number
];

/** A board group: key, confidence, artist, and its copies. */
interface FixtureGroup {
	key: string;
	confidence: Confidence;
	artist: string;
	copies: readonly CopyRow[];
	/** Playlists each copy is in, by copy index. */
	playlists?: readonly (readonly string[])[];
}

const groups: readonly FixtureGroup[] = [
	{
		key: 'dup-creed',
		confidence: 'recording',
		artist: 'Rich Mullins',
		copies: [
			['dup-liturgy', 5, 'Creed', 'flac', 957, 321, 1, 5, 1],
			['dup-songs', 4, 'Creed', 'flac', 1022, 322, 2, 0, 0]
		]
	},
	{
		key: 'dup-hold-me-jesus',
		confidence: 'recording',
		artist: 'Rich Mullins',
		copies: [
			['dup-liturgy', 7, 'Hold Me Jesus', 'flac', 948, 182, 1, 4, 2],
			['dup-songs', 9, 'Hold Me Jesus', 'flac', 951, 182, 0, 0, 1]
		],
		playlists: [['Falling Up, Rested and rated'], ['Ragamuffin Sundays']]
	},
	{
		key: 'dup-here-in-america',
		confidence: 'recording',
		artist: 'Rich Mullins',
		copies: [
			['dup-liturgy', 3, 'Here in America', 'flac', 1003, 236, 1, 3, 1],
			['dup-songs-2', 2, 'Here in America', 'flac', 996, 236, 0, 0, 0]
		]
	},
	{
		key: 'dup-no-plan-b',
		confidence: 'recording',
		artist: 'Manafest',
		copies: [
			['dup-no-plan-b', 1, 'No Plan B', 'flac', 1011, 203, 1, 0, 0],
			['dup-the-chase', 14, 'No Plan B', 'flac', 1009, 203, 0, 0, 0]
		]
	},
	{
		key: 'dup-lion-sleeps',
		confidence: 'recording',
		artist: 'Lebo M',
		copies: [
			['dup-pride-lands', 4, 'The Lion Sleeps Tonight', 'flac', 870, 214, 0, 0, 0],
			['dup-pride-rock', 6, 'The Lion Sleeps Tonight', 'flac', 902, 216, 0, 0, 0]
		]
	},
	{
		key: 'dup-sacrifice',
		confidence: 'probably',
		artist: 'Nevertel',
		copies: [
			['dup-start-again', 3, 'Sacrifice', 'flac', 2116, 168, 3, 4, 1],
			['dup-sacrifice', 1, 'Sacrifice', 'flac', 1020, 168, 1, 0, 0]
		]
	},
	{
		key: 'dup-birthright',
		confidence: 'probably',
		artist: 'Celldweller',
		copies: [
			[
				'dup-cellout',
				9,
				'Birthright (Birthwrong remix by Blue Stahli)',
				'flac',
				1018,
				148,
				0,
				0,
				0
			],
			['dup-voices', 4, 'Birthright (Birthwrong remix by Blue Stahli)', 'flac', 1024, 150, 0, 0, 0]
		]
	},
	{
		key: 'dup-bittersweet',
		confidence: 'audio',
		artist: 'Falling Up',
		copies: [
			['dup-crashings', 1, 'Bittersweet', 'flac', 1034, 215, 6, 4.5, 2],
			['dup-crashings-mp3', 1, 'Bittersweet', 'mp3', 320, 215, 9, 0, 1]
		]
	}
];

/** Blackstar's songs in both folders, as `[track, title, seconds]`, and the one only the Deluxe has. */
const blackstarBoth: readonly (readonly [number, string, number])[] = [
	[1, 'Wish Upon a Blackstar', 334],
	[2, 'It Makes No Difference Who We Are', 344],
	[5, 'End of an Empire', 391],
	[12, 'The Best It’s Gonna Get', 312]
];
const blackstarOnly = [16, 'The Lucky One', 254] as const;
const blackstarPair = 'dup-blackstar-pair';

const albumOf = new Map(albums.map((a) => [a.id, a]));

function ref(id: string): DuplicateAlbum {
	const a = albumOf.get(id);
	return {
		id,
		title: a?.title ?? id,
		artist: a?.artist ?? '',
		...(a === undefined ? {} : { year: a.year })
	};
}

/** Builds a copy's facts from its row. */
function copyOf(groupKey: string, row: CopyRow, index: number): DuplicateCopy {
	const [album, trackNo, title, format, bitrate, seconds, plays, stars, raters] = row;
	const a = albumOf.get(album);
	const lossless = format !== 'mp3';
	const file = `${String(trackNo).padStart(2, '0')} - ${title}.${format}`;
	return {
		track: `${groupKey}-${String(index)}`,
		title,
		album: ref(album),
		trackNo,
		path: `${a?.folder ?? album}/${file}`,
		root: 0,
		format,
		bitrate,
		...(lossless ? { sampleRate: 44100, bitDepth: 16 } : { sampleRate: 44100 }),
		lossless,
		durationMs: seconds * 1000,
		plays,
		stars,
		raters,
		playlists: 0,
		addedAt: a?.addedAt ?? '2025-01-01T00:00:00Z',
		inPlace: a?.folder.includes(' - ') === true,
		original: a?.original === true
	};
}

const reasonOrder: readonly KeepReason[] = [
	'lossless',
	'original',
	'bitrate',
	'inPlace',
	'plays',
	'earlier'
];

/** Compares two copies by one reason, as `internal/duplicates` does: positive when a is better. */
function compare(r: KeepReason, a: DuplicateCopy, b: DuplicateCopy): number {
	switch (r) {
		case 'lossless':
			return Number(a.lossless) - Number(b.lossless);
		case 'original':
			return Number(a.original) - Number(b.original);
		case 'bitrate': {
			const [x, y] = [a.bitrate ?? 0, b.bitrate ?? 0];
			if (x <= 0 || y <= 0 || Math.max(x, y) - Math.min(x, y) <= 0.05 * Math.max(x, y)) {
				return 0;
			}
			return x > y ? 1 : -1;
		}
		case 'inPlace':
			return Number(a.inPlace) - Number(b.inPlace);
		case 'plays':
			return a.plays - b.plays;
		case 'earlier':
			return Date.parse(b.addedAt) - Date.parse(a.addedAt);
	}
}

/** The copy to keep, and why, by the server's order of reasons. */
export function keeperOf(copies: readonly DuplicateCopy[]): {
	keep: number;
	reasons: KeepReason[];
} {
	let keep = 0;
	copies.forEach((c, i) => {
		const best = copies[keep];
		if (best !== undefined && i > 0 && decides(c, best) > 0) {
			keep = i;
		}
	});
	const kept = copies[keep];
	if (kept === undefined) {
		return { keep: -1, reasons: [] };
	}
	const others = copies.filter((_, i) => i !== keep);
	const reasons = reasonOrder.filter((r) => others.every((o) => compare(r, kept, o) > 0));
	if (reasons.length > 0 || others.length === 0) {
		return { keep, reasons };
	}
	// It beats none on every count: what set it apart from the copy alike with it the longest.
	let last = -1;
	for (const o of others) {
		const n = reasonOrder.findIndex((r) => compare(r, kept, o) !== 0);
		last = Math.max(last, n);
	}
	const reason = reasonOrder[last];
	return { keep, reasons: reason === undefined ? [] : [reason] };
}

function decides(a: DuplicateCopy, b: DuplicateCopy): number {
	for (const r of reasonOrder) {
		const c = compare(r, a, b);
		if (c !== 0) {
			return c;
		}
	}
	return 0;
}

/** What keeping `keep` and trashing the rest of `copies` moves, as an administrator sees it. */
function planOf(
	copies: readonly DuplicateCopy[],
	keep: number,
	names: readonly (readonly string[])[]
): MergePlan {
	const kept = copies[keep];
	const gone = copies.filter((_, i) => i !== keep);
	const best = Math.max(0, ...gone.map((c) => c.stars));
	const plan: MergePlan = {
		plays: gone.reduce((sum, c) => sum + c.plays, 0),
		listens: gone.reduce((sum, c) => sum + c.plays, 0),
		stars: { come: 0, stay: 0 },
		loves: 0,
		hearts: 0,
		entries: 0,
		playlists: [],
		otherPlaylists: 0,
		excluded: 0,
		runs: 0,
		queues: 0
	};
	if (kept !== undefined && best > 0) {
		const after = Math.max(kept.stars, best);
		plan.stars.yours = { keeper: kept.stars, other: best, after };
		plan.loves = kept.stars < 4 && after >= 4 ? 1 : 0;
	}
	const others = gone.reduce((sum, c) => sum + c.raters - (c.stars > 0 ? 1 : 0), 0);
	plan.stars.come = kept !== undefined && kept.raters - (kept.stars > 0 ? 1 : 0) === 0 ? others : 0;
	plan.stars.stay = others - plan.stars.come;
	copies.forEach((_, i) => {
		if (i !== keep) {
			for (const name of names[i] ?? []) {
				plan.entries += 1;
				plan.playlists.push({ id: `pl-${name}`, name });
			}
		}
	});
	return plan;
}

/** A fixture album as the player and the Trash read it: the copies on it, as its songs. */
function albumWith(id: string, copies: readonly DuplicateCopy[]): Album {
	const a = albumOf.get(id);
	const tracks: Track[] = copies.map((c) => ({
		id: c.track,
		title: c.title,
		artists: [a?.artist ?? ''],
		trackNo: c.trackNo,
		durationMs: c.durationMs,
		format: c.format,
		genres: [],
		etag: c.track
	}));
	return {
		id,
		title: a?.title ?? id,
		artist: a?.artist ?? '',
		year: a?.year,
		trackCount: tracks.length,
		compilation: false,
		artists: [],
		tracks,
		durationMs: tracks.reduce((sum, t) => sum + t.durationMs, 0),
		formats: [...new Set(tracks.map((t) => t.format))].toSorted(),
		addedAt: a?.addedAt ?? '2025-01-01T00:00:00Z',
		genres: [],
		etag: id,
		discs: [{ songs: tracks.length, durationMs: tracks.reduce((sum, t) => sum + t.durationMs, 0) }]
	};
}

export interface DuplicatesFixtureOptions {
	/** Answers as the fixture source does: after its latency, refusing a listener. */
	respond: <T>(answer: () => T) => Promise<T>;
	/** Whether tag writes are on: off, a merge is refused. */
	writes: () => boolean;
	/** Sends songs to the fixture Trash. */
	trash: (songs: readonly { albumId: string; trackId: string }[]) => TrashChange;
}

/** The fixture's groups and pair, and what was merged or dismissed. */
export interface DuplicatesFixture extends DuplicatesSource {
	/** The fixture albums, with the copies on them as their songs, for the Trash and the player. */
	album(id: string): Album | undefined;
}

export function createDuplicatesFixture(options: DuplicatesFixtureOptions): DuplicatesFixture {
	const built = new Map<string, DuplicateGroup>();
	for (const g of groups) {
		const copies = g.copies.map((row, i) => ({
			...copyOf(g.key, row, i),
			playlists: g.playlists?.[i]?.length ?? 0
		}));
		built.set(g.key, groupOf(g.key, g.confidence, g.artist, copies, g.playlists ?? []));
	}
	// Blackstar: each song in both folders a group of the pair.
	const pairGroups: string[] = [];
	for (const [trackNo, title, seconds] of blackstarBoth) {
		const key = trackNo === 12 ? bestItsGonnaGet : `dup-blackstar-${String(trackNo)}`;
		const copies = (['org-blackstar', 'org-blackstar-deluxe'] as const).map((album, i) =>
			copyOf(
				key,
				[album, trackNo, title, 'flac', i === 0 ? 994 : 993, seconds, i === 0 ? 3 : 1, 0, 0],
				i
			)
		);
		built.set(key, { ...groupOf(key, 'audio', 'Celldweller', copies, []), pair: blackstarPair });
		pairGroups.push(key);
	}
	const lucky = copyOf(
		'dup-blackstar-only',
		[
			'org-blackstar-deluxe',
			blackstarOnly[0],
			blackstarOnly[1],
			'flac',
			993,
			blackstarOnly[2],
			0,
			0,
			0
		],
		0
	);

	const merged = new Set<string>();
	const dismissed = new Set<string>();

	function live(key: string): DuplicateGroup | undefined {
		const g = built.get(key);
		return g === undefined || merged.has(key) ? undefined : g;
	}
	const liveGroups = (): DuplicateGroup[] => [...built.values()].filter((g) => !merged.has(g.key));
	const pairLive = (): boolean => pairGroups.some((k) => !merged.has(k));
	const groupDismissed = (g: DuplicateGroup): boolean =>
		dismissed.has(g.key) || (g.pair !== undefined && dismissed.has(g.pair));

	function counts(): DuplicateCounts {
		const shown = liveGroups().filter((g) => !groupDismissed(g));
		const by = (c: Confidence): number => shown.filter((g) => g.confidence === c).length;
		const pairs = pairLive() && !dismissed.has(blackstarPair) ? 1 : 0;
		const dismissedGroups = liveGroups().filter(
			(g) => dismissed.has(g.key) && g.pair === undefined
		).length;
		return {
			groups: shown.length,
			songs: shown.reduce((sum, g) => sum + g.copies.length, 0),
			pairs,
			audio: by('audio'),
			recording: by('recording'),
			probably: by('probably'),
			dismissed: dismissedGroups + (pairLive() && dismissed.has(blackstarPair) ? 1 : 0),
			library: 7604,
			checked: 7604
		};
	}

	function summary(g: DuplicateGroup): DuplicateGroupSummary {
		const seen = new Map(g.copies.map((c) => [c.album.id, c.album]));
		return {
			key: g.key,
			title: g.title,
			artist: g.artist,
			confidence: g.confidence,
			copies: g.copies.length,
			formats: g.copies.map((c) => c.format),
			albums: [...seen.values()],
			durationMs: g.copies.map((c) => c.durationMs),
			plays: g.copies.reduce((sum, c) => sum + c.plays, 0),
			...(g.pair === undefined ? {} : { pair: g.pair }),
			...(groupDismissed(g) ? { dismissed: true } : {})
		};
	}

	function pairSummary(): DuplicatePairSummary {
		return {
			key: blackstarPair,
			albums: [ref('org-blackstar'), ref('org-blackstar-deluxe')],
			both: pairGroups.filter((k) => !merged.has(k)).length,
			only: [0, 1],
			...(dismissed.has(blackstarPair) ? { dismissed: true } : {})
		};
	}

	const rank: Record<Confidence, number> = { audio: 3, recording: 2, probably: 1 };

	function list(filter: DuplicateFilter): DuplicateList {
		const c = counts();
		const pairShows =
			pairLive() &&
			(filter === 'dismissed'
				? dismissed.has(blackstarPair)
				: (filter === 'all' || filter === 'pairs') && !dismissed.has(blackstarPair));
		const shown = liveGroups()
			.filter((g) => {
				const out = groupDismissed(g);
				switch (filter) {
					case 'all':
						return !out && g.pair === undefined;
					case 'dismissed':
						return dismissed.has(g.key) && g.pair === undefined;
					case 'pairs':
						return false;
					default:
						return !out && g.confidence === filter;
				}
			})
			.toSorted(
				(a, b) =>
					rank[b.confidence] - rank[a.confidence] ||
					b.copies.length - a.copies.length ||
					b.copies.reduce((s, x) => s + x.plays, 0) - a.copies.reduce((s, x) => s + x.plays, 0)
			);
		return {
			counts: c,
			filter,
			matching: shown.length + (pairShows ? 1 : 0),
			pairs: pairShows ? [pairSummary()] : [],
			groups: shown.map(summary)
		};
	}

	function pairDetail(): DuplicatePair {
		const rows: DuplicatePairRow[] = pairGroups
			.map((k) => live(k))
			.filter((g) => g !== undefined)
			.map((g) => ({
				group: g.key,
				copies: [g.copies[0] ?? null, g.copies[1] ?? null] as [
					DuplicateCopy | null,
					DuplicateCopy | null
				]
			}));
		rows.push({ copies: [null, lucky] });
		const plans: KeepPlan[] = [0, 1].map((side) => {
			const sum: MergePlan = { ...emptyPlan(), playlists: [] };
			for (const k of pairGroups) {
				const g = live(k);
				if (g !== undefined) {
					const p = planOf(g.copies, side, []);
					sum.plays += p.plays;
					sum.listens += p.listens;
				}
			}
			return { keep: side === 0 ? 'org-blackstar' : 'org-blackstar-deluxe', plan: sum };
		});
		return {
			key: blackstarPair,
			albums: [ref('org-blackstar'), ref('org-blackstar-deluxe')],
			keeper: 'org-blackstar-deluxe',
			rows,
			plans,
			...(dismissed.has(blackstarPair) ? { dismissed: true } : {})
		};
	}

	function detail(key: string): DuplicateDetail {
		if (key === blackstarPair && pairLive()) {
			return { kind: 'pair', pair: pairDetail() };
		}
		const g = live(key);
		if (g === undefined) {
			throw new DuplicatesError(
				404,
				'no group of duplicates has that key now: the library may have changed'
			);
		}
		return { kind: 'group', group: { ...g, ...(groupDismissed(g) ? { dismissed: true } : {}) } };
	}

	function samePlan(a: MergePlan, b: MergePlan): boolean {
		return JSON.stringify(a) === JSON.stringify(b);
	}

	function merge(key: string, keep: string, trash: readonly string[], plan: MergePlan) {
		if (!options.writes()) {
			throw new DuplicatesError(
				409,
				'tag writes are off, and with them the Trash: library.tag_writes'
			);
		}
		if (key === blackstarPair && pairLive()) {
			const side = keep === 'org-blackstar' ? 0 : 1;
			const sent = pairDetail().plans[side]?.plan;
			if (sent === undefined || !samePlan(sent, plan) || trash.length !== 1) {
				throw new DuplicatesError(409, 'the library changed since the confirm', sent);
			}
			const songs: { albumId: string; trackId: string }[] = [];
			const kept: string[] = [];
			for (const k of pairGroups) {
				const g = live(k);
				const [k0, k1] = [g?.copies[side], g?.copies[1 - side]];
				if (g !== undefined && k0 !== undefined && k1 !== undefined) {
					merged.add(k);
					kept.push(k0.track);
					songs.push({ albumId: k1.album.id, trackId: k1.track });
				}
			}
			return { kept, trashed: options.trash(songs) };
		}
		const g = live(key);
		if (g === undefined) {
			throw new DuplicatesError(
				404,
				'no group of duplicates has that key now: the library may have changed'
			);
		}
		const now = g.plans.find((p) => p.keep === keep)?.plan;
		if (now === undefined) {
			throw new DuplicatesError(422, 'keep one of the group’s copies');
		}
		if (!samePlan(now, plan)) {
			throw new DuplicatesError(409, 'the library changed since the confirm', now);
		}
		merged.add(key);
		const songs = g.copies
			.filter((c) => trash.includes(c.track))
			.map((c) => ({ albumId: c.album.id, trackId: c.track }));
		return { kept: [keep], trashed: options.trash(songs) };
	}

	function albumOfFixture(id: string): Album | undefined {
		const copies = [...built.values()].flatMap((g) => g.copies).filter((c) => c.album.id === id);
		if (id === 'org-blackstar-deluxe') {
			copies.push(lucky);
		}
		return copies.length === 0 ? undefined : albumWith(id, copies);
	}

	return {
		list: (filter) => options.respond(() => list(filter)),
		detail: (key) => options.respond(() => detail(key)),
		merge: (key, request) =>
			options.respond(() => merge(key, request.keep, request.trash, request.plan)),
		dismiss: (key) =>
			options.respond(() => {
				if (live(key) === undefined && !(key === blackstarPair && pairLive())) {
					throw new DuplicatesError(404, 'no group of duplicates has that key now');
				}
				dismissed.add(key);
			}),
		bringBack: (key) =>
			options.respond(() => {
				if (!dismissed.delete(key)) {
					throw new DuplicatesError(404, 'that group wasn’t dismissed');
				}
			}),
		album: albumOfFixture
	};
}

function emptyPlan(): MergePlan {
	return {
		plays: 0,
		listens: 0,
		stars: { come: 0, stay: 0 },
		loves: 0,
		hearts: 0,
		entries: 0,
		playlists: [],
		otherPlaylists: 0,
		excluded: 0,
		runs: 0,
		queues: 0
	};
}

/** A group with its copies, keeper, reasons and a plan for each copy kept. */
function groupOf(
	key: string,
	confidence: Confidence,
	artist: string,
	copies: DuplicateCopy[],
	names: readonly (readonly string[])[]
): DuplicateGroup {
	const { keep, reasons } = keeperOf(copies);
	return {
		key,
		title: copies[0]?.title ?? '',
		artist,
		confidence,
		keeper: copies[keep]?.track ?? '',
		reasons,
		copies,
		plans: copies.map((c, i) => ({ keep: c.track, plan: planOf(copies, i, names) }))
	};
}
