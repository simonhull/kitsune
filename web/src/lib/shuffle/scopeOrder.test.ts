import { describe, expect, it } from 'vitest';
import type { Album, LibraryIndex } from '#lib/library/source.ts';
import { entriesAfter, scopeOrder, type ScopeHolds } from './scopeOrder.ts';

const index: LibraryIndex = {
	artists: [
		{
			name: 'A',
			sortName: 'A',
			albums: [
				{
					id: 'x',
					title: 'X',
					artist: 'A',
					compilation: false,
					tracks: ['x0', 'x1'].map((id) => ({ id, title: id, durationMs: 1, artists: [] }))
				},
				{
					id: 'y',
					title: 'Y',
					artist: 'A',
					compilation: false,
					tracks: ['y0', 'y1'].map((id) => ({ id, title: id, durationMs: 1, artists: [] }))
				}
			]
		}
	]
};

const album = { id: 'x', tracks: [{ id: 'x1' }, { id: 'x0' }] } as unknown as Album;

function holds(extra: Partial<ScopeHolds> = {}): ScopeHolds {
	return {
		index,
		album: (id) => (id === 'x' ? album : undefined),
		artistSongs: (id) => (id === 'artist' ? index : null),
		marks: { stars: (id) => (id.endsWith('1') ? 4.5 : null) },
		...extra
	};
}

const ids = (order: { trackId: string }[] | null): string[] | null =>
	order?.map(({ trackId }) => trackId) ?? null;

describe('scopeOrder', () => {
	it("reads the library in the index's order, and 4 and up through its filter", () => {
		expect(ids(scopeOrder({ kind: 'library' }, holds()))).toEqual(['x0', 'x1', 'y0', 'y1']);
		expect(ids(scopeOrder({ kind: 'rated' }, holds()))).toEqual(['x1', 'y1']);
		expect(scopeOrder({ kind: 'library' }, holds({ index: null }))).toBeNull();
	});

	it("reads picked songs in the Library's order, an album in its own, an artist's in their page's", () => {
		expect(ids(scopeOrder({ kind: 'tracks', tracks: ['y1', 'x0'] }, holds()))).toEqual([
			'x0',
			'y1'
		]);
		expect(ids(scopeOrder({ kind: 'album', id: 'x' }, holds()))).toEqual(['x1', 'x0']);
		expect(scopeOrder({ kind: 'album', id: 'z' }, holds())).toBeNull();
		expect(ids(scopeOrder({ kind: 'artist', id: 'artist' }, holds()))).toHaveLength(4);
		expect(scopeOrder({ kind: 'artist', id: 'other' }, holds())).toBeNull();
	});
});

describe('entriesAfter', () => {
	it('goes on after the song, or from the start for one the order lacks', () => {
		const order = scopeOrder({ kind: 'library' }, holds()) ?? [];
		expect(ids(entriesAfter(order, 'x1'))).toEqual(['y0', 'y1']);
		expect(ids(entriesAfter(order, 'y1'))).toEqual([]);
		expect(ids(entriesAfter(order, 'elsewhere'))).toHaveLength(4);
	});
});
