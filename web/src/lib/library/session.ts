import type { Session } from './library.svelte.ts';

/** The two doors: signing in, and making the first account. Nothing after signing in returns to them. */
const doors = new Set(['/login', '/setup']);

/**
 * Where to go after signing in. `next` comes from the URL, so anyone can write it: only a path on
 * this origin is followed, and never back to a door.
 */
export function safeNext(next: string | null, origin: string): string {
	if (next === null || !next.startsWith('/')) {
		return '/';
	}
	try {
		const url = new URL(next, origin);
		// The resolved path is what gets followed: `/.//evil.example` only becomes `//` once parsed.
		if (url.origin !== origin || url.pathname.startsWith('//') || next.includes('\\')) {
			return '/';
		}
		return doors.has(url.pathname) ? '/' : `${url.pathname}${url.search}`;
	} catch {
		return '/';
	}
}

/** The path that returns to `here` after signing in. */
export function loginPath({ pathname, search }: { pathname: string; search: string }): string {
	const here = `${pathname}${search}`;
	return here === '/' || doors.has(pathname) ? '/login' : `/login?next=${encodeURIComponent(here)}`;
}

/** Where the app opened, as the guard reads it. */
export interface Here {
	pathname: string;
	search: string;
	/** The `next` the URL carries, if any. */
	next: string | null;
}

/**
 * Where the guard sends the app, or null to stay. Signed out, a server with no accounts yet sends
 * every page to `/setup`, the sign-in page included, and otherwise everything but the door goes to
 * it. `/setup` itself never asks here: the page says whether it's still open. Signed in, both doors
 * lead on. A server that's down sends nowhere: the shell says so.
 */
export async function guardTarget(
	here: Here,
	session: Session,
	setupNeeded: () => Promise<boolean>,
	origin: string
): Promise<string | null> {
	const atSetup = here.pathname === '/setup';
	const atLogin = here.pathname === '/login';
	switch (session) {
		case 'signed-in':
			if (atSetup) {
				return '/';
			}
			return atLogin ? safeNext(here.next, origin) : null;
		case 'down':
			return null;
		case 'signed-out':
			if (atSetup) {
				return null;
			}
			if (await setupNeeded()) {
				return '/setup';
			}
			return atLogin ? null : loginPath(here);
	}
}
