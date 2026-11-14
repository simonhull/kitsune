import { describe, expect, it } from 'vitest';
import {
	editsOf,
	editTracks,
	emptyEditDraft,
	findAndReplace,
	isChanged,
	isSharedChanged,
	isTagChanged,
	isTrackEdited,
	numberFromOne,
	problemsOf,
	rebased,
	requestOf,
	sharedFieldId,
	sharedFields,
	shownOf,
	summaryOf,
	tagValueOf,
	titleCase,
	titleFromFile,
	titlesFromFiles,
	titlesInTitleCase,
	valueOf,
	withField,
	withOwn,
	withShared,
	withTag,
	type EditDraft,
	type EditTrack,
	type Ticked
} from './edit.ts';
import type { FieldValues, TrackTags } from './source.ts';
import { fieldId } from './tags.ts';

function song(
	id: string,
	albumId: string,
	fields: FieldValues,
	extra: Partial<TrackTags> = {}
): EditTrack {
	return { id, albumId, tags: { id, file: `${id}.flac`, fields, tags: [], ...extra } };
}

/** Two songs from Glasshouse and one from Tidewrack, as the selection board has them. */
function selection(): EditTrack[] {
	return [
		song('a', 'glass', {
			title: ['Low Light (intro)'],
			album: ['Glasshouse'],
			date: ['2021-04-02'],
			genres: ['Dream pop'],
			track: ['1']
		}),
		song('b', 'glass', {
			title: ['breakwater'],
			album: ['Glasshouse'],
			date: ['2021'],
			genres: ['Dream pop'],
			track: ['2']
		}),
		song('c', 'tide', {
			title: ['Breakwater (Reprise)'],
			album: ['Tidewrack'],
			date: ['2018'],
			genres: ['Dream pop'],
			track: ['7']
		})
	];
}

/** One album of two songs, with a raw tag on the first. */
function album(): EditTrack[] {
	const fields = (n: string): FieldValues => ({
		title: [`T${n}`],
		track: [n],
		disc: ['1'],
		date: ['2001-05-12'],
		genres: ['Rock']
	});
	return [
		song('t1', 'al', fields('1'), { tags: [{ key: 'MOOD', values: ['Slow'] }] }),
		song('t2', 'al', fields('2'))
	];
}

const every = (tracks: readonly EditTrack[]): Ticked => new Set(tracks.map(({ id }) => id));
const etags = new Map([
	['glass', '"g"'],
	['tide', '"t"'],
	['al', '"a"']
]);

/** One track of a list, by ID. */
function one(tracks: readonly EditTrack[], id: string): EditTrack {
	const found = tracks.find((track) => track.id === id);
	if (found === undefined) {
		throw new Error(`no ${id}`);
	}
	return found;
}

describe('the shared fields', () => {
	it('are the board’s eleven, in its order', () => {
		expect(sharedFields.map(({ label }) => label)).toEqual([
			'Title',
			'Album',
			'Album artist',
			'Artists',
			'Genres',
			'Year',
			'Track',
			'Disc',
			'Disc name',
			'Label',
			'Comment'
		]);
	});
});

describe('shownOf', () => {
	it('fills in what the ticked songs share across albums and calls the rest Mixed', () => {
		const tracks = selection();
		const draft = emptyEditDraft();
		expect(shownOf(tracks, draft, 'genres')).toEqual({ values: ['Dream pop'], mixed: false });
		expect(shownOf(tracks, draft, 'album')).toEqual({ values: [], mixed: true });
		expect(shownOf(tracks, draft, 'year')).toEqual({ values: [], mixed: true });
		expect(shownOf(tracks.slice(0, 2), draft, 'year')).toEqual({ values: ['2021'], mixed: false });
		expect(shownOf(tracks, draft, 'label')).toEqual({ values: [], mixed: false });
	});

	it('shows one ticked song’s own values, with nothing Mixed', () => {
		const [first] = selection();
		const shown = sharedFields.map(({ key }) =>
			shownOf([first as EditTrack], emptyEditDraft(), key)
		);
		expect(shown.some(({ mixed }) => mixed)).toBe(false);
		expect(shownOf([first as EditTrack], emptyEditDraft(), 'title').values).toEqual([
			'Low Light (intro)'
		]);
	});

	it('shows what was typed over a mixed field', () => {
		const tracks = selection();
		const draft = withShared(tracks, emptyEditDraft(), 'album', ['Glasshouse']);
		expect(shownOf(tracks, draft, 'album')).toEqual({ values: ['Glasshouse'], mixed: false });
		expect(isSharedChanged(tracks, draft, 'album')).toBe(true);
	});

	it('reads a song’s own value under the shared one', () => {
		const tracks = album();
		const ticked = every(tracks);
		const draft = withField(one(tracks, 't1'), emptyEditDraft(), ticked, 'genres', ['Noise']);
		expect(shownOf(tracks, draft, 'genres').mixed).toBe(true);
	});
});

describe('withShared', () => {
	it('goes to the ticked songs only', () => {
		const tracks = album();
		const ticked = new Set(['t2']);
		const draft = withShared([one(tracks, 't2')], emptyEditDraft(), 'genres', ['Noise']);
		expect(valueOf(one(tracks, 't2'), draft, ticked, 'genres')).toEqual(['Noise']);
		expect(valueOf(one(tracks, 't1'), draft, ticked, 'genres')).toEqual(['Rock']);
		expect(editsOf(tracks, draft, ticked)).toEqual([{ id: 't2', set: { genres: ['Noise'] } }]);
	});

	it('stops going to a song once it is unticked', () => {
		const tracks = album();
		const draft = withShared(tracks, emptyEditDraft(), 'genres', ['Noise']);
		expect(editsOf(tracks, draft, every(tracks))).toHaveLength(2);
		expect(editsOf(tracks, draft, new Set(['t1']))).toEqual([
			{ id: 't1', set: { genres: ['Noise'] } }
		]);
		expect(editsOf(tracks, draft, new Set())).toEqual([]);
	});

	it('forgets a field typed back to what every ticked file holds', () => {
		const tracks = selection();
		const typed = withShared(tracks, emptyEditDraft(), 'genres', ['Shoegaze']);
		expect(withShared(tracks, typed, 'genres', ['Dream pop']).shared).toEqual({});
	});

	it('takes over from what the ticked songs had of their own', () => {
		const tracks = album();
		const ticked = every(tracks);
		const own = withField(one(tracks, 't1'), emptyEditDraft(), ticked, 'title', ['Mine']);
		const draft = withShared(tracks, own, 'title', ['Same']);
		expect(draft.tracks).toEqual({});
		expect(shownOf(tracks, draft, 'title')).toEqual({ values: ['Same'], mixed: false });
	});
});

describe('a song’s own edit', () => {
	it('reads through to the file until something is touched', () => {
		const t1 = one(album(), 't1');
		expect(valueOf(t1, emptyEditDraft(), new Set(), 'title')).toEqual(['T1']);
		expect(isChanged(t1, emptyEditDraft(), new Set(), 'title')).toBe(false);
	});

	it('keeps what is typed as typed, spaces and all', () => {
		const t1 = one(album(), 't1');
		const draft = withField(t1, emptyEditDraft(), new Set(), 'title', ['New ']);
		expect(valueOf(t1, draft, new Set(), 'title')).toEqual(['New ']);
		expect(isChanged(t1, draft, new Set(), 'title')).toBe(true);
	});

	it('goes on top of the shared one, ticked or not', () => {
		const tracks = album();
		const ticked = every(tracks);
		let draft = withShared(tracks, emptyEditDraft(), 'year', ['1999']);
		draft = withField(one(tracks, 't1'), draft, ticked, 'date', ['1999-02-03']);
		expect(editsOf(tracks, draft, ticked)).toEqual([
			{ id: 't1', set: { date: ['1999-02-03'] } },
			{ id: 't2', set: { date: ['1999'] } }
		]);
		const unticked = withField(one(tracks, 't2'), emptyEditDraft(), new Set(), 'title', ['Two']);
		expect(editsOf(tracks, unticked, new Set())).toEqual([{ id: 't2', set: { title: ['Two'] } }]);
	});

	it('says nothing of a song that holds out against the shared edit', () => {
		const tracks = album();
		const ticked = every(tracks);
		let draft = withShared(tracks, emptyEditDraft(), 'genres', ['Noise']);
		draft = withField(one(tracks, 't1'), draft, ticked, 'genres', ['Rock']);
		expect(editsOf(tracks, draft, ticked)).toEqual([{ id: 't2', set: { genres: ['Noise'] } }]);
	});

	it('forgets a field typed back to what lies beneath', () => {
		const t1 = one(album(), 't1');
		const typed = withField(t1, emptyEditDraft(), new Set(), 'title', ['New']);
		expect(withField(t1, typed, new Set(), 'title', ['T1'])).toEqual(emptyEditDraft());
		const tagged = withTag(t1, withTag(t1, emptyEditDraft(), 'MOOD', ['X']), 'MOOD', ['Slow']);
		expect(tagged).toEqual(emptyEditDraft());
	});

	it('reads raw tags the same way, and no values removes one', () => {
		const tracks = album();
		const t1 = one(tracks, 't1');
		const draft = withTag(t1, emptyEditDraft(), 'MOOD', ['Fast']);
		expect(tagValueOf(t1, draft, 'MOOD')).toEqual(['Fast']);
		expect(isTagChanged(t1, draft, 'MOOD')).toBe(true);
		expect(isTagChanged(t1, emptyEditDraft(), 'MOOD')).toBe(false);
		expect(editsOf(tracks, withTag(t1, emptyEditDraft(), 'MOOD', ['']), new Set())).toEqual([
			{ id: 't1', tags: [{ key: 'MOOD', values: [] }] }
		]);
	});

	it('sees what a row does not show, and leaves the shared edit to the shared fields', () => {
		const tracks = album();
		const ticked = every(tracks);
		const draft = withField(one(tracks, 't1'), emptyEditDraft(), ticked, 'bpm', ['92']);
		expect(isTrackEdited(one(tracks, 't1'), draft, ticked)).toBe(true);
		expect(isTrackEdited(one(tracks, 't2'), draft, ticked)).toBe(false);
		const shared = withShared(tracks, emptyEditDraft(), 'year', ['1999']);
		expect(isTrackEdited(one(tracks, 't1'), shared, ticked)).toBe(false);
	});
});

describe('editsOf', () => {
	it('has nothing to send for an untouched edit', () => {
		expect(editsOf(album(), emptyEditDraft(), every(album()))).toEqual([]);
	});

	it('writes a year over a date only where the year changes, and clears what was emptied', () => {
		const tracks = selection().slice(0, 2);
		let draft = withShared(tracks, emptyEditDraft(), 'year', ['2021']);
		expect(editsOf(tracks, draft, every(tracks))).toEqual([]);
		draft = withShared(tracks, emptyEditDraft(), 'year', ['1999']);
		draft = withShared(tracks, draft, 'genres', []);
		expect(editsOf(tracks, draft, every(tracks))).toEqual([
			{ id: 'a', set: { date: ['1999'] }, clear: ['genres'] },
			{ id: 'b', set: { date: ['1999'] }, clear: ['genres'] }
		]);
	});

	it('sends a value and a clearing in one song, apart, trimmed', () => {
		const tracks = album();
		let draft = withField(one(tracks, 't1'), emptyEditDraft(), new Set(), 'title', [' New ']);
		draft = withField(one(tracks, 't1'), draft, new Set(), 'disc', [' ']);
		expect(editsOf(tracks, draft, new Set())).toEqual([
			{ id: 't1', set: { title: ['New'] }, clear: ['disc'] }
		]);
	});

	it('reads a number for its value, and spaces alone change nothing', () => {
		const tracks = album();
		const t1 = one(tracks, 't1');
		expect(
			editsOf(tracks, withField(t1, emptyEditDraft(), new Set(), 'track', ['01']), new Set())
		).toEqual([]);
		expect(
			editsOf(tracks, withField(t1, emptyEditDraft(), new Set(), 'title', [' T1 ']), new Set())
		).toEqual([]);
	});

	it('sends a whole list for a field of several values, never joined text', () => {
		const tracks = album();
		const draft = withField(one(tracks, 't1'), emptyEditDraft(), new Set(), 'composers', [
			'A; B',
			'C'
		]);
		expect(editsOf(tracks, draft, new Set())).toEqual([
			{ id: 't1', set: { composers: ['A; B', 'C'] } }
		]);
	});

	it('takes the first of several values as the one a single field shows', () => {
		const two = [song('t1', 'al', { comment: ['One', 'Two'] })];
		const draft = withField(one(two, 't1'), emptyEditDraft(), new Set(), 'comment', ['One']);
		expect(editsOf(two, draft, new Set())).toEqual([]);
	});
});

describe('requestOf', () => {
	it('sends songs all on one album through that album’s route', () => {
		const tracks = album();
		const draft = withShared(tracks, emptyEditDraft(), 'genres', ['Noise']);
		expect(requestOf(tracks, draft, every(tracks), etags)).toEqual({
			route: 'album',
			albumId: 'al',
			etag: '"a"',
			edit: {
				tracks: [
					{ id: 't1', set: { genres: ['Noise'] } },
					{ id: 't2', set: { genres: ['Noise'] } }
				]
			}
		});
	});

	it('sends songs across albums through POST /edits, naming every album they are on', () => {
		const tracks = selection();
		const draft = withShared(tracks, emptyEditDraft(), 'label', ['Fern']);
		expect(requestOf(tracks, draft, every(tracks), etags)).toEqual({
			route: 'tracks',
			edit: {
				albums: [
					{ id: 'glass', etag: '"g"' },
					{ id: 'tide', etag: '"t"' }
				],
				tracks: ['a', 'b', 'c'].map((id) => ({ id, set: { label: ['Fern'] } }))
			}
		});
	});

	it('goes by the songs that change, not the songs listed', () => {
		const tracks = selection();
		const draft = findAndReplace(tracks, emptyEditDraft(), 'Reprise', 'Coda');
		expect(requestOf(tracks, draft, every(tracks), etags)).toEqual({
			route: 'album',
			albumId: 'tide',
			etag: '"t"',
			edit: { tracks: [{ id: 'c', set: { title: ['Breakwater (Coda)'] } }] }
		});
	});

	it('is null with nothing to change', () => {
		const tracks = selection();
		expect(requestOf(tracks, emptyEditDraft(), every(tracks), etags)).toBeNull();
		expect(
			requestOf(tracks, numberFromOne(tracks.slice(0, 1), emptyEditDraft()), every(tracks), etags)
		).toBeNull();
	});
});

describe('quick jobs', () => {
	const own = (draft: EditDraft, id: string) => draft.tracks[id]?.fields;

	it('number the ticked songs from 1 in album order', () => {
		const tracks = selection().slice(0, 2).reverse();
		const draft = numberFromOne(tracks, emptyEditDraft());
		expect(own(draft, 'b')).toEqual({ track: ['1'] });
		expect(own(draft, 'a')).toEqual({ track: ['2'] });
		expect(shownOf(tracks, draft, 'track').mixed).toBe(true);
	});

	it('touch only the songs given: the ticked ones', () => {
		const tracks = selection();
		const draft = titlesInTitleCase([one(tracks, 'b')], emptyEditDraft());
		expect(Object.keys(draft.tracks)).toEqual(['b']);
	});

	it.each([
		['01 - Breakwater.flac', 'Breakwater'],
		['1-02 Salt in the Lungs.mp3', 'Salt in the Lungs'],
		['07. Panes.m4a', 'Panes'],
		['Small_Weather.flac', 'Small Weather'],
		['1999.flac', '1999'],
		['Tide Table.flac', 'Tide Table']
	])('take a title from %s', (file, title) => {
		expect(titleFromFile(file)).toBe(title);
	});

	it('fill titles from file names', () => {
		const tracks = [
			{
				id: 'x',
				albumId: 'glass',
				tags: { id: 'x', file: '03 Condensation.flac', fields: {}, tags: [] }
			}
		];
		expect(own(titlesFromFiles(tracks, emptyEditDraft()), 'x')).toEqual({
			title: ['Condensation']
		});
	});

	it.each([
		['salt in the lungs', 'Salt in the Lungs'],
		['the end of the world', 'The End of the World'],
		['what we came for', 'What We Came For'],
		['DJ set in the dark', 'DJ Set in the Dark'],
		['love on an iPod', 'Love on an iPod'],
		['  (intro) breakwater', '  (Intro) Breakwater']
	])('put %s in Title Case', (title, want) => {
		expect(titleCase(title)).toBe(want);
	});

	it('Title Case what a song will hold, not what its file did', () => {
		const tracks = selection();
		const renamed = withField(one(tracks, 'b'), emptyEditDraft(), every(tracks), 'title', [
			'the breakwater'
		]);
		const draft = titlesInTitleCase(tracks, renamed);
		expect(own(draft, 'a')).toEqual({ title: ['Low Light (Intro)'] });
		expect(own(draft, 'b')).toEqual({ title: ['The Breakwater'] });
	});

	it('find and replace in titles, case and all', () => {
		const tracks = selection();
		const draft = findAndReplace(tracks, emptyEditDraft(), 'Breakwater', 'Seawall');
		expect(draft.tracks).toEqual({ c: { fields: { title: ['Seawall (Reprise)'] }, tags: {} } });
		expect(findAndReplace(tracks, emptyEditDraft(), 'nowhere', 'x')).toEqual(emptyEditDraft());
	});

	it('give way to the shared field when it is typed after', () => {
		const tracks = selection();
		const numbered = numberFromOne(tracks, emptyEditDraft());
		const typed = withShared(tracks, numbered, 'track', ['4']);
		expect(typed.tracks).toEqual({});
		expect(shownOf(tracks, typed, 'track')).toEqual({ values: ['4'], mixed: false });
	});

	it('take a match’s values the way they take their own', () => {
		const tracks = album();
		const draft = withOwn(
			tracks,
			emptyEditDraft(),
			new Map([['t2', { title: ['Two'], musicbrainzTrackId: ['mb-2'] }]])
		);
		expect(editsOf(tracks, draft, every(tracks))).toEqual([
			{ id: 't2', set: { title: ['Two'], musicbrainzTrackId: ['mb-2'] } }
		]);
	});
});

describe('problemsOf', () => {
	it('finds nothing in an untouched edit, however odd its tags', () => {
		const odd = [song('t1', 'al', { date: ['soon'], track: ['0'], title: [] })];
		expect(problemsOf(odd, emptyEditDraft(), every(odd)).size).toBe(0);
	});

	it('stops what the server would refuse in the shared fields', () => {
		const tracks = selection();
		let draft = withShared(tracks, emptyEditDraft(), 'title', []);
		draft = withShared(tracks, draft, 'year', ['21']);
		draft = withShared(tracks, draft, 'disc', ['0']);
		expect([...problemsOf(tracks, draft, every(tracks)).keys()]).toEqual([
			sharedFieldId('title'),
			sharedFieldId('year'),
			sharedFieldId('disc')
		]);
		expect(problemsOf(tracks, draft, new Set()).size).toBe(0);
	});

	it('takes an emptied year: that clears it', () => {
		const tracks = album();
		const draft = withShared(tracks, emptyEditDraft(), 'year', ['']);
		expect(problemsOf(tracks, draft, every(tracks)).size).toBe(0);
		expect(editsOf(tracks, draft, every(tracks))).toEqual([
			{ id: 't1', clear: ['date'] },
			{ id: 't2', clear: ['date'] }
		]);
	});

	it.each(['0', '65536', '1a', '-1'])('refuses a track number of %s', (value) => {
		const tracks = album();
		const draft = withField(one(tracks, 't1'), emptyEditDraft(), new Set(), 'track', [value]);
		expect([...problemsOf(tracks, draft, new Set()).keys()]).toEqual([fieldId('t1', 'track')]);
	});

	it('refuses a blank title on a song, in the board’s words', () => {
		const tracks = album();
		const draft = withField(one(tracks, 't1'), emptyEditDraft(), new Set(), 'title', ['  ']);
		expect(problemsOf(tracks, draft, new Set()).get(fieldId('t1', 'title'))).toBe(
			'A song needs a title.'
		);
	});

	it('lists problems shared fields first, then song by song', () => {
		const tracks = album();
		const ticked = every(tracks);
		let draft = withShared(tracks, emptyEditDraft(), 'year', ['x']);
		draft = withField(one(tracks, 't2'), draft, ticked, 'title', ['']);
		draft = withField(one(tracks, 't1'), draft, ticked, 'track', ['x']);
		draft = withField(one(tracks, 't1'), draft, ticked, 'title', ['']);
		expect([...problemsOf(tracks, draft, ticked).keys()]).toEqual([
			sharedFieldId('year'),
			fieldId('t1', 'title'),
			fieldId('t1', 'track'),
			fieldId('t2', 'title')
		]);
	});
});

describe('summaryOf', () => {
	it('counts nothing for an untouched edit', () => {
		expect(summaryOf(album(), emptyEditDraft(), every(album()))).toEqual({
			changes: 0,
			files: 0,
			only: null
		});
	});

	it('counts a shared field as one change across the ticked songs’ files', () => {
		const tracks = selection();
		const draft = withShared(tracks, emptyEditDraft(), 'label', ['Fern']);
		expect(summaryOf(tracks, draft, every(tracks))).toEqual({ changes: 1, files: 3, only: null });
		expect(summaryOf(tracks, draft, new Set(['a']))).toEqual({ changes: 1, files: 1, only: null });
	});

	it('says which song, when every change is on one', () => {
		const tracks = album();
		let draft = withField(one(tracks, 't2'), emptyEditDraft(), new Set(), 'title', ['New']);
		draft = withField(one(tracks, 't2'), draft, new Set(), 'bpm', ['92']);
		expect(summaryOf(tracks, draft, new Set())).toEqual({ changes: 2, files: 1, only: 't2' });
	});

	it('counts files, not fields, and adds a raw tag in', () => {
		const tracks = album();
		let draft = withField(one(tracks, 't2'), emptyEditDraft(), new Set(), 'title', ['New']);
		draft = withTag(one(tracks, 't1'), draft, 'MOOD', ['Fast']);
		expect(summaryOf(tracks, draft, new Set())).toEqual({ changes: 2, files: 2, only: null });
	});

	it('leaves out a change that has a problem: it will not be written', () => {
		const tracks = album();
		const blank = withField(one(tracks, 't1'), emptyEditDraft(), new Set(), 'title', ['']);
		expect(summaryOf(tracks, blank, new Set()).changes).toBe(0);
		const year = withShared(tracks, emptyEditDraft(), 'year', ['x']);
		expect(summaryOf(tracks, year, every(tracks)).changes).toBe(0);
	});
});

describe('rebased', () => {
	const fresh = (tracks: TrackTags[]) => ({ etag: 'b', tracks });
	const tags = (track: EditTrack, fields: FieldValues = {}, extra: Partial<TrackTags> = {}) => ({
		...track.tags,
		fields: { ...track.tags.fields, ...fields },
		...extra
	});

	it('keeps what was touched and reads the rest from the files as they are now', () => {
		const tracks = album();
		const draft = withField(one(tracks, 't1'), emptyEditDraft(), new Set(), 'title', ['Mine']);
		const now = fresh([tags(one(tracks, 't1'), { track: ['7'] })]);
		const moved = rebased(draft, now, ['t1', 't2']);
		const [t1] = editTracks([{ id: 'al', tags: now }]);
		expect(valueOf(t1 as EditTrack, moved, new Set(), 'title')).toEqual(['Mine']);
		expect(valueOf(t1 as EditTrack, moved, new Set(), 'track')).toEqual(['7']);
	});

	it('drops a song that has left the album, and keeps other albums’ songs', () => {
		const tracks = [...album(), ...selection()];
		let draft = withField(one(tracks, 't2'), emptyEditDraft(), new Set(), 'title', ['Mine']);
		draft = withField(one(tracks, 'c'), draft, new Set(), 'title', ['Other']);
		const moved = rebased(draft, fresh([tags(one(tracks, 't1'))]), ['t1', 't2']);
		expect(Object.keys(moved.tracks)).toEqual(['c']);
	});

	it('keeps the shared fields, and drops what can no longer be touched', () => {
		const tracks = album();
		let draft = withShared(tracks, emptyEditDraft(), 'year', ['1999']);
		draft = withField(one(tracks, 't1'), draft, new Set(), 'lyrics', ['Plain']);
		draft = withTag(one(tracks, 't1'), draft, 'MOOD', ['Fast']);
		const now = fresh([
			tags(
				one(tracks, 't1'),
				{},
				{ readOnly: ['lyrics'], tags: [{ key: 'MOOD', values: ['Slow'], readOnly: true }] }
			)
		]);
		expect(rebased(draft, now, ['t1', 't2'])).toEqual({ shared: { year: ['1999'] }, tracks: {} });
	});

	it('finishes a partial write: what landed drops out, what did not stays', () => {
		const tracks = album();
		let draft = withField(one(tracks, 't1'), emptyEditDraft(), new Set(), 'title', ['One']);
		draft = withField(one(tracks, 't2'), draft, new Set(), 'title', ['Two']);
		const now = fresh([tags(one(tracks, 't1'), { title: ['One'] }), tags(one(tracks, 't2'))]);
		const after = editTracks([{ id: 'al', tags: now }]);
		expect(editsOf(after, rebased(draft, now, ['t1', 't2']), new Set())).toEqual([
			{ id: 't2', set: { title: ['Two'] } }
		]);
	});
});

describe('editTracks', () => {
	it('keeps album order, albums as given', () => {
		const tags = (ids: string[]) => ({
			etag: '"e"',
			tracks: ids.map((id) => ({ id, file: id, fields: {}, tags: [] }))
		});
		expect(
			editTracks([
				{ id: 'x', tags: tags(['2', '1']) },
				{ id: 'y', tags: tags(['3']) }
			]).map(({ id, albumId }) => `${albumId}/${id}`)
		).toEqual(['x/2', 'x/1', 'y/3']);
	});
});
