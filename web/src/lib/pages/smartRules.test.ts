import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { SmartPreview, SmartPreviewRequest, SmartRules } from '#lib/library/source.ts';
import { isDraftGroup, LivePreview, previewDelayMs, SmartDraft } from './smartRules.svelte.ts';

const saved: SmartRules = {
	match: 'all',
	rules: [
		{ field: 'rating', op: 'atLeast', value: 6 },
		{
			match: 'any',
			rules: [
				{ field: 'year', op: 'atMost', value: 2011 },
				{ field: 'plays', op: 'atLeast', value: 10 }
			]
		}
	],
	limit: { count: 50, unit: 'songs', by: 'leastRecent' },
	order: 'chosen'
};

describe('SmartDraft', () => {
	it('opens as saved, with nothing changed', () => {
		const draft = new SmartDraft(saved, 'Rested');
		expect(draft.rules).toEqual(saved);
		expect(draft.changed).toBe(false);
		expect(draft.canGroup).toBe(false);
	});

	it('starts fresh with one rule, Rating is at least ★★★★, and is never "nothing changed"', () => {
		const draft = SmartDraft.fresh();
		expect(draft.name).toBe('New smart playlist');
		expect(draft.rules.rules).toEqual([{ field: 'rating', op: 'atLeast', value: 8 }]);
		expect(draft.rules.limit).toBeUndefined();
		expect(draft.changed).toBe(true);
	});

	it('adds and takes out rules, and a field brings its own operator and value', () => {
		const draft = new SmartDraft(saved, 'Rested');
		draft.addRule();
		expect(draft.items).toHaveLength(3);
		draft.setField({ at: 2 }, 'title');
		expect(draft.ruleAt({ at: 2 })).toEqual({ field: 'title', op: 'is', value: '' });
		expect(draft.changed).toBe(true);
		draft.remove({ at: 2 });
		expect(draft.changed).toBe(false);
	});

	it('has one level of group: Group only while there is none, and an emptied group goes', () => {
		const draft = new SmartDraft({ match: 'all', rules: [] }, 'x');
		draft.addGroup();
		draft.addGroup();
		expect(draft.items.filter(isDraftGroup)).toHaveLength(1);
		draft.addToGroup(0);
		draft.patch({ at: 0, in: 1 }, { value: 'Captiva' });
		draft.setGroupMatch(0, 'all');
		expect(draft.rules.rules).toEqual([
			{
				match: 'all',
				rules: [
					{ field: 'album', op: 'is', value: '' },
					{ field: 'album', op: 'is', value: 'Captiva' }
				]
			}
		]);
		draft.remove({ at: 0, in: 0 });
		draft.remove({ at: 0, in: 0 });
		expect(draft.items).toEqual([]);
		expect(draft.canGroup).toBe(true);
	});

	it('keeps an empty text rule in the draft: it counts for nothing, and is kept to fill in', () => {
		const draft = new SmartDraft({ match: 'all', rules: [] }, 'x');
		draft.addRule();
		draft.setField({ at: 0 }, 'artist');
		expect(draft.rules.rules).toEqual([{ field: 'artist', op: 'is', value: '' }]);
	});

	it('turns the limit on with the order it chose, and off back to the Library', () => {
		const draft = new SmartDraft({ match: 'all', rules: [] }, 'x');
		draft.toggleLimit();
		expect(draft.rules.limit).toEqual({ count: 50, unit: 'songs', by: 'random' });
		expect(draft.order).toBe('chosen');
		draft.setLimit({ count: 3, unit: 'gb' });
		draft.toggleLimit();
		expect(draft.rules.limit).toBeUndefined();
		expect(draft.order).toBe('library');
		draft.toggleLimit();
		expect(draft.rules.limit).toEqual({ count: 3, unit: 'gb', by: 'random' });
	});
});

describe('LivePreview', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	const answer = (songCount: number): SmartPreview => ({
		songCount,
		matchCount: songCount,
		albumCount: 1,
		durationMs: 0,
		mosaic: [],
		songs: []
	});
	const asked = (n: number): SmartPreviewRequest => ({
		rules: { match: 'all', rules: [{ field: 'plays', op: 'atLeast', value: n }] }
	});

	it('waits for typing to stop, then asks once', async () => {
		const ask = vi.fn((request: SmartPreviewRequest) =>
			Promise.resolve(answer(Number((request.rules.rules[0] as { value: number }).value)))
		);
		const preview = new LivePreview(ask);
		preview.request(asked(1));
		await vi.advanceTimersByTimeAsync(previewDelayMs - 20);
		preview.request(asked(2));
		await vi.advanceTimersByTimeAsync(previewDelayMs - 20);
		preview.request(asked(3));
		expect(ask).not.toHaveBeenCalled();
		await vi.advanceTimersByTimeAsync(previewDelayMs);
		expect(ask).toHaveBeenCalledTimes(1);
		expect(preview.answer?.songCount).toBe(3);
	});

	it('aborts the question before, and keeps its last answer until the next arrives', async () => {
		const signals: AbortSignal[] = [];
		const pending: ((value: SmartPreview) => void)[] = [];
		const preview = new LivePreview(
			(_request, signal) =>
				new Promise((resolve) => {
					signals.push(signal);
					pending.push(resolve);
				})
		);
		void preview.now(asked(1));
		pending[0]?.(answer(19));
		await vi.advanceTimersByTimeAsync(0);
		expect(preview.answer?.songCount).toBe(19);

		preview.request(asked(2));
		await vi.advanceTimersByTimeAsync(previewDelayMs);
		preview.request(asked(3));
		await vi.advanceTimersByTimeAsync(previewDelayMs);
		expect(signals[1]?.aborted).toBe(true);
		expect(signals[2]?.aborted).toBe(false);
		// The aborted one answering late changes nothing; the card still shows 19 meanwhile.
		pending[1]?.(answer(5));
		await vi.advanceTimersByTimeAsync(0);
		expect(preview.answer?.songCount).toBe(19);
		pending[2]?.(answer(12));
		await vi.advanceTimersByTimeAsync(0);
		expect(preview.answer?.songCount).toBe(12);
	});

	it('says a failure without blanking the answer, and stops when the editor closes', async () => {
		let fail = false;
		const ask = vi.fn(() => (fail ? Promise.reject(new Error('no')) : Promise.resolve(answer(4))));
		const preview = new LivePreview(ask);
		await preview.now(asked(1));
		fail = true;
		await preview.now(asked(2));
		expect(preview.failed).toBe(true);
		expect(preview.answer?.songCount).toBe(4);
		preview.request(asked(3));
		preview.stop();
		await vi.advanceTimersByTimeAsync(previewDelayMs * 2);
		expect(ask).toHaveBeenCalledTimes(2);
	});
});
