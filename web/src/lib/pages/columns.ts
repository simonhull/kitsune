import { clock, gain } from '#lib/library/format.ts';
import type {
	ColumnId,
	ColumnsPref,
	FrameId,
	IndexField,
	LibraryAlbum,
	LibraryTrack,
	MovableId
} from '#lib/library/source.ts';

/**
 * How a width lays the Library out. Under 700 the rows are two lines and there's no header; from
 * 700 the columns fill the width; from 2560 type, rows, covers and gutters take one step up, so a 4K
 * screen at 100% reads like 1920.
 */
export type Step = 'phone' | 'columns' | 'large';

export const columnsFrom = 700;
export const largeFrom = 2560;

export function stepFor(width: number): Step {
	if (width < columnsFrom) {
		return 'phone';
	}
	return width < largeFrom ? 'columns' : 'large';
}

/**
 * How big a cover beside its songs is: 208 up to 1440, growing to 256 at 1920 and 420 at 3840, and
 * no bigger. Stacked over its songs it's 112, heading its album.
 */
export function coverFor(width: number, stacked: boolean): number {
	if (stacked) {
		return 112;
	}
	if (width <= 1920) {
		return Math.round(Math.min(256, Math.max(208, 208 + (width - 1440) * 0.1)));
	}
	return Math.round(Math.min(420, 256 + ((width - 1920) * 164) / 1920));
}

/** A width at each step that has columns: as drawn at 1920, and at 3840. */
type Widths = readonly [columns: number, large: number];

export interface ColumnSpec {
	id: ColumnId;
	/** As the menu names it. */
	label: string;
	/** As the header shows it, in capitals. */
	head: string;
	width: Widths;
	/**
	 * A column of names gives up some of its width before it drops: the 1280 board has Artist at 150
	 * beside a title that would otherwise have pushed it out.
	 */
	least?: Widths;
	/** Numbers line up on the right. */
	end?: boolean;
	/** What it asks the index for. */
	field?: IndexField;
	/** Names are in the sans; facts and numbers in the mono. */
	look: 'name' | 'fact' | 'stars' | 'heart';
}

const names: Widths = [128, 180];

/** Every column a person may choose, in the menu's own order for those not shown. */
export const columnSpecs: readonly ColumnSpec[] = [
	{ id: 'artist', label: 'Artist', head: 'ARTIST', width: [200, 280], least: names, look: 'name' },
	{ id: 'genre', label: 'Genre', head: 'GENRE', width: [120, 150], field: 'genre', look: 'name' },
	{ id: 'rating', label: 'Rating', head: 'RATING', width: [84, 110], look: 'stars' },
	{
		id: 'plays',
		label: 'Plays',
		head: 'PLAYS',
		width: [48, 64],
		end: true,
		field: 'plays',
		look: 'fact'
	},
	{
		id: 'lastPlayed',
		label: 'Last played',
		head: 'LAST PLAYED',
		width: [104, 130],
		field: 'lastPlayed',
		look: 'fact'
	},
	{
		id: 'format',
		label: 'Format',
		head: 'FORMAT',
		width: [88, 120],
		field: 'format',
		look: 'fact'
	},
	{ id: 'heart', label: 'Heart', head: '', width: [20, 24], look: 'heart' },
	{
		id: 'albumArtist',
		label: 'Album artist',
		head: 'ALBUM ARTIST',
		width: [170, 220],
		least: names,
		field: 'albumArtist',
		look: 'name'
	},
	{ id: 'album', label: 'Album', head: 'ALBUM', width: [180, 240], least: names, look: 'name' },
	{ id: 'year', label: 'Year', head: 'YEAR', width: [44, 60], field: 'year', look: 'fact' },
	{
		id: 'composer',
		label: 'Composer',
		head: 'COMPOSER',
		width: [150, 200],
		least: names,
		field: 'composer',
		look: 'name'
	},
	{
		id: 'added',
		label: 'Date added',
		head: 'ADDED',
		width: [92, 120],
		field: 'added',
		look: 'fact'
	},
	{
		id: 'bitrate',
		label: 'Bitrate',
		head: 'KBPS',
		width: [48, 64],
		end: true,
		field: 'bitrate',
		look: 'fact'
	},
	{
		id: 'gain',
		label: 'ReplayGain',
		head: 'RG',
		// Room for the widest measured gain, `≈−12.4 dB`, without an ellipsis.
		width: [72, 96],
		end: true,
		field: 'gain',
		look: 'fact'
	}
];

const specs = new Map(columnSpecs.map((spec) => [spec.id, spec]));

export function specOf(id: ColumnId): ColumnSpec {
	const spec = specs.get(id);
	if (spec === undefined) {
		throw new Error(`no column ${id}`);
	}
	return spec;
}

/** Whether an id in an order is a column a person chose, and not the number or the title. */
export function isColumn(id: MovableId): id is ColumnId {
	return id !== 'number' && id !== 'title';
}

/** The columns a person chose in an order, without the number and the title. */
export function columnsOf(order: readonly MovableId[]): ColumnId[] {
	return order.filter(isColumn);
}

/** What the menu and the announcements call a column, the number and the title included. */
export function labelOf(id: MovableId): string {
	switch (id) {
		case 'number':
			return 'Number';
		case 'title':
			return 'Title';
		default:
			return specOf(id).label;
	}
}

/**
 * An order with the number and the title in it, where an old value had neither: the number first,
 * and the title right after the number. The server does the same with what it stores.
 */
export function framed(order: readonly MovableId[]): MovableId[] {
	const out = [...order];
	if (!out.includes('number')) {
		out.unshift('number');
	}
	if (!out.includes('title')) {
		out.splice(out.indexOf('number') + 1, 0, 'title');
	}
	return out;
}

export const defaultColumns: ColumnsPref = {
	order: ['number', 'title', 'artist', 'genre', 'rating', 'plays', 'lastPlayed', 'format'],
	artistWhenDifferent: true
};

const frameIds = new Set<MovableId>(['number', 'title'] satisfies FrameId[]);

/** Checks a saved value, so one from a newer server can't break the page: unknown columns go. */
export function savedColumns(saved: ColumnsPref | null): ColumnsPref {
	if (saved === null) {
		return defaultColumns;
	}
	const order = framed(
		[...new Set(saved.order)].filter((id) => frameIds.has(id) || specs.has(id as ColumnId))
	);
	const widths = Object.fromEntries(
		Object.entries(saved.widths ?? {}).flatMap(([id, width]) =>
			specs.has(id as ColumnId) && typeof width === 'number'
				? [[id, clampWidth(id as ColumnId, width)]]
				: []
		)
	) as Partial<Record<ColumnId, number>>;
	return Object.keys(widths).length === 0
		? { order, artistWhenDifferent: saved.artistWhenDifferent }
		: { order, artistWhenDifferent: saved.artistWhenDifferent, widths };
}

/** How wide a column may be dragged, at the base step: never under its label, never silly wide. */
export function widthRange(id: ColumnId): { min: number; max: number } {
	const spec = specOf(id);
	const base = spec.width[0];
	if (spec.look === 'heart') {
		return { min: 20, max: 48 };
	}
	if (spec.look === 'stars') {
		return { min: 76, max: 160 };
	}
	// The label in the header's 11 px mono, and a little room; an empty label, as an icon's, 48.
	const label = spec.head === '' ? 48 : Math.ceil(spec.head.length * 7.6) + 4;
	return { min: Math.min(base, Math.max(36, label)), max: Math.max(base * 3, 360) };
}

export function clampWidth(id: ColumnId, width: number): number {
	const { min, max } = widthRange(id);
	return Math.round(Math.min(max, Math.max(min, width)));
}

/** A column's width at the base step: the person's, or its default. */
export function baseWidth(id: ColumnId, widths: ColumnsPref['widths']): number {
	return widths?.[id] ?? specOf(id).width[0];
}

/** How a column's width scales from the base step to `step`, as its default widths do. */
export function widthScale(id: ColumnId, step: Exclude<Step, 'phone'>): number {
	const [columns, large] = specOf(id).width;
	return step === 'large' ? large / columns : 1;
}

/** A column's width at a step: the person's base width, taken up the step with it. */
export function widthAt(
	id: ColumnId,
	widths: ColumnsPref['widths'],
	step: Exclude<Step, 'phone'>
): number {
	return Math.round(baseWidth(id, widths) * widthScale(id, step));
}

/** The preference with a column dragged to `width` at the base step, kept in its range. */
export function resized(pref: ColumnsPref, id: ColumnId, width: number): ColumnsPref {
	return { ...pref, widths: { ...pref.widths, [id]: clampWidth(id, width) } };
}

/** A divider's arrow key: 8 px a press, 16 with Shift. */
export function nudged(width: number, key: string, shift: boolean): number {
	const by = shift ? 16 : 8;
	if (key === 'ArrowLeft') {
		return width - by;
	}
	return key === 'ArrowRight' ? width + by : width;
}

/**
 * The columns' widths as the custom properties the grid reads (`--col-genre: 120px`), set on the
 * list's container: a drag changes one property there, and no row. They come in the columns' own
 * order, not the person's, so a move leaves the text as it was and the list's style untouched: a
 * new style there restyles everything under it.
 */
export function columnVars(
	order: readonly MovableId[],
	widths: ColumnsPref['widths'],
	step: Exclude<Step, 'phone'>
): string {
	const shown = new Set(columnsOf(order));
	return columnSpecs
		.filter(({ id }) => shown.has(id))
		.map(({ id }) => `--col-${id}: ${String(widthAt(id, widths, step))}px`)
		.join('; ');
}

/**
 * The parts of a row a list turns on beside the box, the title, the time and ⋯, which every row
 * has: a queue's drag handle, a cover, and the number.
 */
export interface RowParts {
	handle: boolean;
	cover: boolean;
	number: boolean;
}

/** The Library's and an album's: a number, and no cover or handle. */
export const numbered: RowParts = { handle: false, cover: false, number: true };

/** The columns that are always there, and the room between them, at a step. */
export interface Frame {
	box: number;
	number: number;
	handle: number;
	cover: number;
	time: number;
	more: number;
	gap: number;
	pad: number;
	/** Title takes the rest, and never less than this. */
	title: number;
}

const frames: Record<Exclude<Step, 'phone'>, Frame> = {
	columns: {
		box: 20,
		number: 28,
		handle: 20,
		cover: 40,
		time: 44,
		more: 36,
		gap: 12,
		pad: 14,
		title: 200
	},
	large: {
		box: 24,
		number: 32,
		handle: 24,
		cover: 52,
		time: 56,
		more: 44,
		gap: 16,
		pad: 18,
		title: 200
	}
};

export function frameOf(step: Exclude<Step, 'phone'>): Frame {
	return frames[step];
}

const atStep = (widths: Widths, step: Exclude<Step, 'phone'>): number =>
	step === 'large' ? widths[1] : widths[0];

/** The tracks every row has before the ones an order places, at a step: they never move. */
function leading(frame: Frame, parts: RowParts): number[] {
	return [
		...(parts.handle ? [frame.handle] : []),
		frame.box,
		...(parts.cover ? [frame.cover] : [])
	];
}

/** How many grid tracks come before the ones an order places. */
export function leadingCount(parts: RowParts): number {
	return (parts.handle ? 1 : 0) + 1 + (parts.cover ? 1 : 0);
}

/**
 * What a row places between its leading tracks and its time, in order: `shown` with the number and
 * the title put back if it lacks them, and the number left out where the list has none.
 */
export function lineupOf(shown: readonly MovableId[], parts: RowParts = numbered): MovableId[] {
	return framed(shown).filter((id) => id !== 'number' || parts.number);
}

export interface Fit {
	/**
	 * What a row places, in order: the number (where the list has one), the title, and the columns
	 * that fit.
	 */
	shown: MovableId[];
	/** What the "+N" chip counts. */
	hiddenCount: number;
}

/**
 * Finds the columns a row `width` wide has room for, taking them in the person's order and dropping
 * from the end: once one doesn't fit, nothing after it shows either. The number and the title are
 * always there, wherever they sit, and the title keeps its least width, so where they sit never
 * changes which columns fit.
 */
export function fit(
	order: readonly MovableId[],
	width: number,
	step: Exclude<Step, 'phone'>,
	parts: RowParts = numbered,
	widths?: ColumnsPref['widths']
): Fit {
	const frame = frameOf(step);
	// What comes before the title, the title, time and ⋯, and the gaps between them.
	const fixed = [
		...leading(frame, parts),
		...(parts.number ? [frame.number] : []),
		frame.title,
		frame.time,
		frame.more
	];
	let room =
		width -
		2 * frame.pad -
		fixed.reduce((sum, each) => sum + each, 0) -
		(fixed.length - 1) * frame.gap;
	const columns = columnsOf(order);
	const fitted = new Set<ColumnId>();
	for (const id of columns) {
		const spec = specOf(id);
		const wide = widthAt(id, widths, step);
		const need =
			Math.min(spec.least === undefined ? wide : atStep(spec.least, step), wide) + frame.gap;
		if (need > room) {
			break;
		}
		room -= need;
		fitted.add(id);
	}
	return {
		shown: lineupOf(order, parts).filter((id) => !isColumn(id) || fitted.has(id)),
		hiddenCount: columns.length - fitted.size
	};
}

/** The grid every row and the header share, for the columns shown. */
export function template(
	shown: readonly MovableId[],
	step: Exclude<Step, 'phone'>,
	parts: RowParts = numbered,
	titleLeast = frameOf(step).title
): string {
	const frame = frameOf(step);
	// Each width is the container's `--col-<id>`, so a drag changes the property and not the grid.
	const placed = lineupOf(shown, parts).map((id) => {
		if (id === 'number') {
			return `${String(frame.number)}px`;
		}
		if (id === 'title') {
			return `minmax(${String(titleLeast)}px, 1fr)`;
		}
		const spec = specOf(id);
		const width = `var(--col-${id}, ${String(atStep(spec.width, step))}px)`;
		return spec.least === undefined
			? width
			: `minmax(min(${String(atStep(spec.least, step))}px, ${width}), ${width})`;
	});
	return [
		...leading(frame, parts).map((each) => `${String(each)}px`),
		...placed,
		`${String(frame.time)}px`,
		`${String(frame.more)}px`
	].join(' ');
}

/**
 * Where a list stands, for the columns: the Library shows every one chosen; an album page never
 * Album, which it is; search results come from everywhere, so they always say whose and from what.
 */
export type ColumnsContext = 'library' | 'album' | 'search';

/** The person's columns as a list shows them, in their order. */
export function columnsIn(order: readonly MovableId[], context: ColumnsContext): MovableId[] {
	switch (context) {
		case 'library':
			return [...order];
		case 'album':
			return order.filter((id) => id !== 'album');
		case 'search': {
			// Artist and Album always: Artist right after the title, and Album right after Artist,
			// when the person hasn't placed them.
			const out = framed(order);
			if (!out.includes('artist')) {
				out.splice(out.indexOf('title') + 1, 0, 'artist');
			}
			if (!out.includes('album')) {
				out.splice(out.indexOf('artist') + 1, 0, 'album');
			}
			return out;
		}
	}
}

/** The Artist switch as a list applies it: an album page always leaves the album's own artist out. */
export function artistWhenDifferentIn(pref: ColumnsPref, context: ColumnsContext): boolean {
	switch (context) {
		case 'library':
			return pref.artistWhenDifferent;
		case 'album':
			return true;
		case 'search':
			return false;
	}
}

/**
 * What the index must carry for the columns, in a fixed order, so the same columns ask the same.
 * The person's shuffle marks come always: every row shows them, whatever the columns.
 */
export function fieldsFor(order: readonly MovableId[]): IndexField[] {
	const wanted = new Set(columnsOf(order).map((id) => specOf(id).field));
	const all: IndexField[] = [
		'albumArtist',
		'genre',
		'year',
		'composer',
		'format',
		'bitrate',
		'added',
		'plays',
		'lastPlayed',
		'gain'
	];
	return [...all.filter((field) => wanted.has(field)), 'shuffle'];
}

/** As much of an album as a song row shows. */
export type RowAlbum = Pick<LibraryAlbum, 'id' | 'title' | 'artist' | 'year' | 'artwork'>;

/** What a cell needs to say its piece. */
export interface CellContext {
	track: LibraryTrack;
	album: RowAlbum;
	stars: number | null;
	hearted: boolean;
	artistWhenDifferent: boolean;
	/** Unix ms: when "Yesterday" is. */
	now: number;
}

export type Cell =
	| {
			look: 'name' | 'fact';
			text: string;
			/** Says something is missing, in the fox. */ warn?: true;
			/** A gain Kitsune measured, as no tag says it: the cell marks it quietly. */
			measured?: true;
	  }
	| { look: 'stars'; stars: number }
	| { look: 'heart'; hearted: boolean };

const dayMs = 86_400_000;
const monthDay = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' });
const monthYear = new Intl.DateTimeFormat('en-US', { month: 'short', year: 'numeric' });

/** Midnight before `ms`, in the browser's own day. */
function dayOf(ms: number): number {
	const day = new Date(ms);
	day.setHours(0, 0, 0, 0);
	return day.getTime();
}

/** When something last happened, as the boards say it: Today, Yesterday, 3 days ago, Oct 2, Mar 2022. */
export function lastWhen(at: string | undefined, now: number): string {
	if (at === undefined) {
		return 'Never';
	}
	const ms = Date.parse(at);
	const days = Math.round((dayOf(now) - dayOf(ms)) / dayMs);
	if (days <= 0) {
		return 'Today';
	}
	if (days === 1) {
		return 'Yesterday';
	}
	if (days < 7) {
		return `${String(days)} days ago`;
	}
	return new Date(ms).getFullYear() === new Date(now).getFullYear()
		? monthDay.format(ms)
		: monthYear.format(ms);
}

/** A track's format as a column says it: `FLAC 16/44`, `MP3 320`, or just the container. */
export function formatCell({ format, bitDepth, sampleRate, bitrate }: LibraryTrack): string {
	if (format === undefined) {
		return '';
	}
	if (bitDepth !== undefined && sampleRate !== undefined) {
		return `${format} ${String(bitDepth)}/${String(Math.floor(sampleRate / 1000))}`;
	}
	return bitrate === undefined ? format : `${format} ${String(bitrate)}`;
}

/** The track's artists as Artist shows them: nothing, under the switch, where they're the album's. */
export function artistCell(track: LibraryTrack, album: RowAlbum, whenDifferent: boolean): string {
	const named = track.artists.filter((artist) => artist !== '').join(', ');
	return whenDifferent && named === album.artist ? '' : named;
}

export function cellOf(id: ColumnId, context: CellContext): Cell {
	const { track, album, now } = context;
	const fact = (text: string): Cell => ({ look: 'fact', text });
	const name = (text: string): Cell => ({ look: 'name', text });
	switch (id) {
		case 'artist':
			return name(artistCell(track, album, context.artistWhenDifferent));
		case 'albumArtist':
			return name(track.albumArtists?.join(', ') ?? '');
		case 'album':
			return name(album.title);
		case 'genre':
			return name(track.displayGenres?.join(', ') ?? '');
		case 'composer':
			return name(track.composers?.join(', ') ?? '');
		case 'year': {
			const year = track.year ?? album.year;
			return fact(year === undefined ? '' : String(year));
		}
		case 'rating':
			return { look: 'stars', stars: context.stars ?? 0 };
		case 'heart':
			return { look: 'heart', hearted: context.hearted };
		case 'plays':
			return fact(String(track.plays ?? 0));
		case 'lastPlayed':
			return fact(lastWhen(track.lastPlayedAt, now));
		case 'added':
			return fact(track.addedAt === undefined ? '' : monthYear.format(Date.parse(track.addedAt)));
		case 'format':
			return fact(formatCell(track));
		case 'bitrate':
			return fact(track.bitrate === undefined ? '' : String(track.bitrate));
		case 'gain':
			if (track.trackGain === undefined) {
				return { look: 'fact', text: 'no RG', warn: true };
			}
			return track.trackGainSource === 'measured'
				? { look: 'fact', text: `${gain(track.trackGain)} dB`, measured: true }
				: fact(`${gain(track.trackGain)} dB`);
	}
}

/**
 * The columns that say the same for every song of an album: a phone's line leaves them to the
 * album's heading.
 */
export function sameOnAlbum(
	order: readonly MovableId[],
	album: RowAlbum & { tracks: readonly LibraryTrack[] },
	artistWhenDifferent: boolean,
	now: number
): Set<ColumnId> {
	const same = columnsOf(order).filter((id) => {
		const texts = album.tracks.map((track) => {
			const cell = cellOf(id, {
				track,
				album,
				stars: null,
				hearted: false,
				artistWhenDifferent,
				now
			});
			return cell.look === 'name' || cell.look === 'fact' ? cell.text : '';
		});
		return texts.every((text) => text === texts[0]);
	});
	return new Set(same);
}

/** Columns that say something of each song, so they're never left out for being shared. */
const perSong = new Set<ColumnId>(['artist', 'rating', 'heart', 'plays', 'gain']);

/**
 * What a phone row's second line says, from the person's columns in their order, before its time.
 * The title is its first line and the number stays at its left, wherever they are in the order.
 * A column that says the same for every song of the album is left out: the album's heading says
 * it, and the line stays quiet. So is anything with nothing to say.
 */
export function phoneLine(
	order: readonly MovableId[],
	context: CellContext,
	sameOnAlbum: (id: ColumnId) => boolean
): string[] {
	return phoneParts(order, context, sameOnAlbum).map(({ text }) => text);
}

/** A phone line's parts, each with the column it says. */
function phoneParts(
	order: readonly MovableId[],
	context: CellContext,
	sameOnAlbum: (id: ColumnId) => boolean
): { id: ColumnId; text: string }[] {
	const { track, album } = context;
	const parts: { id: ColumnId; text: string }[] = [];
	for (const id of columnsOf(order)) {
		if (id === 'album' || (!perSong.has(id) && sameOnAlbum(id))) {
			continue;
		}
		const part = phonePart(id, context, track, album);
		if (part !== '') {
			parts.push({ id, text: part });
		}
	}
	return parts;
}

/** Why a column isn't on a phone's line: its album's heading says it, or the line ran out. */
export type LineNote = 'heading' | 'over';

/**
 * What the phone's picker notes beside each column, for one real song: "ON THE HEADING" for what's
 * the same on the whole album, and "DOESN'T FIT" for what the line drops at this width.
 */
export function lineNotes(
	order: readonly MovableId[],
	context: CellContext,
	sameOnAlbum: (id: ColumnId) => boolean,
	chars: number
): Map<ColumnId, LineNote> {
	const notes = new Map<ColumnId, LineNote>();
	for (const id of columnsOf(order)) {
		if (id === 'album' || (!perSong.has(id) && sameOnAlbum(id))) {
			notes.set(id, 'heading');
		}
	}
	const parts = phoneParts(order, context, sameOnAlbum);
	const kept = fitLine(
		parts.map(({ text }) => text),
		timeOf(context.track),
		chars
	).length;
	for (const { id } of parts.slice(kept)) {
		notes.set(id, 'over');
	}
	return notes;
}

function phonePart(
	id: ColumnId,
	context: CellContext,
	track: LibraryTrack,
	album: RowAlbum
): string {
	const cell = cellOf(id, context);
	switch (id) {
		case 'artist': {
			if (!context.artistWhenDifferent) {
				return cell.look === 'name' ? cell.text : '';
			}
			const guests = track.artists.filter((artist) => artist !== '' && artist !== album.artist);
			const named = artistCell(track, album, true);
			if (named === '') {
				return '';
			}
			return guests.length < track.artists.length ? `with ${guests.join(', ')}` : named;
		}
		case 'rating':
			return cell.look === 'stars' ? '★'.repeat(cell.stars) : '';
		case 'heart':
			return cell.look === 'heart' && cell.hearted ? '♥' : '';
		case 'plays': {
			const plays = track.plays ?? 0;
			return plays === 0 ? '' : `${String(plays)} ${plays === 1 ? 'play' : 'plays'}`;
		}
		case 'lastPlayed':
			return track.lastPlayedAt === undefined ? '' : lastWhen(track.lastPlayedAt, context.now);
		case 'bitrate':
			return track.bitrate === undefined ? '' : `${String(track.bitrate)} kbps`;
		case 'gain':
			if (track.trackGain === undefined) {
				return 'no RG';
			}
			return `RG ${track.trackGainSource === 'measured' ? '≈' : ''}${gain(track.trackGain)}`;
		default:
			return cell.look === 'name' || cell.look === 'fact' ? cell.text : '';
	}
}

/** How wide a character of the phone's line is, at 12 px of Geist Mono, with room for the stars, which run wider. */
export const monoAdvance = 7.8;

/**
 * Keeps the line's parts that fit in `chars` characters beside the length, whole: a part that
 * won't fit is left out rather than cut, and so is everything after it.
 */
export function fitLine(parts: readonly string[], time: string, chars: number): string[] {
	const kept: string[] = [];
	let used = time.length;
	for (const part of parts) {
		const need = [...part].length + 3;
		if (used + need > chars) {
			break;
		}
		used += need;
		kept.push(part);
	}
	return kept;
}

/** A track's length, as every row ends. */
export function timeOf(track: LibraryTrack): string {
	return clock(track.durationMs / 1000);
}

/**
 * The menu's list: the columns shown, the number and the title among them, in order, then the rest
 * in the menu's own order.
 */
export function menuList(pref: ColumnsPref): { id: MovableId; on: boolean }[] {
	const on = new Set<MovableId>(pref.order);
	return [
		...framed(pref.order).map((id) => ({ id, on: true })),
		...columnSpecs.filter(({ id }) => !on.has(id)).map(({ id }) => ({ id, on: false }))
	];
}

/** Shows a column at the end of the order, or hides it. */
export function toggled(pref: ColumnsPref, id: ColumnId): ColumnsPref {
	return pref.order.includes(id)
		? { ...pref, order: pref.order.filter((each) => each !== id) }
		: { ...pref, order: [...pref.order, id] };
}

/**
 * Moves a column to `to` in the menu's list. Only the shown ones have an order to keep: moving a
 * hidden column among them changes nothing until it's shown.
 */
export function moved(pref: ColumnsPref, id: MovableId, to: number): ColumnsPref {
	const list = menuList(pref);
	const from = list.findIndex((item) => item.id === id);
	if (from < 0) {
		return pref;
	}
	const [item] = list.splice(from, 1);
	if (item === undefined) {
		return pref;
	}
	list.splice(Math.max(0, Math.min(to, list.length)), 0, item);
	return { ...pref, order: list.filter(({ on }) => on).map(({ id: each }) => each) };
}

/**
 * Whether a list lets a column be dragged: everything it places but search's Artist and Album when
 * the person hasn't chosen them, which search puts in itself.
 */
export function movable(id: MovableId, pref: ColumnsPref): boolean {
	return pref.order.includes(id);
}

/**
 * Moves `id` to `to` among what a list shows (`shown`, its lineup), and says what that makes of the
 * person's one order, which every list shares. The column goes after its new left neighbour there,
 * or before its right one when it's first, so a list that leaves a column out (an album page's
 * Album) or puts one in (search's) maps by neighbour. Columns hidden at this width then stay after
 * the ones shown, so which columns fit never changes on a drop.
 */
export function reorderIn(
	pref: ColumnsPref,
	context: ColumnsContext,
	shown: readonly MovableId[],
	id: MovableId,
	to: number
): ColumnsPref {
	const from = shown.indexOf(id);
	if (from < 0 || !pref.order.includes(id)) {
		return pref;
	}
	const lineup = [...shown];
	lineup.splice(from, 1);
	lineup.splice(Math.max(0, Math.min(to, lineup.length)), 0, id);
	const at = lineup.indexOf(id);
	const order = pref.order.filter((each) => each !== id);
	const left = lineup
		.slice(0, at)
		.reverse()
		.find((each) => order.includes(each));
	const right = lineup.slice(at + 1).find((each) => order.includes(each));
	if (left !== undefined) {
		order.splice(order.indexOf(left) + 1, 0, id);
	} else if (right !== undefined) {
		order.splice(order.indexOf(right), 0, id);
	} else {
		order.unshift(id);
	}
	// The list's own columns hidden at this width go back behind the ones it shows, in their order.
	const seen = new Set(columnsIn(pref.order, context));
	const hidden = new Set(
		order.filter((each) => isColumn(each) && seen.has(each) && !shown.includes(each))
	);
	const places = order.flatMap((each, index) =>
		hidden.has(each) || shown.includes(each) ? [index] : []
	);
	const refill = [
		...order.filter((each) => shown.includes(each)),
		...order.filter((each) => hidden.has(each))
	];
	places.forEach((place, index) => {
		const each = refill.at(index);
		if (each !== undefined) {
			order[place] = each;
		}
	});
	const same = order.every((each, index) => pref.order.at(index) === each);
	return same ? pref : { ...pref, order };
}

/**
 * Where Alt with an arrow, Home or End takes the column at `index` of `count`, or null at a wall,
 * or for any other key.
 */
export function keyTarget(index: number, count: number, key: string): number | null {
	const to = (() => {
		switch (key) {
			case 'ArrowLeft':
				return index - 1;
			case 'ArrowRight':
				return index + 1;
			case 'Home':
				return 0;
			case 'End':
				return count - 1;
			default:
				return null;
		}
	})();
	return to === null || to < 0 || to >= count || to === index ? null : to;
}

/** What a screen reader hears after a move: "Genre moved to 4 of 6, after Plays." */
export function moveSaid(lineup: readonly MovableId[], id: MovableId): string {
	const at = lineup.indexOf(id);
	const before = lineup.at(at - 1);
	const where = at <= 0 || before === undefined ? 'first' : `after ${labelOf(before)}`;
	return `${labelOf(id)} moved to ${String(at + 1)} of ${String(lineup.length)}, ${where}.`;
}

/** What it hears when a key would take a column past a wall: "Artist is already first." */
export function wallSaid(id: MovableId, key: string): string {
	return key === 'ArrowLeft' || key === 'Home'
		? `${labelOf(id)} is already first.`
		: `${labelOf(id)} is already last. Time stays put.`;
}

/** A label's name in the header: "Genre, column 2 of 6". */
export function columnName(lineup: readonly MovableId[], id: MovableId): string {
	return `${labelOf(id)}, column ${String(lineup.indexOf(id) + 1)} of ${String(lineup.length)}`;
}
