import { describe, expect, it } from 'vitest';
import { loginPath, safeNext } from './session';

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
		['/login?next=/album/a', '/']
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

	it('needs no return for Home or the door', () => {
		expect(loginPath({ pathname: '/', search: '' })).toBe('/login');
		expect(loginPath({ pathname: '/login', search: '?next=/a' })).toBe('/login');
	});
});
