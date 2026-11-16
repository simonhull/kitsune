/**
 * The client's copy of `internal/organize`'s template language: its parser, the paths it makes and
 * the rule that makes a name safe. The server parses again and is the authority; this draws the
 * template as chips and the example songs' new paths as they are typed. Both are held to
 * `internal/organize/testdata/templates.json`. Positions count characters (code points), and
 * lengths are bytes of UTF-8, as the server counts them.
 */

/** A field, as a template writes it between < and >. */
export type Field =
	| 'Album Artist'
	| 'Artist'
	| 'Artists'
	| 'Album'
	| 'Year'
	| 'Original Year'
	| 'Disc'
	| 'Disc Total'
	| 'Track'
	| 'Title'
	| 'Genre'
	| 'Disc Name'
	| 'Format';

/** Every field, in the token list's order. */
export const fields: readonly Field[] = [
	'Album Artist',
	'Artist',
	'Artists',
	'Album',
	'Year',
	'Original Year',
	'Disc',
	'Disc Total',
	'Track',
	'Title',
	'Genre',
	'Disc Name',
	'Format'
];

/** What each field reads, for the token list. */
export const fieldNotes: Record<Field, string> = {
	'Album Artist': 'The album’s owner',
	Artist: 'The song’s first artist',
	Artists: 'Every artist, joined by commas',
	Album: 'The album’s title',
	Year: 'The album’s year',
	'Original Year': 'The song’s original year',
	Disc: 'Only when there’s more than one',
	'Disc Total': 'Only when there’s more than one',
	Track: 'Two digits, three past 99',
	Title: 'The song’s title',
	Genre: 'The album’s first genre',
	'Disc Name': 'The disc’s subtitle',
	Format: 'FLAC, MP3…'
};

export const defaultTemplate = '<Album Artist>/[<Year> - ]<Album>/[<Disc>-]<Track> - <Title>';
export const defaultCompilations = 'Compilations/<Album>/<Track> - <Title>';

/** The longest a template may be, in characters. */
export const mostTemplateChars = 500;
/** The longest a folder or file name may be, in bytes. */
export const mostSegmentBytes = 180;
/** The longest a whole path under its root may be, in bytes. */
export const mostPathBytes = 1000;
const mostExtBytes = 12;
const mostWidth = 4;
const trackWidth = 2;

/** Where a template went wrong, in characters from its start, and why. */
export class TemplateError extends Error {
	override name = 'TemplateError';
	readonly at: number;
	readonly reason: string;

	constructor(at: number, reason: string) {
		super(`bad template at ${String(at)}: ${reason}`);
		this.at = at;
		this.reason = reason;
	}
}

/** One piece of a template. */
export type Node =
	| { kind: 'literal'; text: string; at: number }
	| { kind: 'token'; fields: Field[]; width: number; at: number; end: number }
	| { kind: 'slash'; at: number }
	| { kind: 'group'; inner: Node[]; at: number; end: number };

/** A parsed template. */
export interface Template {
	text: string;
	nodes: Node[];
}

const numeric = (f: Field): boolean => f === 'Track' || f === 'Disc' || f === 'Disc Total';

/** The field called `name`, ignoring case. */
function lookup(name: string): Field | undefined {
	const lower = name.toLowerCase();
	return fields.find((f) => f.toLowerCase() === lower);
}

class Parser {
	readonly chars: string[];
	at = 0;

	constructor(text: string) {
		this.chars = Array.from(text);
	}

	/** Reads pieces until the end, or a `]` when `inGroup`. Returns true as the second when closed. */
	nodes(inGroup: boolean): [Node[], boolean] {
		const out: Node[] = [];
		let text = '';
		let textAt = 0;
		const flush = (): void => {
			if (text !== '') {
				out.push({ kind: 'literal', text, at: textAt });
				text = '';
			}
		};
		while (this.at < this.chars.length) {
			const at = this.at;
			const c = this.chars[at] ?? '';
			if (!'</[]>'.includes(c)) {
				if (text === '') {
					textAt = at;
				}
				text += c;
				this.at += 1;
				continue;
			}
			flush();
			switch (c) {
				case '<':
					out.push(this.token());
					break;
				case '/':
					this.at += 1;
					out.push({ kind: 'slash', at });
					break;
				case '[':
					if (inGroup) {
						throw new TemplateError(at, "a group can't hold another group");
					}
					out.push(this.group());
					break;
				case ']':
					if (!inGroup) {
						throw new TemplateError(at, 'this ] closes no [');
					}
					return [out, true];
				default:
					throw new TemplateError(at, 'this > closes no <');
			}
		}
		flush();
		return [out, false];
	}

	group(): Node {
		const open = this.at;
		this.at += 1;
		const [inner] = this.nodes(true);
		if (this.at >= this.chars.length) {
			throw new TemplateError(open, 'this [ is never closed');
		}
		this.at += 1;
		if (!inner.some((n) => n.kind === 'token')) {
			throw new TemplateError(open, 'a group needs a field, or it never shows');
		}
		return { kind: 'group', inner, at: open, end: this.at };
	}

	token(): Node {
		const open = this.at;
		const rel = this.chars.slice(open).indexOf('>');
		const body = rel < 0 ? [] : this.chars.slice(open + 1, open + rel);
		if (rel < 0 || body.some((c) => '<[]/'.includes(c))) {
			throw new TemplateError(open, 'this < is never closed');
		}
		this.at = open + rel + 1;
		const whole = body.join('');
		const colon = whole.indexOf(':');
		const name = colon < 0 ? whole : whole.slice(0, colon);
		const found: Field[] = [];
		for (const alt of name.split('|')) {
			const trimmed = alt.trim();
			const f = lookup(trimmed);
			if (f === undefined) {
				if (trimmed.toLowerCase() === 'ext') {
					throw new TemplateError(open, 'the extension is added for you');
				}
				throw new TemplateError(open, `no field is called ${JSON.stringify(trimmed)}`);
			}
			found.push(f);
		}
		let width = 0;
		if (colon >= 0) {
			const text = whole.slice(colon + 1).trim();
			const w = /^[+-]?\d+$/.test(text) ? Number(text) : NaN;
			if (!(w >= 1 && w <= mostWidth)) {
				throw new TemplateError(open, `a width is a number from 1 to ${String(mostWidth)}`);
			}
			if (found.some((f) => !numeric(f))) {
				throw new TemplateError(open, 'only Track, Disc and Disc Total take a width');
			}
			width = w;
		}
		return { kind: 'token', fields: found, width, at: open, end: this.at };
	}
}

/** Reads a template. Throws a {@link TemplateError}, as the server's `Parse` refuses one. */
export function parse(text: string): Template {
	const p = new Parser(text);
	if (p.chars.length === 0) {
		throw new TemplateError(0, "a template can't be empty");
	}
	if (p.chars.length > mostTemplateChars) {
		throw new TemplateError(
			mostTemplateChars,
			`a template is at most ${String(mostTemplateChars)} characters`
		);
	}
	const [nodes] = p.nodes(false);
	nodes.forEach((n, i) => {
		if (n.kind !== 'slash') {
			return;
		}
		if (i === 0 || i === nodes.length - 1 || nodes[i - 1]?.kind === 'slash') {
			throw new TemplateError(n.at, "a folder name can't be empty");
		}
	});
	const last = nodes.at(-1);
	if (last?.kind !== 'token' || (last.fields[0] !== 'Title' && last.fields[0] !== 'Track')) {
		throw new TemplateError(p.chars.length, 'a template ends in <Title> or <Track>');
	}
	return { text, nodes };
}

/** {@link parse}, as a result: the template, or where and why it isn't one. */
export function tryParse(text: string): Template | TemplateError {
	try {
		return parse(text);
	} catch (error) {
		if (error instanceof TemplateError) {
			return error;
		}
		throw error;
	}
}

/** What a template reads of a song. */
export interface Song {
	title: string;
	album: string;
	albumArtist: string;
	artists: string[];
	genre: string;
	discName: string;
	format: string;
	year: number;
	originalYear: number;
	disc: number;
	discs: number;
	track: number;
	discSongs: number;
	compilation: boolean;
}

const number = (n: number): string => (n > 0 ? String(n) : '');

function value(s: Song, f: Field): string {
	switch (f) {
		case 'Album Artist':
			return s.albumArtist;
		case 'Artist':
			return s.artists[0] ?? '';
		case 'Artists':
			return s.artists.join(', ');
		case 'Album':
			return s.album;
		case 'Year':
			return number(s.year);
		case 'Original Year':
			return number(s.originalYear);
		case 'Disc':
			return s.discs > 1 ? number(s.disc) : '';
		case 'Disc Total':
			return s.discs > 1 ? number(s.discs) : '';
		case 'Track':
			return number(s.track);
		case 'Title':
			return s.title;
		case 'Genre':
			return s.genre;
		case 'Disc Name':
			return s.discName;
		case 'Format':
			return s.format.toUpperCase();
	}
}

const encoder = new TextEncoder();

/** Bytes in s, as UTF-8. */
export function byteLength(s: string): number {
	return encoder.encode(s).length;
}

/** The longest prefix of s at most n bytes long that ends on a character boundary. */
function cut(s: string, n: number): string {
	let used = 0;
	let out = '';
	for (const c of s) {
		const size = byteLength(c);
		if (used + size > n) {
			break;
		}
		used += size;
		out += c;
	}
	return out;
}

const reserved = '<>:"/\\|?*';

function isSpace(c: string): boolean {
	const r = c.codePointAt(0) ?? 0;
	return (
		'\t\n\v\f\r \u0085 '.includes(c) ||
		r === 0x1680 ||
		(r >= 0x2000 && r <= 0x200a) ||
		r === 0x2028 ||
		r === 0x2029 ||
		r === 0x202f ||
		r === 0x205f ||
		r === 0x3000
	);
}

/** The character rules: NFC, controls gone, reserved characters to `_`, white space collapsed. */
function clean(s: string): string {
	let out = '';
	let space = false;
	for (const c of s.normalize('NFC')) {
		const r = c.codePointAt(0) ?? 0;
		if (r < 0x20 || r === 0x7f) {
			continue;
		}
		if (isSpace(c)) {
			space = out !== '';
			continue;
		}
		if (space) {
			out += ' ';
			space = false;
		}
		out += reserved.includes(c) ? '_' : c;
	}
	return out;
}

/** Go's `path.Ext`: from the last dot, for a name with no slash. */
function goExt(name: string): string {
	const dot = name.lastIndexOf('.');
	return dot < 0 ? '' : name.slice(dot);
}

function extOf(name: string): string {
	const ext = goExt(name);
	const bytes = byteLength(ext);
	if (ext === name || bytes < 2 || bytes > mostExtBytes || ext.includes(' ')) {
		return '';
	}
	return ext;
}

const devices = new Set([
	'CON',
	'PRN',
	'AUX',
	'NUL',
	...[1, 2, 3, 4, 5, 6, 7, 8, 9].flatMap((n) => [`COM${String(n)}`, `LPT${String(n)}`])
]);

function device(s: string): string {
	const dot = s.indexOf('.');
	const stem = dot < 0 ? s : s.slice(0, dot);
	if (!devices.has(stem.replace(/ +$/, '').toUpperCase())) {
		return s;
	}
	return dot < 0 ? `${stem}_` : `${stem}_.${s.slice(dot + 1)}`;
}

function settle(s: string, most: number): string {
	let out = s.replace(/[. ]+$/, '');
	if (out.startsWith('.') || out.startsWith('@')) {
		out = `_${out.slice(1)}`;
	}
	out = device(out);
	if (byteLength(out) > most) {
		const ext = extOf(out);
		out = cut(out.slice(0, out.length - ext.length), most - byteLength(ext)) + ext;
	}
	return out;
}

function safeWithin(segment: string, most: number): string {
	let s = segment;
	for (let round = 0; round < 8; round++) {
		const next = settle(clean(s), most);
		if (next === s) {
			break;
		}
		s = next;
	}
	return s === '' ? '_' : s;
}

/** A folder or file name every filesystem a library is read from can hold, as the server's `Safe`. */
export function safe(segment: string): string {
	return safeWithin(segment, mostSegmentBytes);
}

function safeFile(stem: string, extension: string): string {
	let ext = clean(extension);
	if (ext === '.' || ext.includes(' ') || byteLength(ext) > mostExtBytes) {
		ext = '';
	}
	return safeWithin(stem, mostSegmentBytes - byteLength(ext)) + ext;
}

type Caps = Partial<Record<Field, number>>;

function fill(n: Extract<Node, { kind: 'token' }>, s: Song, caps: Caps): string {
	for (const f of n.fields) {
		let v = clean(value(s, f));
		if (v === '') {
			continue;
		}
		const most = caps[f];
		if (most !== undefined) {
			v = cut(v, most);
		}
		let width = n.width;
		if (width === 0 && f === 'Track') {
			width = Math.max(trackWidth, String(s.discSongs).length);
		}
		if (numeric(f) && v.length < width) {
			v = '0'.repeat(width - v.length) + v;
		}
		return v;
	}
	return '';
}

function render(nodes: readonly Node[], s: Song, caps: Caps): [string, boolean] {
	let out = '';
	let filled = false;
	for (const n of nodes) {
		switch (n.kind) {
			case 'literal':
				out += n.text;
				break;
			case 'slash':
				out += '/';
				break;
			case 'token': {
				const v = fill(n, s, caps);
				filled ||= v !== '';
				out += v;
				break;
			}
			case 'group': {
				const [inner, any] = render(n.inner, s, caps);
				if (any) {
					out += inner;
					filled = true;
				}
				break;
			}
		}
	}
	return [out, filled];
}

/** Why a song has no path: nothing for its file's name, or too long even cut short. */
export type NoPath = 'noTags' | 'tooLong';

function pathWithin(t: Template, s: Song, ext: string, caps: Caps): string | NoPath {
	const [text] = render(t.nodes, s, caps);
	const segments = text.split('/');
	let from = 0;
	t.nodes.forEach((n, i) => {
		if (n.kind === 'slash') {
			from = i + 1;
		}
	});
	const [name, filled] = render(t.nodes.slice(from), s, caps);
	if (!filled || name.trim() === '') {
		return 'noTags';
	}
	const last = segments.length - 1;
	return segments
		.map((segment, i) => (i === last ? safeFile(segment, ext) : safe(segment)))
		.join('/');
}

/**
 * Where `t` files `s`: a slash-separated path relative to its root, ending in `ext`, the file's
 * extension with its dot. A path past 1,000 bytes has its title cut first, then its album.
 */
export function pathOf(t: Template, s: Song, ext: string): string | NoPath {
	const caps: Caps = {};
	let p = pathWithin(t, s, ext, caps);
	for (const f of ['Title', 'Album'] as const) {
		if (p === 'noTags' || p === 'tooLong') {
			return p;
		}
		const over = byteLength(p) - mostPathBytes;
		if (over <= 0) {
			return p;
		}
		caps[f] = Math.max(byteLength(clean(value(s, f))) - over, 1);
		p = pathWithin(t, s, ext, caps);
	}
	if (p !== 'noTags' && p !== 'tooLong' && byteLength(p) > mostPathBytes) {
		return 'tooLong';
	}
	return p;
}

/** A file's extension, with its dot, as the server takes it from the path. */
export function extensionOf(path: string): string {
	const base = path.slice(path.lastIndexOf('/') + 1);
	return goExt(base);
}
