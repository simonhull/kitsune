import { describe, expect, it } from 'vitest';
import { createFixtureSource, fixtureSetupCode } from '#lib/library/fixtures.ts';
import { SetupError, UnauthorizedError } from '#lib/library/source.ts';
import { checkPasswords, fieldOf, problemLine, setupLine, type SetupProblem } from './setup';

describe('the form’s own checks', () => {
	it.each([
		['short', 'short', { kind: 'short' }],
		['seven characters', '1234567', { kind: 'short' }],
		['four emoji, which are eight UTF-16 units', '🦊🦊🦊🦊', { kind: 'short' }],
		['eight characters that differ', '12345678', { kind: 'mismatch' }]
	])('refuses %s', (_, password, want) => {
		expect(checkPasswords(password, 'something else')).toEqual(want);
	});

	it('passes two that match and are long enough', () => {
		expect(checkPasswords('correct horse', 'correct horse')).toBeNull();
		expect(checkPasswords('🦊🦊🦊🦊🦊🦊🦊🦊', '🦊🦊🦊🦊🦊🦊🦊🦊')).toBeNull();
	});
});

describe('what each failure says', () => {
	it.each<[SetupProblem, string, ReturnType<typeof fieldOf>]>([
		[{ kind: 'short' }, 'Use at least 8 characters.', 'password'],
		[{ kind: 'mismatch' }, 'The two passwords don’t match.', 'password'],
		[{ kind: 'password' }, 'That password is too long.', 'password'],
		[{ kind: 'name' }, 'Use a name of 1 to 64 characters, with no spaces at either end.', 'name'],
		[
			{ kind: 'code' },
			'That code isn’t right. Check the log again: it changes every time the server starts.',
			'code'
		],
		[{ kind: 'throttled', retryAfterSeconds: 60 }, 'Too many tries. Try again in 1 minute.', null],
		[{ kind: 'unavailable' }, "Couldn't reach Kitsune. Try again in a moment.", null],
		[{ kind: 'done' }, '', null]
	])('%o', (problem, line, field) => {
		expect(problemLine(problem, 60)).toBe(line);
		expect(fieldOf(problem)).toBe(field);
	});

	it('counts a long wait in minutes, rounding up', () => {
		expect(problemLine({ kind: 'throttled', retryAfterSeconds: 300 }, 61)).toBe(
			'Too many tries. Try again in 2 minutes.'
		);
	});

	it('says nothing with nothing wrong', () => {
		expect(problemLine(null, 0)).toBe('');
		expect(fieldOf(null)).toBeNull();
	});

	it('heads the page as board B does, and once set up', () => {
		expect(setupLine(false)).toBe('KITSUNE · FIRST RUN · NO ACCOUNTS YET');
		expect(setupLine(true)).toBe('KITSUNE · FIRST RUN · DONE');
	});
});

describe('the fixtures’ first run', () => {
	async function failureOf(promise: Promise<unknown>): Promise<unknown> {
		const error = await promise.catch((caught: unknown) => caught);
		expect(error).toBeInstanceOf(SetupError);
		return (error as SetupError).failure;
	}

	it('starts signed out, needing the code, and makes one account', async () => {
		const source = createFixtureSource({ setup: 'code' });
		await expect(source.me()).rejects.toBeInstanceOf(UnauthorizedError);
		await expect(source.setupStatus()).resolves.toEqual({ needed: true, codeRequired: true });
		expect(await failureOf(source.setup('simon', 'correct horse'))).toEqual({ kind: 'code' });
		expect(await failureOf(source.setup('simon', 'correct horse', 'nope'))).toEqual({
			kind: 'code'
		});
		await source.setup('simon', 'correct horse', fixtureSetupCode.toUpperCase());
		await expect(source.me()).resolves.toMatchObject({ role: 'admin' });
		await expect(source.setupStatus()).resolves.toEqual({ needed: false, codeRequired: false });
		expect(await failureOf(source.setup('ada', 'correct horse'))).toEqual({ kind: 'done' });
	});

	it('waits after six wrong codes, as the server does', async () => {
		const source = createFixtureSource({ setup: 'code' });
		for (let i = 0; i < 6; i++) {
			expect(await failureOf(source.setup('simon', 'correct horse', 'nope'))).toEqual({
				kind: 'code'
			});
		}
		expect(await failureOf(source.setup('simon', 'correct horse', fixtureSetupCode))).toEqual({
			kind: 'throttled',
			retryAfterSeconds: 60
		});
	});

	it('needs no code from home, and checks the name and password', async () => {
		const source = createFixtureSource({ setup: 'needed' });
		await expect(source.setupStatus()).resolves.toEqual({ needed: true, codeRequired: false });
		expect(await failureOf(source.setup(' simon', 'correct horse'))).toEqual({ kind: 'name' });
		expect(await failureOf(source.setup('simon', 'x'.repeat(1025)))).toEqual({
			kind: 'password'
		});
		await expect(source.setup('simon', 'correct horse')).resolves.toMatchObject({
			name: expect.any(String) as string
		});
	});

	it('is closed when done, and still signed out', async () => {
		const source = createFixtureSource({ setup: 'done' });
		await expect(source.setupStatus()).resolves.toEqual({ needed: false, codeRequired: false });
		await expect(source.me()).rejects.toBeInstanceOf(UnauthorizedError);
	});
});
