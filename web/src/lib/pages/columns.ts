import { clock, gain } from '#lib/library/format.ts';
import type {
	ColumnId,
	ColumnsPref,
	IndexField,
	LibraryAlbum,
	LibraryTrack
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
		width: [64, 84],
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

export const defaultColumns: ColumnsPref = {
	order: ['artist', 'genre', 'rating', 'plays', 'lastPlayed', 'format'],
	artistWhenDifferent: true
};

/** Checks a saved value, so one from a newer server can't break the page: unknown columns go. */
export function savedColumns(saved: ColumnsPref | null): ColumnsPref {
	if (saved === null) {
		return defaultColumns;
	}
	const order = [...new Set(saved.order)].filter((id) => specs.has(id));
	return { order, artistWhenDifferent: saved.artistWhenDifferent };
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

/** The tracks every row has before its columns, in order, at a step. */
function leading(frame: Frame, parts: RowParts): number[] {
	return [
		...(parts.handle ? [frame.handle] : []),
		frame.box,
		...(parts.cover ? [frame.cover] : []),
		...(parts.number ? [frame.number] : [])
	];
}

export interface Fit {
	shown: ColumnId[];
	/** What the "+N" chip counts. */
	hiddenCount: number;
}

/**
 * Finds the columns a row `width` wide has room for, taking them in the person's order and dropping
 * from the end: once one doesn't fit, nothing after it shows either. Title keeps its least width.
 */
export function fit(
	order: readonly ColumnId[],
	width: number,
	step: Exclude<Step, 'phone'>,
	parts: RowParts = numbered
): Fit {
	const frame = frameOf(step);
	// What comes before the title, the title, time and ⋯, and the gaps between them.
	const fixed = [...leading(frame, parts), frame.title, frame.time, frame.more];
	let room =
		width -
		2 * frame.pad -
		fixed.reduce((sum, each) => sum + each, 0) -
		(fixed.length - 1) * frame.gap;
	const shown: ColumnId[] = [];
	for (const id of order) {
		const spec = specOf(id);
		const need = atStep(spec.least ?? spec.width, step) + frame.gap;
		if (need > room) {
			break;
		}
		room -= need;
		shown.push(id);
	}
	return { shown, hiddenCount: order.length - shown.length };
}

/** The grid every row and the header share, for the columns shown. */
export function template(
	shown: readonly ColumnId[],
	step: Exclude<Step, 'phone'>,
	parts: RowParts = numbered,
	titleLeast = frameOf(step).title
): string {
	const frame = frameOf(step);
	const columns = shown.map((id) => {
		const spec = specOf(id);
		const width = atStep(spec.width, step);
		return spec.least === undefined
			? `${String(width)}px`
			: `minmax(${String(atStep(spec.least, step))}px, ${String(width)}px)`;
	});
	return [
		...leading(frame, parts).map((each) => `${String(each)}px`),
		`minmax(${String(titleLeast)}px, 1fr)`,
		...columns,
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
export function columnsIn(order: readonly ColumnId[], context: ColumnsContext): ColumnId[] {
	switch (context) {
		case 'library':
			return [...order];
		case 'album':
			return order.filter((id) => id !== 'album');
		case 'search': {
			// Artist and Album always, Album right after Artist when the person hasn't placed it.
			const out = order.includes('artist') ? [...order] : ['artist' as const, ...order];
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
export function fieldsFor(order: readonly ColumnId[]): IndexField[] {
	const wanted = new Set(order.map((id) => specOf(id).field));
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
			return track.trackGain === undefined
				? { look: 'fact', text: 'no RG', warn: true }
				: fact(`${gain(track.trackGain)} dB`);
	}
}

/**
 * The columns that say the same for every song of an album: a phone's line leaves them to the
 * album's heading.
 */
export function sameOnAlbum(
	order: readonly ColumnId[],
	album: RowAlbum & { tracks: readonly LibraryTrack[] },
	artistWhenDifferent: boolean,
	now: number
): Set<ColumnId> {
	const same = order.filter((id) => {
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
 * A column that says the same for every song of the album is left out: the album's heading says
 * it, and the line stays quiet. So is anything with nothing to say.
 */
export function phoneLine(
	order: readonly ColumnId[],
	context: CellContext,
	sameOnAlbum: (id: ColumnId) => boolean
): string[] {
	const { track, album } = context;
	const parts: string[] = [];
	for (const id of order) {
		if (id === 'album' || (!perSong.has(id) && sameOnAlbum(id))) {
			continue;
		}
		const part = phonePart(id, context, track, album);
		if (part !== '') {
			parts.push(part);
		}
	}
	return parts;
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
			return track.trackGain === undefined ? 'no RG' : `RG ${gain(track.trackGain)}`;
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

/** The menu's list: the columns shown, in order, then the rest in the menu's own order. */
export function menuList(pref: ColumnsPref): { id: ColumnId; on: boolean }[] {
	const on = new Set(pref.order);
	return [
		...pref.order.map((id) => ({ id, on: true })),
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
export function moved(pref: ColumnsPref, id: ColumnId, to: number): ColumnsPref {
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
