/**
 * Smart rules worked out over songs held here: what the fixtures answer for a smart playlist and
 * its preview, as the server works them out (internal/store's smartlist.go). Pure.
 */
import { counts, fold, isGroup } from './smartlist.ts';
import type { SmartChooser, SmartCondition, SmartOrder, SmartRule, SmartRules } from './source.ts';

/** What a rule can read of one song. */
export interface SongFacts {
	id: string;
	albumId: string;
	title: string;
	album: string;
	artists: readonly string[];
	albumArtists: readonly string[];
	genres: readonly string[];
	year?: number;
	/** The person's stars in halves; 0 unrated. */
	halves: number;
	plays: number;
	skips: number;
	/** Unix ms, or null never played. */
	lastPlayed: number | null;
	added: number;
	durationMs: number;
	format: string;
	/** kbps. */
	bitrate?: number;
	bytes: number;
}

export interface EvalContext {
	now: number;
	seed: number;
	/** The songs a playlist holds by ID, or undefined for a playlist that's gone. */
	playlist: (id: string) => ReadonlySet<string> | undefined;
}

const dayMs = 86_400_000;

function textOf(rule: SmartRule, song: SongFacts): readonly string[] {
	switch (rule.field) {
		case 'title':
			return [song.title];
		case 'album':
			return [song.album];
		case 'artist':
			return song.artists;
		case 'albumArtist':
			return song.albumArtists;
		case 'genre':
			return song.genres;
		default:
			return [];
	}
}

function numberOf(rule: SmartRule, song: SongFacts): number | undefined {
	switch (rule.field) {
		case 'year':
			return song.year;
		case 'rating':
			return song.halves;
		case 'plays':
			return song.plays;
		case 'skips':
			return song.skips;
		case 'bitrate':
			return song.bitrate;
		case 'length':
			return song.durationMs;
		default:
			return undefined;
	}
}

function test(rule: SmartRule, song: SongFacts, ctx: EvalContext): boolean {
	const v = rule.value;
	switch (rule.field) {
		case 'title':
		case 'album':
		case 'artist':
		case 'albumArtist':
		case 'genre': {
			const want = fold(String(v));
			const have = textOf(rule, song).map(fold);
			switch (rule.op) {
				case 'is':
					return have.includes(want);
				case 'isNot':
					return !have.includes(want);
				case 'contains':
					return have.some((each) => each.includes(want));
				default:
					return !have.some((each) => each.includes(want));
			}
		}
		case 'lastPlayed':
		case 'added': {
			const at = rule.field === 'added' ? song.added : song.lastPlayed;
			const since = ctx.now - Number(v) * dayMs;
			const within = at !== null && at >= since;
			return rule.op === 'inLast' ? within : !within;
		}
		case 'format':
			return (fold(song.format) === fold(String(v))) === (rule.op === 'is');
		case 'playlist': {
			const held = ctx.playlist(String(v));
			if (held === undefined) {
				return false; // A playlist that's gone matches nothing.
			}
			return held.has(song.id) === (rule.op === 'is');
		}
		default: {
			const have = numberOf(rule, song);
			const want = rule.field === 'length' ? Number(v) * 60_000 : Number(v);
			if (have === undefined) {
				return false;
			}
			return rule.op === 'atLeast'
				? have >= want
				: rule.op === 'atMost'
					? have <= want
					: have === want;
		}
	}
}

function matches(
	match: 'all' | 'any',
	conditions: readonly SmartCondition[],
	song: SongFacts,
	ctx: EvalContext
): boolean {
	const live = conditions.filter((c) => (isGroup(c) ? c.rules.some(counts) : counts(c)));
	if (live.length === 0) {
		return true;
	}
	const one = (c: SmartCondition): boolean =>
		isGroup(c) ? matches(c.match, c.rules, song, ctx) : test(c, song, ctx);
	return match === 'all' ? live.every(one) : live.some(one);
}

/** A song's place in the seed's shuffle. */
export function seeded(seed: number, id: string): number {
	let h = (seed % 0x1_0000_0000) ^ 0x811c9dc5;
	for (let i = 0; i < id.length; i += 1) {
		h ^= id.charCodeAt(i);
		h = Math.imul(h, 0x01000193);
	}
	h ^= h >>> 13;
	h = Math.imul(h, 0x5bd1e995);
	h ^= h >>> 15;
	return h >>> 0;
}

type Sorter = (a: SongFacts, b: SongFacts) => number;

function sorterOf(key: SmartChooser | SmartOrder, seed: number, rank: Map<string, number>): Sorter {
	const lib: Sorter = (a, b) => (rank.get(a.id) ?? 0) - (rank.get(b.id) ?? 0);
	const last = (s: SongFacts): number => s.lastPlayed ?? -Infinity;
	const by =
		(f: (s: SongFacts) => number, desc = false): Sorter =>
		(a, b) =>
			(desc ? f(b) - f(a) : f(a) - f(b)) || lib(a, b);
	const text =
		(f: (s: SongFacts) => string): Sorter =>
		(a, b) =>
			f(a).localeCompare(f(b), 'en', { sensitivity: 'base' }) || lib(a, b);
	switch (key) {
		case 'random':
			return (a, b) => seeded(seed, a.id) - seeded(seed, b.id) || lib(a, b);
		case 'mostPlayed':
		case 'plays':
			return by((s) => s.plays, true);
		case 'leastPlayed':
			return by((s) => s.plays);
		case 'highestRated':
		case 'rating':
			return by((s) => s.halves, true);
		case 'newest':
		case 'added':
			return by((s) => s.added, true);
		case 'leastRecent':
		case 'lastPlayed':
			return by(last);
		case 'title':
			return text((s) => s.title);
		case 'album':
			return text((s) => s.album);
		case 'year':
			return by((s) => s.year ?? 0);
		default:
			return lib;
	}
}

/** What rules choose of songs given in the Library's order: every match, and those kept, in order. */
export function evaluate(
	rules: SmartRules,
	songs: readonly SongFacts[],
	ctx: EvalContext
): { matched: SongFacts[]; kept: SongFacts[] } {
	const rank = new Map(songs.map((s, i) => [s.id, i]));
	const matched = songs.filter((s) => matches(rules.match, rules.rules, s, ctx));
	let kept = matched;
	const limit = rules.limit;
	if (limit !== undefined) {
		kept = matched.toSorted(sorterOf(limit.by, ctx.seed, rank));
		if (limit.unit === 'songs') {
			kept = kept.slice(0, limit.count);
		} else {
			const cap = limit.unit === 'minutes' ? limit.count * 60_000 : limit.count * 1e9;
			const out: SongFacts[] = [];
			let sum = 0;
			for (const s of kept) {
				sum += limit.unit === 'minutes' ? s.durationMs : s.bytes;
				if (sum > cap) {
					break;
				}
				out.push(s);
			}
			kept = out;
		}
	}
	const order = rules.order ?? 'library';
	if (order !== 'chosen') {
		kept = kept.toSorted(sorterOf(order, ctx.seed, rank));
	}
	return { matched, kept };
}
