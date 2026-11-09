import type { Account, PasswordFailure, ResetFailure } from '#lib/library/source.ts';
import { minPasswordLength, type FormProblem } from './setup.ts';

/** How long a reset code lasts, as the server keeps it. */
export const resetCodeMs = 15 * 60_000;

/** Characters in a code, without its dashes: 80 bits of Crockford base32. */
const codeLength = 16;

/** Characters between a code's dashes. */
const codeGroup = 4;

/**
 * A code as the server reads it: case, dashes and spaces don't count, and Crockford's look-alikes
 * read as their digits, i and l as 1 and o as 0. A code never holds those letters.
 */
export function normalizeCode(code: string): string {
	return code.toLowerCase().replaceAll(/[\s-]/g, '').replaceAll(/[il]/g, '1').replaceAll('o', '0');
}

/**
 * A code as the field shows it while it's typed: lower case, in the log's groups of four. Anything
 * no code holds is dropped, and nothing past the sixteenth character is kept.
 */
export function formatCode(typed: string): string {
	const plain = normalizeCode(typed)
		.replaceAll(/[^0-9a-z]/g, '')
		.slice(0, codeLength);
	return (plain.match(new RegExp(`.{1,${String(codeGroup)}}`, 'g')) ?? []).join('-');
}

/** Whether a code is all there, so it's worth sending. */
export function codeComplete(code: string): boolean {
	return formatCode(code).replaceAll('-', '').length === codeLength;
}

/** Whether a code asked for at `askedAt` has run out by `now`, by this device's clock. */
export function ranOut(askedAt: number, now: number): boolean {
	return now - askedAt >= resetCodeMs;
}

/** The mono line beside the code: roughly how long it has left by this device's clock. */
export function timeLeftLine(askedAt: number, now: number): string {
	if (ranOut(askedAt, now)) {
		return 'RAN OUT';
	}
	const minutes = Math.ceil((resetCodeMs - (now - askedAt)) / 60_000);
	return minutes <= 1 ? 'UNDER A MINUTE LEFT' : `ABOUT ${String(minutes)} MIN LEFT`;
}

/** Anything that stops a reset, from the form, the server or this device's clock. */
export type ResetProblem = FormProblem | ResetFailure | { kind: 'expired' };

/**
 * What a refusal from the server means here. The server says a stale code and a wrong one alike,
 * so the page tells them apart by its own clock.
 */
export function resetRefusal(failure: ResetFailure, askedAt: number, now: number): ResetProblem {
	return failure.kind === 'code' && ranOut(askedAt, now) ? { kind: 'expired' } : failure;
}

function minutesOf(seconds: number): string {
	const minutes = Math.max(1, Math.ceil(seconds / 60));
	return `${String(minutes)} ${minutes === 1 ? 'minute' : 'minutes'}`;
}

/** The field a reset problem is about, which shows it and takes the focus. */
export function resetFieldOf(problem: ResetProblem | null): 'code' | 'password' | null {
	switch (problem?.kind) {
		case 'code':
		case 'expired':
			return 'code';
		case 'short':
		case 'mismatch':
		case 'password':
			return 'password';
		default:
			return null;
	}
}

/**
 * What the reset says about a problem. `asking` is true on the steps that ask for a code, where a
 * throttle is about asking; `waiting` is the seconds left of it.
 */
export function resetProblemLine(
	problem: ResetProblem | null,
	waiting: number,
	asking = false
): string {
	switch (problem?.kind) {
		case 'short':
			return `Use at least ${String(minPasswordLength)} characters.`;
		case 'mismatch':
			return 'The two passwords don’t match.';
		case 'password':
			return 'That password is too long.';
		case 'code':
			return 'That code isn’t right. Check it against the newest line with your name in the log.';
		case 'expired':
			return 'That code has run out: codes last 15 minutes. Ask for a new one and it’ll be in the log straight away.';
		case 'throttled':
			return asking
				? `You’ve asked a few times. Try again in ${minutesOf(waiting)}.`
				: `Too many tries. Try again in ${minutesOf(waiting)}.`;
		case 'unavailable':
			return "Couldn't reach Kitsune. Try again in a moment.";
		default:
			return '';
	}
}

/** Anything that stops a password change on the You page, or an administrator's set. */
export type PasswordProblem = FormProblem | PasswordFailure;

/** What the You page and the People card say about a problem. */
export function passwordProblemLine(problem: PasswordProblem | null, waiting: number): string {
	switch (problem?.kind) {
		case 'current':
			return 'That isn’t your current password.';
		case 'short':
			return `Use at least ${String(minPasswordLength)} characters.`;
		case 'mismatch':
			return 'The two passwords don’t match.';
		case 'password':
			return 'That password is too long.';
		case 'throttled':
			return `Too many tries. Try again in ${minutesOf(waiting)}.`;
		case 'unavailable':
			return "Couldn't reach Kitsune. Try again in a moment.";
		default:
			return '';
	}
}

/** The command that finds the code, for a server run in Docker as the README has it. */
export const findCodeCommand = 'docker logs kitsune 2>&1 \\\n  | grep resetCode';

/** "Here now", "2 hours ago", "Yesterday", "12 days ago", for an account's last session. */
export function lastSeenWords(lastSeen: string | undefined, now: number): string {
	if (lastSeen === undefined) {
		return 'Never';
	}
	const minutes = Math.floor((now - Date.parse(lastSeen)) / 60_000);
	if (minutes < 60) {
		// Sessions note their use about once an hour, so anything inside one is now.
		return 'Here now';
	}
	const hours = Math.floor(minutes / 60);
	if (hours < 24) {
		return `${String(hours)} ${hours === 1 ? 'hour' : 'hours'} ago`;
	}
	const days = Math.floor(hours / 24);
	return days === 1 ? 'Yesterday' : `${String(days)} days ago`;
}

/** The People card's last-seen cell: when, and on how many devices. */
export function lastSeenLine(account: Account, now: number): string {
	const devices =
		account.sessions === 0
			? 'no devices'
			: `${String(account.sessions)} ${account.sessions === 1 ? 'device' : 'devices'}`;
	return account.lastSeen === undefined && account.sessions === 0
		? 'Not signed in'
		: `${lastSeenWords(account.lastSeen, now)} · ${devices}`;
}

/** What setting someone's password does, in the popover. */
export function signsOutLine(account: Account): string {
	const where =
		account.sessions === 0
			? 'everywhere'
			: account.sessions === 1
				? 'everywhere, on their one device'
				: account.sessions === 2
					? 'everywhere, on both devices'
					: `everywhere, on all ${String(account.sessions)} devices`;
	return `This signs ${account.user.name} out ${where}. Kitsune doesn’t send it to them: tell them yourself.`;
}

/** The role as the People card words it. */
export function roleWords(role: Account['user']['role']): string {
	return role === 'admin' ? 'Administrator' : 'Listener';
}
