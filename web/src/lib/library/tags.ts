import {
	EditError,
	type AlbumEdit,
	type AlbumTags,
	type EditFailure,
	type FieldValues,
	type TagEdit,
	type TagField,
	type TrackTags
} from './source.ts';

/** What the server takes: M4A holds a track or disc number in 16 bits. */
const mostNumber = 65_535;

/** What the numbered fields are called in a sentence. */
const numberNames: Partial<Record<TagField, string>> = {
	track: 'track number',
	trackTotal: 'track total',
	disc: 'disc number',
	discTotal: 'disc total'
};

const dateFields: readonly TagField[] = ['date', 'originalDate'];

/** The fields that may hold several values, as the server's table has them. */
const multiFields: readonly TagField[] = [
	'artists',
	'albumArtists',
	'genres',
	'composers',
	'conductor',
	'lyricist',
	'performers',
	'remixer',
	'label',
	'catalogNumber',
	'barcode',
	'isrc',
	'language'
];

/** Every field an edit can name, in the order of the page. */
const pageOrder: readonly TagField[] = [
	'disc',
	'track',
	'title',
	'trackTotal',
	'discTotal',
	'discSubtitle',
	'date',
	'genres',
	'composers',
	'conductor',
	'lyricist',
	'performers',
	'remixer',
	'grouping',
	'label',
	'catalogNumber',
	'barcode',
	'isrc',
	'originalDate',
	'language',
	'bpm',
	'key',
	'copyright',
	'comment',
	'sortTitle',
	'sortArtist',
	'sortAlbum',
	'sortAlbumArtist',
	'lyrics'
];

/** One field of a track's whole tag list. */
export interface FieldSpec {
	field: TagField;
	label: string;
	/** What it's read out as, where the label alone says too little. */
	name?: string;
	/** How many of the group's four columns it takes. */
	span: 1 | 2;
	kind?: 'number';
	/** Shown and not edited here. */
	locked?: boolean;
}

/** Every field a track shows, by group. Label, group and kind are the client's to say. */
export const tagGroups: readonly { name: string; fields: readonly FieldSpec[] }[] = [
	{
		name: 'THE BASICS',
		fields: [
			{ field: 'title', label: 'Title', span: 2 },
			{ field: 'artists', label: 'Artists', span: 2 },
			{ field: 'album', label: 'Album', span: 2 },
			{ field: 'albumArtists', label: 'Album artist', span: 2 },
			{ field: 'track', label: 'Track', span: 1, kind: 'number' },
			{ field: 'trackTotal', label: 'of', name: 'Track total', span: 1, kind: 'number' },
			{ field: 'genres', label: 'Genres', span: 2 },
			{ field: 'disc', label: 'Disc', span: 1, kind: 'number' },
			{ field: 'discTotal', label: 'of', name: 'Disc total', span: 1, kind: 'number' },
			{ field: 'discSubtitle', label: 'Disc name', span: 2 },
			{ field: 'date', label: 'Date', span: 2 },
			{ field: 'compilation', label: 'Compilation', span: 2, locked: true }
		]
	},
	{
		name: 'CREDITS',
		fields: [
			{ field: 'composers', label: 'Composers', span: 2 },
			{ field: 'conductor', label: 'Conductor', span: 1 },
			{ field: 'lyricist', label: 'Lyricist', span: 1 },
			{ field: 'performers', label: 'Performers', span: 2 },
			{ field: 'remixer', label: 'Remixer', span: 1 },
			{ field: 'grouping', label: 'Grouping', span: 1 }
		]
	},
	{
		name: 'RELEASE',
		fields: [
			{ field: 'label', label: 'Label', span: 1 },
			{ field: 'catalogNumber', label: 'Catalog number', span: 1 },
			{ field: 'barcode', label: 'Barcode', span: 1 },
			{ field: 'isrc', label: 'ISRC', span: 1 },
			{ field: 'originalDate', label: 'Original date', span: 1 },
			{ field: 'language', label: 'Language', span: 1 },
			{ field: 'bpm', label: 'BPM', span: 1 },
			{ field: 'key', label: 'Key', span: 1 },
			{ field: 'copyright', label: 'Copyright', span: 2 },
			{ field: 'comment', label: 'Comment', span: 2 }
		]
	},
	{
		name: 'SORTING',
		fields: [
			{ field: 'sortTitle', label: 'Sort title', span: 1 },
			{ field: 'sortArtist', label: 'Sort artist', span: 1 },
			{ field: 'sortAlbum', label: 'Sort album', span: 1 },
			{ field: 'sortAlbumArtist', label: 'Sort album artist', span: 1 }
		]
	}
];

/** What someone has touched on one track, as typed. */
export interface TrackDraft {
	fields: FieldValues;
	/** By the key the server showed. No values removes the tag. */
	tags: Record<string, string[]>;
}

/**
 * What someone has touched, as typed, and nothing else: whatever isn't here reads through to the
 * files. Year and genres on the album page go to every track, and a track's own value goes on top,
 * as `POST /albums/{id}/edits` layers them.
 */
export interface Draft {
	/** Null until touched. */
	year: string | null;
	genres: string[] | null;
	tracks: Record<string, TrackDraft>;
}

/** What the album page shows of the fields it edits for every track at once. */
export interface AlbumWide {
	/** Empty when the tracks have none, or differ. */
	year: string;
	yearsDiffer: boolean;
	/** Every genre of the tracks, each once, in track order. */
	genres: string[];
	genresDiffer: boolean;
}

/** What a save would do, as the bar counts it. An edit of every track is one change. */
export interface Summary {
	changes: number;
	files: number;
	/** The one track every change is on, if there is one. */
	only: string | null;
}

/** What a save needs from the library, which also keeps everything on screen up to date. */
export interface Tagger {
	albumTags(id: string): Promise<AlbumTags>;
	editAlbum(id: string, etag: string, edit: AlbumEdit): Promise<void>;
	reread(id: string): Promise<void>;
}

export type SaveStop =
	| EditFailure
	/** The ETag read back is the one refused: the store is behind a file, and only a scan helps. */
	| { kind: 'drifted' };

export interface Saved {
	stop: SaveStop | null;
	/** The files as they are now, when what's held turned out to be behind. */
	fresh: AlbumTags | null;
}

/** What to tell someone about a save that stopped. */
export interface Report {
	message: string;
	/** The server's own words, when they say more. */
	detail: string | undefined;
	/** The tracks that hold the edit, by title. */
	written: string[];
	/** The tracks that don't. */
	unwritten: string[];
	/** Whether writing again could work. */
	retry: boolean;
}

/** The problem key of the album page's year. */
export const yearField = 'year';

/** Names a field of a track, as an element ID and as the key of a problem. */
export function fieldId(trackId: string, field: TagField): string {
	return `${field}-${trackId}`;
}

/** Names a raw tag of a track, the same way. */
export function tagId(trackId: string, key: string): string {
	return `tag-${key}-${trackId}`;
}

export function emptyDraft(): Draft {
	return { year: null, genres: null, tracks: {} };
}

/** Reports whether a field holds a list, shown as chips and never as joined text. */
export function isMulti(field: TagField): boolean {
	return multiFields.includes(field);
}

function cleaned(values: readonly string[]): string[] {
	return values.map((value) => value.trim()).filter((value) => value !== '');
}

function sameList(a: readonly string[], b: readonly string[]): boolean {
	return a.length === b.length && a.every((item, index) => item === b[index]);
}

function isWhole(value: string): boolean {
	return /^\d+$/.test(value);
}

/** Whether two sets of values mean the same to the field: `01` is track 1, whatever's typed. */
function same(field: TagField, a: readonly string[], b: readonly string[]): boolean {
	const [one, other] = [cleaned(a), cleaned(b)];
	if (multiFields.includes(field)) {
		return sameList(one, other);
	}
	const [first = '', second = ''] = [one.at(0), other.at(0)];
	return field in numberNames && isWhole(first) && isWhole(second)
		? Number(first) === Number(second)
		: first === second;
}

function yearOf(values: readonly string[] | undefined): string {
	return /^\d{4}/.exec(values?.at(0)?.trim() ?? '')?.[0] ?? '';
}

const wides = new WeakMap<AlbumTags, AlbumWide>();
const byId = new WeakMap<AlbumTags, Map<string, TrackTags>>();

/** Reads what the tracks agree on, for the album page's year and genres. Tags never change. */
export function albumWide(tags: AlbumTags): AlbumWide {
	const held = wides.get(tags);
	if (held !== undefined) {
		return held;
	}
	const { tracks } = tags;
	const years = new Set(tracks.map(({ fields }) => yearOf(fields.date)));
	const [year = ''] = years;
	const genres = [...new Set(tracks.flatMap(({ fields }) => fields.genres ?? []))];
	const wide = {
		year: years.size === 1 ? year : '',
		yearsDiffer: years.size > 1,
		genres,
		genresDiffer: tracks.some((track) => (track.fields.genres ?? []).length !== genres.length)
	};
	wides.set(tags, wide);
	return wide;
}

function yearChanged(draft: Draft, wide: AlbumWide): boolean {
	return draft.year !== null && draft.year.trim() !== wide.year;
}

function genresChanged(draft: Draft, wide: AlbumWide): boolean {
	return draft.genres !== null && !sameList(cleaned(draft.genres), wide.genres);
}

/** What the album page's edit gives a field of every track, if it gives it anything. */
function acrossAlbum(draft: Draft, wide: AlbumWide, field: TagField): string[] | undefined {
	if (field === 'date' && yearChanged(draft, wide)) {
		return cleaned([draft.year ?? '']);
	}
	if (field === 'genres' && genresChanged(draft, wide)) {
		return cleaned(draft.genres ?? []);
	}
	return undefined;
}

function trackIn(tags: AlbumTags, id: string): TrackTags | undefined {
	const tracks = byId.get(tags) ?? new Map(tags.tracks.map((track) => [track.id, track]));
	byId.set(tags, tracks);
	return tracks.get(id);
}

/** What a field holds in the file. */
export function wasOf(tags: AlbumTags, id: string, field: TagField): string[] {
	return trackIn(tags, id)?.fields[field] ?? [];
}

/** What a field will hold if nothing more of it is touched on this track. */
function beneath(tags: AlbumTags, draft: Draft, id: string, field: TagField): string[] {
	return acrossAlbum(draft, albumWide(tags), field) ?? wasOf(tags, id, field);
}

/** What a field shows now. */
export function valueOf(tags: AlbumTags, draft: Draft, id: string, field: TagField): string[] {
	return draft.tracks[id]?.fields[field] ?? beneath(tags, draft, id, field);
}

/** Whether a field shows something the file doesn't hold. */
export function isChanged(tags: AlbumTags, draft: Draft, id: string, field: TagField): boolean {
	return !same(field, valueOf(tags, draft, id, field), wasOf(tags, id, field));
}

/** What a row of the album page says it was: the old title, and any old numbers. */
export function rowNote(tags: AlbumTags, draft: Draft, id: string): { old: string; text: string } {
	const number = (field: 'disc' | 'track'): string[] => {
		if (!isChanged(tags, draft, id, field)) {
			return [];
		}
		const [was] = wasOf(tags, id, field);
		return [was === undefined ? `had no ${field} number` : `was ${field} ${was}`];
	};
	const [old = ''] = isChanged(tags, draft, id, 'title') ? wasOf(tags, id, 'title') : [];
	return { old, text: [...number('disc'), ...number('track')].join(' · ') };
}

function tagWas(tags: AlbumTags, id: string, key: string): string[] {
	return trackIn(tags, id)?.tags.find((tag) => tag.key === key)?.values ?? [];
}

/** What a raw tag shows now. */
export function tagValueOf(tags: AlbumTags, draft: Draft, id: string, key: string): string[] {
	return draft.tracks[id]?.tags[key] ?? tagWas(tags, id, key);
}

/** Whether a raw tag shows something the file doesn't hold. */
export function isTagChanged(tags: AlbumTags, draft: Draft, id: string, key: string): boolean {
	return !sameList(cleaned(tagValueOf(tags, draft, id, key)), cleaned(tagWas(tags, id, key)));
}

function without<V>(record: Partial<Record<string, V>>, key: string): Record<string, V> {
	return Object.fromEntries(Object.entries(record).filter(([name]) => name !== key)) as Record<
		string,
		V
	>;
}

function withTrack(draft: Draft, id: string, track: TrackDraft): Draft {
	const others = without(draft.tracks, id);
	const blank = Object.keys(track.fields).length === 0 && Object.keys(track.tags).length === 0;
	return { ...draft, tracks: blank ? others : { ...others, [id]: track } };
}

/** Takes what's typed into a track's field. Typing the old value back forgets the field. */
export function withField(
	tags: AlbumTags,
	draft: Draft,
	id: string,
	field: TagField,
	values: string[]
): Draft {
	const others = without(draft.tracks[id]?.fields ?? {}, field);
	const undone = sameList(values, beneath(tags, draft, id, field));
	return withTrack(draft, id, {
		tags: draft.tracks[id]?.tags ?? {},
		fields: undone ? others : { ...others, [field]: values }
	});
}

/** Takes what's typed into a raw tag, the same way. */
export function withTag(
	tags: AlbumTags,
	draft: Draft,
	id: string,
	key: string,
	values: string[]
): Draft {
	const others = without(draft.tracks[id]?.tags ?? {}, key);
	const undone = sameList(values, tagWas(tags, id, key));
	return withTrack(draft, id, {
		fields: draft.tracks[id]?.fields ?? {},
		tags: undone ? others : { ...others, [key]: values }
	});
}

/** What a track's own edit says, field by field: only what differs from what lies beneath it. */
function ownFields(tags: AlbumTags, draft: Draft, id: string): [TagField, string[]][] {
	const fields = draft.tracks[id]?.fields ?? {};
	return pageOrder.flatMap((field) => {
		const values = fields[field];
		return values === undefined || same(field, values, beneath(tags, draft, id, field))
			? []
			: [[field, cleaned(values)] as [TagField, string[]]];
	});
}

function ownTags(tags: AlbumTags, draft: Draft, id: string): [string, string[]][] {
	return Object.entries(draft.tracks[id]?.tags ?? {}).flatMap(([key, values]) =>
		sameList(cleaned(values), cleaned(tagWas(tags, id, key))) ? [] : [[key, cleaned(values)]]
	) as [string, string[]][];
}

/** Reports whether a track's own edit changes anything, shown on its row or not. */
export function isTrackEdited(tags: AlbumTags, draft: Draft, id: string): boolean {
	return ownFields(tags, draft, id).length > 0 || ownTags(tags, draft, id).length > 0;
}

function isDate(value: string): boolean {
	const [, year, month, day] = /^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?$/.exec(value) ?? [];
	if (year === undefined || Number(year) === 0) {
		return false;
	}
	const y = Number(year);
	const m = Number(month ?? 1);
	const d = Number(day ?? 1);
	const date = new Date(Date.UTC(y, m - 1, d));
	date.setUTCFullYear(y);
	return date.getUTCMonth() === m - 1 && date.getUTCDate() === d;
}

function problemWith(field: TagField, values: readonly string[]): string | undefined {
	const [value] = values;
	if (value === undefined) {
		return field === 'title' ? 'A track needs a title.' : undefined;
	}
	const name = numberNames[field];
	if (name !== undefined && (!isWhole(value) || Number(value) < 1 || Number(value) > mostNumber)) {
		return `A ${name} runs from 1 to ${String(mostNumber)}.`;
	}
	if (dateFields.includes(field) && !isDate(value)) {
		return 'A date is YYYY, YYYY-MM or YYYY-MM-DD.';
	}
	return undefined;
}

/**
 * Finds what the server would refuse, by field, in the order of the page. Only what changed counts:
 * a tag that's wrong already can stay wrong.
 */
export function problemsIn(tags: AlbumTags, draft: Draft): Map<string, string> {
	const problems = new Map<string, string>();
	if (yearChanged(draft, albumWide(tags)) && !/^(?!0000)\d{4}$|^$/.test(draft.year?.trim() ?? '')) {
		problems.set(yearField, 'A year is four digits.');
	}
	for (const { id } of tags.tracks) {
		for (const [field, values] of ownFields(tags, draft, id)) {
			const problem = problemWith(field, values);
			if (problem !== undefined) {
				problems.set(fieldId(id, field), problem);
			}
		}
	}
	return problems;
}

/** Counts what a save would write. A field with a problem won't be written, so it doesn't count. */
export function summaryOf(tags: AlbumTags, draft: Draft): Summary {
	const wide = albumWide(tags);
	const problems = problemsIn(tags, draft);
	const across =
		Number(yearChanged(draft, wide) && !problems.has(yearField)) +
		Number(genresChanged(draft, wide));
	const counts = tags.tracks.flatMap(({ id }) => {
		const count =
			ownFields(tags, draft, id).filter(([field]) => !problems.has(fieldId(id, field))).length +
			ownTags(tags, draft, id).length;
		return count === 0 ? [] : [{ id, count }];
	});
	const [first] = counts;
	return {
		changes: across + counts.reduce((sum, { count }) => sum + count, 0),
		files: across > 0 ? tags.tracks.length : counts.length,
		only: across === 0 && counts.length === 1 && first !== undefined ? first.id : null
	};
}

function tagEdit(fields: [TagField, string[]][], tags: [string, string[]][] = []): TagEdit {
	const set = fields.filter(([, values]) => values.length > 0);
	const clear = fields.filter(([, values]) => values.length === 0).map(([field]) => field);
	return {
		...(set.length > 0 ? { set: Object.fromEntries(set) } : {}),
		...(clear.length > 0 ? { clear } : {}),
		...(tags.length > 0 ? { tags: tags.map(([key, values]) => ({ key, values })) } : {})
	};
}

function isEmpty(edit: object): boolean {
	return Object.keys(edit).length === 0;
}

/** Builds the one request that turns the files into the draft, or null if there's nothing to do. */
export function editOf(tags: AlbumTags, draft: Draft): AlbumEdit | null {
	const wide = albumWide(tags);
	const all = tagEdit([
		...(yearChanged(draft, wide)
			? ([['date', cleaned([draft.year ?? ''])]] as [TagField, string[]][])
			: []),
		...(genresChanged(draft, wide)
			? ([['genres', cleaned(draft.genres ?? [])]] as [TagField, string[]][])
			: [])
	]);
	const tracks = tags.tracks.flatMap(({ id }) => {
		const edit = tagEdit(ownFields(tags, draft, id), ownTags(tags, draft, id));
		return isEmpty(edit) ? [] : [{ id, ...edit }];
	});
	if (isEmpty(all) && tracks.length === 0) {
		return null;
	}
	return { ...(isEmpty(all) ? {} : { all }), ...(tracks.length > 0 ? { tracks } : {}) };
}

/**
 * Carries someone's edits over to files that moved on: what they touched stays theirs, and what
 * can't be touched any more goes. Everything else reads through to the new files by itself.
 */
export function rebased(draft: Draft, fresh: AlbumTags): Draft {
	const tracks: Record<string, TrackDraft> = {};
	for (const track of fresh.tracks) {
		const held = draft.tracks[track.id];
		if (held === undefined) {
			continue;
		}
		const fields = Object.fromEntries(
			Object.entries(held.fields).filter(([field]) => !track.readOnly?.includes(field as TagField))
		);
		const open = track.tags.filter(({ readOnly = false }) => !readOnly).map(({ key }) => key);
		const kept = Object.entries(held.tags).filter(([key]) => open.includes(key));
		if (Object.keys(fields).length > 0 || kept.length > 0) {
			tracks[track.id] = { fields, tags: Object.fromEntries(kept) };
		}
	}
	return { ...draft, tracks };
}

function isStopped(failure: SaveStop): failure is Extract<EditFailure, { changed: string[] }> {
	return 'changed' in failure;
}

/** Whether a stop left some files holding the edit. */
function isPartway(failure: SaveStop): boolean {
	return isStopped(failure) && failure.changed.length > 0;
}

/** Finds out what a failed edit left. Anything but a plain refusal means what's held is behind. */
async function aftermath(
	tagger: Tagger,
	albumId: string,
	sent: string,
	failure: EditFailure
): Promise<Saved> {
	const partway = isPartway(failure);
	if (failure.kind !== 'stale' && !partway) {
		return { stop: failure, fresh: null };
	}
	try {
		const [fresh] = await Promise.all([tagger.albumTags(albumId), tagger.reread(albumId)]);
		const drifted = !partway && fresh.etag === sent;
		return { stop: drifted ? { kind: 'drifted' } : failure, fresh };
	} catch {
		return { stop: partway ? failure : { kind: 'unavailable' }, fresh: null };
	}
}

/**
 * Sends one edit, on the ETag the tags came with. It never rejects: a lost session has already gone
 * to the door. The library follows the files however far the edit got.
 */
export async function save(
	tagger: Tagger,
	albumId: string,
	etag: string,
	edit: AlbumEdit
): Promise<Saved> {
	try {
		await tagger.editAlbum(albumId, etag, edit);
		return { stop: null, fresh: null };
	} catch (error) {
		const failure = error instanceof EditError ? error.failure : { kind: 'unavailable' as const };
		return aftermath(tagger, albumId, etag, failure);
	}
}

function seconds(count: number): string {
	return `${String(count)} ${count === 1 ? 'second' : 'seconds'}`;
}

function messageFor(stop: SaveStop): string {
	const partway = isPartway(stop);
	switch (stop.kind) {
		case 'busy':
			return `A scan or another edit is running. Your edits are still here: try again in ${seconds(stop.retryAfterSeconds)}.`;
		case 'stale':
			return partway
				? 'A file changed while Kitsune was writing. Your edits are still here: check them and write again.'
				: 'This album changed while you were editing. Your edits are still here: check them and write again.';
		case 'drifted':
			return 'A file changed outside Kitsune. It needs a scan before it can be edited.';
		case 'forbidden':
			return "This server doesn't allow tag edits.";
		case 'gone':
			return "This album isn't in the library any more.";
		case 'missing':
			return 'A file of this album is gone. It needs a scan before it can be edited.';
		case 'refused':
			return partway
				? "A file wouldn't take this edit, so the write stopped partway."
				: "The files won't take this edit. Nothing was written.";
		case 'unwritable':
			return partway
				? "Kitsune can't write to a file, so the write stopped partway."
				: "Kitsune can't write to these files. Nothing was written.";
		case 'failed':
			return 'Something went wrong writing a file. Every file it reached is still valid.';
		case 'unavailable':
			return "Couldn't reach Kitsune, so this edit may not have landed. Check the album, then try again.";
	}
}

/** Writes up a save that stopped, naming tracks by `titleOf`. */
export function reportOf(stop: SaveStop, titleOf: (id: string) => string): Report {
	const partway = isPartway(stop);
	const stopped = isStopped(stop) ? stop : undefined;
	return {
		message: messageFor(stop),
		detail: stop.kind === 'refused' || stop.kind === 'unwritable' ? stop.detail : undefined,
		written: partway ? (stopped?.changed.map(titleOf) ?? []) : [],
		unwritten: partway ? (stopped?.remaining.map(titleOf) ?? []) : [],
		retry: !['drifted', 'forbidden', 'gone', 'missing'].includes(stop.kind)
	};
}
