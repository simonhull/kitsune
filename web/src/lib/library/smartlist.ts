/**
 * Smart playlists' rules as people read them: the client's twin of the server's field table
 * (internal/smartlist), with each field's label, kind, operators and unit, and the words a rule, a
 * group and a limit read back as. Pure, so each rule of wording is a test.
 */

import type {
	ColumnId,
	SmartChooser,
	SmartCondition,
	SmartField,
	SmartGroup,
	SmartLimit,
	SmartOp,
	SmartOrder,
	SmartRule,
	SmartRules
} from './source.ts';

/** What sort of value a field holds, which decides its operators and its input. */
export type SmartKind = 'text' | 'num' | 'stars' | 'days' | 'min' | 'pick';

export interface FieldSpec {
	field: SmartField;
	label: string;
	kind: SmartKind;
	/** A text field's placeholder. */
	hint?: string;
	/** What a number counts: "plays", "kbps". */
	unit?: string;
}

/** Every field, in the order the editor offers them. */
export const fields: readonly FieldSpec[] = [
	{ field: 'title', label: 'Title', kind: 'text', hint: 'Part of a title' },
	{ field: 'album', label: 'Album', kind: 'text', hint: 'Any album' },
	{ field: 'artist', label: 'Artist', kind: 'text', hint: 'Any artist' },
	{ field: 'albumArtist', label: 'Album artist', kind: 'text', hint: 'Any album artist' },
	{ field: 'genre', label: 'Genre', kind: 'text', hint: 'Any genre' },
	{ field: 'year', label: 'Year', kind: 'num' },
	{ field: 'rating', label: 'Rating', kind: 'stars' },
	{ field: 'plays', label: 'Plays', kind: 'num', unit: 'plays' },
	{ field: 'skips', label: 'Skips', kind: 'num', unit: 'skips' },
	{ field: 'lastPlayed', label: 'Last played', kind: 'days' },
	{ field: 'added', label: 'Date added', kind: 'days' },
	{ field: 'length', label: 'Length', kind: 'min' },
	{ field: 'format', label: 'Format', kind: 'pick' },
	{ field: 'bitrate', label: 'Bitrate', kind: 'num', unit: 'kbps' },
	{ field: 'playlist', label: 'Playlist', kind: 'pick' }
];

/** The formats a Format rule offers. */
export const formats: readonly string[] = [
	'FLAC',
	'ALAC',
	'MP3',
	'AAC',
	'Opus',
	'Vorbis',
	'WAV',
	'AIFF'
];

export function specOf(field: SmartField): FieldSpec {
	const found = fields.find((spec) => spec.field === field);
	if (found === undefined) {
		throw new Error(`no such field: ${field}`);
	}
	return found;
}

/** Each kind's operators, in the order the editor offers them, with their words. */
export const ops: Readonly<Record<SmartKind, readonly (readonly [SmartOp, string])[]>> = {
	text: [
		['is', 'is'],
		['isNot', 'is not'],
		['contains', 'contains'],
		['notContains', 'does not contain']
	],
	num: [
		['atLeast', 'is at least'],
		['atMost', 'is at most'],
		['is', 'is']
	],
	stars: [
		['atLeast', 'is at least'],
		['atMost', 'is at most'],
		['is', 'is']
	],
	days: [
		['notInLast', 'not in the last'],
		['inLast', 'in the last']
	],
	min: [
		['atLeast', 'is at least'],
		['atMost', 'is at most']
	],
	pick: [
		['is', 'is'],
		['isNot', 'is not']
	]
};

/** The words of an operator on a field. */
export function opWords(field: SmartField, op: SmartOp): string {
	return ops[specOf(field).kind].find(([each]) => each === op)?.[1] ?? op;
}

/** Ways a limit chooses, with their words. */
export const choosers: readonly (readonly [SmartChooser, string])[] = [
	['random', 'random'],
	['mostPlayed', 'most played'],
	['leastPlayed', 'least played'],
	['highestRated', 'highest rated'],
	['newest', 'most recently added'],
	['leastRecent', 'least recently played']
];

/** Orders, with their words. `chosen` is offered only while a limit is on. */
export const orders: readonly (readonly [SmartOrder, string])[] = [
	['chosen', 'how they were chosen'],
	['library', 'the Library (album, then disc and track)'],
	['title', 'title'],
	['album', 'album'],
	['year', 'year, oldest first'],
	['rating', 'rating, highest first'],
	['plays', 'plays, most first'],
	['lastPlayed', 'last played, longest ago first'],
	['added', 'date added, newest first'],
	['random', 'shuffled once']
];

export function isGroup(condition: SmartCondition): condition is SmartGroup {
	return 'rules' in condition;
}

/** A rule takes part unless it's a text rule with nothing typed: that one counts for nothing. */
export function counts(rule: SmartRule): boolean {
	return specOf(rule.field).kind !== 'text' || String(rule.value).trim() !== '';
}

/** A group takes part while one of its rules does. */
export function conditionCounts(condition: SmartCondition): boolean {
	return isGroup(condition) ? condition.rules.some(counts) : counts(condition);
}

/** Half stars as stars: 7 is "★★★½". */
export function starsText(halves: number): string {
	if (halves <= 0) {
		return '—';
	}
	return '★'.repeat(Math.floor(halves / 2)) + (halves % 2 === 1 ? '½' : '');
}

/** Half stars in a word: 7 is "3½★". */
export function starsWord(halves: number): string {
	const whole = Math.floor(halves / 2);
	const half = halves % 2 === 1;
	return `${whole === 0 && half ? '' : String(whole)}${half ? '½' : ''}★`;
}

/** What names a playlist rule's playlist: its name, or undefined when it's gone. */
export type PlaylistNames = (id: string) => string | undefined;

/** A rule read back, as the head's sentence has it: "★★★ and up", "not played in the last 7 days". */
export function ruleWords(rule: SmartRule, names: PlaylistNames = () => undefined): string {
	const spec = specOf(rule.field);
	const v = rule.value;
	const n = Number(v);
	switch (rule.field) {
		case 'rating':
			return rule.op === 'atLeast'
				? `${starsText(n)} and up`
				: rule.op === 'atMost'
					? `${starsText(n)} or less`
					: `rated ${starsText(n)}`;
		case 'lastPlayed':
			return `${rule.op === 'inLast' ? 'played' : 'not played'} in the last ${days(n)}`;
		case 'added':
			return `${rule.op === 'inLast' ? 'added' : 'not added'} in the last ${days(n)}`;
		case 'year':
			return rule.op === 'atMost'
				? `from ${String(n)} or before`
				: rule.op === 'atLeast'
					? `from ${String(n)} on`
					: `from ${String(n)}`;
		case 'plays':
		case 'skips': {
			const word = spec.unit ?? '';
			const one = n === 1 ? word.slice(0, -1) : word;
			return rule.op === 'atLeast'
				? `${String(n)}+ ${word}`
				: rule.op === 'atMost'
					? `at most ${String(n)} ${one}`
					: `${String(n)} ${one}`;
		}
		case 'length':
			return `${rule.op === 'atLeast' ? 'at least' : 'at most'} ${String(n)} min long`;
		case 'playlist': {
			const name = names(String(v));
			const where = name === undefined ? 'a playlist that’s gone' : name;
			return `${rule.op === 'is' ? 'in' : 'not in'} ${where}`;
		}
		default:
			return `${spec.label.toLowerCase()} ${opWords(rule.field, rule.op)} ${String(v)}${
				spec.unit === undefined ? '' : ` ${spec.unit}`
			}`;
	}
}

function days(n: number): string {
	return n === 1 ? 'day' : `${String(n)} days`;
}

/** A group read back: "either from 2011 or before or 10+ plays", "both … and …". */
export function groupWords(group: SmartGroup, names?: PlaylistNames): string {
	const parts = group.rules.filter(counts).map((rule) => ruleWords(rule, names));
	if (parts.length === 1) {
		return parts[0] ?? '';
	}
	const joiner = group.match === 'any' ? ' or ' : ' and ';
	if (parts.length === 2) {
		return `${group.match === 'any' ? 'either' : 'both'} ${parts.join(joiner)}`;
	}
	return `${group.match === 'any' ? 'any of' : 'all of'} ${parts.join(joiner)}`;
}

/** A limit read back: "50 songs at most, least recently played first". */
export function limitWords(limit: SmartLimit): string {
	const unit = limit.unit === 'gb' ? 'GB' : limit.unit;
	const by = choosers.find(([each]) => each === limit.by)?.[1] ?? limit.by;
	return `${limit.count.toLocaleString('en-US')} ${unit} at most, ${by} first`;
}

/** One pill of the head's sentence, and the mono word before it. */
export interface SentencePart {
	text: string;
	/** AND, OR or ·; absent before the first. */
	join?: string;
	/** A group's pill wears a dashed fox border. */
	group: boolean;
}

/** The head's sentence: what comes after {@link sentenceLead}, a pill each. */
export function sentence(rules: SmartRules, names?: PlaylistNames): SentencePart[] {
	const join = rules.match === 'all' ? 'AND' : 'OR';
	const out: SentencePart[] = [];
	for (const condition of rules.rules.filter(conditionCounts)) {
		const text = isGroup(condition) ? groupWords(condition, names) : ruleWords(condition, names);
		out.push({ text, group: isGroup(condition), ...(out.length > 0 ? { join } : {}) });
	}
	if (rules.limit !== undefined) {
		out.push({ text: limitWords(rules.limit), join: '·', group: false });
	}
	return out;
}

/** What the sentence starts with. */
export function sentenceLead(rules: SmartRules): string {
	return rules.match === 'all' ? 'SONGS WHERE ALL OF' : 'SONGS WHERE ANY OF';
}

/**
 * The phone's one-block sentence: "album artist is Falling Up · ★★★ and up · …", the rules alone:
 * the limit has no room there, and the editor shows it.
 */
export function phoneSentence(rules: SmartRules, names?: PlaylistNames): string {
	return sentence(rules, names)
		.filter(({ join }) => join !== '·')
		.map(({ text }) => text)
		.join(' · ');
}

/** Each rule, those in groups too. */
export function eachRule(rules: SmartRules): SmartRule[] {
	return rules.rules.flatMap((condition) => (isGroup(condition) ? condition.rules : [condition]));
}

/** How many rules there are, those in groups too: what the dock counts. */
export function ruleCount(rules: SmartRules): number {
	return eachRule(rules).length;
}

/** The columns a field brings to a smart playlist's list, if any. */
const columnOf: Partial<Record<SmartField, ColumnId>> = {
	rating: 'rating',
	plays: 'plays',
	lastPlayed: 'lastPlayed',
	added: 'added',
	year: 'year',
	format: 'format',
	bitrate: 'bitrate'
};

/** What a chooser or an order reads, as a field. */
const readsOf: Partial<Record<SmartChooser | SmartOrder, SmartField>> = {
	mostPlayed: 'plays',
	leastPlayed: 'plays',
	plays: 'plays',
	highestRated: 'rating',
	rating: 'rating',
	newest: 'added',
	added: 'added',
	leastRecent: 'lastPlayed',
	lastPlayed: 'lastPlayed',
	year: 'year'
};

/** The rule columns, in the order a list shows them, and how many it shows at most. */
const columnOrder: readonly ColumnId[] = [
	'rating',
	'plays',
	'lastPlayed',
	'added',
	'year',
	'format',
	'bitrate'
];
const mostRuleColumns = 3;

/**
 * The columns a smart playlist's list adds after the album: those its rules, limit and order read,
 * that a column can show, at most three, in a fixed order. Their heads are in the fox.
 */
export function ruleColumns(rules: SmartRules): ColumnId[] {
	const read = new Set<SmartField>(
		eachRule(rules)
			.filter(counts)
			.map(({ field }) => field)
	);
	for (const key of [rules.limit?.by, rules.order]) {
		const field = key === undefined ? undefined : readsOf[key];
		if (field !== undefined) {
			read.add(field);
		}
	}
	const wanted = new Set([...read].flatMap((field) => columnOf[field] ?? []));
	return columnOrder.filter((id) => wanted.has(id)).slice(0, mostRuleColumns);
}

/** A rule's starting value, for a field just chosen. */
export function startValue(field: SmartField, picks: readonly string[] = []): string | number {
	switch (specOf(field).kind) {
		case 'text':
			return '';
		case 'stars':
			return 8;
		case 'days':
			return 30;
		case 'min':
			return 5;
		case 'pick':
			return picks[0] ?? '';
		case 'num':
			return field === 'year' ? 2000 : 0;
	}
}

/** A rule of a field, with its first operator and its starting value. */
export function ruleOf(field: SmartField, picks: readonly string[] = []): SmartRule {
	const [first] = ops[specOf(field).kind];
	return { field, op: first?.[0] ?? 'is', value: startValue(field, picks) };
}

/** The rule every new smart playlist starts with: Rating is at least ★★★★. */
export function freshRules(): SmartRules {
	return { match: 'all', rules: [{ field: 'rating', op: 'atLeast', value: 8 }], order: 'library' };
}

/** What a new smart playlist is called until it's named. */
export const newSmartName = 'New smart playlist';

/** Folds text for comparison, as the server's foldSearch does: case and accents go. */
export function fold(text: string): string {
	return text
		.normalize('NFKD')
		.replace(/\p{Mn}/gu, '')
		.toLowerCase()
		.trim();
}

/** Whether two rule sets say the same, as the dock's "Nothing changed" decides. */
export function sameRules(a: SmartRules, b: SmartRules): boolean {
	return JSON.stringify(normal(a)) === JSON.stringify(normal(b));
}

function normal(rules: SmartRules): SmartRules {
	return {
		match: rules.match,
		rules: rules.rules,
		...(rules.limit === undefined ? {} : { limit: rules.limit }),
		order: rules.order ?? 'library'
	};
}

/** The rules as saved: text values trimmed, groups with no rule left out. */
export function cleaned(rules: SmartRules): SmartRules {
	const trim = (rule: SmartRule): SmartRule =>
		typeof rule.value === 'string' ? { ...rule, value: rule.value.trim() } : rule;
	return {
		...rules,
		rules: rules.rules
			.map((c) => (isGroup(c) ? { ...c, rules: c.rules.map(trim) } : trim(c)))
			.filter((c) => !isGroup(c) || c.rules.length > 0)
	};
}
