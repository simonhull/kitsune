/**
 * The review of a match, as pure functions: what would change, what's ticked, which song pairs with
 * which, and the one edit Apply sends. Nothing here writes: `POST /albums/{id}/edits` does, through
 * the tag editor's own path.
 */
import type { AlbumEdit, FieldValues, TagField, TrackTags } from '#lib/library/source.ts';
import type { MatchReview, MatchTrack } from './types.ts';

/** What the review compares: the release, the files as they are, and each song's length. */
export interface Basis {
	review: MatchReview;
	/** The album's files, as `GET /albums/{id}/tags` read them. */
	tags: readonly TrackTags[];
	/** Each song's length, by track ID. */
	lengths: Readonly<Record<string, number>>;
}

/** What a person has chosen. Everything is a plain record, so a state can be kept and compared. */
export interface Choices {
	/** The release track each song takes its tags from, by index; null for none. */
	pairs: Record<string, number | null>;
	/** Whether a song's file is written at all. */
	songs: Record<string, boolean>;
	/** Album rows ticked, by row key. */
	fields: Record<string, boolean>;
	/** A song's change left out, as `trackId:kind`. */
	off: Record<string, boolean>;
}

/** Two lengths further apart than this make a pair suspect. */
export const pairSlackMs = 3000;

/** The kinds of change a song can take, each with its pill. */
export type SongKind = 'title' | 'artists' | 'numbers' | 'isrc';

export const songKinds: readonly { kind: SongKind; name: string; fields: readonly TagField[] }[] = [
	{ kind: 'title', name: 'Titles', fields: ['title'] },
	{ kind: 'artists', name: 'Artists', fields: ['artists'] },
	{ kind: 'numbers', name: 'Track numbers', fields: ['track', 'trackTotal', 'disc', 'discTotal'] },
	{ kind: 'isrc', name: 'ISRCs', fields: ['isrc'] }
];

/** The fields every song of the album takes alike, in the board's order. */
const albumFields: readonly { field: TagField; label: string }[] = [
	{ field: 'album', label: 'Album' },
	{ field: 'albumArtists', label: 'Album artist' },
	{ field: 'date', label: 'Date' },
	{ field: 'originalDate', label: 'Original date' },
	{ field: 'label', label: 'Label' },
	{ field: 'catalogNumber', label: 'Catalog number' },
	{ field: 'barcode', label: 'Barcode' },
	{ field: 'genres', label: 'Genres' }
];

/** Fields that start unticked: they stay the person's unless asked. */
const askFirst: readonly TagField[] = ['genres'];

/** Album fields that decide which album a song groups into. */
const grouping: readonly TagField[] = ['album', 'albumArtists'];

const same = (a: readonly string[] = [], b: readonly string[] = []): boolean =>
	a.length === b.length && a.every((v, i) => v === b[i]);

/** One album field that would change. */
export interface FieldRow {
	key: string;
	field: TagField;
	label: string;
	/** What the songs hold now; null when they differ. */
	now: string[] | null;
	next: string[];
	on: boolean;
	/** Said under it. */
	note?: string;
}

/** A song's change of one kind. */
export interface Change {
	kind: SongKind;
	now: string;
	next: string;
	on: boolean;
}

/** One song of the album and its pair. */
export interface SongRow {
	id: string;
	/** Its title now, or its file's name. */
	title: string;
	/** Its artists now. */
	artists: string;
	/** Disc and number now, as `1·03`. */
	position: string;
	pair: number | null;
	release: MatchTrack | null;
	lengthMs: number;
	/** The pair's lengths differ by more than {@link pairSlackMs}. */
	suspect: boolean;
	/** Written at all. */
	on: boolean;
	changes: Change[];
	/** Its disc in the release, or now. */
	disc: number;
}

function trackOf(basis: Basis, id: string): TrackTags | undefined {
	return basis.tags.find((t) => t.id === id);
}

/** Album order: the pairs list every song once, in it. */
function songIds(basis: Basis): string[] {
	return basis.review.pairs.map(({ track }) => track);
}

/** Whether a song and a release track have lengths too far apart to be the same. */
export function suspect(basis: Basis, id: string, pair: number | null): boolean {
	if (pair === null) {
		return false;
	}
	const mine = basis.lengths[id];
	const theirs = basis.review.tracks[pair]?.lengthMs;
	return mine !== undefined && theirs !== undefined && Math.abs(mine - theirs) > pairSlackMs;
}

/** What starts ticked: everything that differs, but genres; a song only when its pair looks right. */
export function initialChoices(basis: Basis): Choices {
	const choices: Choices = { pairs: {}, songs: {}, fields: {}, off: {} };
	for (const { track, release } of basis.review.pairs) {
		const pair = release ?? null;
		choices.pairs[track] = pair;
		choices.songs[track] = pair !== null && !suspect(basis, track, pair);
	}
	for (const row of fieldRows(basis, choices).rows) {
		choices.fields[row.key] = !askFirst.includes(row.field);
	}
	return choices;
}

/** What the songs hold of a field now: one value list, or null when they differ. */
function heldBy(basis: Basis, ids: readonly string[], field: TagField): string[] | null {
	let held: string[] | undefined;
	for (const id of ids) {
		const values = trackOf(basis, id)?.fields[field] ?? [];
		if (held === undefined) {
			held = values;
		} else if (!same(held, values)) {
			return null;
		}
	}
	return held ?? [];
}

/** The album fields that would change, and the names of those that already agree. */
export function fieldRows(
	basis: Basis,
	choices: Choices
): { rows: FieldRow[]; agreeing: string[] } {
	const ids = songIds(basis);
	const rows: FieldRow[] = [];
	const agreeing: string[] = [];
	const add = (
		key: string,
		field: TagField,
		label: string,
		on: readonly string[],
		next: string[]
	) => {
		const differs = on.some((id) => !same(trackOf(basis, id)?.fields[field], next));
		if (!differs) {
			agreeing.push(label);
			return;
		}
		rows.push({
			key,
			field,
			label,
			now: heldBy(basis, on, field),
			next,
			on: choices.fields[key] ?? !askFirst.includes(field),
			...(field === 'genres' ? { note: 'Genres stay yours unless you tick them.' } : {})
		});
	};
	for (const { field, label } of albumFields) {
		const next = basis.review.album[field];
		if (next !== undefined && next.length > 0) {
			add(field, field, label, ids, next);
		}
	}
	// A disc's name goes to the songs paired onto that disc.
	const named = new Map<number, string[]>();
	for (const t of basis.review.tracks) {
		const name = t.proposed.discSubtitle;
		if (name !== undefined && !named.has(t.disc)) {
			named.set(t.disc, name);
		}
	}
	for (const [disc, next] of named) {
		const on = ids.filter((id) => discOf(basis, choices, id) === disc);
		if (on.length > 0) {
			add(`discSubtitle:${String(disc)}`, 'discSubtitle', `Disc ${String(disc)} name`, on, next);
		}
	}
	const genres = rows.findIndex((row) => row.field === 'genres');
	if (genres >= 0) {
		rows.push(...rows.splice(genres, 1)); // Genres last, as the board has them.
	}
	return { rows, agreeing };
}

/** The release disc a song is paired onto. */
function discOf(basis: Basis, choices: Choices, id: string): number | undefined {
	const pair = choices.pairs[id] ?? null;
	return pair === null ? undefined : basis.review.tracks[pair]?.disc;
}

const two = (n: string | undefined): string => (n ?? '').padStart(2, '0');

/** A song's place as the board writes it: disc and number, `1·03`. */
export function placeOf(fields: FieldValues): string {
	const track = fields.track?.at(0);
	if (track === undefined) {
		return '—';
	}
	return `${fields.disc?.at(0) ?? '1'}·${two(track)}`;
}

/** A song's place with its totals, when they matter. */
function numbersOf(fields: FieldValues, totals: boolean): string {
	const place = placeOf(fields);
	if (!totals) {
		return place;
	}
	const of = fields.trackTotal?.at(0);
	const discs = fields.discTotal?.at(0);
	return `${place}${of === undefined ? '' : ` of ${of}`}${discs === undefined ? '' : `, ${discs} discs`}`;
}

/** The changes a song would take from the release track it pairs with. */
function changesOf(now: FieldValues, next: FieldValues, id: string, choices: Choices): Change[] {
	const out: Change[] = [];
	for (const { kind, fields } of songKinds) {
		const differs = fields.some((f) => next[f] !== undefined && !same(now[f], next[f]));
		if (!differs) {
			continue;
		}
		const on = choices.off[`${id}:${kind}`] !== true;
		if (kind === 'numbers') {
			// Totals show only when the places agree, so the change reads.
			const totals = placeOf(now) === placeOf({ ...now, ...pick(next, fields) });
			out.push({
				kind,
				now: numbersOf(now, totals),
				next: numbersOf({ ...now, ...pick(next, fields) }, totals),
				on
			});
		} else {
			const field = fields[0] ?? 'title';
			out.push({
				kind,
				now: (now[field] ?? []).join(', '),
				next: (next[field] ?? []).join(', '),
				on
			});
		}
	}
	return out;
}

function pick(values: FieldValues, fields: readonly TagField[]): FieldValues {
	const out: FieldValues = {};
	for (const f of fields) {
		if (values[f] !== undefined) {
			out[f] = values[f];
		}
	}
	return out;
}

/** Every song of the album, in album order, with its pair and what it would take. */
export function songRows(basis: Basis, choices: Choices): SongRow[] {
	return songIds(basis).map((id) => {
		const tags = trackOf(basis, id);
		const now = tags?.fields ?? {};
		const pair = choices.pairs[id] ?? null;
		const release = pair === null ? null : (basis.review.tracks[pair] ?? null);
		return {
			id,
			title: now.title?.at(0) ?? tags?.file ?? '',
			artists: (now.artists ?? []).join(', '),
			position: placeOf(now),
			pair,
			release,
			lengthMs: basis.lengths[id] ?? 0,
			suspect: suspect(basis, id, pair),
			on: release !== null && choices.songs[id] === true,
			changes: release === null ? [] : changesOf(now, release.proposed, id, choices),
			disc: release?.disc ?? Number(now.disc?.at(0) ?? 1)
		};
	});
}

/** How a pill stands: all its changes on, some, or none. */
export type PillState = 'all' | 'some' | 'none';

/** Each kind's changes across the songs: how many, how many on, and so how its pill stands. */
export function kindStates(
	rows: readonly SongRow[]
): Record<SongKind, { total: number; on: number; state: PillState }> {
	const out = {} as Record<SongKind, { total: number; on: number; state: PillState }>;
	for (const { kind } of songKinds) {
		const changes = rows
			.filter((r) => r.release !== null)
			.flatMap((r) => r.changes.filter((c) => c.kind === kind).map((c) => c.on && r.on));
		const on = changes.filter(Boolean).length;
		out[kind] = {
			total: changes.length,
			on,
			state: on === 0 ? 'none' : on === changes.length ? 'all' : 'some'
		};
	}
	return out;
}

/** Ticks or unticks a kind across every song: a pill. Unticked songs it turns on are ticked. */
export function setKind(basis: Basis, choices: Choices, kind: SongKind, on: boolean): Choices {
	const next: Choices = { ...choices, off: { ...choices.off }, songs: { ...choices.songs } };
	for (const row of songRows(basis, choices)) {
		if (row.changes.some((c) => c.kind === kind)) {
			next.off[`${row.id}:${kind}`] = !on;
			if (on && !row.suspect && row.release !== null) {
				next.songs[row.id] = true;
			}
		}
	}
	return next;
}

/** Ticks or unticks one song's change of one kind. */
export function setChange(choices: Choices, id: string, kind: SongKind, on: boolean): Choices {
	return { ...choices, off: { ...choices.off, [`${id}:${kind}`]: !on } };
}

/** Ticks or unticks a whole song: its file is written or left alone. */
export function setSong(choices: Choices, id: string, on: boolean): Choices {
	return { ...choices, songs: { ...choices.songs, [id]: on } };
}

/** Ticks or unticks an album row. */
export function setField(choices: Choices, key: string, on: boolean): Choices {
	return { ...choices, fields: { ...choices.fields, [key]: on } };
}

/** Ticks every album row, or none, as the box at the head of them does. */
export function setFields(basis: Basis, choices: Choices, on: boolean): Choices {
	const fields = { ...choices.fields };
	for (const row of fieldRows(basis, choices).rows) {
		fields[row.key] = on;
	}
	return { ...choices, fields };
}

/**
 * Pairs a song with another release track, or with none. A song already holding that track takes
 * the first song's old one, so the two swap. Each is ticked when its new pair's lengths agree.
 */
export function repair(
	basis: Basis,
	choices: Choices,
	id: string,
	release: number | null
): Choices {
	const pairs = { ...choices.pairs };
	const songs = { ...choices.songs };
	const was = pairs[id] ?? null;
	if (release !== null) {
		const holder = Object.keys(pairs).find((other) => other !== id && pairs[other] === release);
		if (holder !== undefined) {
			pairs[holder] = was;
			songs[holder] = was !== null && !suspect(basis, holder, was);
		}
	}
	pairs[id] = release;
	songs[id] = release !== null && !suspect(basis, id, release);
	return { ...choices, pairs, songs };
}

/** The release tracks a song could pair with, best first: lengths that agree, then nearest. */
export function pairOptions(
	basis: Basis,
	id: string
): { index: number; track: MatchTrack; gapMs: number | null }[] {
	const mine = basis.lengths[id];
	return basis.review.tracks
		.map((track, index) => ({
			index,
			track,
			gapMs: mine === undefined || track.lengthMs === undefined ? null : track.lengthMs - mine
		}))
		.sort((a, b) => Math.abs(a.gapMs ?? Infinity) - Math.abs(b.gapMs ?? Infinity));
}

/** What Apply would do, as the bar says it. */
export interface Summary {
	fields: number;
	songChanges: number;
	files: number;
	of: number;
	/** Songs left as they are: no pair, or unticked. */
	waiting: number;
	/** An album or album artist change leaves some songs out, so they'd show as an album of their own. */
	splits: boolean;
}

export function summaryOf(basis: Basis, choices: Choices): Summary {
	const rows = songRows(basis, choices);
	const written = rows.filter((r) => r.on);
	const ticked = fieldRows(basis, choices).rows.filter((r) => r.on);
	return {
		fields: ticked.length,
		songChanges: written.reduce((n, r) => n + r.changes.filter((c) => c.on).length, 0),
		files: written.length,
		of: rows.length,
		waiting: rows.length - written.length,
		splits:
			written.length < rows.length &&
			written.length > 0 &&
			ticked.some((r) => grouping.includes(r.field))
	};
}

/** The IDs a song takes with it: always, but artists' only where they line up with its artists. */
function idsFor(
	basis: Basis,
	release: MatchTrack,
	set: FieldValues,
	now: FieldValues
): FieldValues {
	const ids: FieldValues = {};
	const { albumIds } = basis.review;
	for (const field of ['musicbrainzAlbumId', 'musicbrainzReleaseGroupId'] as const) {
		if (albumIds[field] !== undefined) {
			ids[field] = albumIds[field];
		}
	}
	for (const field of ['musicbrainzRecordingId', 'musicbrainzTrackId'] as const) {
		if (release.ids[field] !== undefined) {
			ids[field] = release.ids[field];
		}
	}
	const albumArtists = set.albumArtists ?? now.albumArtists ?? [];
	const albumArtistIds = albumIds.musicbrainzAlbumArtistId;
	if (albumArtistIds !== undefined && albumArtistIds.length === albumArtists.length) {
		ids.musicbrainzAlbumArtistId = albumArtistIds;
	}
	const artists = set.artists ?? now.artists ?? [];
	const artistIds = release.ids.musicbrainzArtistId;
	if (artistIds !== undefined && artistIds.length === artists.length) {
		ids.musicbrainzArtistId = artistIds;
	}
	return ids;
}

/**
 * The edit Apply sends: one per ticked song, and nothing for any other, so an unticked song's file
 * is left completely alone. Each takes the ticked album rows, its ticked changes, and the IDs.
 * Values a file already holds are left out.
 */
export function editOf(basis: Basis, choices: Choices): AlbumEdit {
	const ticked = fieldRows(basis, choices).rows.filter((r) => r.on);
	const tracks: ({ id: string } & { set: FieldValues })[] = [];
	for (const row of songRows(basis, choices)) {
		if (!row.on || row.release === null) {
			continue;
		}
		const now = trackOf(basis, row.id)?.fields ?? {};
		const set: FieldValues = {};
		for (const field of ticked) {
			if (
				field.field !== 'discSubtitle' ||
				field.key === `discSubtitle:${String(row.release.disc)}`
			) {
				set[field.field] = field.next;
			}
		}
		for (const change of row.changes) {
			if (change.on) {
				const kind = songKinds.find((k) => k.kind === change.kind);
				Object.assign(set, pick(row.release.proposed, kind?.fields ?? []));
			}
		}
		Object.assign(set, idsFor(basis, row.release, set, now));
		const changed = Object.fromEntries(
			Object.entries(set).filter(([field, values]) => !same(values, now[field as TagField]))
		) as FieldValues;
		if (Object.keys(changed).length > 0) {
			tracks.push({ id: row.id, set: changed });
		}
	}
	return { tracks };
}
