import { describe, expect, it } from 'vitest';
import type { Account } from '#lib/library/source.ts';
import {
	codeComplete,
	formatCode,
	lastSeenLine,
	normalizeCode,
	passwordProblemLine,
	ranOut,
	resetCodeMs,
	resetFieldOf,
	resetProblemLine,
	resetRefusal,
	signsOutLine,
	timeLeftLine,
	type ResetProblem
} from './passwords';

const code = 'k7m2-q9xd-4tbn-r8ve';
const minute = 60_000;

describe('a code, as the server reads it', () => {
	it.each([
		['as logged', code],
		['in capitals', 'K7M2-Q9XD-4TBN-R8VE'],
		['without dashes', 'k7m2q9xd4tbnr8ve'],
		['with spaces', 'k7m2 q9xd 4tbn r8ve'],
		['spaced and dashed in odd places', ' k7-m2q9 xd4t-bnr8ve ']
	])('ignores how it was typed: %s', (_, typed) => {
		expect(normalizeCode(typed)).toBe('k7m2q9xd4tbnr8ve');
	});

	it('reads Crockford’s look-alikes as their digits', () => {
		expect(normalizeCode('IlOo')).toBe('1100');
	});
});

describe('the code field', () => {
	it.each([
		['k7m2q9xd4tbnr8ve', code],
		['K7M2 Q9XD 4TBN R8VE', code],
		['k7m2-q9', 'k7m2-q9'],
		['k7m2', 'k7m2'],
		['k7m2-', 'k7m2'],
		['', ''],
		['k7m2-q9xd-4tbn-r8ve-extra', code],
		['k7m2!q9xd', 'k7m2-q9xd'],
		['Ol', '01']
	])('shows %j as %j', (typed, shown) => {
		expect(formatCode(typed)).toBe(shown);
	});

	it('is complete at sixteen characters, however they were typed', () => {
		expect(codeComplete('K7M2Q9XD4TBNR8VE')).toBe(true);
		expect(codeComplete('k7m2-q9xd-4tbn-r8v')).toBe(false);
	});
});

describe('the countdown', () => {
	const asked = 1_000_000;

	it.each([
		[0, 'ABOUT 15 MIN LEFT'],
		[3 * minute, 'ABOUT 12 MIN LEFT'],
		[3 * minute + 1, 'ABOUT 12 MIN LEFT'],
		[13 * minute + 1, 'ABOUT 2 MIN LEFT'],
		[14 * minute, 'UNDER A MINUTE LEFT'],
		[resetCodeMs - 1, 'UNDER A MINUTE LEFT'],
		[resetCodeMs, 'RAN OUT'],
		[2 * resetCodeMs, 'RAN OUT']
	])('after %d ms says %s', (elapsed, line) => {
		expect(timeLeftLine(asked, asked + elapsed)).toBe(line);
	});

	it('runs out at fifteen minutes by this device’s clock', () => {
		expect(ranOut(asked, asked + resetCodeMs - 1)).toBe(false);
		expect(ranOut(asked, asked + resetCodeMs)).toBe(true);
	});
});

describe('a refused code', () => {
	const asked = 0;

	it('is wrong while the clock says it should still work', () => {
		expect(resetRefusal({ kind: 'code' }, asked, 14 * minute)).toEqual({ kind: 'code' });
	});

	it('has run out once the clock says so, since the server can’t tell the two apart', () => {
		expect(resetRefusal({ kind: 'code' }, asked, resetCodeMs)).toEqual({ kind: 'expired' });
	});

	it('leaves anything else as it was', () => {
		const throttled = { kind: 'throttled', retryAfterSeconds: 30 } as const;
		expect(resetRefusal(throttled, asked, resetCodeMs)).toBe(throttled);
	});
});

describe('what the reset says', () => {
	it.each<[ResetProblem, string, ReturnType<typeof resetFieldOf>]>([
		[
			{ kind: 'code' },
			'That code isn’t right. Check it against the newest line with your name in the log.',
			'code'
		],
		[
			{ kind: 'expired' },
			'That code has run out: codes last 15 minutes. Ask for a new one and it’ll be in the log straight away.',
			'code'
		],
		[{ kind: 'mismatch' }, 'The two passwords don’t match.', 'password'],
		[{ kind: 'short' }, 'Use at least 8 characters.', 'password'],
		[{ kind: 'password' }, 'That password is too long.', 'password'],
		[{ kind: 'unavailable' }, "Couldn't reach Kitsune. Try again in a moment.", null]
	])('%j', (problem, line, field) => {
		expect(resetProblemLine(problem, 0)).toBe(line);
		expect(resetFieldOf(problem)).toBe(field);
	});

	it('counts a throttle down in minutes, worded for trying or for asking', () => {
		const throttled = { kind: 'throttled', retryAfterSeconds: 240 } as const;
		expect(resetProblemLine(throttled, 240)).toBe('Too many tries. Try again in 4 minutes.');
		expect(resetProblemLine(throttled, 61, true)).toBe(
			'You’ve asked a few times. Try again in 2 minutes.'
		);
		expect(resetProblemLine(throttled, 5)).toBe('Too many tries. Try again in 1 minute.');
	});

	it('says nothing without a problem', () => {
		expect(resetProblemLine(null, 0)).toBe('');
	});
});

describe('what the You page says', () => {
	it('names a wrong current password', () => {
		expect(passwordProblemLine({ kind: 'current' }, 0)).toBe('That isn’t your current password.');
	});
});

describe('the People card', () => {
	const now = Date.parse('2026-11-08T12:00:00Z');
	const account = (lastSeen: string | undefined, sessions: number): Account => ({
		user: { id: 2, name: 'anna', role: 'user', createdAt: '2026-01-01T00:00:00Z' },
		...(lastSeen === undefined ? {} : { lastSeen }),
		sessions
	});

	it.each([
		['2026-11-08T11:40:00Z', 3, 'Here now · 3 devices'],
		['2026-11-08T10:00:00Z', 2, '2 hours ago · 2 devices'],
		['2026-11-07T10:00:00Z', 1, 'Yesterday · 1 device'],
		['2026-10-27T10:00:00Z', 1, '12 days ago · 1 device'],
		[undefined, 0, 'Not signed in']
	])('words %s with %d sessions', (lastSeen, sessions, line) => {
		expect(lastSeenLine(account(lastSeen, sessions), now)).toBe(line);
	});

	it('says how far setting a password reaches', () => {
		expect(signsOutLine(account(undefined, 2))).toBe(
			'This signs anna out everywhere, on both devices. Kitsune doesn’t send it to them: tell them yourself.'
		);
	});
});
