import { describe, expect, it } from 'vitest';
import { partOf, patchIndex } from './indexPatch';
import type { LibraryAlbum, LibraryArtist, LibraryIndex } from './source';

function album(id: string): LibraryAlbum {
	return { id, title: id, artist: '', compilation: false, hearted: false, tracks: [] };
}

function artist(name: string, ...albums: string[]): LibraryArtist {
	return { id: `ar-${name}`, name, sortName: name, hearted: false, albums: albums.map(album) };
}

/** A heading with nobody to link to, as a compilation's. */
function nobody(name: string, ...albums: string[]): LibraryArtist {
	return { name, sortName: name, hearted: false, albums: albums.map(album) };
}

const before: LibraryIndex = {
	artists: [
		artist('Ann', 'a1', 'a2'),
		artist('Bob', 'b1'),
		nobody('Various', 'v1'),
		artist('Cat', 'c1')
	]
};

/** Patches `from` with what changed on the way to `to`, as the server would answer. */
function patched(from: LibraryIndex, to: LibraryIndex, ids: string[]): LibraryIndex | null {
	return patchIndex(from, ids, partOf(to, ids));
}

describe('patching the index', () => {
	const cases: [string, LibraryIndex, string[]][] = [
		[
			'an album added under its artist',
			{ artists: [artist('Ann', 'a1', 'a3', 'a2'), ...before.artists.slice(1)] },
			['a3']
		],
		[
			'an album first in the whole index',
			{ artists: [artist('Aaa', 'z1'), ...before.artists] },
			['z1']
		],
		[
			'a new artist between two',
			{
				artists: [...before.artists.slice(0, 2), artist('Bobby', 'bb1'), ...before.artists.slice(2)]
			},
			['bb1']
		],
		['a new artist at the end', { artists: [...before.artists, artist('Dan', 'd1')] }, ['d1']],
		['an album gone', { artists: [artist('Ann', 'a1'), ...before.artists.slice(1)] }, ['a2']],
		[
			"an artist's only album gone",
			{ artists: [before.artists[0], ...before.artists.slice(2)] as LibraryArtist[] },
			['b1']
		],
		[
			'an album moved to another artist',
			{
				artists: [artist('Ann', 'a1'), artist('Bob', 'a2', 'b1'), ...before.artists.slice(2)]
			},
			['a2']
		],
		[
			'an album added to a heading with nobody to link to',
			{
				artists: [...before.artists.slice(0, 2), nobody('Various', 'v0', 'v1'), artist('Cat', 'c1')]
			},
			['v0']
		],
		[
			'every album changed at once',
			{ artists: [artist('Cat', 'c1'), artist('Ann', 'a2', 'a1'), nobody('Various', 'v1')] },
			['a1', 'a2', 'b1', 'v1', 'c1']
		]
	];

	for (const [name, after, ids] of cases) {
		it(`gives the whole index for ${name}`, () => {
			expect(patched(before, after, ids)).toEqual(after);
		});
	}

	it('keeps the albums that did not change as they were', () => {
		const after: LibraryIndex = {
			artists: [artist('Ann', 'a1', 'a3', 'a2'), ...before.artists.slice(1)]
		};
		const next = patched(before, after, ['a3']);
		expect(next?.artists[1]).toBe(before.artists[1]);
	});

	it('gives up on an album to follow that it lacks', () => {
		const part: LibraryIndex = {
			artists: [{ ...artist('Ann'), albums: [{ ...album('x'), after: 'nope' }] }]
		};
		expect(patchIndex(before, ['x'], part)).toBeNull();
	});
});
