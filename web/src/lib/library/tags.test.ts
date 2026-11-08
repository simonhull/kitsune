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
import {
	albumWide,
	editOf,
	emptyDraft,
	fieldId,
	isChanged,
	isMulti,
	isTagChanged,
	isTrackEdited,
	problemsIn,
	rebased,
	reportOf,
	rowNote,
	save,
	summaryOf,
	tagGroups,
	tagValueOf,
	valueOf,
	withField,
	withTag,
	yearField,
	type Draft,
	type Tagger
} from './tags.ts';

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

const titled = (id: string, title: string, on: Draft = emptyDraft()): Draft =>
	withField(tags, on, id, 'title', [title]);

describe('tagGroups', () => {
	const specs = tagGroups.flatMap(({ fields }) => fields);

	it('lists the board’s groups, in its order', () => {
		expect(tagGroups.map(({ name }) => name)).toEqual([
			'THE BASICS',
			'CREDITS',
			'RELEASE',
			'SORTING'
		]);
	});

	it('has one date field: a year is a date', () => {
		expect(specs.filter(({ label }) => label === 'Year')).toEqual([]);
		expect(specs.filter(({ field }) => field === 'date')).toHaveLength(1);
	});

	it('lets album, artists and album artist be edited, and shows compilation only', () => {
		expect(specs.filter(({ locked = false }) => locked).map(({ field }) => field)).toEqual([
			'compilation'
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

describe('albumWide', () => {
	it('reads the year the tracks agree on, from whole dates', () => {
		expect(albumWide(tags)).toMatchObject({ year: '2001', yearsDiffer: false });
	});

	it('says when years differ, and offers none', () => {
		const mixed = { ...tags, tracks: [track('t1'), track('t2', { date: ['1999'] })] };
		expect(albumWide(mixed)).toMatchObject({ year: '', yearsDiffer: true });
	});

	it('counts a track with no date as differing', () => {
		const mixed = { ...tags, tracks: [track('t1'), track('t2', { date: [] })] };
		expect(albumWide(mixed).yearsDiffer).toBe(true);
	});

	it('gathers genres in track order, and says when tracks differ', () => {
		const mixed = {
			...tags,
			tracks: [track('t1'), track('t2', { genres: ['Rock', 'Noise'] })]
		};
		expect(albumWide(mixed)).toMatchObject({ genres: ['Rock', 'Noise'], genresDiffer: true });
		expect(albumWide(tags)).toMatchObject({ genres: ['Rock'], genresDiffer: false });
	});
});

describe('editOf', () => {
	it('has nothing to send for an untouched album', () => {
		expect(editOf(tags, emptyDraft())).toBeNull();
	});

	it('sends only what changed, on the track that changed', () => {
		expect(editOf(tags, titled('t2', 'New'))).toEqual({
			tracks: [{ id: 't2', set: { title: ['New'] } }]
		});
	});

	it('sends a year and genres to every track, in one go', () => {
		expect(editOf(tags, { ...emptyDraft(), year: '1999', genres: ['Noise'] })).toEqual({
			all: { set: { date: ['1999'], genres: ['Noise'] } }
		});
	});

	it('clears for an emptied year, an emptied list and an emptied field', () => {
		const draft = withField(tags, emptyDraft(), 't1', 'track', ['']);
		expect(editOf(tags, { ...draft, year: '', genres: [] })).toEqual({
			all: { clear: ['date', 'genres'] },
			tracks: [{ id: 't1', clear: ['track'] }]
		});
	});

	it('sends a value and a clearing in one track, apart', () => {
		const draft = withField(tags, titled('t1', 'New'), 't1', 'disc', [' ']);
		expect(editOf(tags, draft)).toEqual({
			tracks: [{ id: 't1', set: { title: ['New'] }, clear: ['disc'] }]
		});
	});

	it('sends what is typed trimmed, and spaces alone change nothing', () => {
		expect(editOf(tags, titled('t1', ' T1 '))).toBeNull();
		expect(editOf(tags, titled('t1', ' Fixed '))).toEqual({
			tracks: [{ id: 't1', set: { title: ['Fixed'] } }]
		});
	});

	it('reads a number for its value', () => {
		expect(editOf(tags, withField(tags, emptyDraft(), 't1', 'track', ['01']))).toBeNull();
		expect(editOf(tags, withField(tags, emptyDraft(), 't1', 'track', ['03']))).toEqual({
			tracks: [{ id: 't1', set: { track: ['03'] } }]
		});
	});

	it('puts a track on top of the album: its own date wins', () => {
		const draft = withField(tags, { ...emptyDraft(), year: '1999' }, 't1', 'date', ['1999-02-03']);
		expect(editOf(tags, draft)).toEqual({
			all: { set: { date: ['1999'] } },
			tracks: [{ id: 't1', set: { date: ['1999-02-03'] } }]
		});
	});

	it('keeps a track out of the album edit when it clears the field', () => {
		const draft = withField(tags, { ...emptyDraft(), year: '1999' }, 't1', 'date', ['']);
		expect(editOf(tags, draft)).toEqual({
			all: { set: { date: ['1999'] } },
			tracks: [{ id: 't1', clear: ['date'] }]
		});
	});

	it('says nothing of a track that agrees with the album edit', () => {
		const draft = withField(tags, { ...emptyDraft(), year: '1999' }, 't1', 'date', ['1999']);
		expect(editOf(tags, draft)).toEqual({ all: { set: { date: ['1999'] } } });
	});

	it('sends the old value when a track holds out against the album edit', () => {
		const draft = withField(tags, { ...emptyDraft(), genres: ['Noise'] }, 't1', 'genres', ['Rock']);
		expect(editOf(tags, draft)).toEqual({
			all: { set: { genres: ['Noise'] } },
			tracks: [{ id: 't1', set: { genres: ['Rock'] } }]
		});
	});

	it('sends a whole list for a field of several values, never joined text', () => {
		const draft = withField(tags, emptyDraft(), 't1', 'composers', ['A; B', 'C']);
		expect(editOf(tags, draft)).toEqual({
			tracks: [{ id: 't1', set: { composers: ['A; B', 'C'] } }]
		});
	});

	it('changes a raw tag, and no values removes it', () => {
		const changed = withTag(tags, emptyDraft(), 't1', 'MOOD', ['Fast']);
		expect(editOf(tags, changed)).toEqual({
			tracks: [{ id: 't1', tags: [{ key: 'MOOD', values: ['Fast'] }] }]
		});
		const removed = withTag(tags, emptyDraft(), 't1', 'MOOD', ['']);
		expect(editOf(tags, removed)).toEqual({
			tracks: [{ id: 't1', tags: [{ key: 'MOOD', values: [] }] }]
		});
	});

	it('takes the first of several values as the one a single field shows', () => {
		const two = { ...tags, tracks: [track('t1', { comment: ['One', 'Two'] })] };
		expect(editOf(two, withField(two, emptyDraft(), 't1', 'comment', ['One']))).toBeNull();
	});
});

describe('valueOf', () => {
	it('reads through to the file until something is touched', () => {
		expect(valueOf(tags, emptyDraft(), 't1', 'title')).toEqual(['T1']);
		expect(isChanged(tags, emptyDraft(), 't1', 'title')).toBe(false);
	});

	it('keeps what is typed as typed, spaces and all', () => {
		const draft = titled('t1', 'New ');
		expect(valueOf(tags, draft, 't1', 'title')).toEqual(['New ']);
		expect(isChanged(tags, draft, 't1', 'title')).toBe(true);
	});

	it('shows the album edit on a track, as changed', () => {
		const draft = { ...emptyDraft(), year: '1999' };
		expect(valueOf(tags, draft, 't1', 'date')).toEqual(['1999']);
		expect(isChanged(tags, draft, 't1', 'date')).toBe(true);
	});

	it('forgets a field typed back to what it was', () => {
		expect(titled('t1', 'T1', titled('t1', 'New'))).toEqual(emptyDraft());
		const tagged = withTag(tags, withTag(tags, emptyDraft(), 't1', 'MOOD', ['X']), 't1', 'MOOD', [
			'Slow'
		]);
		expect(tagged).toEqual(emptyDraft());
	});

	it('reads raw tags the same way', () => {
		const draft = withTag(tags, emptyDraft(), 't1', 'MOOD', ['Fast']);
		expect(tagValueOf(tags, draft, 't1', 'MOOD')).toEqual(['Fast']);
		expect(isTagChanged(tags, draft, 't1', 'MOOD')).toBe(true);
		expect(isTagChanged(tags, emptyDraft(), 't1', 'MOOD')).toBe(false);
	});
});

describe('rowNote', () => {
	it('says nothing of an untouched row', () => {
		expect(rowNote(tags, emptyDraft(), 't1')).toEqual({ old: '', text: '' });
	});

	it('gives the old title to strike through', () => {
		expect(rowNote(tags, titled('t1', 'New'), 't1')).toEqual({ old: 'T1', text: '' });
	});

	it('says what number a track was, in the board’s words', () => {
		const draft = withField(tags, emptyDraft(), 't2', 'track', ['4']);
		expect(rowNote(tags, draft, 't2').text).toBe('was track 2');
	});

	it('says so when a number is cleared, and when there was none', () => {
		const bare = { ...tags, tracks: [track('t1', { disc: [] })] };
		const draft = withField(bare, emptyDraft(), 't1', 'track', ['']);
		expect(rowNote(bare, draft, 't1').text).toBe('was track 1');
		const given = withField(bare, emptyDraft(), 't1', 'disc', ['2']);
		expect(rowNote(bare, given, 't1').text).toBe('had no disc number');
	});

	it('gives both numbers when both moved', () => {
		const draft = withField(
			tags,
			withField(tags, emptyDraft(), 't2', 'disc', ['2']),
			't2',
			'track',
			['1']
		);
		expect(rowNote(tags, draft, 't2').text).toBe('was disc 1 · was track 2');
	});
});

describe('isTrackEdited', () => {
	it('sees what a row does not show', () => {
		const draft = withTag(tags, withField(tags, emptyDraft(), 't1', 'bpm', ['92']), 't2', 'X', [
			''
		]);
		expect(isTrackEdited(tags, draft, 't1')).toBe(true);
		expect(isTrackEdited(tags, draft, 't2')).toBe(false);
	});

	it('leaves the album edit to the album', () => {
		expect(isTrackEdited(tags, { ...emptyDraft(), year: '1999' }, 't1')).toBe(false);
	});
});

describe('problemsIn', () => {
	const number = (field: 'track' | 'trackTotal', value: string): Draft =>
		withField(tags, emptyDraft(), 't1', field, [value]);

	it('finds nothing in an untouched album, however odd its tags', () => {
		const odd = { ...tags, tracks: [track('t1', { date: ['soon'], track: ['0'], title: [] })] };
		expect(problemsIn(odd, emptyDraft()).size).toBe(0);
	});

	it.each(['999', '10000', 'soon', '19 99', '0000'])('refuses a year of %s', (year) => {
		expect([...problemsIn(tags, { ...emptyDraft(), year }).keys()]).toEqual([yearField]);
	});

	it('takes an emptied year: that clears it', () => {
		expect(problemsIn(tags, { ...emptyDraft(), year: '' }).size).toBe(0);
	});

	it.each(['0', '65536', '1a', '-1'])('refuses a track number of %s', (value) => {
		expect([...problemsIn(tags, number('track', value)).keys()]).toEqual([fieldId('t1', 'track')]);
	});

	it('takes the ends of the range, and no number at all', () => {
		expect(problemsIn(tags, number('track', '65535')).size).toBe(0);
		expect(problemsIn(tags, number('trackTotal', '1')).size).toBe(0);
		expect(problemsIn(tags, number('track', '')).size).toBe(0);
	});

	it('refuses a blank title, in the words of the board', () => {
		const problems = problemsIn(tags, titled('t1', '  '));
		expect(problems.get(fieldId('t1', 'title'))).toBe('A track needs a title.');
	});

	it.each(['2021', '2021-03', '2021-03-12', '2024-02-29'])('takes a date of %s', (date) => {
		expect(problemsIn(tags, withField(tags, emptyDraft(), 't1', 'date', [date])).size).toBe(0);
	});

	it.each(['2021-13', '2023-02-29', '12/03/2021', '21', '0000', '2021-3'])(
		'refuses a date of %s',
		(date) => {
			const draft = withField(tags, emptyDraft(), 't1', 'originalDate', [date]);
			expect([...problemsIn(tags, draft).keys()]).toEqual([fieldId('t1', 'originalDate')]);
		}
	);

	it('lists problems in the order of the page, row by row', () => {
		let draft = withField(tags, { ...emptyDraft(), year: 'x' }, 't2', 'title', ['']);
		draft = withField(tags, draft, 't1', 'title', ['']);
		draft = withField(tags, draft, 't1', 'track', ['x']);
		expect([...problemsIn(tags, draft).keys()]).toEqual([
			yearField,
			fieldId('t1', 'track'),
			fieldId('t1', 'title'),
			fieldId('t2', 'title')
		]);
	});
});

describe('summaryOf', () => {
	it('counts nothing for an untouched album', () => {
		expect(summaryOf(tags, emptyDraft())).toEqual({ changes: 0, files: 0, only: null });
	});

	it('counts an edit of every track once, across every file', () => {
		expect(summaryOf(tags, { ...emptyDraft(), year: '1999' })).toEqual({
			changes: 1,
			files: 2,
			only: null
		});
	});

	it('says which track, when every change is on one', () => {
		const draft = withField(tags, titled('t2', 'New'), 't2', 'bpm', ['92']);
		expect(summaryOf(tags, draft)).toEqual({ changes: 2, files: 1, only: 't2' });
	});

	it('counts files, not fields, and adds a raw tag in', () => {
		const draft = withTag(tags, titled('t2', 'New'), 't1', 'MOOD', ['Fast']);
		expect(summaryOf(tags, draft)).toEqual({ changes: 2, files: 2, only: null });
	});

	it('leaves out a change that has a problem: it will not be written', () => {
		expect(summaryOf(tags, titled('t1', ''))).toEqual({ changes: 0, files: 0, only: null });
		expect(summaryOf(tags, { ...emptyDraft(), year: 'x' }).changes).toBe(0);
	});
});

describe('rebased', () => {
	it('keeps what was touched and reads the rest from the files as they are now', () => {
		const draft = titled('t1', 'Mine');
		const fresh = { etag: 'b', tracks: [track('t1', { track: ['7'] }), track('t3')] };
		const moved = rebased(draft, fresh);
		expect(valueOf(fresh, moved, 't1', 'title')).toEqual(['Mine']);
		expect(valueOf(fresh, moved, 't1', 'track')).toEqual(['7']);
	});

	it('drops a track that has gone', () => {
		const fresh = { etag: 'b', tracks: [track('t1')] };
		expect(rebased(titled('t2', 'Mine'), fresh)).toEqual(emptyDraft());
	});

	it('keeps the album edit', () => {
		const fresh = { etag: 'b', tracks: [track('t1')] };
		expect(rebased({ ...emptyDraft(), year: '1999' }, fresh).year).toBe('1999');
	});

	it('drops what can no longer be touched', () => {
		const draft = withTag(
			tags,
			withField(tags, emptyDraft(), 't1', 'lyrics', ['Plain']),
			't1',
			'MOOD',
			['Fast']
		);
		const fresh = {
			etag: 'b',
			tracks: [
				track(
					't1',
					{},
					{ readOnly: ['lyrics'], tags: [{ key: 'MOOD', values: ['Slow'], readOnly: true }] }
				)
			]
		};
		expect(rebased(draft, fresh)).toEqual(emptyDraft());
	});

	it('finishes a partial write: what landed drops out, what did not stays', () => {
		const draft = titled('t2', 'Two', titled('t1', 'One'));
		const fresh = { etag: 'b', tracks: [track('t1', { title: ['One'] }), track('t2')] };
		expect(editOf(fresh, rebased(draft, fresh))).toEqual({
			tracks: [{ id: 't2', set: { title: ['Two'] } }]
		});
	});
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

const edit = { all: { set: { date: ['1999'] } } };
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
