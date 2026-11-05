import {
	EditError,
	type Album,
	type AlbumEdit,
	type EditFailure,
	type Track,
	type TrackEdit
} from './source.ts';

/** What the server takes: M4A holds a track or disc number in 16 bits. */
const mostNumber = 65_535;
const leastYear = 1000;
const mostYear = 9999;

/** A track as the editor's fields hold it. */
export interface TrackDraft {
	id: string;
	title: string;
	trackNo: string;
	discNo: string;
}

/** An album as the editor's fields hold it: what was typed, and not yet anything more. */
export interface Draft {
	year: string;
	genres: string[];
	tracks: TrackDraft[];
}

/** One PATCH. A save is a chain of these. */
export type Step =
	{ kind: 'album'; edit: AlbumEdit } | { kind: 'track'; id: string; edit: TrackEdit };

/** What a save needs from the library, which also keeps everything on screen up to date. */
export interface Tagger {
	album(id: string): Album | undefined;
	editAlbum(id: string, edit: AlbumEdit): Promise<void>;
	editTrack(albumId: string, trackId: string, edit: TrackEdit): Promise<void>;
	reread(id: string): Promise<Album>;
}

export type SaveStop =
	| EditFailure
	/** The ETag read back is the one refused: the store is behind a file, and only a scan helps. */
	| { kind: 'drifted' };

export interface Saved {
	done: Step[];
	/** The step that stopped, then those it held up. */
	left: Step[];
	stop: SaveStop | null;
}

/** What to tell someone about a save that stopped. */
export interface Report {
	message: string;
	/** The server's own words, when they say more. */
	detail: string | undefined;
	changed: string[];
	unchanged: string[];
	/** What to call {@link Report.unchanged}: with no answer, nobody knows. */
	unchangedHeading: string;
	/** Whether saving again could work. */
	retry: boolean;
}

function digits(value: number | undefined): string {
	return value === undefined ? '' : String(value);
}

/** The fields for an album as it stands. */
export function draftOf({ year, genres, tracks }: Album): Draft {
	return {
		year: digits(year),
		genres: [...genres],
		tracks: tracks.map(({ id, title, trackNo, discNo }) => ({
			id,
			title,
			trackNo: digits(trackNo),
			discNo: digits(discNo)
		}))
	};
}

/** Tracks as the server orders them, so a new number lands where a reload would put it. */
export function inAlbumOrder(tracks: readonly Track[]): Track[] {
	const after = (a: string, b: string): number => (a < b ? -1 : Number(a > b));
	return tracks.toSorted(
		(a, b) =>
			(a.discNo ?? 1) - (b.discNo ?? 1) ||
			(a.trackNo ?? Infinity) - (b.trackNo ?? Infinity) ||
			after(a.title, b.title) ||
			after(a.id, b.id)
	);
}

/** Names a field, as an element ID and as the key of a problem. */
export function fieldId(trackId: string, field: 'title' | 'trackNo' | 'discNo'): string {
	return `${field}-${trackId}`;
}

export const yearField = 'year';

/** The number a field holds, or undefined if it isn't one. Blank isn't one: it means no change. */
function whole(typed: string): number | undefined {
	return /^\d+$/.test(typed.trim()) ? Number(typed) : undefined;
}

/** A number that differs from what the field started with. Clearing a number isn't an edit. */
function newNumber(typed: string, was: string): number | undefined {
	const number = whole(typed);
	return number === whole(was) ? undefined : number;
}

function sameList(a: readonly string[], b: readonly string[]): boolean {
	return a.length === b.length && a.every((item, index) => item === b[index]);
}

function trackStep(was: TrackDraft | undefined, row: TrackDraft): Step | undefined {
	const title = row.title.trim();
	const edit: TrackEdit = {
		...(title === was?.title.trim() ? {} : { title }),
		...optional('trackNo', newNumber(row.trackNo, was?.trackNo ?? '')),
		...optional('discNo', newNumber(row.discNo, was?.discNo ?? ''))
	};
	return Object.keys(edit).length === 0 ? undefined : { kind: 'track', id: row.id, edit };
}

function optional<K extends string>(key: K, value: number | undefined): Partial<Record<K, number>> {
	return value === undefined ? {} : ({ [key]: value } as Record<K, number>);
}

/** Lists the PATCHes that turn `base` into `draft`: the album first, because it rewrites every file. */
export function stepsOf(base: Draft, draft: Draft): Step[] {
	const was = new Map(base.tracks.map((row) => [row.id, row]));
	const album: AlbumEdit = {
		...optional('year', newNumber(draft.year, base.year)),
		...(sameList(base.genres, draft.genres) ? {} : { genres: draft.genres })
	};
	return [
		...(Object.keys(album).length === 0 ? [] : [{ kind: 'album', edit: album } as const]),
		...draft.tracks.flatMap((row) => trackStep(was.get(row.id), row) ?? [])
	];
}

function outOfRange(typed: string, was: string, least: number, most: number): boolean {
	if (typed.trim() === '' || whole(typed) === whole(was)) {
		return false;
	}
	const number = whole(typed);
	return number === undefined || number < least || number > most;
}

/**
 * Finds what the server would refuse, by field, in the order of the page. Only what changed counts:
 * a tag that's wrong already can stay wrong.
 */
export function problemsIn(base: Draft, draft: Draft): Map<string, string> {
	const was = new Map(base.tracks.map((row) => [row.id, row]));
	const problems = new Map<string, string>();
	if (outOfRange(draft.year, base.year, leastYear, mostYear)) {
		problems.set(yearField, 'A year runs from 1000 to 9999.');
	}
	for (const row of draft.tracks) {
		const old = was.get(row.id);
		if (outOfRange(row.discNo, old?.discNo ?? '', 1, mostNumber)) {
			problems.set(fieldId(row.id, 'discNo'), 'A disc number runs from 1 to 65535.');
		}
		if (outOfRange(row.trackNo, old?.trackNo ?? '', 1, mostNumber)) {
			problems.set(fieldId(row.id, 'trackNo'), 'A track number runs from 1 to 65535.');
		}
		if (row.title.trim() === '') {
			problems.set(fieldId(row.id, 'title'), 'A track needs a title.');
		}
	}
	return problems;
}

/** Moves `base` on past steps that landed, so they aren't sent again. */
export function advanced(base: Draft, steps: readonly Step[]): Draft {
	return steps.reduce<Draft>(
		(draft, step) =>
			step.kind === 'album'
				? {
						...draft,
						year: step.edit.year === undefined ? draft.year : String(step.edit.year),
						genres: step.edit.genres ?? draft.genres
					}
				: {
						...draft,
						tracks: draft.tracks.map((row) =>
							row.id === step.id
								? {
										...row,
										title: step.edit.title ?? row.title,
										trackNo:
											step.edit.trackNo === undefined ? row.trackNo : String(step.edit.trackNo),
										discNo: step.edit.discNo === undefined ? row.discNo : String(step.edit.discNo)
									}
								: row
						)
					},
		base
	);
}

/** Carries someone's edits over to an album that moved on: what they touched stays theirs. */
export function rebased(base: Draft, draft: Draft, fresh: Draft): Draft {
	const was = new Map(base.tracks.map((row) => [row.id, row]));
	const typed = new Map(draft.tracks.map((row) => [row.id, row]));
	return {
		year: draft.year === base.year ? fresh.year : draft.year,
		genres: sameList(draft.genres, base.genres) ? fresh.genres : draft.genres,
		tracks: fresh.tracks.map((row) => {
			const old = was.get(row.id);
			const mine = typed.get(row.id);
			if (old === undefined || mine === undefined) {
				return row;
			}
			return {
				id: row.id,
				title: mine.title === old.title ? row.title : mine.title,
				trackNo: mine.trackNo === old.trackNo ? row.trackNo : mine.trackNo,
				discNo: mine.discNo === old.discNo ? row.discNo : mine.discNo
			};
		})
	};
}

function etagFor(album: Album | undefined, step: Step): string | undefined {
	return step.kind === 'album' ? album?.etag : album?.tracks.find(({ id }) => id === step.id)?.etag;
}

function isStopped(failure: EditFailure): failure is Extract<EditFailure, { changed: string[] }> {
	return 'changed' in failure;
}

/** Finds out what a failed step left. Anything but a plain refusal means what's held is behind. */
async function aftermath(
	tagger: Tagger,
	albumId: string,
	step: Step,
	sent: string | undefined,
	failure: EditFailure
): Promise<SaveStop> {
	const partway = isStopped(failure) && failure.changed.length > 0;
	if (failure.kind !== 'stale' && !partway) {
		return failure;
	}
	try {
		const fresh = await tagger.reread(albumId);
		return failure.kind === 'stale' && etagFor(fresh, step) === sent
			? { kind: 'drifted' }
			: failure;
	} catch {
		return partway ? failure : { kind: 'unavailable' };
	}
}

/**
 * Saves steps one after another, each on the ETag the last one left: the server answers two at once
 * with a 409. It stops at the first that fails, because each step is atomic and a save isn't, and
 * what landed stays landed. It never rejects: a lost session has already gone to the door.
 */
export async function save(
	tagger: Tagger,
	albumId: string,
	steps: readonly Step[]
): Promise<Saved> {
	for (const [index, step] of steps.entries()) {
		const sent = etagFor(tagger.album(albumId), step);
		try {
			await (step.kind === 'album'
				? tagger.editAlbum(albumId, step.edit)
				: tagger.editTrack(albumId, step.id, step.edit));
		} catch (error) {
			const failure = error instanceof EditError ? error.failure : { kind: 'unavailable' as const };
			return {
				done: steps.slice(0, index),
				left: steps.slice(index),
				stop: await aftermath(tagger, albumId, step, sent, failure)
			};
		}
	}
	return { done: [...steps], left: [], stop: null };
}

function seconds(count: number): string {
	return `${String(count)} ${count === 1 ? 'second' : 'seconds'}`;
}

function messageFor(stop: SaveStop): string {
	switch (stop.kind) {
		case 'busy':
			return `A scan or another edit is running. Try again in ${seconds(stop.retryAfterSeconds)}.`;
		case 'stale':
			return 'This album changed while you were editing. Your edits are still here: check them and save again.';
		case 'drifted':
			return 'A file changed outside Kitsune. It needs a scan before it can be edited.';
		case 'forbidden':
			return "This server doesn't allow tag edits.";
		case 'gone':
			return "This album isn't in the library any more.";
		case 'refused':
			return "The file won't take this edit.";
		case 'unwritable':
			return "Kitsune can't write to this file.";
		case 'failed':
			return 'Something went wrong writing a file. Whatever it reached is still valid.';
		case 'unavailable':
			return "Couldn't reach Kitsune, so the last edit may not have landed. Check the album, then try again.";
	}
}

function fieldsOf(edit: AlbumEdit): string {
	const fields = [
		...(edit.year === undefined ? [] : ['Year']),
		...(edit.genres === undefined ? [] : ['Genres'])
	];
	return fields.join(' and ');
}

function tracksOf(count: number, total: number): string {
	return `${String(count)} of ${String(total)} ${total === 1 ? 'track' : 'tracks'}`;
}

/** Writes up a save that stopped, naming tracks by `titleOf`. */
export function reportOf(
	{ done, left, stop }: Saved,
	titleOf: (id: string) => string
): Report | null {
	if (stop === null) {
		return null;
	}
	const named = (step: Step): string =>
		step.kind === 'album' ? fieldsOf(step.edit) : titleOf(step.id);
	const changed = done.map(named);
	const unchanged = left.map(named);
	const [first, ...rest] = left;
	const stopped = 'changed' in stop ? stop : undefined;
	if (first?.kind === 'album' && stopped !== undefined && stopped.changed.length > 0) {
		const total = stopped.changed.length + stopped.remaining.length;
		changed.push(`${named(first)} on ${tracksOf(stopped.changed.length, total)}`);
		unchanged.splice(
			0,
			unchanged.length,
			`${named(first)} on ${tracksOf(stopped.remaining.length, total)}`,
			...rest.map(named)
		);
	}
	return {
		message: messageFor(stop),
		detail: stop.kind === 'refused' || stop.kind === 'unwritable' ? stop.detail : undefined,
		changed,
		unchanged,
		unchangedHeading: stop.kind === 'unavailable' ? 'Not confirmed' : 'Not saved',
		retry: !['drifted', 'forbidden', 'gone'].includes(stop.kind)
	};
}
