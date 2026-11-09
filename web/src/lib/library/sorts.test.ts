import { describe, expect, it } from 'vitest';
import {
	albumOrders,
	artistOrders,
	byKnown,
	defaultAlbumSort,
	defaultArtistSort,
	defaultLibrarySort,
	libraryArtistOrders,
	pick,
	savedAlbumSort,
	savedArtistSort,
	savedLibrarySort,
	wayOf
} from './sorts.ts';

describe('pick', () => {
	it('turns the order already chosen round', () => {
		expect(pick(albumOrders, { by: 'year', dir: 'desc' }, 'year')).toEqual({
			by: 'year',
			dir: 'asc'
		});
		expect(pick(albumOrders, { by: 'year', dir: 'asc' }, 'year')).toEqual({
			by: 'year',
			dir: 'desc'
		});
	});

	it('starts any other the way it reads best', () => {
		expect(pick(albumOrders, defaultAlbumSort, 'year')).toEqual({ by: 'year', dir: 'desc' });
		expect(pick(albumOrders, { by: 'year', dir: 'asc' }, 'name')).toEqual({
			by: 'name',
			dir: 'asc'
		});
		expect(pick(libraryArtistOrders, defaultLibrarySort.artists, 'plays')).toEqual({
			by: 'plays',
			dir: 'desc'
		});
		expect(pick(artistOrders, defaultArtistSort, 'albums')).toEqual({ by: 'albums', dir: 'desc' });
	});
});

describe('wayOf', () => {
	it.each([
		['name', 'asc', 'A→Z'],
		['name', 'desc', 'Z→A'],
		['year', 'desc', 'NEWEST FIRST'],
		['added', 'asc', 'OLDEST FIRST'],
		['plays', 'desc', 'MOST FIRST'],
		['rating', 'asc', 'LOWEST FIRST']
	] as const)('says %s %s as %s', (by, dir, words) => {
		expect(wayOf(by, dir)).toBe(words);
	});
});

describe('a saved order', () => {
	it('is the default when there is none, or one this client doesn’t know', () => {
		expect(savedLibrarySort(null)).toEqual(defaultLibrarySort);
		expect(savedAlbumSort({ by: 'mood', dir: 'asc' })).toEqual(defaultAlbumSort);
		expect(savedArtistSort({ by: 'name', dir: 'up' })).toEqual(defaultArtistSort);
		expect(savedArtistSort('name')).toEqual(defaultArtistSort);
	});

	it('keeps the half of the Library’s it knows', () => {
		expect(
			savedLibrarySort({
				artists: { by: 'plays', dir: 'desc' },
				albums: { by: 'mood', dir: 'asc' }
			})
		).toEqual({ artists: { by: 'plays', dir: 'desc' }, albums: defaultLibrarySort.albums });
	});

	it('is kept as saved', () => {
		expect(savedAlbumSort({ by: 'rating', dir: 'asc' })).toEqual({ by: 'rating', dir: 'asc' });
	});
});

describe('byKnown', () => {
	it('puts the unknown last either way', () => {
		const list = [3, undefined, 1];
		expect(list.toSorted((a, b) => byKnown(a, b, 'asc'))).toEqual([1, 3, undefined]);
		expect(list.toSorted((a, b) => byKnown(a, b, 'desc'))).toEqual([3, 1, undefined]);
	});
});
