import { describe, expect, it, vi } from 'vitest';
import { createApiSource } from './api';
import {
	PasswordError,
	ResetError,
	UnauthorizedError,
	type PasswordFailure,
	type ResetFailure
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

function sent(send: ReturnType<typeof fake>['send']): { path: unknown; body: unknown } {
	const [path, init] = send.mock.calls.at(0) ?? [];
	return { path, body: JSON.parse(String(init?.body)) };
}

describe('a reset', () => {
	it('asks for a code by name, and takes the 202 whoever it was', async () => {
		const { send, source } = fake(new Response(null, { status: 202 }));
		await expect(source.requestReset('simon')).resolves.toBeUndefined();
		expect(sent(send)).toEqual({ path: '/api/v1/auth/reset', body: { name: 'simon' } });
	});

	it('confirms with the code and the new password, asking for a cookie', async () => {
		const { send, source } = fake(json(200, { user: { name: 'simon' }, expiresAt: 'x' }));
		await expect(source.confirmReset('simon', 'k7m2', 'battery staple')).resolves.toEqual({
			name: 'simon'
		});
		expect(sent(send)).toEqual({
			path: '/api/v1/auth/reset/confirm',
			body: { name: 'simon', code: 'k7m2', new: 'battery staple', cookie: true }
		});
	});

	it.each<[string, Response | Error, ResetFailure]>([
		['a 403, wrong or stale alike', json(403), { kind: 'code' }],
		[
			'a 429',
			json(429, {}, { 'Retry-After': '240' }),
			{ kind: 'throttled', retryAfterSeconds: 240 }
		],
		[
			'a 400 about the password',
			json(400, { detail: 'password too long: …' }),
			{ kind: 'password' }
		],
		['a 400 about the body', json(400, { detail: 'request body …' }), { kind: 'unavailable' }],
		['a 500', json(500), { kind: 'unavailable' }],
		['no answer', new Error('offline'), { kind: 'unavailable' }]
	])('turns %s into a reset failure, without the door', async (_, answer, failure) => {
		const { onUnauthorized, source } = fake(answer);
		const error = await source.confirmReset('simon', 'c', 'p').catch((caught: unknown) => caught);
		expect(error).toBeInstanceOf(ResetError);
		expect((error as ResetError).failure).toEqual(failure);
		expect(onUnauthorized).not.toHaveBeenCalled();
	});
});

describe('a password change', () => {
	it('sends the current and the new password', async () => {
		const { send, source } = fake(new Response(null, { status: 204 }));
		await source.changePassword('old one', 'new one!');
		expect(sent(send)).toEqual({
			path: '/api/v1/me/password',
			body: { current: 'old one', new: 'new one!' }
		});
	});

	it.each<[string, Response, PasswordFailure]>([
		['a 403', json(403), { kind: 'current' }],
		['a 429', json(429, {}, { 'Retry-After': '30' }), { kind: 'throttled', retryAfterSeconds: 30 }],
		['a 400', json(400, { detail: 'password too short: …' }), { kind: 'password' }]
	])('turns %s into a failure', async (_, answer, failure) => {
		const { source } = fake(answer);
		const error = await source.changePassword('a', 'b').catch((caught: unknown) => caught);
		expect((error as PasswordError).failure).toEqual(failure);
	});

	it('goes to the door on a 401', async () => {
		const { onUnauthorized, source } = fake(json(401));
		await expect(source.changePassword('a', 'b')).rejects.toBeInstanceOf(UnauthorizedError);
		expect(onUnauthorized).toHaveBeenCalledOnce();
	});
});

describe('an administrator', () => {
	it('lists the accounts', async () => {
		const users = [{ user: { id: 1, name: 'simon' }, sessions: 2, lastSeen: 'x' }];
		const { send, source } = fake(json(200, { users }));
		await expect(source.accounts()).resolves.toEqual(users);
		expect(send.mock.calls.at(0)?.at(0)).toBe('/api/v1/admin/users');
	});

	it('sets someone’s password, and never calls a 403 a wrong current password', async () => {
		const { send, source } = fake(new Response(null, { status: 204 }), json(403));
		await source.setPassword(7, 'battery staple');
		expect(sent(send)).toEqual({
			path: '/api/v1/admin/users/7/password',
			body: { new: 'battery staple' }
		});
		const error = await source.setPassword(1, 'x').catch((caught: unknown) => caught);
		expect((error as PasswordError).failure).toEqual({ kind: 'unavailable' });
	});
});
