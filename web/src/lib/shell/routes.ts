import type { RouteId } from '$app/types';
import type { RouteName } from '#lib/morph/choreography.ts';

/** How the nav dresses: `door` is Home's layout, unseen, so the fox has its seat before anything shows. */
export type NavLook = 'home' | 'album' | 'gone' | 'door';

/** A place in the nav a route can belong to. */
export type NavItem = 'home' | 'albums' | 'artists';

/** Three things about a route that only look like one: they change on their own. */
export interface RouteFacts {
	/** The nav's `data-route`. */
	look: NavLook;
	/** The nav item the pill sits under. With none, the pill goes away. */
	current: NavItem | null;
	/** Who this route is to the morphs. A pair with no journey blends. */
	journey: RouteName;
}

/** Every route. `RouteId` makes leaving one out a type error, because a route missing here cuts. */
const table = {
	'/': { look: 'home', current: 'home', journey: 'home' },
	'/album/[id]': { look: 'album', current: 'albums', journey: 'album' },
	'/login': { look: 'door', current: 'home', journey: 'login' },
	'/now-playing': { look: 'gone', current: null, journey: 'now-playing' }
} as const satisfies Record<RouteId, RouteFacts>;

const routes = new Map<string, RouteFacts>(Object.entries(table));

/** Every route, for loading their code ahead of the first click. */
export const routeIds = Object.keys(table) as RouteId[];

/** Finds a route's facts. An error page has none: it's dressed as Home, with nothing current. */
export function navOf(id: string | null | undefined): Pick<RouteFacts, 'look' | 'current'> {
	return routes.get(id ?? '') ?? { look: 'home', current: null };
}

/** Finds who a route is to the morphs. An error page is nobody, and arrives with no journey. */
export function journeyOf(id: string | null | undefined): RouteName | undefined {
	return routes.get(id ?? '')?.journey;
}

/** The part of a page or navigation target that says which album it's on. */
export interface RouteTarget {
	route: { id: string | null };
	params: Record<string, string | undefined> | null;
}

/** Finds the album a page is on: `params.id` only means that on an album's own route. */
export function albumOn(target: RouteTarget | null | undefined): string | undefined {
	return target?.route.id === '/album/[id]' ? target.params?.['id'] : undefined;
}
