import type { AlbumTags, TagEdit, TagField, TrackTags, TracksEdit } from './source.ts';

/** What the server takes: M4A holds a track or disc number in 16 bits. */
const mostNumber = 65_535;

/** A field of many songs at once: board D's eleven, in its order, and the disc's name. */
export type BulkKey =
	| 'title'
	| 'album'
	| 'albumArtists'
	| 'artists'
	| 'composers'
	| 'genres'
	| 'year'
	| 'track'
	| 'disc'
	| 'discSubtitle'
	| 'label'
	| 'comment';

export interface BulkSpec {
	key: BulkKey;
	/** The field it writes. A year replaces the whole date. */
	field: TagField;
	label: string;
	/** How many of the two columns it takes. */
	span: 1 | 2;
	/** A list, shown as chips. */
	multi: boolean;
	number: boolean;
}

function spec(key: BulkKey, label: string, more: Partial<BulkSpec> = {}): BulkSpec {
	return { key, field: key as TagField, label, span: 1, multi: false, number: false, ...more };
}

/** Board D's fields, and the disc's name beside its number. */
export const bulkFields: readonly BulkSpec[] = [
	spec('title', 'Title', { span: 2 }),
	spec('album', 'Album'),
	spec('albumArtists', 'Album artist', { multi: true }),
	spec('artists', 'Artists', { multi: true }),
	spec('composers', 'Composers', { multi: true }),
	spec('genres', 'Genres', { multi: true }),
	spec('year', 'Year', { field: 'date' }),
	spec('track', 'Track', { number: true }),
	spec('disc', 'Disc', { number: true }),
	spec('discSubtitle', 'Disc name', { span: 2 }),
	spec('label', 'Label', { multi: true }),
	spec('comment', 'Comment')
];

const specOf = new Map(bulkFields.map((each) => [each.key, each]));

/** One song of a bulk edit: its file's tags and the album it's on. */
export interface BulkTrack {
	id: string;
	albumId: string;
	tags: TrackTags;
}

/** The fields a quick job fills per song. */
export type OwnKey = 'title' | 'track';

/**
 * What someone has touched, and nothing else. A field typed in the form goes to every song picked;
 * a quick job's values go to each song on its own, on top.
 */
export interface BulkDraft {
	fields: Partial<Record<BulkKey, string[]>>;
	own: Record<string, Partial<Record<OwnKey, string>>>;
}

export function emptyBulkDraft(): BulkDraft {
	return { fields: {}, own: {} };
}

/** Every song the albums hold, in album order, albums in the order given. */
export function bulkTracks(albums: readonly { id: string; tags: AlbumTags }[]): BulkTrack[] {
	return albums.flatMap(({ id, tags }) =>
		tags.tracks.map((track) => ({ id: track.id, albumId: id, tags: track }))
	);
}

function cleaned(values: readonly string[]): string[] {
	return values.map((value) => value.trim()).filter((value) => value !== '');
}

function sameList(a: readonly string[], b: readonly string[]): boolean {
	return a.length === b.length && a.every((item, index) => item === b[index]);
}

function yearOf(values: readonly string[] | undefined): string[] {
	const year = /^\d{4}/.exec(values?.at(0)?.trim() ?? '')?.[0];
	return year === undefined ? [] : [year];
}

/** What the file holds of a field, as the form reads it: a date as its year. */
function fileValue(track: BulkTrack, key: BulkKey): string[] {
	const { field } = specOf.get(key) ?? spec(key, key);
	const values = track.tags.fields[field];
	return key === 'year' ? yearOf(values) : cleaned(values ?? []);
}

/** Whether two values mean the same to a field: `01` is track 1. */
function same(key: BulkKey, a: readonly string[], b: readonly string[]): boolean {
	const [one, other] = [cleaned(a), cleaned(b)];
	if (specOf.get(key)?.number === true && one.length === 1 && other.length === 1) {
		return Number(one[0]) === Number(other[0]) && /^\d+$/.test(one[0] ?? '');
	}
	return sameList(one, other);
}

/** What a song's field will hold once written. */
export function effective(track: BulkTrack, draft: BulkDraft, key: BulkKey): string[] {
	const own = key === 'title' || key === 'track' ? draft.own[track.id]?.[key] : undefined;
	return own === undefined ? (draft.fields[key] ?? fileValue(track, key)) : cleaned([own]);
}

/** What the form shows of a field: the value every song shares, or "Mixed" when they differ. */
export interface Shown {
	values: string[];
	mixed: boolean;
}

/** Reads what the songs share of a field, as it will be once written. */
export function shownOf(tracks: readonly BulkTrack[], draft: BulkDraft, key: BulkKey): Shown {
	const typed = draft.fields[key];
	const hasOwn = tracks.some((track) => draft.own[track.id]?.[key as OwnKey] !== undefined);
	if (typed !== undefined && !hasOwn) {
		return { values: typed, mixed: false };
	}
	const [first, ...rest] = tracks.map((track) => effective(track, draft, key));
	if (first === undefined) {
		return { values: [], mixed: false };
	}
	const mixed = rest.some((values) => !same(key, values, first));
	return { values: mixed ? [] : first, mixed };
}

/** Whether a field of any song will hold something its file doesn't. */
export function isBulkChanged(
	tracks: readonly BulkTrack[],
	draft: BulkDraft,
	key: BulkKey
): boolean {
	return tracks.some((track) => !same(key, effective(track, draft, key), fileValue(track, key)));
}

/** Takes what's typed into the form: it goes to every song, and replaces a quick job's values. */
export function withBulkField(
	tracks: readonly BulkTrack[],
	draft: BulkDraft,
	key: BulkKey,
	values: string[]
): BulkDraft {
	const own = Object.fromEntries(
		Object.entries(draft.own).map(([id, held]) => [
			id,
			Object.fromEntries(Object.entries(held).filter(([name]) => name !== key))
		])
	);
	const fields = { ...draft.fields, [key]: values };
	const next = { fields, own };
	// Typing back what every file holds forgets the field.
	if (tracks.every((track) => same(key, values, fileValue(track, key)))) {
		return { ...next, fields: without(fields, key) };
	}
	return next;
}

function without<V>(
	record: Partial<Record<BulkKey, V>>,
	key: BulkKey
): Partial<Record<BulkKey, V>> {
	return Object.fromEntries(Object.entries(record).filter(([name]) => name !== key));
}

/** Each album's etag, by ID. */
export function etagsOf(albums: readonly { id: string; tags: AlbumTags }[]): Map<string, string> {
	return new Map(albums.map(({ id, tags }) => [id, tags.etag]));
}

/** Each value once, in the order first given. */
export function once<T>(values: readonly T[]): T[] {
	return values.filter((value, index) => values.indexOf(value) === index);
}

/** Sets each song's own value of a field, as a quick job does, forgetting what the form held. */
function withOwn(draft: BulkDraft, key: OwnKey, values: ReadonlyMap<string, string>): BulkDraft {
	const fields = without(draft.fields, key);
	const own = { ...draft.own };
	for (const [id, value] of values) {
		own[id] = { ...own[id], [key]: value };
	}
	return { fields, own };
}

/** Numbers the songs from 1 in the order given, which is album order. */
export function numberFromOne(tracks: readonly BulkTrack[], draft: BulkDraft): BulkDraft {
	return withOwn(draft, 'track', new Map(tracks.map(({ id }, index) => [id, String(index + 1)])));
}

/** A title from a file's name: its extension and any leading track number go. */
export function titleFromFile(file: string): string {
	const stem = file.replace(/\.[^.]+$/, '');
	const bare = stem.replace(/^\s*(?:\d{1,2}[-.]\s*)?\d{1,3}(?:\s*[-._)]\s*|\s+)(?=\S)/, '');
	return bare.replaceAll('_', ' ').replace(/\s+/g, ' ').trim() || stem;
}

export function titlesFromFiles(tracks: readonly BulkTrack[], draft: BulkDraft): BulkDraft {
	return withOwn(
		draft,
		'title',
		new Map(tracks.map((track) => [track.id, titleFromFile(track.tags.file)]))
	);
}

/** Small words a title keeps lower-case, unless it starts or ends with one. */
const smallWords = new Set([
	'a',
	'an',
	'and',
	'as',
	'at',
	'but',
	'by',
	'for',
	'from',
	'in',
	'into',
	'nor',
	'of',
	'on',
	'or',
	'over',
	'the',
	'to',
	'vs',
	'with'
]);

/**
 * Title Case, as a careful tagger does it: each word starts upper-case but the small ones in the
 * middle, and a word that already has a capital after its first letter ("DJ", "iPod") is left alone.
 */
export function titleCase(title: string): string {
	const words = title.split(/(\s+)/);
	const last = words.findLastIndex((word) => word.trim() !== '');
	const first = words.findIndex((word) => word.trim() !== '');
	return words
		.map((word, index) => {
			if (word.trim() === '' || /\p{Lu}/u.test(word.slice(1))) {
				return word;
			}
			const lower = word.toLowerCase();
			const bare = lower.replace(/^\W+|\W+$/g, '');
			if (index !== first && index !== last && smallWords.has(bare)) {
				return lower;
			}
			const at = lower.search(/\p{L}/u);
			return at < 0
				? lower
				: lower.slice(0, at) + lower.charAt(at).toUpperCase() + lower.slice(at + 1);
		})
		.join('');
}

export function titlesInTitleCase(tracks: readonly BulkTrack[], draft: BulkDraft): BulkDraft {
	return withOwn(
		draft,
		'title',
		new Map(
			tracks.map((track) => [track.id, titleCase(effective(track, draft, 'title').at(0) ?? '')])
		)
	);
}

/** Replaces every `find` in each title, as typed: case matters. Finding nothing changes nothing. */
export function findAndReplace(
	tracks: readonly BulkTrack[],
	draft: BulkDraft,
	find: string,
	replace: string
): BulkDraft {
	if (find === '') {
		return draft;
	}
	const titles = new Map<string, string>();
	for (const track of tracks) {
		const title = effective(track, draft, 'title').at(0) ?? '';
		if (title.includes(find)) {
			titles.set(track.id, title.replaceAll(find, replace));
		}
	}
	return titles.size === 0 ? draft : withOwn(draft, 'title', titles);
}

/** The problem key of a form field, and of a song's own value. */
export function bulkFieldId(key: BulkKey, trackId?: string): string {
	return trackId === undefined ? `bulk-${key}` : `bulk-${key}-${trackId}`;
}

function numberProblem(key: BulkKey, value: string): string | undefined {
	const n = Number(value);
	return /^\d+$/.test(value) && n >= 1 && n <= mostNumber
		? undefined
		: `A ${key} number runs from 1 to ${String(mostNumber)}.`;
}

/** Finds what the server would refuse, form fields first. Only what changed counts. */
export function bulkProblems(tracks: readonly BulkTrack[], draft: BulkDraft): Map<string, string> {
	const problems = new Map<string, string>();
	for (const [key, values] of Object.entries(draft.fields) as [BulkKey, string[]][]) {
		const [value] = cleaned(values);
		const at = bulkFieldId(key);
		if (value === undefined) {
			if (key === 'title') {
				problems.set(at, 'Every song needs a title.');
			}
		} else if (key === 'year' && !/^(?!0000)\d{4}$/.test(value)) {
			problems.set(at, 'A year is four digits.');
		} else if (specOf.get(key)?.number === true) {
			const problem = numberProblem(key, value);
			if (problem !== undefined) {
				problems.set(at, problem);
			}
		}
	}
	for (const track of tracks) {
		const own = draft.own[track.id];
		if (own?.title !== undefined && own.title.trim() === '') {
			problems.set(bulkFieldId('title', track.id), `${track.tags.file} would have no title.`);
		}
		const problem = own?.track === undefined ? undefined : numberProblem('track', own.track);
		if (problem !== undefined) {
			problems.set(bulkFieldId('track', track.id), problem);
		}
	}
	return problems;
}

function tagEdit(fields: [TagField, string[]][]): TagEdit {
	const set = fields.filter(([, values]) => values.length > 0);
	const clear = fields.filter(([, values]) => values.length === 0).map(([field]) => field);
	return {
		...(set.length > 0 ? { set: Object.fromEntries(set) } : {}),
		...(clear.length > 0 ? { clear } : {})
	};
}

function isEmpty(edit: object): boolean {
	return Object.keys(edit).length === 0;
}

/** The form's fields that change some song, as a field and its values. */
function sharedChanges(tracks: readonly BulkTrack[], draft: BulkDraft): [TagField, string[]][] {
	return bulkFields.flatMap(({ key, field }) => {
		const values = draft.fields[key];
		const changes =
			values !== undefined && tracks.some((track) => !same(key, values, fileValue(track, key)));
		return changes ? [[field, cleaned(values)] as [TagField, string[]]] : [];
	});
}

/** Each song's own changes: what a quick job gave it that differs from what lies beneath. */
function ownChanges(track: BulkTrack, draft: BulkDraft): [TagField, string[]][] {
	const own = draft.own[track.id] ?? {};
	return (['title', 'track'] as const).flatMap((key) => {
		const value = own[key];
		const beneath = draft.fields[key] ?? fileValue(track, key);
		return value === undefined || same(key, [value], beneath)
			? []
			: [[key, cleaned([value])] as [TagField, string[]]];
	});
}

/**
 * Builds the one request that turns the picked songs' files into the draft, or null if there's
 * nothing to do. `etags` is each album's, as read.
 */
export function bulkEditOf(
	tracks: readonly BulkTrack[],
	draft: BulkDraft,
	etags: ReadonlyMap<string, string>
): TracksEdit | null {
	const all = tagEdit(sharedChanges(tracks, draft));
	const own = tracks.map((track) => ({ id: track.id, ...tagEdit(ownChanges(track, draft)) }));
	const named = isEmpty(all) ? own.filter((edit) => Object.keys(edit).length > 1) : own;
	if (named.length === 0) {
		return null;
	}
	const albumIds = [
		...new Set(
			tracks.filter(({ id }) => named.some((edit) => edit.id === id)).map(({ albumId }) => albumId)
		)
	];
	return {
		albums: albumIds.map((id) => ({ id, etag: etags.get(id) ?? '' })),
		...(isEmpty(all) ? {} : { all }),
		tracks: named
	};
}

/** What a save would write, as the bar counts it: a field of every song is one change. */
export interface BulkSummary {
	changes: number;
	files: number;
}

export function bulkSummaryOf(tracks: readonly BulkTrack[], draft: BulkDraft): BulkSummary {
	const problems = bulkProblems(tracks, draft);
	const shared = bulkFields.filter(
		({ key }) =>
			draft.fields[key] !== undefined &&
			!problems.has(bulkFieldId(key)) &&
			isBulkChanged(tracks, { fields: { [key]: draft.fields[key] }, own: {} }, key)
	).length;
	const own = tracks.map((track) => ownChanges(track, draft).length);
	const ownTotal = own.reduce((sum, count) => sum + count, 0);
	return {
		changes: shared + ownTotal,
		files: shared > 0 ? tracks.length : own.filter((count) => count > 0).length
	};
}
