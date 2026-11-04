/**
 * Where to go after signing in. `next` comes from the URL, so anyone can write it: only a path on
 * this origin is followed, and never back to the door itself.
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
		return url.pathname === '/login' ? '/' : `${url.pathname}${url.search}`;
	} catch {
		return '/';
	}
}

/** The path that returns to `here` after signing in. */
export function loginPath({ pathname, search }: { pathname: string; search: string }): string {
	const here = `${pathname}${search}`;
	return here === '/' || pathname === '/login'
		? '/login'
		: `/login?next=${encodeURIComponent(here)}`;
}
