import { describe, expect, it, vi } from 'vitest';
import { artworkUrl, createApiSource } from './api';
import {
	AlbumNotFoundError,
	ArtistNotFoundError,
	EditError,
	LoginError,
	ServerError,
	UnauthorizedError,
	type EditFailure
} from './source';

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

	it('reports a play, with the CSRF header, and takes a 204 for an answer', async () => {
		const { send, source } = fake(new Response(null, { status: 204 }));
		const play = { trackId: 't', playedAt: 'p', source: 'web', clientId: 'c' };
		await expect(source.reportPlay(play)).resolves.toBeUndefined();
		expect(send.mock.calls.at(0)?.at(0)).toBe('/api/v1/me/plays');
		expect(init(send).method).toBe('POST');
		expect(init(send).headers).toMatchObject({
			'Content-Type': 'application/json',
			'X-Kitsune-Request': '1'
		});
		expect(JSON.parse(String(init(send).body))).toEqual(play);
	});

	it('keeps the door out of a background report and a background look at the session', async () => {
		const { onUnauthorized, source } = fake(json(401), json(401));
		const play = { trackId: 't', playedAt: 'p', source: 'web', clientId: 'c' };
		await expect(source.reportPlay(play)).rejects.toBeInstanceOf(UnauthorizedError);
		await expect(source.sessionUser()).rejects.toBeInstanceOf(UnauthorizedError);
		expect(onUnauthorized).not.toHaveBeenCalled();
	});

	it('pages by cursor and sort', async () => {
		const { send, source } = fake(json(200, { albums: [] }));
		await source.albumsPage('name', 'abc', 6);
		expect(send.mock.calls.at(0)?.at(0)).toBe('/api/v1/albums?sort=name&cursor=abc&limit=6');
	});

	it('starts a scan with the CSRF header, and hands back where it stands', async () => {
		const { send, source } = fake(json(202, { state: 'running', read: 0, total: 0 }));
		await expect(source.startScan('full')).resolves.toMatchObject({ state: 'running' });
		expect(send.mock.calls.at(0)?.at(0)).toBe('/api/v1/admin/library/scan');
		expect(init(send).method).toBe('POST');
		expect(init(send).body).toBe('{"mode":"full"}');
		expect(init(send).headers).toHaveProperty('X-Kitsune-Request', '1');
	});

	it('refuses a second scan with its status', async () => {
		const { source } = fake(json(409));
		await expect(source.startScan('quick')).rejects.toMatchObject({ status: 409 });
	});

	it('lists the issues', async () => {
		const { send, source } = fake(json(200, { issues: [], warnings: [] }));
		await expect(source.issues()).resolves.toEqual({ issues: [], warnings: [] });
		expect(send.mock.calls.at(0)?.at(0)).toBe('/api/v1/admin/library/issues');
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

	it('searches with the query encoded, and carries the signal', async () => {
		const { send, source } = fake(json(200, { artists: [], albums: [], tracks: [] }));
		const { signal } = new AbortController();
		await source.search('a&b c', signal);
		expect(send.mock.calls.at(0)?.[0]).toBe('/api/v1/search?q=a%26b+c');
		expect(init(send).signal).toBe(signal);
	});

	it('lets an aborted search reject as an abort', async () => {
		const { source } = fake(new DOMException('aborted', 'AbortError'));
		await expect(source.search('fox')).rejects.toMatchObject({ name: 'AbortError' });
	});

	it('reports a failed search', async () => {
		const { source } = fake(json(500));
		await expect(source.search('fox')).rejects.toBeInstanceOf(ServerError);
	});

	it('unwraps the artists', async () => {
		const artists = [{ id: 'a', name: 'A', albumCount: 1 }];
		const { source } = fake(json(200, { artists }));
		await expect(source.artists()).resolves.toEqual(artists);
	});

	it('tells an artist that does not exist from a server that failed', async () => {
		const { source } = fake(json(404), json(500));
		await expect(source.artist('a/b')).rejects.toBeInstanceOf(ArtistNotFoundError);
		await expect(source.artist('a')).rejects.toBeInstanceOf(ServerError);
	});
});

describe('edits', () => {
	const stopped = { detail: 'it is read-only', changed: ['t1'], remaining: ['t2'] };

	async function failureOf(answer: Response | Error): Promise<EditFailure> {
		const { source } = fake(answer);
		const error: unknown = await source.editAlbum('a', 'tag', {}).catch((e) => e);
		if (!(error instanceof EditError)) {
			throw new Error('not an edit error');
		}
		return error.failure;
	}

	it('sends one edit, with its ETag verbatim, past the CSRF guard', async () => {
		const { send, source } = fake(json(200, { id: 'a', etag: 'next' }));
		const edit = { all: { clear: ['genres' as const] } };
		await expect(source.editAlbum('a/1', '"tag"', edit)).resolves.toMatchObject({ etag: 'next' });
		expect(send.mock.calls.at(0)?.at(0)).toBe('/api/v1/albums/a%2F1/edits');
		expect(init(send).method).toBe('POST');
		expect(init(send).body).toBe('{"all":{"clear":["genres"]}}');
		expect(init(send).headers).toMatchObject({
			'Content-Type': 'application/json',
			'If-Match': '"tag"',
			'X-Kitsune-Request': '1'
		});
	});

	it('reads an album’s tags', async () => {
		const { send, source } = fake(json(200, { etag: 'tag', tracks: [] }));
		await expect(source.albumTags('a/1')).resolves.toEqual({ etag: 'tag', tracks: [] });
		expect(send.mock.calls.at(0)?.at(0)).toBe('/api/v1/albums/a%2F1/tags');
	});

	it.each([
		[403, 'forbidden'],
		[404, 'gone'],
		[409, 'missing'],
		[500, 'unavailable']
	])('reads a %i from the tags as %s', async (status, kind) => {
		const { source } = fake(json(status));
		await expect(source.albumTags('a')).rejects.toMatchObject({ failure: { kind } });
	});

	it('hands a 401 from the tags to the door', async () => {
		const { source, onUnauthorized } = fake(json(401));
		await expect(source.albumTags('a')).rejects.toBeInstanceOf(UnauthorizedError);
		expect(onUnauthorized).toHaveBeenCalledOnce();
	});

	it('hands a 401 to the door', async () => {
		const { source, onUnauthorized } = fake(json(401));
		await expect(source.editAlbum('a', 'tag', {})).rejects.toBeInstanceOf(UnauthorizedError);
		expect(onUnauthorized).toHaveBeenCalledOnce();
	});

	it('reads when to come back from a 409, or picks a time', async () => {
		await expect(failureOf(json(409, {}, { 'Retry-After': '30' }))).resolves.toEqual({
			kind: 'busy',
			retryAfterSeconds: 30
		});
		await expect(failureOf(json(409))).resolves.toEqual({ kind: 'busy', retryAfterSeconds: 60 });
	});

	it.each([
		[404, 'gone'],
		[412, 'stale'],
		[403, 'forbidden'],
		[400, 'unavailable'],
		[428, 'unavailable'],
		[502, 'unavailable']
	])('reads a plain %i as %s', async (status, kind) => {
		await expect(failureOf(json(status))).resolves.toEqual({ kind });
	});

	it.each([
		[403, 'unwritable'],
		[412, 'stale'],
		[422, 'refused'],
		[500, 'failed']
	])('reads how far an edit got from a %i', async (status, kind) => {
		await expect(failureOf(json(status, stopped))).resolves.toEqual({ kind, ...stopped });
	});

	it('survives a body that is not JSON', async () => {
		await expect(failureOf(new Response('oops', { status: 500 }))).resolves.toEqual({
			kind: 'unavailable'
		});
	});

	it('reads no answer as unknown', async () => {
		await expect(failureOf(new TypeError('offline'))).resolves.toEqual({ kind: 'unavailable' });
	});
});
