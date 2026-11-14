import { describe, expect, it, vi } from 'vitest';
import {
	AlbumNotFoundError,
	EditError,
	UnauthorizedError,
	type AlbumTags,
	type EditFailure,
	type FieldValues,
	type TrackTags
} from './source.ts';
import { isMulti, problemOf, reportOf, save, tagGroups, type Tagger } from './tags.ts';

function track(id: string, fields: FieldValues = {}, extra: Partial<TrackTags> = {}): TrackTags {
	return {
		id,
		file: `${id}.flac`,
		fields: {
			title: [id.toUpperCase()],
			track: ['1'],
			disc: ['1'],
			date: ['2001-05-12'],
			genres: ['Rock'],
			...fields
		},
		tags: [],
		...extra
	};
}

const tags: AlbumTags = {
	etag: 'a:0',
	tracks: [
		track('t1', { track: ['1'] }, { tags: [{ key: 'MOOD', values: ['Slow'] }] }),
		track('t2', { track: ['2'] })
	]
};

describe('tagGroups', () => {
	const specs = tagGroups.flatMap(({ fields }) => fields);

	it('lists the board’s groups, in its order', () => {
		expect(tagGroups.map(({ name }) => name)).toEqual([
			'THE BASICS',
			'CREDITS',
			'RELEASE',
			'SORTING',
			'MUSICBRAINZ'
		]);
	});

	it('has one date field: a year is a date', () => {
		expect(specs.filter(({ label }) => label === 'Year')).toEqual([]);
		expect(specs.filter(({ field }) => field === 'date')).toHaveLength(1);
	});

	it('lets album, artists and album artist be edited, and shows compilation and MBIDs only', () => {
		expect(specs.filter(({ locked = false }) => locked).map(({ field }) => field)).toEqual([
			'compilation',
			'musicbrainzAlbumId',
			'musicbrainzReleaseGroupId',
			'musicbrainzRecordingId',
			'musicbrainzTrackId',
			'musicbrainzArtistId',
			'musicbrainzAlbumArtistId'
		]);
	});

	it('names each field once, and leaves lyrics to their own box', () => {
		const fields = specs.map(({ field }) => field);
		expect(new Set(fields).size).toBe(fields.length);
		expect(fields).not.toContain('lyrics');
	});

	it('fills each row of four', () => {
		for (const { fields } of tagGroups) {
			expect(fields.reduce((sum, { span }) => sum + span, 0) % 4).toBe(0);
		}
	});

	it('knows which fields are lists, as the server does', () => {
		expect(isMulti('composers')).toBe(true);
		expect(isMulti('grouping')).toBe(false);
	});
});

describe('problemOf', () => {
	it.each(['0', '65536', '1a', '-1'])('refuses a track number of %s', (value) => {
		expect(problemOf('track', [value])).toBe('A track number runs from 1 to 65535.');
	});

	it('takes the ends of the range, and no number at all', () => {
		expect(problemOf('track', ['65535'])).toBeUndefined();
		expect(problemOf('trackTotal', ['1'])).toBeUndefined();
		expect(problemOf('track', [])).toBeUndefined();
	});

	it('refuses no title, and nothing else for being empty', () => {
		expect(problemOf('title', [])).toBe('A song needs a title.');
		expect(problemOf('comment', [])).toBeUndefined();
	});

	it.each(['2021', '2021-03', '2021-03-12', '2024-02-29'])('takes a date of %s', (date) => {
		expect(problemOf('date', [date])).toBeUndefined();
	});

	it.each(['2021-13', '2023-02-29', '12/03/2021', '21', '0000', '2021-3'])(
		'refuses a date of %s',
		(date) => {
			expect(problemOf('originalDate', [date])).toBe('A date is YYYY, YYYY-MM or YYYY-MM-DD.');
		}
	);
});

/** A tagger that answers from a script. */
function tagger(answer?: EditFailure | Error, fresh: AlbumTags = tags) {
	const editAlbum = vi.fn(() =>
		answer === undefined
			? Promise.resolve()
			: Promise.reject(answer instanceof Error ? answer : new EditError(answer))
	);
	const albumTags = vi.fn(() => Promise.resolve(fresh));
	const reread = vi.fn(() => Promise.resolve());
	const tagged: Tagger = { albumTags, editAlbum, reread };
	return { tagged, editAlbum, albumTags, reread };
}

const edit = { tracks: [{ id: 't1', set: { date: ['1999'] } }] };
const partial: EditFailure = {
	kind: 'failed',
	detail: 'internal error',
	changed: ['t1'],
	remaining: ['t2']
};

describe('save', () => {
	it('sends one edit on the ETag it read, and is done', async () => {
		const { tagged, editAlbum, albumTags } = tagger();
		await expect(save(tagged, 'a', 'a:0', edit)).resolves.toEqual({ stop: null, fresh: null });
		expect(editAlbum).toHaveBeenCalledExactlyOnceWith('a', 'a:0', edit);
		expect(albumTags).not.toHaveBeenCalled();
	});

	it('hands back a 409 untouched: nothing has changed, and the draft stays', async () => {
		const failure: EditFailure = { kind: 'busy', retryAfterSeconds: 30 };
		const { tagged, albumTags } = tagger(failure);
		await expect(save(tagged, 'a', 'a:0', edit)).resolves.toEqual({ stop: failure, fresh: null });
		expect(albumTags).not.toHaveBeenCalled();
	});

	it('calls a 412 drift when reading again brings back the same ETag', async () => {
		const { tagged, reread } = tagger({ kind: 'stale' });
		const saved = await save(tagged, 'a', 'a:0', edit);
		expect(saved.stop).toEqual({ kind: 'drifted' });
		expect(reread).toHaveBeenCalledOnce();
	});

	it('leaves a 412 as stale when reading again brings something new', async () => {
		const fresh = { ...tags, etag: 'somebody-else' };
		const { tagged } = tagger({ kind: 'stale' }, fresh);
		await expect(save(tagged, 'a', 'a:0', edit)).resolves.toEqual({
			stop: { kind: 'stale' },
			fresh
		});
	});

	it('reads the files again after a stop partway, for what landed', async () => {
		const fresh = { ...tags, etag: 'moved' };
		const { tagged, reread } = tagger(partial, fresh);
		await expect(save(tagged, 'a', 'a:0', edit)).resolves.toEqual({ stop: partial, fresh });
		expect(reread).toHaveBeenCalledOnce();
	});

	it('never calls a write that stopped partway a drift', async () => {
		const failure: EditFailure = { ...partial, kind: 'stale' };
		const { tagged } = tagger(failure);
		expect((await save(tagged, 'a', 'a:0', edit)).stop).toEqual(failure);
	});

	it('leaves a refusal that changed nothing alone', async () => {
		const failure: EditFailure = {
			kind: 'refused',
			detail: 'no',
			changed: [],
			remaining: ['t1', 't2']
		};
		const { tagged, albumTags } = tagger(failure);
		expect((await save(tagged, 'a', 'a:0', edit)).stop).toEqual(failure);
		expect(albumTags).not.toHaveBeenCalled();
	});

	it('claims nothing about a 412 it could not read back', async () => {
		const { tagged, albumTags } = tagger({ kind: 'stale' });
		albumTags.mockRejectedValue(new AlbumNotFoundError('a'));
		await expect(save(tagged, 'a', 'a:0', edit)).resolves.toEqual({
			stop: { kind: 'unavailable' },
			fresh: null
		});
	});

	it('keeps saying how far a write got, when it could not read back', async () => {
		const { tagged, albumTags } = tagger(partial);
		albumTags.mockRejectedValue(new Error('offline'));
		expect((await save(tagged, 'a', 'a:0', edit)).stop).toEqual(partial);
	});

	it('never rejects, even as the session goes', async () => {
		const { tagged } = tagger(new UnauthorizedError());
		expect((await save(tagged, 'a', 'a:0', edit)).stop).toEqual({ kind: 'unavailable' });
	});
});

describe('reportOf', () => {
	const titleOf = (id: string): string => id.toUpperCase();

	it('says when to come back from a 409, and that the edits are safe', () => {
		const report = reportOf({ kind: 'busy', retryAfterSeconds: 30 }, titleOf);
		expect(report.message).toContain('30 seconds');
		expect(report.message).toContain('still here');
		expect(report.retry).toBe(true);
	});

	it.each(['drifted', 'forbidden', 'gone', 'missing'] as const)(
		'offers no retry after %s',
		(kind) => {
			expect(reportOf({ kind }, titleOf).retry).toBe(false);
		}
	);

	it('names a drift as something only a scan fixes', () => {
		expect(reportOf({ kind: 'drifted' }, titleOf).message).toMatch(/outside Kitsune.*scan/);
	});

	it('tells a 412 that can be retried from one that cannot', () => {
		const report = reportOf({ kind: 'stale' }, titleOf);
		expect(report.message).toContain('changed while you were editing');
		expect(report.retry).toBe(true);
	});

	it('gives a 422 in the server’s words, and says nothing landed', () => {
		const report = reportOf(
			{ kind: 'refused', detail: 'no good', changed: [], remaining: ['t1', 't2'] },
			titleOf
		);
		expect(report.message).toContain('Nothing was written');
		expect(report.detail).toBe('no good');
		expect(report.written).toEqual([]);
		expect(report.unwritten).toEqual([]);
	});

	it('names what landed and what did not after a stop partway', () => {
		const report = reportOf(partial, titleOf);
		expect(report.written).toEqual(['T1']);
		expect(report.unwritten).toEqual(['T2']);
		expect(report.detail).toBeUndefined();
		expect(report.retry).toBe(true);
	});

	it('does not say nothing landed when something did', () => {
		const report = reportOf(
			{ kind: 'refused', detail: 'no', changed: ['t1'], remaining: ['t2'] },
			titleOf
		);
		expect(report.message).toContain('partway');
		expect(report.message).not.toContain('Nothing was written');
	});

	it('claims nothing about an edit that got no answer', () => {
		const report = reportOf({ kind: 'unavailable' }, titleOf);
		expect(report.message).toContain('may not have landed');
		expect(report.written).toEqual([]);
		expect(report.retry).toBe(true);
	});
});
