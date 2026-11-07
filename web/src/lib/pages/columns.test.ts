import { describe, expect, it } from 'vitest';
import type { ColumnsPref, LibraryAlbum, LibraryTrack } from '#lib/library/source.ts';
import {
	cellOf,
	defaultColumns,
	fieldsFor,
	fit,
	formatCell,
	lastWhen,
	menuList,
	moved,
	phoneLine,
	savedColumns,
	stepFor,
	template,
	toggled,
	type CellContext
} from './columns.ts';

const album: LibraryAlbum = {
	id: 'a',
	title: 'Glasshouse',
	artist: 'Low Tide Choir',
	compilation: false,
	year: 2021,
	hearted: false,
	tracks: []
};

function track(extra: Partial<LibraryTrack> = {}): LibraryTrack {
	return {
		id: 't',
		title: 'Breakwater',
		durationMs: 238_000,
		artists: ['Low Tide Choir'],
		hearted: false,
		...extra
	};
}

const now = Date.parse('2026-11-07T12:00:00');

function context(extra: Partial<CellContext> = {}): CellContext {
	return {
		track: track(),
		album,
		stars: null,
		hearted: false,
		artistWhenDifferent: true,
		now,
		...extra
	};
}

describe('stepFor', () => {
	it('is two lines under 700, columns from 700, one size up from 2560', () => {
		expect([390, 699, 700, 1920, 2559, 2560, 3840].map(stepFor)).toEqual([
			'phone',
			'phone',
			'columns',
			'columns',
			'columns',
			'large',
			'large'
		]);
	});
});

describe('fit', () => {
	it('fits every default column at 1920, as the board has them', () => {
		expect(fit(defaultColumns.order, 1104, 'columns')).toEqual({
			shown: defaultColumns.order,
			hiddenCount: 0
		});
	});

	it('drops from the end of the order at 1280, keeping Artist as the board does', () => {
		expect(fit(defaultColumns.order, 558, 'columns')).toEqual({
			shown: ['artist'],
			hiddenCount: 5
		});
	});

	it('keeps the title at 200 or more', () => {
		// The row with no columns: 14 + 20 + 12 + 28 + 12 + title + 12 + 44 + 12 + 36 + 14.
		const bare = 14 + 20 + 12 + 28 + 12 + 200 + 12 + 44 + 12 + 36 + 14;
		expect(fit(['year'], bare + 44 + 12 - 1, 'columns').shown).toEqual([]);
		expect(fit(['year'], bare + 44 + 12, 'columns').shown).toEqual(['year']);
	});

	it('stops at the first column that does not fit, even when a later one would', () => {
		expect(fit(['artist', 'year'], 504, 'columns')).toEqual({
			shown: [],
			hiddenCount: 2
		});
	});

	it('fits the 3840 board, every column of it', () => {
		const order: ColumnsPref['order'] = [
			'artist',
			'albumArtist',
			'genre',
			'year',
			'rating',
			'heart',
			'plays',
			'lastPlayed',
			'added',
			'composer',
			'format',
			'bitrate'
		];
		expect(fit(order, 2572, 'large').hiddenCount).toBe(0);
	});
});

describe('template', () => {
	it('lays the board out, names giving up width before they drop', () => {
		expect(template(defaultColumns.order, 'columns')).toBe(
			'20px 28px minmax(200px, 1fr) minmax(136px, 200px) 120px 84px 48px 104px 88px 44px 36px'
		);
		expect(template([], 'large')).toBe('24px 32px minmax(200px, 1fr) 56px 44px');
	});
});

describe('fieldsFor', () => {
	it('asks for what the columns show, in a fixed order, once', () => {
		expect(fieldsFor(defaultColumns.order)).toEqual(['genre', 'format', 'plays', 'lastPlayed']);
		expect(fieldsFor(['bitrate', 'format', 'album', 'rating'])).toEqual(['format', 'bitrate']);
	});
});

describe('savedColumns', () => {
	it('starts from the defaults, and leaves out what it does not know', () => {
		expect(savedColumns(null)).toEqual(defaultColumns);
		expect(
			savedColumns({ order: ['skips', 'year', 'year'] as never, artistWhenDifferent: false })
		).toEqual({ order: ['year'], artistWhenDifferent: false });
	});
});

describe('cells', () => {
	it('leaves Artist empty where it is the album’s, under the switch', () => {
		expect(cellOf('artist', context())).toEqual({ look: 'name', text: '' });
		expect(cellOf('artist', context({ artistWhenDifferent: false }))).toEqual({
			look: 'name',
			text: 'Low Tide Choir'
		});
		const guest = track({ artists: ['Low Tide Choir', 'Mara Vey'] });
		expect(cellOf('artist', context({ track: guest }))).toEqual({
			look: 'name',
			text: 'Low Tide Choir, Mara Vey'
		});
	});

	it('says formats as the boards do', () => {
		expect(
			formatCell(track({ format: 'FLAC', bitDepth: 16, sampleRate: 44_100, bitrate: 912 }))
		).toBe('FLAC 16/44');
		expect(formatCell(track({ format: 'MP3', sampleRate: 44_100, bitrate: 320 }))).toBe('MP3 320');
		expect(formatCell(track())).toBe('');
	});

	it('says when, as the boards do', () => {
		expect(lastWhen(undefined, now)).toBe('Never');
		expect(lastWhen('2026-11-07T08:00:00', now)).toBe('Today');
		expect(lastWhen('2026-11-06T23:00:00', now)).toBe('Yesterday');
		expect(lastWhen('2026-11-05T10:00:00', now)).toBe('2 days ago');
		expect(lastWhen('2026-10-02T10:00:00', now)).toBe('Oct 2');
		expect(lastWhen('2022-03-02T10:00:00', now)).toBe('Mar 2022');
	});

	it('falls back to the album’s year, and counts plays from none', () => {
		expect(cellOf('year', context())).toEqual({ look: 'fact', text: '2021' });
		expect(cellOf('plays', context())).toEqual({ look: 'fact', text: '0' });
	});
});

describe('phoneLine', () => {
	const order = defaultColumns.order;
	const nothingShared = (): boolean => false;

	it('reads as the phone board does', () => {
		const rated = context({ track: track({ plays: 31 }), stars: 4 });
		expect(phoneLine(order, rated, () => true)).toEqual(['★★★★', '31 plays']);
		const guest = context({ track: track({ artists: ['Low Tide Choir', 'Mara Vey'] }), stars: 3 });
		expect(phoneLine(order, guest, () => true)).toEqual(['with Mara Vey', '★★★']);
	});

	it('leaves out what every song of the album shares', () => {
		const tagged = context({ track: track({ displayGenres: ['Dream pop'], plays: 2 }) });
		expect(phoneLine(order, tagged, nothingShared)).toEqual(['Dream pop', '2 plays']);
		expect(phoneLine(order, tagged, (id) => id === 'genre')).toEqual(['2 plays']);
	});
});

describe('the menu', () => {
	it('lists the shown columns first, in order, then the rest', () => {
		const list = menuList({ order: ['plays', 'artist'], artistWhenDifferent: true });
		expect(list.slice(0, 3)).toEqual([
			{ id: 'plays', on: true },
			{ id: 'artist', on: true },
			{ id: 'genre', on: false }
		]);
		expect(list).toHaveLength(13);
	});

	it('reorders the shown columns', () => {
		expect(moved(defaultColumns, 'format', 0).order).toEqual([
			'format',
			'artist',
			'genre',
			'rating',
			'plays',
			'lastPlayed'
		]);
		expect(moved(defaultColumns, 'artist', 3).order).toEqual([
			'genre',
			'rating',
			'plays',
			'artist',
			'lastPlayed',
			'format'
		]);
	});

	it('keeps the order when a hidden column moves', () => {
		expect(moved(defaultColumns, 'year', 1).order).toEqual(defaultColumns.order);
	});

	it('shows a column at the end, and hides one', () => {
		expect(toggled(defaultColumns, 'year').order.at(-1)).toBe('year');
		expect(toggled(defaultColumns, 'genre').order).not.toContain('genre');
	});
});
