/**
 * The one tag editor's draft: songs from one album or several, the ones ticked, what the shared
 * fields give every ticked song, and what each song has been given of its own, on top. Pure: the
 * session in `lib/pages/tagEditor.svelte.ts` holds it and says when it changes.
 */
import type {
	AlbumEdit,
	AlbumTags,
	FieldValues,
	TagEdit,
	TagField,
	TracksEdit,
	TrackTags
} from './source.ts';
import {
	cleaned,
	fieldId,
	fieldOrder,
	problemOf,
	sameList,
	sameValues,
	yearOf,
	type Summary
} from './tags.ts';

/** What the server takes: M4A holds a track or disc number in 16 bits. */
const mostNumber = 65_535;

/** A field the ticked songs share, as the Tags tab has them. */
export type SharedKey =
	| 'title'
	| 'album'
	| 'albumArtists'
	| 'artists'
	| 'genres'
	| 'year'
	| 'track'
	| 'disc'
	| 'discSubtitle'
	| 'label'
	| 'comment';

export interface SharedSpec {
	key: SharedKey;
	/** The field it writes. A year replaces a date whose year it changes. */
	field: TagField;
	label: string;
	/** How many of the two columns it takes. */
	span: 1 | 2;
	/** A list, shown as chips. */
	multi: boolean;
	kind: 'text' | 'number' | 'year';
}

function spec(key: SharedKey, label: string, more: Partial<SharedSpec> = {}): SharedSpec {
	return { key, field: key as TagField, label, span: 1, multi: false, kind: 'text', ...more };
}

/** The board's eleven, in its order. */
export const sharedFields: readonly SharedSpec[] = [
	spec('title', 'Title', { span: 2 }),
	spec('album', 'Album'),
	spec('albumArtists', 'Album artist', { multi: true }),
	spec('artists', 'Artists', { multi: true }),
	spec('genres', 'Genres', { multi: true }),
	spec('year', 'Year', { field: 'date', kind: 'year' }),
	spec('track', 'Track', { kind: 'number' }),
	spec('disc', 'Disc', { kind: 'number' }),
	spec('discSubtitle', 'Disc name'),
	spec('label', 'Label', { multi: true }),
	spec('comment', 'Comment', { span: 2 })
];

const specOf = new Map(sharedFields.map((each) => [each.key, each]));
const keyOf = new Map(sharedFields.map(({ key, field }) => [field, key]));

/** One song of an edit: its file's tags and the album it's on. */
export interface EditTrack {
	id: string;
	albumId: string;
	tags: TrackTags;
}

/** What someone has touched on one song, as typed. */
export interface TrackDraft {
	fields: FieldValues;
	/** By the key the server showed. No values removes the tag. */
	tags: Record<string, string[]>;
}

/**
 * What someone has touched, as typed, and nothing else: whatever isn't here reads through to the
 * files. A shared field goes to every ticked song; a song's own value goes on top, ticked or not.
 */
export interface EditDraft {
	shared: Partial<Record<SharedKey, string[]>>;
	tracks: Record<string, TrackDraft>;
}

/** Which songs the shared fields and the Shuffle tab act on. */
export type Ticked = ReadonlySet<string>;

export function emptyEditDraft(): EditDraft {
	return { shared: {}, tracks: {} };
}

/** Every song the albums hold, in album order, albums in the order given. */
export function editTracks(albums: readonly { id: string; tags: AlbumTags }[]): EditTrack[] {
	return albums.flatMap(({ id, tags }) =>
		tags.tracks.map((track) => ({ id: track.id, albumId: id, tags: track }))
	);
}

/** Each value once, in the order first given. */
export function once<T>(values: readonly T[]): T[] {
	return values.filter((value, index) => values.indexOf(value) === index);
}

/** Each album's etag, by ID. */
export function etagsOf(albums: readonly { id: string; tags: AlbumTags }[]): Map<string, string> {
	return new Map(albums.map(({ id, tags }) => [id, tags.etag]));
}

/** The problem key, and element ID, of a shared field. */
export function sharedFieldId(key: SharedKey): string {
	return `shared-${key}`;
}

/** What a field holds in the file. */
export function wasOf(track: EditTrack, field: TagField): string[] {
	return track.tags.fields[field] ?? [];
}

function tagWas(track: EditTrack, key: string): string[] {
	return track.tags.tags.find((tag) => tag.key === key)?.values ?? [];
}

/** What the shared fields give a field of a song, if they give it anything. */
function sharedOf(
	track: EditTrack,
	draft: EditDraft,
	ticked: Ticked,
	field: TagField
): string[] | undefined {
	const key = keyOf.get(field);
	const values = key === undefined ? undefined : draft.shared[key];
	if (values === undefined || !ticked.has(track.id)) {
		return undefined;
	}
	if (key !== 'year') {
		return values;
	}
	// A year leaves a date alone that's in it already: 2001-05-12 stays for 2001.
	const [year] = cleaned(values);
	if (year === undefined) {
		return [];
	}
	return yearOf(wasOf(track, 'date')) === year ? wasOf(track, 'date') : [year];
}

/** What a field will hold if nothing more of it is touched on this song. */
function beneath(track: EditTrack, draft: EditDraft, ticked: Ticked, field: TagField): string[] {
	return sharedOf(track, draft, ticked, field) ?? wasOf(track, field);
}

/** What a field of a song shows now. */
export function valueOf(
	track: EditTrack,
	draft: EditDraft,
	ticked: Ticked,
	field: TagField
): string[] {
	return draft.tracks[track.id]?.fields[field] ?? beneath(track, draft, ticked, field);
}

/** Whether a field of a song shows something its file doesn't hold. */
export function isChanged(
	track: EditTrack,
	draft: EditDraft,
	ticked: Ticked,
	field: TagField
): boolean {
	return !sameValues(field, valueOf(track, draft, ticked, field), wasOf(track, field));
}

/** What a raw tag shows now. */
export function tagValueOf(track: EditTrack, draft: EditDraft, key: string): string[] {
	return draft.tracks[track.id]?.tags[key] ?? tagWas(track, key);
}

/** Whether a raw tag shows something the file doesn't hold. */
export function isTagChanged(track: EditTrack, draft: EditDraft, key: string): boolean {
	return !sameList(cleaned(tagValueOf(track, draft, key)), cleaned(tagWas(track, key)));
}

/** What a shared field reads of one song: a date as its year. */
function readOf(track: EditTrack, draft: EditDraft, ticked: Ticked, key: SharedKey): string[] {
	const { field } = specOf.get(key) ?? spec(key, key);
	const values = valueOf(track, draft, ticked, field);
	if (key === 'year') {
		const year = yearOf(values);
		return year === '' ? [] : [year];
	}
	return cleaned(values);
}

/** What a shared field shows: the value every ticked song shares, or "Mixed" when they differ. */
export interface Shown {
	values: string[];
	mixed: boolean;
}

/** Reads what the ticked songs share of a field, as it will be once written. */
export function shownOf(ticked: readonly EditTrack[], draft: EditDraft, key: SharedKey): Shown {
	const { field } = specOf.get(key) ?? spec(key, key);
	const typed = draft.shared[key];
	const own = ticked.some((track) => draft.tracks[track.id]?.fields[field] !== undefined);
	if (typed !== undefined && !own) {
		return { values: typed, mixed: false };
	}
	const set = new Set(ticked.map(({ id }) => id));
	const [first, ...rest] = ticked.map((track) => readOf(track, draft, set, key));
	if (first === undefined) {
		return { values: [], mixed: false };
	}
	const mixed = rest.some((values) => !sameValues(field, values, first));
	return { values: mixed ? [] : first, mixed };
}

/** Whether a shared field will change any ticked song's file. */
export function isSharedChanged(
	ticked: readonly EditTrack[],
	draft: EditDraft,
	key: SharedKey
): boolean {
	const { field } = specOf.get(key) ?? spec(key, key);
	const set = new Set(ticked.map(({ id }) => id));
	return ticked.some((track) => isChanged(track, draft, set, field));
}

function without<V>(record: Partial<Record<string, V>>, key: string): Record<string, V> {
	return Object.fromEntries(Object.entries(record).filter(([name]) => name !== key)) as Record<
		string,
		V
	>;
}

function withTrack(draft: EditDraft, id: string, track: TrackDraft): EditDraft {
	const others = without(draft.tracks, id);
	const blank = Object.keys(track.fields).length === 0 && Object.keys(track.tags).length === 0;
	return { ...draft, tracks: blank ? others : { ...others, [id]: track } };
}

/** Forgets the songs' own values of a field: the shared one, or a quick job's, takes over. */
function forgetOwn(draft: EditDraft, ids: readonly string[], field: TagField): EditDraft {
	return ids.reduce((held, id) => {
		const track = held.tracks[id];
		return track === undefined
			? held
			: withTrack(held, id, { ...track, fields: without(track.fields, field) });
	}, draft);
}

/**
 * Takes what's typed into a shared field: it goes to every ticked song, over what each had of its
 * own. Typing back what every ticked file holds forgets it.
 */
export function withShared(
	ticked: readonly EditTrack[],
	draft: EditDraft,
	key: SharedKey,
	values: string[]
): EditDraft {
	const { field } = specOf.get(key) ?? spec(key, key);
	const next = forgetOwn(
		draft,
		ticked.map(({ id }) => id),
		field
	);
	const file = (track: EditTrack): string[] => {
		if (key !== 'year') {
			return wasOf(track, field);
		}
		const year = yearOf(wasOf(track, 'date'));
		return year === '' ? [] : [year];
	};
	if (ticked.every((track) => sameValues(field, values, file(track)))) {
		return { ...next, shared: without(next.shared, key) };
	}
	return { ...next, shared: { ...next.shared, [key]: values } };
}

/** Takes what's typed into a song's own field. Typing back what lies beneath forgets it. */
export function withField(
	track: EditTrack,
	draft: EditDraft,
	ticked: Ticked,
	field: TagField,
	values: string[]
): EditDraft {
	const held = draft.tracks[track.id];
	const others = without(held?.fields ?? {}, field);
	const undone = sameList(values, beneath(track, draft, ticked, field));
	return withTrack(draft, track.id, {
		tags: held?.tags ?? {},
		fields: undone ? others : { ...others, [field]: values }
	});
}

/** Takes what's typed into a raw tag, the same way. */
export function withTag(
	track: EditTrack,
	draft: EditDraft,
	key: string,
	values: string[]
): EditDraft {
	const held = draft.tracks[track.id];
	const others = without(held?.tags ?? {}, key);
	const undone = sameList(values, tagWas(track, key));
	return withTrack(draft, track.id, {
		fields: held?.fields ?? {},
		tags: undone ? others : { ...others, [key]: values }
	});
}

/** Gives each song its own value of a field, as a quick job and a match do. */
export function withOwn(
	tracks: readonly EditTrack[],
	draft: EditDraft,
	values: ReadonlyMap<string, FieldValues>
): EditDraft {
	return tracks.reduce((held, track) => {
		const given = values.get(track.id);
		if (given === undefined) {
			return held;
		}
		const own = held.tracks[track.id];
		return withTrack(held, track.id, {
			tags: own?.tags ?? {},
			fields: { ...own?.fields, ...given }
		});
	}, draft);
}

/** A quick job's values, one field: the shared field gives way to them. */
function quickJob(
	tracks: readonly EditTrack[],
	draft: EditDraft,
	field: 'title' | 'track',
	values: ReadonlyMap<string, string>
): EditDraft {
	const next = { ...draft, shared: without(draft.shared, field) };
	return withOwn(
		tracks,
		next,
		new Map([...values].map(([id, value]) => [id, { [field]: [value] }]))
	);
}

/** Numbers the ticked songs from 1 in the order given, which is album order. */
export function numberFromOne(tracks: readonly EditTrack[], draft: EditDraft): EditDraft {
	return quickJob(
		tracks,
		draft,
		'track',
		new Map(tracks.map(({ id }, index) => [id, String(index + 1)]))
	);
}

/** A title from a file's name: its extension and any leading track number go. */
export function titleFromFile(file: string): string {
	const stem = file.replace(/\.[^.]+$/, '');
	const bare = stem.replace(/^\s*(?:\d{1,2}[-.]\s*)?\d{1,3}(?:\s*[-._)]\s*|\s+)(?=\S)/, '');
	return bare.replaceAll('_', ' ').replace(/\s+/g, ' ').trim() || stem;
}

export function titlesFromFiles(tracks: readonly EditTrack[], draft: EditDraft): EditDraft {
	return quickJob(
		tracks,
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

/** What a ticked song's title will be once written, for the jobs that work on titles. */
function titleNow(track: EditTrack, draft: EditDraft, tracks: readonly EditTrack[]): string {
	return valueOf(track, draft, new Set(tracks.map(({ id }) => id)), 'title').at(0) ?? '';
}

export function titlesInTitleCase(tracks: readonly EditTrack[], draft: EditDraft): EditDraft {
	return quickJob(
		tracks,
		draft,
		'title',
		new Map(tracks.map((track) => [track.id, titleCase(titleNow(track, draft, tracks))]))
	);
}

/** Replaces every `find` in each title, as typed: case matters. Finding nothing changes nothing. */
export function findAndReplace(
	tracks: readonly EditTrack[],
	draft: EditDraft,
	find: string,
	replace: string
): EditDraft {
	if (find === '') {
		return draft;
	}
	const titles = new Map<string, string>();
	for (const track of tracks) {
		const title = titleNow(track, draft, tracks);
		if (title.includes(find)) {
			titles.set(track.id, title.replaceAll(find, replace));
		}
	}
	return titles.size === 0 ? draft : quickJob(tracks, draft, 'title', titles);
}

/** A song's own fields that change what lies beneath them, as they'd be written. */
function ownFields(track: EditTrack, draft: EditDraft, ticked: Ticked): [TagField, string[]][] {
	const fields = draft.tracks[track.id]?.fields ?? {};
	return fieldOrder.flatMap((field) => {
		const values = fields[field];
		return values === undefined || sameValues(field, values, beneath(track, draft, ticked, field))
			? []
			: [[field, cleaned(values)] as [TagField, string[]]];
	});
}

function ownTags(track: EditTrack, draft: EditDraft): [string, string[]][] {
	return Object.entries(draft.tracks[track.id]?.tags ?? {}).flatMap(([key, values]) =>
		sameList(cleaned(values), cleaned(tagWas(track, key))) ? [] : [[key, cleaned(values)]]
	) as [string, string[]][];
}

function sharedProblem(key: SharedKey, values: readonly string[]): string | undefined {
	const [value] = cleaned(values);
	if (value === undefined) {
		return key === 'title' ? 'Every song needs a title.' : undefined;
	}
	if (key === 'year') {
		return /^(?!0000)\d{4}$/.test(value) ? undefined : 'A year is four digits.';
	}
	const n = Number(value);
	return specOf.get(key)?.kind !== 'number' || (/^\d+$/.test(value) && n >= 1 && n <= mostNumber)
		? undefined
		: `A ${key} number runs from 1 to ${String(mostNumber)}.`;
}

/**
 * Finds what the server would refuse: the shared fields first, then song by song in the order of the
 * list. Only what changed counts: a tag that's wrong already can stay wrong.
 */
export function problemsOf(
	tracks: readonly EditTrack[],
	draft: EditDraft,
	ticked: Ticked
): Map<string, string> {
	const problems = new Map<string, string>();
	if (tracks.some(({ id }) => ticked.has(id))) {
		for (const { key } of sharedFields) {
			const values = draft.shared[key];
			const problem = values === undefined ? undefined : sharedProblem(key, values);
			if (problem !== undefined) {
				problems.set(sharedFieldId(key), problem);
			}
		}
	}
	for (const track of tracks) {
		for (const [field, values] of ownFields(track, draft, ticked)) {
			const problem = problemOf(field, values);
			if (problem !== undefined) {
				problems.set(fieldId(track.id, field), problem);
			}
		}
	}
	return problems;
}

/** Whether a song's own edit changes anything, shown on its row or not. */
export function isTrackEdited(track: EditTrack, draft: EditDraft, ticked: Ticked): boolean {
	return ownFields(track, draft, ticked).length > 0 || ownTags(track, draft).length > 0;
}

/**
 * Counts what a save would write: a shared field is one change, however many songs it reaches. A
 * field with a problem won't be written, so it doesn't count.
 */
export function summaryOf(tracks: readonly EditTrack[], draft: EditDraft, ticked: Ticked): Summary {
	const problems = problemsOf(tracks, draft, ticked);
	const onTicked = tracks.filter(({ id }) => ticked.has(id));
	const shared = sharedFields.filter(
		({ key, field }) =>
			draft.shared[key] !== undefined &&
			!problems.has(sharedFieldId(key)) &&
			onTicked.some(
				(track) =>
					!sameValues(field, sharedOf(track, draft, ticked, field) ?? [], wasOf(track, field))
			)
	);
	const sharedOn = new Set(
		onTicked
			.filter((track) =>
				shared.some(
					({ field }) =>
						!sameValues(field, sharedOf(track, draft, ticked, field) ?? [], wasOf(track, field))
				)
			)
			.map(({ id }) => id)
	);
	const counts = tracks.flatMap((track) => {
		const count =
			ownFields(track, draft, ticked).filter(([field]) => !problems.has(fieldId(track.id, field)))
				.length + ownTags(track, draft).length;
		return count === 0 ? [] : [{ id: track.id, count }];
	});
	const [first] = counts;
	return {
		changes: shared.length + counts.reduce((sum, { count }) => sum + count, 0),
		files: new Set([...sharedOn, ...counts.map(({ id }) => id)]).size,
		only: shared.length === 0 && counts.length === 1 && first !== undefined ? first.id : null
	};
}

function tagEdit(fields: [TagField, string[]][], tags: [string, string[]][]): TagEdit {
	const set = fields.filter(([, values]) => values.length > 0);
	const clear = fields.filter(([, values]) => values.length === 0).map(([field]) => field);
	return {
		...(set.length > 0 ? { set: Object.fromEntries(set) } : {}),
		...(clear.length > 0 ? { clear } : {}),
		...(tags.length > 0 ? { tags: tags.map(([key, values]) => ({ key, values })) } : {})
	};
}

/** Each song's edit: every field it will hold that its file doesn't, and its raw tags. */
export function editsOf(
	tracks: readonly EditTrack[],
	draft: EditDraft,
	ticked: Ticked
): ({ id: string } & TagEdit)[] {
	return tracks.flatMap((track) => {
		const fields = fieldOrder.filter(
			(field) =>
				draft.tracks[track.id]?.fields[field] !== undefined ||
				sharedOf(track, draft, ticked, field) !== undefined
		);
		const changed = fields.flatMap((field) => {
			const values = cleaned(valueOf(track, draft, ticked, field));
			return sameValues(field, values, wasOf(track, field))
				? []
				: [[field, values] as [TagField, string[]]];
		});
		const edit = tagEdit(changed, ownTags(track, draft));
		return Object.keys(edit).length === 0 ? [] : [{ id: track.id, ...edit }];
	});
}

/** The one request a write sends: an album's own route, or the route across albums. */
export type EditRequest =
	| { route: 'album'; albumId: string; etag: string; edit: AlbumEdit }
	| { route: 'tracks'; edit: TracksEdit };

/**
 * Builds the request that turns the files into the draft, or null if there's nothing to do. Songs
 * all on one album go through that album's route, as its editor always has; songs across albums go
 * through `POST /edits`, which checks every album's etag before writing any.
 */
export function requestOf(
	tracks: readonly EditTrack[],
	draft: EditDraft,
	ticked: Ticked,
	etags: ReadonlyMap<string, string>
): EditRequest | null {
	const edits = editsOf(tracks, draft, ticked);
	if (edits.length === 0) {
		return null;
	}
	const named = new Set(edits.map(({ id }) => id));
	const albumIds = once(tracks.filter(({ id }) => named.has(id)).map(({ albumId }) => albumId));
	const [only] = albumIds;
	if (albumIds.length === 1 && only !== undefined) {
		return { route: 'album', albumId: only, etag: etags.get(only) ?? '', edit: { tracks: edits } };
	}
	return {
		route: 'tracks',
		edit: { albums: albumIds.map((id) => ({ id, etag: etags.get(id) ?? '' })), tracks: edits }
	};
}

/**
 * Carries someone's edits over to an album's files that moved on: what they touched stays theirs,
 * and what can't be touched any more goes, as do songs no longer on it. `before` is the album's
 * songs as they were read; every other song's draft is left as it is.
 */
export function rebased(draft: EditDraft, fresh: AlbumTags, before: readonly string[]): EditDraft {
	const tracks = { ...draft.tracks };
	for (const id of before) {
		Reflect.deleteProperty(tracks, id);
	}
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
