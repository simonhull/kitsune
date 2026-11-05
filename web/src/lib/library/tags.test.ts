import { describe, expect, it, vi } from 'vitest';
import {
	AlbumNotFoundError,
	EditError,
	UnauthorizedError,
	type Album,
	type EditFailure,
	type Track
} from './source.ts';
import {
	advanced,
	draftOf,
	fieldId,
	inAlbumOrder,
	problemsIn,
	rebased,
	reportOf,
	save,
	stepsOf,
	yearField,
	type Step,
	type Tagger
} from './tags.ts';

function track(id: string, extra: Partial<Track> = {}): Track {
	return {
		id,
		title: id.toUpperCase(),
		artists: [],
		durationMs: 1000,
		format: 'FLAC',
		genres: ['Rock'],
		etag: `${id}:0`,
		...extra
	};
}

const album: Album = {
	id: 'a',
	title: 'A',
	artist: 'B',
	artists: [],
	year: 2001,
	trackCount: 2,
	compilation: false,
	tracks: [track('t1', { trackNo: 1, discNo: 1 }), track('t2', { trackNo: 2, discNo: 1 })],
	durationMs: 2000,
	formats: ['FLAC'],
	addedAt: '2026-10-12T18:00:00Z',
	genres: ['Rock'],
	etag: 'a:0'
};

function row(index: number) {
	const draft = draftOf(album);
	const found = draft.tracks.at(index);
	if (found === undefined) {
		throw new Error('no such row');
	}
	return { draft, row: found };
}

describe('stepsOf', () => {
	it('has nothing to do for an untouched album', () => {
		expect(stepsOf(draftOf(album), draftOf(album))).toEqual([]);
	});

	it('sends only what changed, the album first', () => {
		const { draft, row: second } = row(1);
		second.title = 'New';
		draft.year = '1999';
		expect(stepsOf(draftOf(album), draft)).toEqual([
			{ kind: 'album', edit: { year: 1999 } },
			{ kind: 'track', id: 't2', edit: { title: 'New' } }
		]);
	});

	it('takes genres as a list, and an empty one removes them', () => {
		const draft = draftOf(album);
		draft.genres = [];
		expect(stepsOf(draftOf(album), draft)).toEqual([{ kind: 'album', edit: { genres: [] } }]);
	});

	it('leaves a cleared year or number alone: neither can be removed', () => {
		const { draft, row: first } = row(0);
		draft.year = '';
		first.trackNo = ' ';
		first.discNo = '';
		expect(stepsOf(draftOf(album), draft)).toEqual([]);
	});

	it('reads a number for its value', () => {
		const { draft, row: first } = row(0);
		first.trackNo = '01';
		expect(stepsOf(draftOf(album), draft)).toEqual([]);
		first.trackNo = '03';
		first.discNo = '2';
		expect(stepsOf(draftOf(album), draft)).toEqual([
			{ kind: 'track', id: 't1', edit: { trackNo: 3, discNo: 2 } }
		]);
	});

	it('trims a title, and spaces alone change nothing', () => {
		const { draft, row: first } = row(0);
		first.title = ' T1 ';
		expect(stepsOf(draftOf(album), draft)).toEqual([]);
		first.title = ' Fixed ';
		expect(stepsOf(draftOf(album), draft)).toEqual([
			{ kind: 'track', id: 't1', edit: { title: 'Fixed' } }
		]);
	});

	it('can give a track its first number', () => {
		const bare = { ...album, tracks: [track('t1')] };
		const draft = draftOf(bare);
		const [first] = draft.tracks;
		if (first !== undefined) {
			first.trackNo = '4';
		}
		expect(stepsOf(draftOf(bare), draft)).toEqual([
			{ kind: 'track', id: 't1', edit: { trackNo: 4 } }
		]);
	});
});

describe('problemsIn', () => {
	it('finds nothing in an untouched album, however odd its tags', () => {
		const odd = { ...album, year: 12, tracks: [track('t1', { trackNo: 0 })] };
		expect(problemsIn(draftOf(odd), draftOf(odd)).size).toBe(0);
	});

	it.each(['999', '10000', 'soon', '19 99'])('refuses a year of %s', (year) => {
		const draft = draftOf(album);
		draft.year = year;
		expect([...problemsIn(draftOf(album), draft).keys()]).toEqual([yearField]);
	});

	it.each(['0', '65536', '1a', '-1'])('refuses a track number of %s', (number) => {
		const { draft, row: first } = row(0);
		first.trackNo = number;
		expect([...problemsIn(draftOf(album), draft).keys()]).toEqual([fieldId('t1', 'trackNo')]);
	});

	it('takes the ends of each range', () => {
		const { draft, row: first } = row(0);
		draft.year = '1000';
		first.trackNo = '65535';
		first.discNo = '1';
		expect(problemsIn(draftOf(album), draft).size).toBe(0);
	});

	it('refuses a blank title, and says nothing of a cleared number', () => {
		const { draft, row: first } = row(0);
		first.title = '  ';
		first.trackNo = '';
		expect([...problemsIn(draftOf(album), draft).keys()]).toEqual([fieldId('t1', 'title')]);
	});
});

describe('inAlbumOrder', () => {
	it('orders as the server does: disc, number with none last, title, ID', () => {
		const tracks = [
			track('none', { title: 'A' }),
			track('second-disc', { discNo: 2, trackNo: 1 }),
			track('b', { trackNo: 2, title: 'Same' }),
			track('a', { trackNo: 2, title: 'Same' }),
			track('first', { trackNo: 1, discNo: 1 })
		];
		expect(inAlbumOrder(tracks).map(({ id }) => id)).toEqual([
			'first',
			'a',
			'b',
			'none',
			'second-disc'
		]);
	});
});

describe('advanced', () => {
	it('moves past what landed, and only that', () => {
		const base = draftOf(album);
		const moved = advanced(base, [
			{ kind: 'album', edit: { year: 1999 } },
			{ kind: 'track', id: 't2', edit: { title: 'New', trackNo: 9 } }
		]);
		expect(moved.year).toBe('1999');
		expect(moved.genres).toEqual(['Rock']);
		expect(moved.tracks.at(0)).toEqual(base.tracks.at(0));
		expect(moved.tracks.at(1)).toEqual({ id: 't2', title: 'New', trackNo: '9', discNo: '1' });
	});
});

describe('rebased', () => {
	it('keeps what was touched and takes the rest from the album as it is now', () => {
		const base = draftOf(album);
		const { draft, row: first } = row(0);
		first.title = 'Mine';
		const fresh = draftOf({
			...album,
			year: 1980,
			tracks: [track('t1', { title: 'Theirs', trackNo: 7 }), track('t3')]
		});
		const rows = rebased(base, draft, fresh);
		expect(rows.year).toBe('1980');
		expect(rows.tracks).toEqual([
			{ id: 't1', title: 'Mine', trackNo: '7', discNo: '' },
			{ id: 't3', title: 'T3', trackNo: '', discNo: '' }
		]);
	});
});

/** A tagger that answers from a script, moving ETags on as a server would. */
function tagger(answers: Partial<Record<string, EditFailure | Error>> = {}, fresh = album) {
	const held = { current: album };
	const calls: string[] = [];
	const answer = (name: string, etag: string | undefined): Promise<void> => {
		calls.push(`${name}@${String(etag)}`);
		const scripted = answers[name];
		if (scripted === undefined) {
			held.current = { ...held.current, etag: `${held.current.etag}+` };
			return Promise.resolve();
		}
		return Promise.reject(scripted instanceof Error ? scripted : new EditError(scripted));
	};
	const reread = vi.fn(() => Promise.resolve(fresh));
	const tags: Tagger = {
		album: () => held.current,
		editAlbum: () => answer('album', held.current.etag),
		editTrack: (_album, id) => answer(id, held.current.etag),
		reread
	};
	return { tags, calls, reread };
}

const steps: Step[] = [
	{ kind: 'album', edit: { year: 1999 } },
	{ kind: 'track', id: 't1', edit: { title: 'One' } },
	{ kind: 'track', id: 't2', edit: { title: 'Two' } }
];

describe('save', () => {
	it('chains: each step waits for the last, on what it left', async () => {
		const { tags, calls } = tagger();
		const saved = await save(tags, 'a', steps);
		expect(calls).toEqual(['album@a:0', 't1@a:0+', 't2@a:0++']);
		expect(saved).toEqual({ done: steps, left: [], stop: null });
	});

	it('stops at the first failure, keeping what landed', async () => {
		const failure: EditFailure = { kind: 'busy', retryAfterSeconds: 30 };
		const { tags, calls, reread } = tagger({ t1: failure });
		const saved = await save(tags, 'a', steps);
		expect(calls).toHaveLength(2);
		expect(saved.done).toEqual(steps.slice(0, 1));
		expect(saved.left).toEqual(steps.slice(1));
		expect(saved.stop).toEqual(failure);
		expect(reread).not.toHaveBeenCalled();
	});

	it('calls a 412 drift when reading again brings back the same ETag', async () => {
		const { tags, reread } = tagger({ album: { kind: 'stale' } });
		const saved = await save(tags, 'a', steps);
		expect(saved.stop).toEqual({ kind: 'drifted' });
		expect(reread).toHaveBeenCalledOnce();
	});

	it('compares the ETag of the track that was refused, not the album', async () => {
		const { tags } = tagger({ t1: { kind: 'stale' } }, { ...album, etag: 'moved' });
		const saved = await save(tags, 'a', steps.slice(1));
		expect(saved.stop).toEqual({ kind: 'drifted' });
	});

	it('leaves a 412 as stale when reading again brings something new', async () => {
		const { tags, reread } = tagger(
			{ album: { kind: 'stale' } },
			{ ...album, etag: 'somebody-else' }
		);
		const saved = await save(tags, 'a', steps);
		expect(saved.stop).toEqual({ kind: 'stale' });
		expect(reread).toHaveBeenCalledOnce();
	});

	it('reads the album again after a stop partway, for what landed', async () => {
		const failure: EditFailure = {
			kind: 'failed',
			detail: 'internal error',
			changed: ['t1'],
			remaining: ['t2']
		};
		const { tags, reread } = tagger({ album: failure });
		const saved = await save(tags, 'a', steps);
		expect(saved.stop).toEqual(failure);
		expect(reread).toHaveBeenCalledOnce();
	});

	it('leaves a refusal that changed nothing alone', async () => {
		const failure: EditFailure = {
			kind: 'refused',
			detail: 'no',
			changed: [],
			remaining: ['t1', 't2']
		};
		const { tags, reread } = tagger({ album: failure });
		expect((await save(tags, 'a', steps)).stop).toEqual(failure);
		expect(reread).not.toHaveBeenCalled();
	});

	it('claims nothing about a 412 it could not read back', async () => {
		const { tags, reread } = tagger({ album: { kind: 'stale' } });
		reread.mockRejectedValue(new AlbumNotFoundError('a'));
		expect((await save(tags, 'a', steps)).stop).toEqual({ kind: 'unavailable' });
	});

	it('never rejects, even as the session goes', async () => {
		const { tags } = tagger({ album: new UnauthorizedError() });
		expect((await save(tags, 'a', steps)).stop).toEqual({ kind: 'unavailable' });
	});
});

describe('reportOf', () => {
	const titleOf = (id: string): string => id.toUpperCase();

	it('has nothing to say about a save that finished', () => {
		expect(reportOf({ done: steps, left: [], stop: null }, titleOf)).toBeNull();
	});

	it('says when to come back from a 409', () => {
		const report = reportOf(
			{ done: [], left: steps, stop: { kind: 'busy', retryAfterSeconds: 30 } },
			titleOf
		);
		expect(report?.message).toContain('30 seconds');
		expect(report?.retry).toBe(true);
	});

	it.each(['drifted', 'forbidden', 'gone'] as const)('offers no retry after %s', (kind) => {
		expect(reportOf({ done: [], left: steps, stop: { kind } }, titleOf)?.retry).toBe(false);
	});

	it('names a drift as something only a scan fixes', () => {
		const report = reportOf({ done: [], left: steps, stop: { kind: 'drifted' } }, titleOf);
		expect(report?.message).toMatch(/outside Kitsune.*scan/);
	});

	it('shows what changed and what did not across a chain', () => {
		const report = reportOf(
			{
				done: steps.slice(0, 2),
				left: steps.slice(2),
				stop: { kind: 'unwritable', detail: 'it is read-only', changed: [], remaining: ['t2'] }
			},
			titleOf
		);
		expect(report?.changed).toEqual(['Year', 'T1']);
		expect(report?.unchanged).toEqual(['T2']);
		expect(report?.detail).toBe('it is read-only');
		expect(report?.unchangedHeading).toBe('Not saved');
	});

	it('counts tracks when an album stopped partway', () => {
		const report = reportOf(
			{
				done: [],
				left: [{ kind: 'album', edit: { year: 1999, genres: [] } }, ...steps.slice(1)],
				stop: { kind: 'failed', detail: 'internal error', changed: ['t1'], remaining: ['t2', 't3'] }
			},
			titleOf
		);
		expect(report?.changed).toEqual(['Year and Genres on 1 of 3 tracks']);
		expect(report?.unchanged).toEqual(['Year and Genres on 2 of 3 tracks', 'T1', 'T2']);
		expect(report?.detail).toBeUndefined();
	});

	it('claims nothing about a step that got no answer', () => {
		const report = reportOf({ done: [], left: steps, stop: { kind: 'unavailable' } }, titleOf);
		expect(report?.unchangedHeading).toBe('Not confirmed');
		expect(report?.message).toContain('may not have landed');
	});
});
