import { describe, expect, it } from 'vitest';
import type { LibraryAlbum, LibraryIndex } from './source.ts';
import { Trashed } from './trashed.svelte.ts';

function album(id: string, tracks: string[]): LibraryAlbum {
	return {
		id,
		title: id,
		artist: 'A',
		trackCount: tracks.length,
		compilation: false,
		tracks: tracks.map((track) => ({ id: track, title: track, artists: ['A'], durationMs: 1000 }))
	} as LibraryAlbum;
}

const index: LibraryIndex = {
	artists: [
		{
			id: 'ar1',
			name: 'A',
			sortName: 'A',
			albums: [album('a1', ['t1', 't2']), album('a2', ['t3'])]
		},
		{ id: 'ar2', name: 'B', sortName: 'B', albums: [album('b1', ['t4'])] }
	]
};
const ready = { kind: 'ready', value: index } as const;

function ids(load: ReturnType<Trashed['index']>): string[] {
	return load.kind === 'ready'
		? load.value.artists.flatMap(({ albums }) =>
				albums.flatMap(({ tracks }) => tracks.map(({ id }) => id))
			)
		: [];
}

describe('Trashed', () => {
	it('gives back the very same index while nothing is on its way', () => {
		expect(new Trashed().index(ready)).toBe(ready);
	});

	it('leaves out songs on their way, and albums and headings left empty', () => {
		const trashed = new Trashed();
		trashed.hide([
			{ trackId: 't1', albumId: 'a1' },
			{ trackId: 't4', albumId: 'b1' }
		]);
		const shown = trashed.index(ready);
		expect(ids(shown)).toEqual(['t2', 't3']);
		expect(shown.kind === 'ready' ? shown.value.artists.length : 0).toBe(1);
		// Read twice, it's the same object, so a list doesn't lay itself out again.
		expect(trashed.index(ready)).toBe(shown);
	});

	it('leaves out an album moved whole before its songs are known', () => {
		const trashed = new Trashed();
		trashed.hide([], ['a2']);
		expect(ids(trashed.index(ready))).toEqual(['t1', 't2', 't4']);
		expect(trashed.hidesAlbum('a2')).toBe(true);
	});

	it('gives them back in place on an Undo', () => {
		const trashed = new Trashed();
		trashed.hide([{ trackId: 't1', albumId: 'a1' }]);
		trashed.show(['t1']);
		expect(ids(trashed.index(ready))).toEqual(['t1', 't2', 't3', 't4']);
	});

	it('lets go of what the server took once it has told of their albums, and no sooner', () => {
		const trashed = new Trashed();
		trashed.hide([
			{ trackId: 't1', albumId: 'a1' },
			{ trackId: 't3', albumId: 'a2' }
		]);
		trashed.settled(['a1']);
		expect(trashed.hides('t1')).toBe(true); // Not taken yet: an older change mustn't bring it back.
		trashed.confirm({ tracks: ['t1'], albums: ['a1'] }, false);
		trashed.settled(['a1']);
		expect(trashed.hides('t1')).toBe(false);
		expect(trashed.hides('t3')).toBe(true);
	});
});
