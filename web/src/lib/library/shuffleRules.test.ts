import { describe, expect, it } from 'vitest';
import {
	combineOn,
	combinedElsewhere,
	emptyRulesDraft,
	excludeChanges,
	excludedNote,
	moveIn,
	orderOf,
	runChanges,
	runJoins
} from './shuffleRules.ts';
import type { ShuffleRules } from './source.ts';

const none: ShuffleRules = { runs: [], excluded: [] };

describe('moveIn', () => {
	it('moves an item up and down, as Alt with an arrow does', () => {
		expect(moveIn(['a', 'b', 'c'], 2, 0)).toEqual(['c', 'a', 'b']);
		expect(moveIn(['a', 'b', 'c'], 0, 1)).toEqual(['b', 'a', 'c']);
	});

	it('leaves the list alone past either end', () => {
		expect(moveIn(['a', 'b'], 0, -1)).toEqual(['a', 'b']);
		expect(moveIn(['a', 'b'], 1, 2)).toEqual(['a', 'b']);
	});
});

describe('the run order', () => {
	it('starts in the order the songs were picked, or the run they make', () => {
		expect(orderOf(['a', 'b', 'c'], none, emptyRulesDraft())).toEqual(['a', 'b', 'c']);
		const rules = { runs: [{ id: 'r', tracks: ['c', 'a', 'b'] }], excluded: [] };
		expect(orderOf(['a', 'b', 'c'], rules, emptyRulesDraft())).toEqual(['c', 'a', 'b']);
		expect(combineOn(['a', 'b', 'c'], rules, emptyRulesDraft())).toBe(true);
	});

	it('follows a drag', () => {
		const draft = { ...emptyRulesDraft(), order: ['b', 'a'] };
		expect(orderOf(['a', 'b'], none, draft)).toEqual(['b', 'a']);
	});

	it("can't combine one song", () => {
		expect(combineOn(['a'], none, { ...emptyRulesDraft(), combine: true })).toBe(false);
	});
});

describe('runChanges', () => {
	it('sends nothing untouched', () => {
		expect(runChanges(['a', 'b'], none, emptyRulesDraft())).toEqual([]);
	});

	it('combines in the order shown', () => {
		const draft = { combine: true, order: ['b', 'a'], exclude: null };
		expect(runChanges(['a', 'b'], none, draft)).toEqual([{ kind: 'run', tracks: ['b', 'a'] }]);
	});

	it('sends a reorder of the run they make, and nothing for the same order', () => {
		const rules = { runs: [{ id: 'r', tracks: ['a', 'b'] }], excluded: [] };
		expect(
			runChanges(['a', 'b'], rules, { combine: null, order: ['b', 'a'], exclude: null })
		).toEqual([{ kind: 'run', tracks: ['b', 'a'] }]);
		expect(runChanges(['a', 'b'], rules, { combine: true, order: null, exclude: null })).toEqual(
			[]
		);
	});

	it('undoes the run they make when Combine goes off', () => {
		const rules = { runs: [{ id: 'r', tracks: ['a', 'b'] }], excluded: [] };
		expect(runChanges(['a', 'b'], rules, { combine: false, order: null, exclude: null })).toEqual([
			{ kind: 'unrun', id: 'r' }
		]);
	});

	it('excludes only the songs not left out yet, and lets back only those that are', () => {
		const rules = { runs: [], excluded: ['a'] };
		expect(excludeChanges(['a', 'b'], rules, () => true)).toEqual([
			{ kind: 'exclude', tracks: ['b'], on: true }
		]);
		expect(excludeChanges(['a', 'b'], rules, () => false)).toEqual([
			{ kind: 'exclude', tracks: ['a'], on: false }
		]);
		expect(excludeChanges(['a', 'b'], rules, (id) => id === 'a')).toEqual([]);
		expect(excludeChanges(['a', 'b'], rules, (id) => id === 'b')).toEqual([
			{ kind: 'exclude', tracks: ['b'], on: true },
			{ kind: 'exclude', tracks: ['a'], on: false }
		]);
	});
});

describe('what the tab says', () => {
	it('counts the songs combined elsewhere', () => {
		const rules = { runs: [{ id: 'r', tracks: ['a', 'x'] }], excluded: [] };
		expect(combinedElsewhere(['a', 'b'], rules)).toBe(1);
	});

	it('says how many are excluded now', () => {
		expect(excludedNote(['a', 'b', 'c'], none)).toBe('None of them is excluded now.');
		expect(excludedNote(['a', 'b'], { runs: [], excluded: ['a'] })).toBe(
			'1 of them is excluded now.'
		);
		expect(excludedNote(['a'], { runs: [], excluded: ['a'] })).toBe("It's excluded now.");
	});
});

describe('runJoins', () => {
	it('joins rows next to each other in their run, in its order', () => {
		const rows = [
			{ shuffle: { run: 'r', position: 1, length: 3 } },
			{ shuffle: { run: 'r', position: 2, length: 3 } },
			{},
			{ shuffle: { run: 'r', position: 3, length: 3 } },
			{ shuffle: { run: 's', position: 2, length: 2 } },
			{ shuffle: { run: 's', position: 1, length: 2 } }
		];
		expect(rows.map((_, index) => runJoins(rows, index))).toEqual([
			{ up: false, down: true },
			{ up: true, down: false },
			{ up: false, down: false },
			{ up: false, down: false },
			{ up: false, down: false },
			{ up: false, down: false }
		]);
	});
});
