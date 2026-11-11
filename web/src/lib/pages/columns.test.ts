import { describe, expect, it } from 'vitest';
import type { ColumnsPref, LibraryAlbum, LibraryTrack, MovableId } from '#lib/library/source.ts';
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
	framed,
	keyTarget,
	lineNotes,
	moveSaid,
	movable,
	reorderIn,
	wallSaid,
	columnName,
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
			shown: ['number', 'title', 'artist'],
			hiddenCount: 5
		});
	});

	it('keeps the title at 200 or more', () => {
		// The row with no columns: 14 + 20 + 12 + 28 + 12 + title + 12 + 44 + 12 + 36 + 14.
		const bare = 14 + 20 + 12 + 28 + 12 + 200 + 12 + 44 + 12 + 36 + 14;
		expect(fit(['year'], bare + 44 + 12 - 1, 'columns').shown).toEqual(['number', 'title']);
		expect(fit(['year'], bare + 44 + 12, 'columns').shown).toEqual(['number', 'title', 'year']);
	});

	it('stops at the first column that does not fit, even when a later one would', () => {
		expect(fit(['artist', 'year'], 504, 'columns')).toEqual({
			shown: ['number', 'title'],
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
		).toEqual({ order: ['number', 'title', 'year'], artistWhenDifferent: false });
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

	it('writes a half as ½, in the same ink', () => {
		const half = context({ track: track({ plays: 31 }), stars: 4.5 });
		expect(phoneLine(order, half, () => true)).toEqual(['★★★★½', '31 plays']);
		const least = context({ track: track({ plays: 31 }), stars: 0.5 });
		expect(phoneLine(order, least, () => true)).toEqual(['½', '31 plays']);
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
		const list = menuList({ order: ['plays', 'title', 'artist'], artistWhenDifferent: true });
		expect(list.slice(0, 5)).toEqual([
			{ id: 'number', on: true },
			{ id: 'plays', on: true },
			{ id: 'title', on: true },
			{ id: 'artist', on: true },
			{ id: 'genre', on: false }
		]);
		expect(list).toHaveLength(16);
	});

	it('reorders the shown columns', () => {
		expect(moved(defaultColumns, 'format', 2).order).toEqual([
			'number',
			'title',
			'format',
			'artist',
			'genre',
			'rating',
			'plays',
			'lastPlayed'
		]);
		expect(moved(defaultColumns, 'artist', 5).order).toEqual([
			'number',
			'title',
			'genre',
			'rating',
			'plays',
			'artist',
			'lastPlayed',
			'format'
		]);
	});

	it('moves the number and the title like any column', () => {
		expect(moved(defaultColumns, 'title', 4).order).toEqual([
			'number',
			'artist',
			'genre',
			'rating',
			'title',
			'plays',
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
			'number',
			'title',
			'genre',
			'artist',
			'album',
			'rating'
		]);
		// Search's own Artist comes right after the title, wherever the title is.
		expect(columnsIn(['rating', 'number', 'title'], 'search')).toEqual([
			'rating',
			'number',
			'title',
			'artist',
			'album'
		]);
		expect(columnsIn(['number', 'title', ...order], 'search')).toEqual([
			'number',
			'title',
			...order
		]);
	});

	it("forces Album into an artist's flat list, and drops it by album", () => {
		expect(columnsIn(['genre', 'artist', 'rating'], 'artist')).toEqual([
			'number',
			'title',
			'genre',
			'artist',
			'album',
			'rating'
		]);
		expect(columnsIn(['genre', 'rating'], 'artist')).toEqual([
			'number',
			'title',
			'album',
			'genre',
			'rating'
		]);
		expect(columnsIn([...order], 'artistAlbums')).toEqual(['genre', 'artist', 'rating']);
		const pref = {
			order: ['number', 'title', 'genre', 'rating'] as const,
			artistWhenDifferent: false
		};
		// The forced Album is frame: it can't be dragged, and a drop maps past it by neighbor.
		expect(movable('album', { ...pref, order: [...pref.order] })).toBe(false);
		const shown = columnsIn([...pref.order], 'artist');
		expect(
			reorderIn({ ...pref, order: [...pref.order] }, 'artist', shown, 'rating', 2).order
		).toEqual(['number', 'title', 'rating', 'genre']);
	});

	it('reads one shared preference: an album page leaves out the album artist always', () => {
		const pref = { order: [...order], artistWhenDifferent: false };
		expect(artistWhenDifferentIn(pref, 'library')).toBe(false);
		expect(artistWhenDifferentIn(pref, 'album')).toBe(true);
		expect(artistWhenDifferentIn({ ...pref, artistWhenDifferent: true }, 'search')).toBe(false);
		expect(artistWhenDifferentIn(pref, 'artist')).toBe(true);
		expect(artistWhenDifferentIn(pref, 'artistAlbums')).toBe(true);
	});
});

describe("an artist page's Artist cell", () => {
	it("leaves out a song that is only the page's artist's, whoever the album is by", () => {
		const theirs = { ...album, artist: 'Various Artists' };
		expect(cellOf('artist', context({ album: theirs, pageArtist: 'Low Tide Choir' }))).toEqual({
			look: 'name',
			text: ''
		});
		const guest = track({ artists: ['Mara Vey', 'Low Tide Choir'] });
		expect(
			cellOf('artist', context({ album: theirs, track: guest, pageArtist: 'Low Tide Choir' }))
		).toEqual({ look: 'name', text: 'Mara Vey, Low Tide Choir' });
	});

	it("says the album on a phone's line in a list with no album headings", () => {
		const order = ['number', 'title', 'album', 'genre'] as const;
		const withGenre = track({ displayGenres: ['Pop'] });
		expect(phoneLine([...order], context({ track: withGenre }), () => false)).toEqual(['Pop']);
		expect(
			phoneLine([...order], context({ track: withGenre, albumOnLine: true }), () => false)
		).toEqual(['Glasshouse', 'Pop']);
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
		expect(fit(['artist'], 600, 'columns', parts).shown).toEqual(['title', 'artist']);
		expect(fit(['artist'], 540, 'columns', parts).shown).toEqual(['title']);
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

	it('says the widths the same whatever the order, so a move leaves the list’s style alone', () => {
		const order: MovableId[] = ['number', 'title', 'artist', 'genre', 'rating', 'plays'];
		const moved = reorderIn({ ...defaultColumns, order }, 'library', order, 'plays', 2);
		expect(moved.order).not.toEqual(order);
		expect(columnVars(moved.order, undefined, 'columns')).toBe(
			columnVars(order, undefined, 'columns')
		);
	});

	it('resizes from the keyboard, 8 px a press and 16 with Shift', () => {
		expect(nudged(120, 'ArrowRight', false)).toBe(128);
		expect(nudged(120, 'ArrowLeft', true)).toBe(104);
		expect(nudged(120, 'Enter', false)).toBe(120);
	});

	it('fits the columns to their dragged widths', () => {
		const wide = resized(defaultColumns, 'genre', 360).widths;
		expect(fit(['genre', 'rating'], 700, 'columns').shown).toEqual([
			'number',
			'title',
			'genre',
			'rating'
		]);
		expect(fit(['genre', 'rating'], 700, 'columns', numbered, wide).shown).toEqual([
			'number',
			'title'
		]);
	});

	it('round-trips through the saved value, which may have none, and drops what it doesn’t know', () => {
		const pref = resized(defaultColumns, 'artist', 240);
		expect(savedColumns(JSON.parse(JSON.stringify(pref)) as ColumnsPref)).toEqual(pref);
		expect(savedColumns({ order: ['genre'], artistWhenDifferent: true })).toEqual({
			order: ['number', 'title', 'genre'],
			artistWhenDifferent: true
		});
		const odd = { order: [], artistWhenDifferent: true, widths: { skips: 90, genre: 5 } };
		expect(savedColumns(odd as unknown as ColumnsPref).widths).toEqual({ genre: 42 });
	});

	it('resets with the rest: the defaults have no widths', () => {
		expect(defaultColumns.widths).toBeUndefined();
	});
});

describe('the number and the title in the order', () => {
	it('puts them back where an old value had neither: the number first, the title after it', () => {
		expect(framed(['genre', 'artist'])).toEqual(['number', 'title', 'genre', 'artist']);
		expect(framed(['genre', 'number'])).toEqual(['genre', 'number', 'title']);
		expect(framed(['genre', 'title'])).toEqual(['number', 'genre', 'title']);
		expect(framed(['artist', 'title', 'number'])).toEqual(['artist', 'title', 'number']);
		expect(savedColumns({ order: ['genre'], artistWhenDifferent: true }).order).toEqual([
			'number',
			'title',
			'genre'
		]);
	});

	it('lays the title out flexible wherever it sits, and the number at its width', () => {
		expect(template(['artist', 'number', 'title', 'year'], 'columns')).toBe(
			'20px minmax(min(128px, var(--col-artist, 200px)), var(--col-artist, 200px)) 28px ' +
				'minmax(200px, 1fr) var(--col-year, 44px) 44px 36px'
		);
		// Beside a cover there's no number, whatever the order says.
		expect(
			template(['year', 'number', 'title'], 'columns', {
				handle: false,
				cover: true,
				number: false
			})
		).toBe('20px 40px var(--col-year, 44px) minmax(200px, 1fr) 44px 36px');
	});

	it('fits the same columns wherever the number and the title are', () => {
		const at = 900;
		const base = fit(defaultColumns.order, at, 'columns');
		const moved = fit(
			['artist', 'genre', 'title', 'rating', 'number', 'plays', 'lastPlayed', 'format'],
			at,
			'columns'
		);
		expect(new Set(moved.shown)).toEqual(new Set(base.shown));
		expect(moved.hiddenCount).toBe(base.hiddenCount);
	});
});

/** Every order of `items`. */
function orders<T>(items: readonly T[]): T[][] {
	if (items.length <= 1) {
		return [[...items]];
	}
	return items.flatMap((item, index) =>
		orders([...items.slice(0, index), ...items.slice(index + 1)]).map((rest) => [item, ...rest])
	);
}

describe('reorderIn', () => {
	const pref: ColumnsPref = defaultColumns;

	it('moves a column within the Library, by its new place', () => {
		const shown = fit(pref.order, 1104, 'columns').shown;
		expect(reorderIn(pref, 'library', shown, 'genre', 4).order).toEqual([
			'number',
			'title',
			'artist',
			'rating',
			'genre',
			'plays',
			'lastPlayed',
			'format'
		]);
		expect(reorderIn(pref, 'library', shown, 'format', 0).order).toEqual([
			'format',
			'number',
			'title',
			'artist',
			'genre',
			'rating',
			'plays',
			'lastPlayed'
		]);
		expect(reorderIn(pref, 'library', shown, 'title', 7).order.at(-1)).toBe('title');
	});

	it('changes nothing for a drop in place', () => {
		const shown = fit(pref.order, 1104, 'columns').shown;
		expect(reorderIn(pref, 'library', shown, 'genre', 3)).toBe(pref);
	});

	it('maps an album page, which leaves Album out, by neighbour', () => {
		const withAlbum: ColumnsPref = {
			...pref,
			order: ['number', 'title', 'artist', 'album', 'genre', 'rating']
		};
		const shown = columnsIn(withAlbum.order, 'album');
		// Genre dropped after the title: Album stays after Artist, its neighbour.
		expect(reorderIn(withAlbum, 'album', shown, 'genre', 2).order).toEqual([
			'number',
			'title',
			'genre',
			'artist',
			'album',
			'rating'
		]);
		// Rating dropped after Artist goes after Artist, before Album, in the Library too.
		expect(reorderIn(withAlbum, 'album', shown, 'rating', 3).order).toEqual([
			'number',
			'title',
			'artist',
			'rating',
			'album',
			'genre'
		]);
	});

	it('maps search, which puts in Artist and Album, by the neighbours the person has', () => {
		const own: ColumnsPref = { ...pref, order: ['number', 'title', 'genre', 'rating'] };
		const shown = fit(columnsIn(own.order, 'search'), 1600, 'columns', {
			handle: false,
			cover: true,
			number: false
		}).shown;
		expect(shown).toEqual(['title', 'artist', 'album', 'genre', 'rating']);
		// Rating dropped first: before the title, its right neighbour.
		expect(reorderIn(own, 'search', shown, 'rating', 0).order).toEqual([
			'number',
			'rating',
			'title',
			'genre'
		]);
		// Search's own columns aren't the person's to move.
		expect(reorderIn(own, 'search', shown, 'artist', 4)).toBe(own);
	});

	it('keeps the columns hidden at this width behind the ones shown', () => {
		const tight: ColumnsPref = {
			...pref,
			order: ['number', 'artist', 'year', 'genre', 'title']
		};
		// Room for one column: Artist shows, Year and Genre don't, and the title is placed last.
		const shown: ColumnsPref['order'] = ['number', 'artist', 'title'];
		const next = reorderIn(tight, 'library', shown, 'artist', 2);
		expect(next.order).toEqual(['number', 'title', 'artist', 'year', 'genre']);
	});

	it('never changes which columns fit, for every drop of every column', () => {
		for (const width of [700, 900, 1104, 1600]) {
			const shown = fit(pref.order, width, 'columns').shown;
			for (const id of shown) {
				for (let to = 0; to < shown.length; to++) {
					const next = reorderIn(pref, 'library', shown, id, to);
					const after = fit(next.order, width, 'columns');
					expect(new Set(after.shown)).toEqual(new Set(shown));
					expect(after.shown).toEqual(
						(() => {
							const lineup = shown.filter((each) => each !== id);
							lineup.splice(to, 0, id);
							return lineup;
						})()
					);
				}
			}
		}
	});

	it('fits the same set in any order of a fitted lineup', () => {
		const shown = fit(pref.order, 900, 'columns').shown;
		for (const order of orders(shown)) {
			expect(new Set(fit(order, 900, 'columns').shown)).toEqual(new Set(shown));
		}
	});
});

describe('the keys', () => {
	it('moves one place, or to an end, and stops at the walls', () => {
		expect(keyTarget(2, 6, 'ArrowLeft')).toBe(1);
		expect(keyTarget(2, 6, 'ArrowRight')).toBe(3);
		expect(keyTarget(2, 6, 'Home')).toBe(0);
		expect(keyTarget(2, 6, 'End')).toBe(5);
		expect(keyTarget(0, 6, 'ArrowLeft')).toBeNull();
		expect(keyTarget(0, 6, 'Home')).toBeNull();
		expect(keyTarget(5, 6, 'ArrowRight')).toBeNull();
		expect(keyTarget(5, 6, 'End')).toBeNull();
		expect(keyTarget(2, 6, 'Enter')).toBeNull();
	});

	it('says what happened, politely', () => {
		const lineup: ColumnsPref['order'] = ['number', 'title', 'artist', 'rating', 'plays', 'genre'];
		expect(moveSaid(lineup, 'genre')).toBe('Genre moved to 6 of 6, after Plays.');
		expect(moveSaid(lineup, 'number')).toBe('Number moved to 1 of 6, first.');
		expect(wallSaid('artist', 'ArrowLeft')).toBe('Artist is already first.');
		expect(wallSaid('title', 'End')).toBe('Title is already last. Time stays put.');
		expect(columnName(lineup, 'rating')).toBe('Rating, column 4 of 6');
	});
});

describe('lineNotes', () => {
	it('notes what the heading says and what the line has no room for', () => {
		const rated = context({
			track: track({
				plays: 31,
				displayGenres: ['Dream pop'],
				lastPlayedAt: '2026-11-06T10:00:00'
			}),
			stars: 4
		});
		const order = defaultColumns.order;
		const notes = lineNotes(order, rated, (id) => id === 'genre', 24);
		expect(notes.get('genre')).toBe('heading');
		expect(notes.get('rating')).toBeUndefined();
		expect(notes.get('plays')).toBeUndefined();
		expect(notes.get('lastPlayed')).toBe('over');
		// The number and the title are never parts of the line, wherever they are.
		expect(phoneLine(['plays', 'title', 'number'], rated, () => false)).toEqual(['31 plays']);
	});
});
