import { describe, expect, it } from 'vitest';
import { evaluate, type SongFacts } from './smartEval.ts';
import type { SmartRules } from './source.ts';

const day = 86_400_000;
const now = Date.parse('2026-11-15T12:00:00Z');

function song(id: string, over: Partial<SongFacts> = {}): SongFacts {
	return {
		id,
		albumId: 'a',
		title: id,
		album: 'Album',
		artists: ['Falling Up'],
		albumArtists: ['Falling Up'],
		genres: [],
		halves: 0,
		plays: 0,
		skips: 0,
		lastPlayed: null,
		added: now - 100 * day,
		durationMs: 4 * 60_000,
		format: 'FLAC',
		bytes: 30e6,
		...over
	};
}

const ctx = { now, seed: 1, playlist: () => undefined };
const ids = (songs: SongFacts[]): string[] => songs.map(({ id }) => id);

describe('evaluate', () => {
	const songs = [
		song('a', { halves: 8, plays: 3, lastPlayed: now - 2 * day, title: 'Hydro' }),
		song('b', { halves: 4, plays: 9, lastPlayed: now - 20 * day, artists: ['Sigur Rós'] }),
		song('c', { plays: 1 }),
		song('d', { halves: 10, plays: 9, lastPlayed: now - 40 * day, durationMs: 9 * 60_000 })
	];

	it('folds text, and counts unrated under every line', () => {
		const rules: SmartRules = {
			match: 'any',
			rules: [
				{ field: 'artist', op: 'is', value: 'sigur ros' },
				{ field: 'rating', op: 'atMost', value: 2 }
			]
		};
		expect(ids(evaluate(rules, songs, ctx).kept)).toEqual(['b', 'c']);
	});

	it('takes never played as not played in any last days, and leaves a text rule with nothing typed out', () => {
		const rules: SmartRules = {
			match: 'all',
			rules: [
				{ field: 'lastPlayed', op: 'notInLast', value: 7 },
				{ field: 'title', op: 'contains', value: '' }
			]
		};
		expect(ids(evaluate(rules, songs, ctx).kept)).toEqual(['b', 'c', 'd']);
	});

	it('chooses by its chooser, ties in the Library order, and stops before going over', () => {
		const rules: SmartRules = {
			match: 'all',
			rules: [],
			limit: { count: 12, unit: 'minutes', by: 'mostPlayed' },
			order: 'chosen'
		};
		const { matched, kept } = evaluate(rules, songs, ctx);
		expect(matched).toHaveLength(4);
		// b and d tie on 9 plays: b comes first in the Library. d's 9 minutes would go over 12.
		expect(ids(kept)).toEqual(['b']);
	});

	it('orders what it kept, ties in the Library order', () => {
		const rules: SmartRules = {
			match: 'all',
			rules: [],
			limit: { count: 3, unit: 'songs', by: 'leastPlayed' },
			order: 'rating'
		};
		expect(ids(evaluate(rules, songs, ctx).kept)).toEqual(['a', 'b', 'c']);
	});

	it('shuffles once by the seed: the same each time, another for another seed', () => {
		const many = Array.from({ length: 20 }, (_, i) => song(`s${String(i)}`));
		const rules: SmartRules = { match: 'all', rules: [], order: 'random' };
		const one = ids(evaluate(rules, many, ctx).kept);
		expect(ids(evaluate(rules, many, ctx).kept)).toEqual(one);
		expect(ids(evaluate(rules, many, { ...ctx, seed: 2 }).kept)).not.toEqual(one);
	});

	it('matches nothing for a playlist that is gone', () => {
		const rules: SmartRules = {
			match: 'all',
			rules: [{ field: 'playlist', op: 'isNot', value: 'gone' }]
		};
		expect(evaluate(rules, songs, ctx).kept).toEqual([]);
	});
});
