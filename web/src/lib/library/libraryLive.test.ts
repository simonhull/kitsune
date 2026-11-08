import { afterEach, describe, expect, it, vi } from 'vitest';
import { theme } from '#lib/theme/theme.svelte.ts';
import { FakeStream } from './fakeStream';
import { createFixtureSource } from './fixtures';
import { partOf } from './indexPatch';
import { Library } from './library.svelte';
import type { LibraryIndex, LibrarySource, ScanStatus } from './source';

/** A library over the fixtures whose index the test changes, hearing what `stream` says. */
async function heard(start: LibraryIndex, overrides: Partial<LibrarySource> = {}) {
	const stream = new FakeStream();
	const state = { whole: start, wholeReads: 0 };
	const source: LibrarySource = {
		...createFixtureSource(),
		libraryIndex: () => {
			state.wholeReads += 1;
			return Promise.resolve(state.whole);
		},
		libraryIndexOf: (ids) => Promise.resolve(partOf(state.whole, ids)),
		...overrides
	};
	const library = new Library(source, { events: () => stream });
	await library.loadShell();
	await library.loadIndex();
	stream.open();
	stream.send('ready', {});
	return { library, stream, state };
}

const album = (id: string, artistId = 'ar1') => ({
	id,
	title: id,
	artist: artistId,
	compilation: false,
	hearted: false,
	tracks: [{ id: `${id}-t`, title: 't', durationMs: 1, artists: [], hearted: false }]
});

const one: LibraryIndex = {
	artists: [{ id: 'ar1', name: 'Ann', sortName: 'Ann', hearted: false, albums: [album('a1')] }]
};
const two: LibraryIndex = {
	artists: [
		{ id: 'ar1', name: 'Ann', sortName: 'Ann', hearted: false, albums: [album('a1'), album('a2')] }
	]
};

describe('following the server', () => {
	afterEach(() => {
		theme.forget();
	});

	it('patches the index with just the albums that changed', async () => {
		const { library, stream, state } = await heard(one);
		state.whole = two;
		stream.send('library', { albums: ['a2'], all: false });
		await vi.waitFor(() => {
			expect(library.index).toEqual({ kind: 'ready', value: two });
		});
		expect(state.wholeReads).toBe(1);
		expect(library.indexStale).toBe(false);
	});

	it('reads the whole index when too much changed to list', async () => {
		const { library, stream, state } = await heard(one);
		state.whole = two;
		stream.send('library', { albums: [], all: true });
		await vi.waitFor(() => {
			expect(library.index).toEqual({ kind: 'ready', value: two });
		});
		expect(state.wholeReads).toBe(2);
	});

	it('shows hearts and stars changed on another device', async () => {
		const { library, stream } = await heard(one);
		stream.send('marks', { kind: 'albums', id: 'a1', hearted: true });
		stream.send('marks', { kind: 'tracks', id: 'a1-t', stars: 4 });
		expect(library.marks.hearted('albums', 'a1')).toBe(true);
		expect(library.marks.stars('a1-t')).toBe(4);
		stream.send('marks', { kind: 'tracks', id: 'a1-t', stars: 0 });
		expect(library.marks.stars('a1-t')).toBeNull();
	});

	it('takes the theme chosen on another device', async () => {
		let saved: 'light' | 'dark' = 'dark';
		const { stream } = await heard(one, {
			pref: (() => Promise.resolve(saved)) as LibrarySource['pref']
		});
		saved = 'light';
		stream.send('prefs', { key: 'ui.theme' });
		await vi.waitFor(() => {
			expect(theme.choice).toBe('light');
		});
	});

	it('follows a scan without asking, and stops polling while it hears', async () => {
		const facts = vi.fn(createFixtureSource().facts);
		const { library, stream } = await heard(one, { facts });
		const running: ScanStatus = { state: 'running', mode: 'quick', read: 3, total: 9 };
		stream.send('scan', running);
		expect(library.facts?.scan).toEqual(running);
		stream.send('fingerprints', { done: 1, total: 4 });
		expect(library.facts?.fingerprints).toEqual({ done: 1, total: 4 });
		const asked = facts.mock.calls.length;
		stream.send('scan', { state: 'idle', read: 0, total: 0 });
		await vi.waitFor(() => {
			expect(facts.mock.calls.length).toBe(asked + 1); // The end reads the counts, once.
		});
	});

	it('says when the settings changed, for an open Server page', async () => {
		const { library, stream } = await heard(one);
		const before = library.settingsHeard;
		stream.send('settings', { keys: ['library.roots'] });
		expect(library.settingsHeard).toBe(before + 1);
	});

	it('reads everything again after a gap, and closes at sign-out', async () => {
		const { library, stream, state } = await heard(one);
		state.whole = two;
		stream.send('resync', {});
		await vi.waitFor(() => {
			expect(library.index).toEqual({ kind: 'ready', value: two });
		});
		library.reset();
		expect(stream.closed).toBe(true);
	});
});
