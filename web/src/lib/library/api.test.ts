import { describe, expect, it, vi } from 'vitest';
import { artworkUrl, createApiSource } from './api';
import { AlbumNotFoundError, LoginError, ServerError, UnauthorizedError } from './source';

function json(status: number, body: unknown = {}, headers: HeadersInit = {}): Response {
	return new Response(JSON.stringify(body), { status, headers });
}

function fake(...answers: (Response | Error)[]) {
	const queue = [...answers];
	const send = vi.fn<typeof fetch>(() => {
		const answer = queue.shift() ?? new Error('no answer queued');
		return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer);
	});
	const onUnauthorized = vi.fn<() => void>();
	return { send, onUnauthorized, source: createApiSource({ fetch: send, onUnauthorized }) };
}

function init(send: ReturnType<typeof fake>['send']): RequestInit {
	const call = send.mock.calls.at(0);
	if (call === undefined) {
		throw new Error('never called');
	}
	return call[1] ?? {};
}

describe('ApiSource', () => {
	it('reads through /api/v1 on the cookie, with no CSRF header on a safe method', async () => {
		const { send, source } = fake(json(200, { name: 'simon' }));
		await expect(source.me()).resolves.toEqual({ name: 'simon' });
		expect(send.mock.calls.at(0)?.at(0)).toBe('/api/v1/me');
		expect(init(send).credentials).toBe('same-origin');
		expect(init(send).headers).not.toHaveProperty('X-Kitsune-Request');
	});

	it('pages by cursor and sort', async () => {
		const { send, source } = fake(json(200, { albums: [] }));
		await source.albumsPage('name', 'abc', 6);
		expect(send.mock.calls.at(0)?.at(0)).toBe('/api/v1/albums?sort=name&cursor=abc&limit=6');
	});

	it('sends the app to the door on a 401, from any call', async () => {
		const { onUnauthorized, source } = fake(json(401));
		await expect(source.facts()).rejects.toBeInstanceOf(UnauthorizedError);
		expect(onUnauthorized).toHaveBeenCalledOnce();
	});

	it('tells a missing album from a server failure', async () => {
		const { source } = fake(json(404), json(503));
		await expect(source.album('a')).rejects.toBeInstanceOf(AlbumNotFoundError);
		await expect(source.album('a')).rejects.toBeInstanceOf(ServerError);
	});

	it('lets a network failure through for the caller to tell from a 404', async () => {
		const { source } = fake(new TypeError('offline'));
		await expect(source.album('a')).rejects.toBeInstanceOf(TypeError);
	});

	it('logs in for a cookie, with the CSRF header', async () => {
		const { send, source } = fake(json(200, { user: { name: 'simon' } }));
		await expect(source.login('simon', 'pw')).resolves.toEqual({ name: 'simon' });
		const { method, body, headers } = init(send);
		expect(method).toBe('POST');
		expect(JSON.parse(String(body))).toEqual({ name: 'simon', password: 'pw', cookie: true });
		expect(headers).toMatchObject({ 'X-Kitsune-Request': '1' });
	});

	it.each([
		['invalid credentials', json(401), { kind: 'credentials' }],
		[
			'a rate limit',
			json(429, {}, { 'Retry-After': '120' }),
			{ kind: 'throttled', retryAfterSeconds: 120 }
		],
		['a rate limit with no time', json(429), { kind: 'throttled', retryAfterSeconds: 60 }],
		['a bad request', json(400), { kind: 'unavailable' }],
		['a server error', json(500), { kind: 'unavailable' }],
		['no answer', new TypeError('offline'), { kind: 'unavailable' }]
	])('turns %s into a login failure, without leaving for the door', async (_, answer, failure) => {
		const { onUnauthorized, source } = fake(answer);
		const error = await source.login('simon', 'pw').catch((caught: unknown) => caught);
		expect(error).toBeInstanceOf(LoginError);
		expect((error as LoginError).failure).toEqual(failure);
		expect(onUnauthorized).not.toHaveBeenCalled();
	});

	it('reads readiness from the status alone', async () => {
		const { source } = fake(new Response(null, { status: 204 }), json(503));
		await expect(source.ready()).resolves.toBe(true);
		await expect(source.ready()).resolves.toBe(false);
	});

	it('names a rendition', () => {
		expect(artworkUrl('ab', 512)).toBe('/api/v1/artwork/ab?size=512');
	});
});
