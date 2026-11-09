import { describe, expect, it } from 'vitest';
import { artistFigure, orderArtists } from './artistOrder.ts';
import type { ListedArtist } from './source.ts';
import type { ArtistOrder, Direction } from './sorts.ts';

function listed(
	id: string,
	albumCount: number,
	addedAt: string,
	plays: number,
	name = id
): ListedArtist {
	return { id, name, albumCount, addedAt, plays };
}

// As the server sends them: by sort name, then ID.
const artists = [
	listed('ann', 3, '2026-03-01T00:00:00Z', 5),
	listed('bob-1', 1, '2026-05-01T00:00:00Z', 5, 'Bob'),
	listed('bob-2', 1, '2026-05-01T00:00:00Z', 0, 'Bob'),
	listed('zed', 2, 'not a time', 9)
];

function ids(by: ArtistOrder, dir: Direction): string[] {
	return orderArtists(artists, { by, dir }).map(({ id }) => id);
}

describe('orderArtists', () => {
	it('keeps the server’s order by name, and turns it round a name at a time', () => {
		expect(ids('name', 'asc')).toEqual(['ann', 'bob-1', 'bob-2', 'zed']);
		expect(ids('name', 'desc')).toEqual(['zed', 'bob-1', 'bob-2', 'ann']);
	});

	it('sorts by albums owned, ties by name', () => {
		expect(ids('albums', 'desc')).toEqual(['ann', 'zed', 'bob-1', 'bob-2']);
		expect(ids('albums', 'asc')).toEqual(['bob-1', 'bob-2', 'zed', 'ann']);
	});

	it('sorts by the newest album’s arrival, an unknown one last either way', () => {
		expect(ids('added', 'desc')).toEqual(['bob-1', 'bob-2', 'ann', 'zed']);
		expect(ids('added', 'asc')).toEqual(['ann', 'bob-1', 'bob-2', 'zed']);
	});

	it('sorts by the caller’s plays, ties by name', () => {
		expect(ids('plays', 'desc')).toEqual(['zed', 'ann', 'bob-1', 'bob-2']);
		expect(ids('plays', 'asc')).toEqual(['bob-2', 'ann', 'bob-1', 'zed']);
	});
});

describe('artistFigure', () => {
	it('says what the list is sorted by', () => {
		const [ann] = artists;
		if (ann === undefined) {
			throw new Error('no artist');
		}
		expect(artistFigure(ann, { by: 'name', dir: 'asc' })).toBe('3');
		expect(artistFigure(ann, { by: 'plays', dir: 'desc' })).toBe('5 plays');
		expect(artistFigure({ ...ann, plays: 1 }, { by: 'plays', dir: 'desc' })).toBe('1 play');
		expect(artistFigure(ann, { by: 'added', dir: 'desc' })).toBe('Mar 2026');
	});
});
