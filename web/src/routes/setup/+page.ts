import { library } from '#lib/library/library.svelte.ts';
import type { PageLoad } from './$types';

/** Asked before the page shows, so it never draws a form it's about to close. */
export const load: PageLoad = async () => ({ status: await library.setupStatus() });
