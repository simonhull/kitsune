import { redirect } from '@sveltejs/kit';
import { library } from '#lib/library/library.svelte.ts';
import { loginPath, safeNext } from '#lib/library/session.ts';
import type { LayoutLoad } from './$types';

// A client-only app: the Go server serves static files and the API.
export const ssr = false;

/**
 * The guard. It runs once: `untrack` keeps the URL out of this load's dependencies, because Kit
 * calls `onNavigate` only after `load`, and every morph would otherwise wait for `/me`. It can't
 * live in `onMount` either, or the nav flashes before the redirect.
 */
export const load: LayoutLoad = async ({ url, untrack }) => {
	const here = untrack(() => ({
		pathname: url.pathname,
		search: url.search,
		next: url.searchParams.get('next')
	}));
	const session = await library.checkSession();
	const atTheDoor = here.pathname === '/login';
	if (session === 'signed-out' && !atTheDoor) {
		redirect(307, loginPath(here));
	}
	if (session === 'signed-in' && atTheDoor) {
		redirect(307, safeNext(here.next, url.origin));
	}
	if (session !== 'signed-out') {
		void library.loadShell();
	}
};
