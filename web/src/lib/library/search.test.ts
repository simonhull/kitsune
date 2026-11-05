import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { foundLine, resultsOf, Search, searchLine, type Ask } from './search.svelte';
import { UnauthorizedError, type SearchResults } from './source';

const nothing: SearchResults = { artists: [], albums: [], tracks: [] };
const some: SearchResults = { artists: [{ id: 'a', name: 'A' }], albums: [], tracks: [] };

/** An answer the test hands over when it chooses. */
function deferred() {
	const asked: {
		query: string;
		signal: AbortSignal;
		answer: (results: SearchResults) => void;
		fail: (error: unknown) => void;
	}[] = [];
	const ask: Ask = (query, signal) =>
		new Promise((answer, fail) => {
			asked.push({ query, signal, answer, fail });
		});
	return { ask, asked };
}

function at(asked: ReturnType<typeof deferred>['asked'], index: number) {
	const question = asked.at(index);
	if (question === undefined) {
		throw new Error('never asked');
	}
	return question;
}

describe('Search', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('rests until there is something to look for', () => {
		const { ask, asked } = deferred();
		const search = new Search(ask);
		search.run('');
		expect(search.state.kind).toBe('idle');
		expect(asked).toHaveLength(0);
	});

	it('lands an answer', async () => {
		const { ask, asked } = deferred();
		const search = new Search(ask);
		search.run('fox');
		at(asked, 0).answer(some);
		await vi.advanceTimersByTimeAsync(0);
		expect(search.state).toEqual({ kind: 'found', query: 'fox', results: some });
	});

	it('takes the last question back, so a stale answer cannot win', async () => {
		const { ask, asked } = deferred();
		const search = new Search(ask);
		search.run('f');
		search.run('fo');
		expect(at(asked, 0).signal.aborted).toBe(true);
		at(asked, 1).answer(some);
		at(asked, 0).answer(nothing);
		await vi.advanceTimersByTimeAsync(0);
		expect(search.state).toMatchObject({ query: 'fo', results: some });
	});

	it('ignores the abort of a question it took back', async () => {
		const { ask, asked } = deferred();
		const search = new Search(ask);
		search.run('f');
		search.run('fo');
		at(asked, 0).fail(new DOMException('aborted', 'AbortError'));
		await vi.advanceTimersByTimeAsync(0);
		expect(search.state).toMatchObject({ kind: 'looking', query: 'fo' });
	});

	it('keeps the last results up while it looks', async () => {
		const { ask, asked } = deferred();
		const search = new Search(ask);
		search.run('f');
		at(asked, 0).answer(some);
		await vi.advanceTimersByTimeAsync(0);
		search.run('fo');
		expect(resultsOf(search.state)).toBe(some);
	});

	it('says it is looking only once it has been a while', async () => {
		const { ask, asked } = deferred();
		const search = new Search(ask);
		search.run('fox');
		await vi.advanceTimersByTimeAsync(199);
		expect(search.state).toMatchObject({ slow: false });
		await vi.advanceTimersByTimeAsync(1);
		expect(search.state).toMatchObject({ slow: true });
		at(asked, 0).answer(some);
		await vi.advanceTimersByTimeAsync(500);
		expect(search.state.kind).toBe('found');
	});

	it('reports a failure, and can ask again', async () => {
		const { ask, asked } = deferred();
		const search = new Search(ask);
		search.run('fox');
		at(asked, 0).fail(new TypeError('offline'));
		await vi.advanceTimersByTimeAsync(500);
		expect(search.state).toEqual({ kind: 'failed', query: 'fox' });
		search.run('fox');
		at(asked, 1).answer(nothing);
		await vi.advanceTimersByTimeAsync(0);
		expect(search.state.kind).toBe('found');
	});

	it('says nothing of a lost session: the app has gone to the door', async () => {
		const { ask, asked } = deferred();
		const search = new Search(ask);
		search.run('fox');
		at(asked, 0).fail(new UnauthorizedError());
		await vi.advanceTimersByTimeAsync(0);
		expect(search.state.kind).toBe('looking');
	});

	it('takes a question back when the page goes, and keeps what it found', async () => {
		const { ask, asked } = deferred();
		const search = new Search(ask);
		search.run('f');
		at(asked, 0).answer(some);
		await vi.advanceTimersByTimeAsync(0);
		search.run('fo');
		search.stop();
		expect(at(asked, 1).signal.aborted).toBe(true);
		expect(resultsOf(search.state)).toBe(some);
	});

	it('forgets on reset', () => {
		const { ask, asked } = deferred();
		const search = new Search(ask);
		search.run('fox');
		search.reset();
		expect(at(asked, 0).signal.aborted).toBe(true);
		expect(search.state.kind).toBe('idle');
	});
});

describe('the line under the box', () => {
	it('invites at rest', () => {
		expect(searchLine({ kind: 'idle' }).text).toContain('Typos');
	});

	it('counts what it found, leaving out empty groups', () => {
		const track = { id: 't', title: 't', artists: [], albumId: 'a', album: 'a', durationMs: 1000 };
		expect(foundLine({ ...some, tracks: [track, track] })).toBe('1 artist · 2 tracks');
		expect(searchLine({ kind: 'found', query: 'a', results: some })).toEqual({
			text: '1 artist',
			fact: true
		});
	});

	it('names the query when nothing matched', () => {
		expect(searchLine({ kind: 'found', query: 'zzz', results: nothing })).toEqual({
			text: 'Nothing in the library goes by “zzz”.',
			fact: false
		});
	});

	it('stays quiet through a quick answer', () => {
		const looking = { kind: 'looking', query: 'f', slow: false } as const;
		expect(searchLine({ ...looking, shown: null }).text).toBe('');
		expect(searchLine({ ...looking, shown: some }).text).toBe('1 artist');
	});

	it('owns up to a slow one', () => {
		const state = { kind: 'looking', query: 'f', shown: some, slow: true } as const;
		expect(searchLine(state)).toEqual({ text: 'Looking…', fact: false });
	});

	it('owns up to a failure', () => {
		expect(searchLine({ kind: 'failed', query: 'f' }).text).toContain("didn't come back");
	});
});
