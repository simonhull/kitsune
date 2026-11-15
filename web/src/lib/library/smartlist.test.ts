import { describe, expect, it } from 'vitest';
import {
	cleaned,
	counts,
	fields,
	fold,
	groupWords,
	limitWords,
	ops,
	phoneSentence,
	ruleColumns,
	ruleOf,
	ruleWords,
	sameRules,
	sentence,
	sentenceLead,
	starsText,
	starsWord
} from './smartlist.ts';
import type { SmartField, SmartOp, SmartRule, SmartRules } from './source.ts';

const rule = (field: SmartField, op: SmartOp, value: string | number): SmartRule => ({
	field,
	op,
	value
});

describe('ruleWords', () => {
	// Every field with every operator it takes, as the head's sentence reads it back.
	const cases: [SmartField, SmartOp, string | number, string][] = [
		['title', 'is', 'Hydro', 'title is Hydro'],
		['title', 'isNot', 'Hydro', 'title is not Hydro'],
		['title', 'contains', 'love', 'title contains love'],
		['title', 'notContains', 'love', 'title does not contain love'],
		['album', 'is', 'Captiva', 'album is Captiva'],
		['album', 'isNot', 'Captiva', 'album is not Captiva'],
		['album', 'contains', 'Cap', 'album contains Cap'],
		['album', 'notContains', 'Cap', 'album does not contain Cap'],
		['artist', 'is', 'Falling Up', 'artist is Falling Up'],
		['artist', 'isNot', 'Falling Up', 'artist is not Falling Up'],
		['artist', 'contains', 'Up', 'artist contains Up'],
		['artist', 'notContains', 'Up', 'artist does not contain Up'],
		['albumArtist', 'is', 'Falling Up', 'album artist is Falling Up'],
		['albumArtist', 'isNot', 'Falling Up', 'album artist is not Falling Up'],
		['albumArtist', 'contains', 'Up', 'album artist contains Up'],
		['albumArtist', 'notContains', 'Up', 'album artist does not contain Up'],
		['genre', 'is', 'Ambient', 'genre is Ambient'],
		['genre', 'isNot', 'Ambient', 'genre is not Ambient'],
		['genre', 'contains', 'rock', 'genre contains rock'],
		['genre', 'notContains', 'rock', 'genre does not contain rock'],
		['year', 'atLeast', 2011, 'from 2011 on'],
		['year', 'atMost', 2011, 'from 2011 or before'],
		['year', 'is', 2011, 'from 2011'],
		['rating', 'atLeast', 6, '★★★ and up'],
		['rating', 'atMost', 7, '★★★½ or less'],
		['rating', 'is', 10, 'rated ★★★★★'],
		['plays', 'atLeast', 10, '10+ plays'],
		['plays', 'atMost', 3, 'at most 3 plays'],
		['plays', 'is', 0, '0 plays'],
		['skips', 'atLeast', 2, '2+ skips'],
		['skips', 'atMost', 0, 'at most 0 skips'],
		['skips', 'is', 1, '1 skip'],
		['lastPlayed', 'notInLast', 7, 'not played in the last 7 days'],
		['lastPlayed', 'inLast', 1, 'played in the last day'],
		['added', 'inLast', 30, 'added in the last 30 days'],
		['added', 'notInLast', 30, 'not added in the last 30 days'],
		['length', 'atLeast', 5, 'at least 5 min long'],
		['length', 'atMost', 3, 'at most 3 min long'],
		['format', 'is', 'FLAC', 'format is FLAC'],
		['format', 'isNot', 'MP3', 'format is not MP3'],
		['bitrate', 'atLeast', 320, 'bitrate is at least 320 kbps'],
		['bitrate', 'atMost', 128, 'bitrate is at most 128 kbps'],
		['bitrate', 'is', 256, 'bitrate is 256 kbps'],
		['playlist', 'is', 'falling-up', 'in Falling Up'],
		['playlist', 'isNot', 'falling-up', 'not in Falling Up']
	];

	it.each(cases)('%s %s %s reads "%s"', (field, op, value, words) => {
		expect(
			ruleWords(rule(field, op, value), (id) => (id === 'falling-up' ? 'Falling Up' : undefined))
		).toBe(words);
	});

	it('covers every field with every operator it takes', () => {
		for (const { field, kind } of fields) {
			for (const [op] of ops[kind]) {
				expect(
					cases.some(([f, o]) => f === field && o === op),
					`${field} ${op}`
				).toBe(true);
			}
		}
	});

	it('says a playlist rule whose playlist is gone', () => {
		expect(ruleWords(rule('playlist', 'is', 'gone'))).toBe('in a playlist that’s gone');
	});
});

describe('the head sentence', () => {
	const board: SmartRules = {
		match: 'all',
		rules: [
			rule('albumArtist', 'is', 'Falling Up'),
			rule('rating', 'atLeast', 6),
			rule('lastPlayed', 'notInLast', 7),
			{ match: 'any', rules: [rule('year', 'atMost', 2011), rule('plays', 'atLeast', 10)] }
		],
		limit: { count: 50, unit: 'songs', by: 'leastRecent' },
		order: 'chosen'
	};

	it('reads the board back: a pill each, a group as one, the limit last', () => {
		expect(sentenceLead(board)).toBe('SONGS WHERE ALL OF');
		expect(sentence(board)).toEqual([
			{ text: 'album artist is Falling Up', group: false },
			{ text: '★★★ and up', join: 'AND', group: false },
			{ text: 'not played in the last 7 days', join: 'AND', group: false },
			{ text: 'either from 2011 or before or 10+ plays', join: 'AND', group: true },
			{ text: '50 songs at most, least recently played first', join: '·', group: false }
		]);
	});

	it('on a phone, is one line of the rules alone', () => {
		expect(phoneSentence(board)).toBe(
			'album artist is Falling Up · ★★★ and up · not played in the last 7 days · either from 2011 or before or 10+ plays'
		);
	});

	it('joins by OR for any, and leaves out a text rule with nothing typed', () => {
		const any: SmartRules = {
			match: 'any',
			rules: [rule('title', 'contains', ''), rule('rating', 'is', 8), rule('year', 'is', 1999)]
		};
		expect(sentenceLead(any)).toBe('SONGS WHERE ANY OF');
		expect(sentence(any).map((p) => p.join)).toEqual([undefined, 'OR']);
	});

	it('words a group of all, and one of three', () => {
		const both = {
			match: 'all' as const,
			rules: [rule('year', 'atLeast', 2000), rule('plays', 'atLeast', 1)]
		};
		expect(groupWords(both)).toBe('both from 2000 on and 1+ plays');
		expect(groupWords({ ...both, rules: [...both.rules, rule('rating', 'is', 2)] })).toBe(
			'all of from 2000 on and 1+ plays and rated ★'
		);
	});

	it('words a limit in GB and minutes', () => {
		expect(limitWords({ count: 2, unit: 'gb', by: 'random' })).toBe('2 GB at most, random first');
		expect(limitWords({ count: 90, unit: 'minutes', by: 'mostPlayed' })).toBe(
			'90 minutes at most, most played first'
		);
	});
});

describe('ruleColumns', () => {
	it('brings the columns its rules, limit and order read, in a fixed order, three at most', () => {
		const rules: SmartRules = {
			match: 'all',
			rules: [
				rule('albumArtist', 'is', 'Falling Up'),
				rule('year', 'atMost', 2011),
				rule('lastPlayed', 'notInLast', 7),
				rule('rating', 'atLeast', 6),
				rule('plays', 'atLeast', 10)
			],
			limit: { count: 50, unit: 'songs', by: 'leastRecent' }
		};
		expect(ruleColumns(rules)).toEqual(['rating', 'plays', 'lastPlayed']);
	});

	it('brings what a chooser or an order reads, and nothing for text', () => {
		expect(
			ruleColumns({ match: 'all', rules: [rule('title', 'contains', 'x')], order: 'added' })
		).toEqual(['added']);
		expect(
			ruleColumns({
				match: 'all',
				rules: [rule('bitrate', 'atLeast', 320)],
				limit: { count: 1, unit: 'songs', by: 'highestRated' }
			})
		).toEqual(['rating', 'bitrate']);
	});

	it('leaves out a text rule with nothing typed', () => {
		expect(ruleColumns({ match: 'all', rules: [rule('format', 'is', 'FLAC')] })).toEqual([
			'format'
		]);
	});
});

describe('the rest of the table', () => {
	it('counts a text rule only with something typed', () => {
		expect(counts(rule('title', 'is', '  '))).toBe(false);
		expect(counts(rule('title', 'is', 'a'))).toBe(true);
		expect(counts(rule('plays', 'is', 0))).toBe(true);
	});

	it('starts a field with its first operator and a value', () => {
		expect(ruleOf('lastPlayed')).toEqual({ field: 'lastPlayed', op: 'notInLast', value: 30 });
		expect(ruleOf('format', ['FLAC'])).toEqual({ field: 'format', op: 'is', value: 'FLAC' });
		expect(ruleOf('album')).toEqual({ field: 'album', op: 'is', value: '' });
	});

	it('writes stars in halves', () => {
		expect(starsText(7)).toBe('★★★½');
		expect(starsWord(1)).toBe('½★');
		expect(starsWord(8)).toBe('4★');
	});

	it('folds case and accents as the server does', () => {
		expect(fold('  Sigur Rós ')).toBe('sigur ros');
	});

	it('tells the same rules apart from changed ones, an absent order being the Library', () => {
		const a: SmartRules = { match: 'all', rules: [rule('rating', 'atLeast', 8)] };
		expect(sameRules(a, { ...a, order: 'library' })).toBe(true);
		expect(sameRules(a, { ...a, match: 'any' })).toBe(false);
	});

	it('cleans what it saves: text trimmed, empty groups gone', () => {
		expect(
			cleaned({ match: 'all', rules: [rule('title', 'is', ' x '), { match: 'any', rules: [] }] })
		).toEqual({ match: 'all', rules: [rule('title', 'is', 'x')] });
	});
});
