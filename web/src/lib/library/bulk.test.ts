import { describe, expect, it } from 'vitest';
import {
	bulkEditOf,
	bulkFieldId,
	bulkFields,
	bulkProblems,
	bulkSummaryOf,
	bulkTracks,
	emptyBulkDraft,
	findAndReplace,
	isBulkChanged,
	numberFromOne,
	shownOf,
	titleCase,
	titleFromFile,
	titlesFromFiles,
	titlesInTitleCase,
	withBulkField,
	type BulkTrack
} from './bulk.ts';
import type { FieldValues } from './source.ts';

function song(id: string, albumId: string, fields: FieldValues, file = `${id}.flac`): BulkTrack {
	return { id, albumId, tags: { id, file, fields, tags: [] } };
}

/** Two songs from Glasshouse and one from Tidewrack, as board F has them. */
function selection(): BulkTrack[] {
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

const etags = new Map([
	['glass', '"g"'],
	['tide', '"t"']
]);

describe('board D', () => {
	it('has its eleven fields in order', () => {
		expect(bulkFields.map(({ label }) => label)).toEqual([
			'Title',
			'Album',
			'Album artist',
			'Artists',
			'Composers',
			'Genres',
			'Year',
			'Track',
			'Disc',
			'Label',
			'Comment'
		]);
	});
});

describe('shownOf', () => {
	it('fills in what the songs share across albums and calls the rest Mixed', () => {
		const tracks = selection();
		const draft = emptyBulkDraft();
		expect(shownOf(tracks, draft, 'genres')).toEqual({ values: ['Dream pop'], mixed: false });
		expect(shownOf(tracks, draft, 'album')).toEqual({ values: [], mixed: true });
		expect(shownOf(tracks, draft, 'year')).toEqual({ values: [], mixed: true });
		expect(shownOf(tracks.slice(0, 2), draft, 'year')).toEqual({ values: ['2021'], mixed: false });
		expect(shownOf(tracks, draft, 'composers')).toEqual({ values: [], mixed: false });
	});

	it('shows what was typed over a mixed field', () => {
		const tracks = selection();
		const draft = withBulkField(tracks, emptyBulkDraft(), 'album', ['Glasshouse']);
		expect(shownOf(tracks, draft, 'album')).toEqual({ values: ['Glasshouse'], mixed: false });
		expect(isBulkChanged(tracks, draft, 'album')).toBe(true);
	});
});

describe('withBulkField', () => {
	it('forgets a field typed back to what every file holds', () => {
		const tracks = selection();
		const typed = withBulkField(tracks, emptyBulkDraft(), 'genres', ['Shoegaze']);
		expect(withBulkField(tracks, typed, 'genres', ['Dream pop']).fields).toEqual({});
	});
});

describe('quick jobs', () => {
	it('numbers from 1 in album order', () => {
		const tracks = selection().slice(0, 2).reverse();
		const draft = numberFromOne(tracks, emptyBulkDraft());
		expect(draft.own).toEqual({ b: { track: '1' }, a: { track: '2' } });
		expect(shownOf(tracks, draft, 'track').mixed).toBe(true);
	});

	it.each([
		['01 - Breakwater.flac', 'Breakwater'],
		['1-02 Salt in the Lungs.mp3', 'Salt in the Lungs'],
		['07. Panes.m4a', 'Panes'],
		['Small_Weather.flac', 'Small Weather'],
		['1999.flac', '1999'],
		['Tide Table.flac', 'Tide Table']
	])('takes a title from %s', (file, title) => {
		expect(titleFromFile(file)).toBe(title);
	});

	it('fills titles from file names', () => {
		const tracks = [song('x', 'glass', { title: ['Track 1'] }, '03 Condensation.flac')];
		expect(titlesFromFiles(tracks, emptyBulkDraft()).own).toEqual({ x: { title: 'Condensation' } });
	});

	it.each([
		['salt in the lungs', 'Salt in the Lungs'],
		['the end of the world', 'The End of the World'],
		['what we came for', 'What We Came For'],
		['DJ set in the dark', 'DJ Set in the Dark'],
		['love on an iPod', 'Love on an iPod'],
		['  (intro) breakwater', '  (Intro) Breakwater']
	])('puts %s in Title Case', (title, want) => {
		expect(titleCase(title)).toBe(want);
	});

	it('Title Cases what a song will hold, not what its file did', () => {
		const tracks = selection();
		const draft = titlesInTitleCase(tracks, emptyBulkDraft());
		expect(draft.own.a).toEqual({ title: 'Low Light (Intro)' });
		expect(draft.own.b).toEqual({ title: 'Breakwater' });
	});

	it('finds and replaces in titles, case and all', () => {
		const tracks = selection();
		const draft = findAndReplace(tracks, emptyBulkDraft(), 'Breakwater', 'Seawall');
		expect(draft.own).toEqual({ c: { title: 'Seawall (Reprise)' } });
		expect(findAndReplace(tracks, emptyBulkDraft(), 'nowhere', 'x')).toEqual(emptyBulkDraft());
	});

	it('gives way to the form when a field is typed after', () => {
		const tracks = selection();
		const numbered = numberFromOne(tracks, emptyBulkDraft());
		const typed = withBulkField(tracks, numbered, 'track', ['4']);
		expect(typed.own).toEqual({ a: {}, b: {}, c: {} });
		expect(shownOf(tracks, typed, 'track')).toEqual({ values: ['4'], mixed: false });
	});
});

describe('bulkEditOf', () => {
	it('sends the form to every song and each song its own on top, naming every album', () => {
		const tracks = selection();
		let draft = withBulkField(tracks, emptyBulkDraft(), 'composers', ['Mara Quill']);
		draft = titlesInTitleCase(tracks, draft);
		expect(bulkEditOf(tracks, draft, etags)).toEqual({
			albums: [
				{ id: 'glass', etag: '"g"' },
				{ id: 'tide', etag: '"t"' }
			],
			all: { set: { composers: ['Mara Quill'] } },
			tracks: [
				{ id: 'a', set: { title: ['Low Light (Intro)'] } },
				{ id: 'b', set: { title: ['Breakwater'] } },
				{ id: 'c' }
			]
		});
	});

	it('writes a year as the whole date, and clears what was emptied', () => {
		const tracks = selection().slice(0, 2);
		let draft = withBulkField(tracks, emptyBulkDraft(), 'year', ['1999']);
		draft = withBulkField(tracks, draft, 'genres', []);
		expect(bulkEditOf(tracks, draft, etags)?.all).toEqual({
			set: { date: ['1999'] },
			clear: ['genres']
		});
	});

	it('names only the songs and albums a quick job changed', () => {
		const tracks = selection();
		const draft = findAndReplace(tracks, emptyBulkDraft(), 'Reprise', 'Coda');
		expect(bulkEditOf(tracks, draft, etags)).toEqual({
			albums: [{ id: 'tide', etag: '"t"' }],
			tracks: [{ id: 'c', set: { title: ['Breakwater (Coda)'] } }]
		});
	});

	it('is null with nothing to change', () => {
		expect(bulkEditOf(selection(), emptyBulkDraft(), etags)).toBeNull();
		const tracks = selection();
		expect(
			bulkEditOf(tracks, numberFromOne(tracks.slice(0, 1), emptyBulkDraft()), etags)
		).toBeNull();
	});
});

describe('bulkProblems and bulkSummaryOf', () => {
	it('stops what the server would refuse', () => {
		const tracks = selection();
		let draft = withBulkField(tracks, emptyBulkDraft(), 'title', []);
		draft = withBulkField(tracks, draft, 'year', ['21']);
		draft = withBulkField(tracks, draft, 'disc', ['0']);
		expect([...bulkProblems(tracks, draft).keys()]).toEqual([
			bulkFieldId('title'),
			bulkFieldId('year'),
			bulkFieldId('disc')
		]);
	});

	it('counts a field of every song as one change across their files', () => {
		const tracks = selection();
		const draft = withBulkField(tracks, emptyBulkDraft(), 'composers', ['Mara Quill']);
		expect(bulkSummaryOf(tracks, draft)).toEqual({ changes: 1, files: 3 });
		const fixed = findAndReplace(tracks, emptyBulkDraft(), 'Reprise', 'Coda');
		expect(bulkSummaryOf(tracks, fixed)).toEqual({ changes: 1, files: 1 });
	});
});

describe('bulkTracks', () => {
	it('keeps album order, albums as given', () => {
		const tags = (ids: string[]) => ({
			etag: '"e"',
			tracks: ids.map((id) => ({ id, file: id, fields: {}, tags: [] }))
		});
		expect(
			bulkTracks([
				{ id: 'x', tags: tags(['2', '1']) },
				{ id: 'y', tags: tags(['3']) }
			]).map(({ id, albumId }) => `${albumId}/${id}`)
		).toEqual(['x/2', 'x/1', 'y/3']);
	});
});
