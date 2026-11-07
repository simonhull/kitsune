import { describe, expect, it } from 'vitest';
import { guardTarget, loginPath, safeNext } from './session';

const origin = 'https://music.example';

describe('safeNext', () => {
	it.each([
		['/album/01ABC', '/album/01ABC'],
		['/now-playing?x=1', '/now-playing?x=1'],
		[null, '/'],
		['', '/'],
		['album/01ABC', '/'],
		['https://evil.example/', '/'],
		['//evil.example/', '/'],
		['/\\evil.example/', '/'],
		['/.//evil.example/', '/'],
		['/a/..//evil.example/', '/'],
		['/login', '/'],
		['/login?next=/album/a', '/'],
		['/setup', '/']
	])('reads %s as %s', (next, want) => {
		expect(safeNext(next, origin)).toBe(want);
	});
});

describe('loginPath', () => {
	it('remembers where the user was', () => {
		expect(loginPath({ pathname: '/album/a', search: '?x=1' })).toBe(
			'/login?next=%2Falbum%2Fa%3Fx%3D1'
		);
	});

	it('needs no return for Home or a door', () => {
		expect(loginPath({ pathname: '/', search: '' })).toBe('/login');
		expect(loginPath({ pathname: '/login', search: '?next=/a' })).toBe('/login');
		expect(loginPath({ pathname: '/setup', search: '' })).toBe('/login');
	});
});

describe('the guard', () => {
	const at = (pathname: string, search = '', next: string | null = null) => ({
		pathname,
		search,
		next
	});
	const needed = () => Promise.resolve(true);
	const notNeeded = () => Promise.resolve(false);
	const never = () => Promise.reject(new Error('the guard must not ask'));

	it.each(['/', '/album/a', '/login', '/you'])(
		'sends %s to setup while there are no accounts',
		async (page) => {
			await expect(guardTarget(at(page), 'signed-out', needed, origin)).resolves.toBe('/setup');
		}
	);

	it('leaves setup alone, signed out: the page says whether it is open', async () => {
		await expect(guardTarget(at('/setup'), 'signed-out', never, origin)).resolves.toBeNull();
	});

	it('goes to the door once set up, remembering where it was', async () => {
		await expect(guardTarget(at('/album/a'), 'signed-out', notNeeded, origin)).resolves.toBe(
			'/login?next=%2Falbum%2Fa'
		);
		await expect(guardTarget(at('/login'), 'signed-out', notNeeded, origin)).resolves.toBeNull();
	});

	it('never asks about setup when signed in, and leads on from both doors', async () => {
		await expect(guardTarget(at('/setup'), 'signed-in', never, origin)).resolves.toBe('/');
		await expect(
			guardTarget(at('/login', '?next=/album/a', '/album/a'), 'signed-in', never, origin)
		).resolves.toBe('/album/a');
		await expect(guardTarget(at('/album/a'), 'signed-in', never, origin)).resolves.toBeNull();
	});

	it('sends nowhere while the server is down', async () => {
		await expect(guardTarget(at('/'), 'down', never, origin)).resolves.toBeNull();
	});
});
