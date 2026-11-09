import { describe, expect, it } from 'vitest';
import type { ColumnsPref, LibraryAlbum, LibraryTrack } from '#lib/library/source.ts';
import {
	artistWhenDifferentIn,
	cellOf,
	clampWidth,
	columnVars,
	nudged,
	numbered,
	resized,
	widthAt,
	widthRange,
	columnsIn,
	coverFor,
	defaultColumns,
	fieldsFor,
	fit,
	fitLine,
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

describe('coverFor', () => {
	it('grows from 208 at 1440 to 256 at 1920 and 420 at 3840, and 112 stacked', () => {
		expect([1280, 1440, 1680, 1920, 2560, 3840, 5120].map((w) => coverFor(w, false))).toEqual([
			208, 208, 232, 256, 311, 420, 420
		]);
		expect(coverFor(390, true)).toBe(112);
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
			'20px 28px minmax(200px, 1fr) minmax(min(128px, var(--col-artist, 200px)), var(--col-artist, 200px)) ' +
				'var(--col-genre, 120px) var(--col-rating, 84px) var(--col-plays, 48px) ' +
				'var(--col-lastPlayed, 104px) var(--col-format, 88px) 44px 36px'
		);
		expect(template([], 'large')).toBe('24px 32px minmax(200px, 1fr) 56px 44px');
	});
});

describe('fieldsFor', () => {
	it('asks for what the columns show, in a fixed order, once, and the shuffle marks', () => {
		expect(fieldsFor(defaultColumns.order)).toEqual([
			'genre',
			'format',
			'plays',
			'lastPlayed',
			'shuffle'
		]);
		expect(fieldsFor(['bitrate', 'format', 'album', 'rating'])).toEqual([
			'format',
			'bitrate',
			'shuffle'
		]);
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

describe('fitLine', () => {
	it('keeps whole parts that fit beside the length, in order', () => {
		const parts = ['★★★★', '31 plays', 'Oct 23'];
		expect(fitLine(parts, '3:58', 40)).toEqual(parts);
		expect(fitLine(parts, '3:58', 26)).toEqual(['★★★★', '31 plays']);
		expect(fitLine(parts, '3:58', 8)).toEqual([]);
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
		expect(list).toHaveLength(14);
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

describe('columnsIn', () => {
	const order = ['genre', 'artist', 'album', 'rating'] as const;

	it('shows the Library every column, and an album page all but Album', () => {
		expect(columnsIn([...order], 'library')).toEqual(order);
		expect(columnsIn([...order], 'album')).toEqual(['genre', 'artist', 'rating']);
	});

	it('keeps Artist and Album on in search, Album after Artist where the person has none', () => {
		expect(columnsIn(['genre', 'artist', 'rating'], 'search')).toEqual([
			'genre',
			'artist',
			'album',
			'rating'
		]);
		expect(columnsIn(['rating'], 'search')).toEqual(['artist', 'album', 'rating']);
		expect(columnsIn([...order], 'search')).toEqual(order);
	});

	it('reads one shared preference: an album page leaves out the album artist always', () => {
		const pref = { order: [...order], artistWhenDifferent: false };
		expect(artistWhenDifferentIn(pref, 'library')).toBe(false);
		expect(artistWhenDifferentIn(pref, 'album')).toBe(true);
		expect(artistWhenDifferentIn({ ...pref, artistWhenDifferent: true }, 'search')).toBe(false);
	});
});

describe('the ReplayGain column', () => {
	it('says the gain, and warns when there is none', () => {
		expect(cellOf('gain', context({ track: track({ trackGain: -7.25 }) }))).toEqual({
			look: 'fact',
			text: '−7.3 dB'
		});
		expect(cellOf('gain', context())).toEqual({ look: 'fact', text: 'no RG', warn: true });
		expect(fieldsFor(['gain'])).toEqual(['gain', 'shuffle']);
	});

	it('marks a gain Kitsune measured, without the warning', () => {
		const measured = track({ trackGain: 2, trackGainSource: 'measured' });
		expect(cellOf('gain', context({ track: measured }))).toEqual({
			look: 'fact',
			text: '+2.0 dB',
			measured: true
		});
		const tagged = track({ trackGain: -6.2, trackGainSource: 'tags' });
		expect(cellOf('gain', context({ track: tagged }))).toEqual({ look: 'fact', text: '−6.2 dB' });
	});

	it('says on a phone which gain was measured', () => {
		const line = (fields: Partial<LibraryTrack>) =>
			phoneLine(['gain'], context({ track: track(fields) }), () => true);
		expect(line({ trackGain: -6.2, trackGainSource: 'tags' })).toEqual(['RG −6.2']);
		expect(line({ trackGain: -6.2, trackGainSource: 'measured' })).toEqual(['RG ≈−6.2']);
		expect(line({})).toEqual(['no RG']);
	});

	it('is off by default', () => {
		expect(defaultColumns.order).not.toContain('gain');
	});
});

describe('fit with a cover', () => {
	it('fits fewer columns beside a cover than beside a number', () => {
		const parts = { handle: false, cover: true, number: false };
		expect(template([], 'columns', parts)).toBe('20px 40px minmax(200px, 1fr) 44px 36px');
		expect(fit(['artist'], 600, 'columns', parts).shown).toEqual(['artist']);
		expect(fit(['artist'], 540, 'columns', parts).shown).toEqual([]);
	});
});

describe('column widths', () => {
	it('keeps a dragged width within its column’s range: its label at least, never silly wide', () => {
		expect(widthRange('genre')).toEqual({ min: 42, max: 360 });
		expect(clampWidth('genre', 10)).toBe(42);
		expect(clampWidth('genre', 2000)).toBe(360);
		expect(clampWidth('plays', 20)).toBe(42);
		expect(widthRange('heart')).toEqual({ min: 20, max: 48 });
		expect(widthRange('rating').min).toBeGreaterThanOrEqual(76);
		expect(clampWidth('artist', 230.4)).toBe(230);
	});

	it('takes a width up the step with the rest, kept at the base step', () => {
		const pref = resized(defaultColumns, 'genre', 160);
		expect(pref.widths).toEqual({ genre: 160 });
		expect(widthAt('genre', pref.widths, 'columns')).toBe(160);
		expect(widthAt('genre', pref.widths, 'large')).toBe(200);
		expect(widthAt('rating', pref.widths, 'large')).toBe(110);
		expect(columnVars(['genre', 'plays'], pref.widths, 'columns')).toBe(
			'--col-genre: 160px; --col-plays: 48px'
		);
	});

	it('resizes from the keyboard, 8 px a press and 16 with Shift', () => {
		expect(nudged(120, 'ArrowRight', false)).toBe(128);
		expect(nudged(120, 'ArrowLeft', true)).toBe(104);
		expect(nudged(120, 'Enter', false)).toBe(120);
	});

	it('fits the columns to their dragged widths', () => {
		const wide = resized(defaultColumns, 'genre', 360).widths;
		expect(fit(['genre', 'rating'], 700, 'columns').shown).toEqual(['genre', 'rating']);
		expect(fit(['genre', 'rating'], 700, 'columns', numbered, wide).shown).toEqual([]);
	});

	it('round-trips through the saved value, which may have none, and drops what it doesn’t know', () => {
		const pref = resized(defaultColumns, 'artist', 240);
		expect(savedColumns(JSON.parse(JSON.stringify(pref)) as ColumnsPref)).toEqual(pref);
		expect(savedColumns({ order: ['genre'], artistWhenDifferent: true })).toEqual({
			order: ['genre'],
			artistWhenDifferent: true
		});
		const odd = { order: [], artistWhenDifferent: true, widths: { skips: 90, genre: 5 } };
		expect(savedColumns(odd as unknown as ColumnsPref).widths).toEqual({ genre: 42 });
	});

	it('resets with the rest: the defaults have no widths', () => {
		expect(defaultColumns.widths).toBeUndefined();
	});
});
