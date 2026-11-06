import { describe, expect, it } from 'vitest';
import { albumOn, journeyOf, navOf, routeIds } from './routes';

describe('the route table', () => {
	it('keeps the look, the current item and the journey apart', () => {
		expect(navOf('/album/[id]')).toMatchObject({ look: 'album', current: 'albums' });
		expect(journeyOf('/album/[id]')).toBe('album');
		expect(navOf('/login')).toMatchObject({ look: 'door', current: 'home' });
		expect(journeyOf('/login')).toBe('login');
	});

	it('dresses both artist pages alike, as different journeys', () => {
		for (const id of ['/artists', '/artists/[id]']) {
			expect(navOf(id)).toMatchObject({ look: 'home', current: 'artists' });
		}
		expect(journeyOf('/artists')).not.toBe(journeyOf('/artists/[id]'));
	});

	it('keeps Albums current on the list and on an album, dressed for each', () => {
		expect(navOf('/albums')).toMatchObject({ look: 'home', current: 'albums' });
		expect(navOf('/album/[id]')).toMatchObject({ look: 'album', current: 'albums' });
	});

	it('gives Albums no journey of Home’s: its list can change in the same navigation', () => {
		expect(journeyOf('/albums')).toBe('albums');
	});

	it('has no current item on Now playing', () => {
		expect(navOf('/now-playing')).toMatchObject({ look: 'gone', current: null });
	});

	it('has no current item on Search, which is no nav item', () => {
		expect(navOf('/search')).toMatchObject({ look: 'home', current: null });
		expect(journeyOf('/search')).toBe('search');
	});

	it('gives every route a journey name', () => {
		for (const id of routeIds) {
			expect(journeyOf(id)).toBeDefined();
		}
	});

	it('dresses an error page as Home, with nothing current and no journey', () => {
		expect(navOf(null)).toEqual({ look: 'home', current: null });
		expect(journeyOf(null)).toBeUndefined();
	});
});

describe('albumOn', () => {
	it("reads an album's route", () => {
		expect(albumOn({ route: { id: '/album/[id]' }, params: { id: 'a' } })).toBe('a');
	});

	it('leaves an ID alone on any other route', () => {
		expect(albumOn({ route: { id: '/artists/[id]' }, params: { id: 'a' } })).toBeUndefined();
	});

	it('copes with nowhere', () => {
		expect(albumOn(null)).toBeUndefined();
		expect(albumOn({ route: { id: null }, params: null })).toBeUndefined();
	});
});
