import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	artistLine,
	colorSource,
	foundLine,
	resultsOf,
	Search,
	searchLine,
	spotlight,
	type Ask
} from './search.svelte';
import { UnauthorizedError, type AlbumEntry, type SearchResults, type TrackHit } from './source';

const nothing: SearchResults = { artists: [], albums: [], tracks: [] };
const some: SearchResults = { artists: [{ id: 'a', name: 'A' }], albums: [], tracks: [] };

const artwork = { hash: 'h', accent: '#112233', ground: '#000000', width: 1, height: 1 };

function album(id: string, title: string, artist = 'Someone'): AlbumEntry {
	return { id, title, artist, trackCount: 1, compilation: false };
}

function hit(id: string, title: string, artists = ['Someone']): TrackHit {
	return { id, title, artists, albumId: `album-${id}`, album: 'Album', durationMs: 1000, artwork };
}

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

describe('Search.answered', () => {
	it('stays with the results on screen until the next answer lands', async () => {
		const { ask, asked } = deferred();
		const search = new Search(ask);
		search.run('fox');
		at(asked, 0).answer(some);
		await Promise.resolve();
		await Promise.resolve();
		search.run('foxes');
		expect(search.answered).toBe('fox');
		at(asked, 1).answer(some);
		await Promise.resolve();
		await Promise.resolve();
		expect(search.answered).toBe('foxes');
	});
});

describe('spotlight', () => {
	const artist = { id: 'a', name: 'Low Tide Choir' };
	const other = { id: 'b', name: 'Tide' };

	it('finds nothing in nothing', () => {
		expect(spotlight(nothing, 'fox')).toBeNull();
	});

	it("takes an exact artist over the server's order, and out of its group", () => {
		const found = spotlight(
			{ artists: [artist, other], albums: [album('x', 'Tide')], tracks: [] },
			'tide'
		);
		expect(found?.top).toEqual({ kind: 'artist', artist: other });
		expect(found?.artists).toEqual([artist]);
		expect(found?.albums).toHaveLength(1);
	});

	it('takes an exact album over an exact track, and either over a near artist', () => {
		const tracks = [hit('t', 'Tidewater')];
		const albums = [album('y', 'Other'), album('x', 'Tidewater')];
		const found = spotlight({ artists: [artist], albums, tracks }, 'tidewater');
		expect(found?.top).toMatchObject({ kind: 'album', album: { id: 'x' } });
		expect(found?.albums.map(({ id }) => id)).toEqual(['y']);
		expect(found?.tracks).toEqual(tracks);
		expect(spotlight({ artists: [artist], albums: [], tracks }, 'tidewater')?.top.kind).toBe(
			'track'
		);
	});

	it('reads past case, accents and spacing', () => {
		const found = spotlight(
			{ artists: [artist, { id: 'c', name: 'Sigur Rós' }], albums: [], tracks: [] },
			'  sigur   ros '
		);
		expect(found?.top).toMatchObject({ kind: 'artist', artist: { id: 'c' } });
	});

	it("keeps the server's order with nothing exact", () => {
		const albums = [album('x', 'Low Light')];
		const tracks = [hit('t', 'Low Water')];
		expect(spotlight({ artists: [artist], albums, tracks }, 'low')?.top.kind).toBe('artist');
		expect(spotlight({ artists: [], albums, tracks }, 'low')?.top.kind).toBe('album');
		expect(spotlight({ artists: [], albums: [], tracks }, 'low')?.top.kind).toBe('track');
	});
});

describe('colorSource', () => {
	const artist = { id: 'a', name: 'Low Tide Choir' };

	it("is the hit's own album", () => {
		const entry = album('x', 'Glasshouse');
		expect(colorSource({ kind: 'album', album: entry }, nothing)).toBe(entry);
		expect(colorSource({ kind: 'track', track: hit('t', 'Panes') }, nothing)).toEqual({
			id: 'album-t',
			artwork
		});
	});

	it('borrows for an artist, from an album before a track', () => {
		const theirs = album('x', 'Glasshouse', 'low tide choir');
		const tracks = [hit('t', 'Panes', ['Guest', 'Low Tide Choir'])];
		const top = { kind: 'artist', artist } as const;
		const albums = [album('y', 'Other'), theirs];
		expect(colorSource(top, { artists: [], albums, tracks })).toBe(theirs);
		expect(colorSource(top, { artists: [], albums: [], tracks })).toEqual({
			id: 'album-t',
			artwork
		});
	});

	it('leaves an artist with nothing here to the fox', () => {
		const results = { artists: [], albums: [album('y', 'Other')], tracks: [hit('t', 'Panes')] };
		expect(colorSource({ kind: 'artist', artist }, results)).toBeUndefined();
	});
});

describe('artistLine', () => {
	const artist = { id: 'a', name: 'Mara' };

	it('counts only what this search found by them', () => {
		const results = {
			artists: [],
			albums: [album('x', 'One', 'Mara'), album('y', 'Two')],
			tracks: [hit('t', 'Three', ['Mara']), hit('u', 'Four', ['Mara'])]
		};
		expect(artistLine(artist, results)).toBe('1 album · 2 tracks in this search');
	});

	it('says nothing rather than zero', () => {
		expect(artistLine(artist, nothing)).toBe('');
	});
});
