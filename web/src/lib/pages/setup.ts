import type { SetupFailure } from '#lib/library/source.ts';

/** The shortest password the server takes, counted in characters as it counts them. */
export const minPasswordLength = 8;

/** What the form catches before it asks the server. */
export type FormProblem = { kind: 'short' } | { kind: 'mismatch' };

/** Anything that stops the first account being made, from the form or from the server. */
export type SetupProblem = FormProblem | SetupFailure;

/** The field a problem is about, which shows it and takes the focus. */
export type SetupField = 'name' | 'password' | 'code';

/**
 * Checks the two passwords before they're sent. Characters are counted as code points, as the
 * server does, so four emoji are four characters and not eight.
 */
export function checkPasswords(password: string, again: string): FormProblem | null {
	if ([...password].length < minPasswordLength) {
		return { kind: 'short' };
	}
	return password === again ? null : { kind: 'mismatch' };
}

/** Which field a problem is about, if one is. */
export function fieldOf(problem: SetupProblem | null): SetupField | null {
	switch (problem?.kind) {
		case 'short':
		case 'mismatch':
		case 'password':
			return 'password';
		case 'code':
			return 'code';
		case 'name':
			return 'name';
		default:
			return null;
	}
}

/** What the page says about a problem; `waiting` is the seconds left of a throttle. */
export function problemLine(problem: SetupProblem | null, waiting: number): string {
	switch (problem?.kind) {
		case 'short':
			return `Use at least ${String(minPasswordLength)} characters.`;
		case 'mismatch':
			return 'The two passwords don’t match.';
		case 'password':
			return 'That password is too long.';
		case 'name':
			return 'Use a name of 1 to 64 characters, with no spaces at either end.';
		case 'code':
			return 'That code isn’t right. Check the log again: it changes every time the server starts.';
		case 'throttled': {
			const minutes = Math.max(1, Math.ceil(waiting / 60));
			return `Too many tries. Try again in ${String(minutes)} ${minutes === 1 ? 'minute' : 'minutes'}.`;
		}
		case 'unavailable':
			return "Couldn't reach Kitsune. Try again in a moment.";
		default:
			return '';
	}
}

/** The mono line over the headline. */
export function setupLine(done: boolean): string {
	return done ? 'KITSUNE · FIRST RUN · DONE' : 'KITSUNE · FIRST RUN · NO ACCOUNTS YET';
}
