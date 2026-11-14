import {
	EditError,
	type AlbumEdit,
	type AlbumTags,
	type EditFailure,
	type TagField
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
	'language',
	'musicbrainzAlbumArtistId',
	'musicbrainzArtistId'
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
	},
	{
		// Written by Find matches, never typed: shown so they can be checked.
		name: 'MUSICBRAINZ',
		fields: [
			{
				field: 'musicbrainzAlbumId',
				label: 'Release',
				name: 'MusicBrainz release ID',
				span: 2,
				locked: true
			},
			{
				field: 'musicbrainzReleaseGroupId',
				label: 'Release group',
				name: 'MusicBrainz release group ID',
				span: 2,
				locked: true
			},
			{
				field: 'musicbrainzRecordingId',
				label: 'Recording',
				name: 'MusicBrainz recording ID',
				span: 2,
				locked: true
			},
			{
				field: 'musicbrainzTrackId',
				label: 'Track',
				name: 'MusicBrainz track ID',
				span: 2,
				locked: true
			},
			{
				field: 'musicbrainzArtistId',
				label: 'Artists',
				name: 'MusicBrainz artist IDs',
				span: 2,
				locked: true
			},
			{
				field: 'musicbrainzAlbumArtistId',
				label: 'Album artists',
				name: 'MusicBrainz album artist IDs',
				span: 2,
				locked: true
			}
		]
	}
];

/** Every field a track's own view edits, in its order: the groups', then the lyrics. */
export const fieldOrder: readonly TagField[] = [
	...tagGroups.flatMap(({ fields }) => fields.map(({ field }) => field)),
	'lyrics'
];

/** What a save would do, as the bar counts it. A field of every song is one change. */
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

/** Names a field of a track, as an element ID and as the key of a problem. */
export function fieldId(trackId: string, field: TagField): string {
	return `${field}-${trackId}`;
}

/** Names a raw tag of a track, the same way. */
export function tagId(trackId: string, key: string): string {
	return `tag-${key}-${trackId}`;
}

/** Reports whether a field holds a list, shown as chips and never as joined text. */
export function isMulti(field: TagField): boolean {
	return multiFields.includes(field);
}

/** Values as they're written: trimmed, and the empty ones gone. */
export function cleaned(values: readonly string[]): string[] {
	return values.map((value) => value.trim()).filter((value) => value !== '');
}

export function sameList(a: readonly string[], b: readonly string[]): boolean {
	return a.length === b.length && a.every((item, index) => item === b[index]);
}

function isWhole(value: string): boolean {
	return /^\d+$/.test(value);
}

/** Whether two sets of values mean the same to the field: `01` is track 1, whatever's typed. */
export function sameValues(field: TagField, a: readonly string[], b: readonly string[]): boolean {
	const [one, other] = [cleaned(a), cleaned(b)];
	if (multiFields.includes(field)) {
		return sameList(one, other);
	}
	const [first = '', second = ''] = [one.at(0), other.at(0)];
	return field in numberNames && isWhole(first) && isWhole(second)
		? Number(first) === Number(second)
		: first === second;
}

/** The year a date begins with, or nothing. */
export function yearOf(values: readonly string[] | undefined): string {
	return /^\d{4}/.exec(values?.at(0)?.trim() ?? '')?.[0] ?? '';
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

/** What the server would refuse in a field's new values, in a person's words. */
export function problemOf(field: TagField, values: readonly string[]): string | undefined {
	const [value] = values;
	if (value === undefined) {
		return field === 'title' ? 'A song needs a title.' : undefined;
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
