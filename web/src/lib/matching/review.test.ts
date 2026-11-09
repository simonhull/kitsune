import { describe, expect, it } from 'vitest';
import type { FieldValues, TrackTags } from '#lib/library/source.ts';
import {
	editOf,
	fieldRows,
	initialChoices,
	kindStates,
	pairOptions,
	repair,
	setChange,
	setKind,
	setSong,
	songRows,
	summaryOf,
	type Basis
} from './review.ts';
import type { MatchReview, MatchTrack } from './types.ts';

const release = 'r0000000-0000-4000-8000-000000000000';

function track(
	position: number,
	title: string,
	lengthMs: number,
	extra: FieldValues = {}
): MatchTrack {
	return {
		disc: 1,
		position,
		title,
		artist: 'Low Tide Choir',
		lengthMs,
		proposed: {
			title: [title],
			artists: ['Low Tide Choir'],
			track: [String(position)],
			trackTotal: ['4'],
			disc: ['1'],
			discTotal: ['1'],
			...extra
		},
		ids: {
			musicbrainzRecordingId: [`rec-${String(position)}`],
			musicbrainzTrackId: [`trk-${String(position)}`],
			musicbrainzArtistId: ['artist-1']
		}
	};
}

function file(id: string, n: number, title: string, extra: FieldValues = {}): TrackTags {
	return {
		id,
		file: `${title}.flac`,
		tags: [],
		fields: {
			title: [title],
			artists: ['Low Tide Choir'],
			album: ['Glasshouse'],
			albumArtists: ['Low Tide Choir'],
			track: [String(n)],
			trackTotal: ['4'],
			disc: ['1'],
			discTotal: ['1'],
			date: ['2024'],
			genres: ['Dream pop'],
			...extra
		}
	};
}

const review: MatchReview = {
	candidate: {
		id: 'a.musicbrainz.r',
		score: 96,
		evidence: [],
		page: '',
		release: {
			id: release,
			title: 'Glasshouse',
			artist: 'Low Tide Choir',
			artists: [],
			labels: [],
			media: [],
			trackCount: 4
		}
	},
	album: {
		album: ['Glasshouse'],
		albumArtists: ['Low Tide Choir'],
		date: ['2021-03-12'],
		label: ['Saltmarsh Records'],
		genres: ['Dream pop', 'Ambient pop']
	},
	albumIds: {
		musicbrainzAlbumId: [release],
		musicbrainzReleaseGroupId: ['group'],
		musicbrainzAlbumArtistId: ['artist-1']
	},
	tracks: [
		track(1, 'Breakwater', 238_000),
		track(2, 'Breakwater (Room Version)', 252_000),
		track(3, 'Kelp & Glass', 201_000),
		track(4, 'Panes (Room Version)', 177_000)
	],
	pairs: [
		{ track: 'a', release: 0 },
		{ track: 'b', release: 1 },
		{ track: 'c', release: 2 },
		{ track: 'd', release: 3 }
	]
};

/** The files: the second's title differs, and the last two are each other's. */
function basis(): Basis {
	return {
		review,
		tags: [
			file('a', 1, 'Breakwater'),
			file('b', 2, 'Breakwater (Room)'),
			file('c', 3, 'Panes (Room)'),
			file('d', 4, 'Kelp and Glass')
		],
		lengths: { a: 238_400, b: 252_300, c: 177_500, d: 200_800 }
	};
}

describe('initialChoices', () => {
	it('ticks what differs, but genres, and only songs whose lengths agree', () => {
		const b = basis();
		const choices = initialChoices(b);
		expect(choices.songs).toEqual({ a: true, b: true, c: false, d: false });
		const { rows, agreeing } = fieldRows(b, choices);
		expect(rows.map(({ key, on }) => [key, on])).toEqual([
			['date', true],
			['label', true],
			['genres', false]
		]);
		expect(agreeing).toEqual(['Album', 'Album artist']);
		expect(rows[1]?.now).toEqual([]);
	});
});

describe('songRows', () => {
	it('lists each song with its pair, its changes and whether the pair looks wrong', () => {
		const b = basis();
		const rows = songRows(b, initialChoices(b));
		expect(rows.map(({ id, suspect, on }) => [id, suspect, on])).toEqual([
			['a', false, true],
			['b', false, true],
			['c', true, false],
			['d', true, false]
		]);
		expect(rows[0]?.changes).toEqual([]);
		expect(rows[1]?.changes).toEqual([
			{ kind: 'title', now: 'Breakwater (Room)', next: 'Breakwater (Room Version)', on: true }
		]);
	});

	it('shows a song’s place and totals as the board writes them', () => {
		const b = basis();
		const choices = repair(b, initialChoices(b), 'c', 3);
		const c = songRows(b, choices).find(({ id }) => id === 'c');
		expect(c?.changes.find(({ kind }) => kind === 'numbers')).toEqual({
			kind: 'numbers',
			now: '1·03',
			next: '1·04',
			on: true
		});
	});
});

describe('repair', () => {
	it('swaps two songs’ pairs, and ticks each once its lengths agree', () => {
		const b = basis();
		const choices = repair(b, initialChoices(b), 'c', 3);
		expect(choices.pairs).toEqual({ a: 0, b: 1, c: 3, d: 2 });
		expect(choices.songs).toEqual({ a: true, b: true, c: true, d: true });
	});

	it('leaves a song without a pair, and unticked', () => {
		const b = basis();
		const choices = repair(b, initialChoices(b), 'b', null);
		expect(choices.pairs['b']).toBeNull();
		expect(choices.songs['b']).toBe(false);
	});

	it('offers the tracks whose lengths agree first', () => {
		expect(pairOptions(basis(), 'c').map(({ index }) => index)).toEqual([3, 2, 0, 1]);
	});
});

describe('pills', () => {
	it('count a kind’s changes and take it across every song', () => {
		const b = basis();
		let choices = repair(b, initialChoices(b), 'c', 3);
		expect(kindStates(songRows(b, choices)).title).toEqual({ total: 3, on: 3, state: 'all' });
		choices = setChange(choices, 'b', 'title', false);
		expect(kindStates(songRows(b, choices)).title.state).toBe('some');
		choices = setKind(b, choices, 'title', false);
		expect(kindStates(songRows(b, choices)).title).toEqual({ total: 3, on: 0, state: 'none' });
	});
});

describe('editOf', () => {
	it('writes the ticked fields and IDs to ticked songs, and nothing to the others', () => {
		const b = basis();
		const edit = editOf(b, initialChoices(b));
		expect(edit.tracks?.map(({ id }) => id)).toEqual(['a', 'b']);
		expect(edit.tracks?.[1]?.set).toEqual({
			date: ['2021-03-12'],
			label: ['Saltmarsh Records'],
			title: ['Breakwater (Room Version)'],
			musicbrainzAlbumId: [release],
			musicbrainzReleaseGroupId: ['group'],
			musicbrainzRecordingId: ['rec-2'],
			musicbrainzTrackId: ['trk-2'],
			musicbrainzAlbumArtistId: ['artist-1'],
			musicbrainzArtistId: ['artist-1']
		});
	});

	it('keeps a song’s own title when its change is unticked, but still writes its IDs', () => {
		const b = basis();
		const set = editOf(b, setChange(initialChoices(b), 'b', 'title', false)).tracks?.[1]?.set;
		expect(set?.title).toBeUndefined();
		expect(set?.musicbrainzTrackId).toEqual(['trk-2']);
	});

	it('leaves out artist IDs that wouldn’t line up with the song’s artists', () => {
		const b = basis();
		b.tags = b.tags.map((t) =>
			t.id === 'a' ? { ...t, fields: { ...t.fields, artists: ['One', 'Two'] } } : t
		);
		const choices = setKind(b, initialChoices(b), 'artists', false);
		expect(editOf(b, choices).tracks?.[0]?.set?.musicbrainzArtistId).toBeUndefined();
	});

	it('sends nothing when no song is ticked', () => {
		const b = basis();
		let choices = initialChoices(b);
		choices = setSong(setSong(choices, 'a', false), 'b', false);
		expect(editOf(b, choices)).toEqual({ tracks: [] });
	});
});

describe('summaryOf', () => {
	it('counts what Apply writes, and warns when a rename would split the album', () => {
		const b = basis();
		expect(summaryOf(b, initialChoices(b))).toEqual({
			fields: 2,
			songChanges: 1,
			files: 2,
			of: 4,
			waiting: 2,
			splits: false
		});
		b.tags = b.tags.map((t) => ({ ...t, fields: { ...t.fields, album: ['Glass House'] } }));
		expect(summaryOf(b, initialChoices(b)).splits).toBe(true);
	});
});
