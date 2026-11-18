import type { OnNavigate } from '$app/navigation';
import {
	blend,
	choreographyFor,
	parseTime,
	resolveMotion,
	type Choreography,
	type Lead,
	type LeadOf,
	type Motion,
	type MotionSpec,
	type MotionToken,
	type RouteName
} from './choreography.ts';
import { preload } from '#lib/cover/rungs.ts';
import { library } from '#lib/library/library.svelte.ts';
import { markClick } from './hud.svelte.ts';
import { pick, recordOrigins, type Origins } from './pairs.ts';
import { albumExit } from './path.ts';
import { registry, type MorphEntry } from './registry.ts';
import { albumOn, artistOn, journeyOf, playlistOn, type RouteTarget } from '#lib/shell/routes.ts';

/** One navigation's plan. */
export interface Journey {
	choreography: Choreography;
	/** The shared ids of this navigation; empty when nothing travels. */
	ids: string[];
	sources: Map<string, MorphEntry>;
	/** How long each id waits before it leaves, in milliseconds, when it waits at all. */
	waits: Map<string, number>;
	/** Where a room unfolds from: the nav item, field, fox or link pressed. Null on other journeys. */
	origin: Origin | null;
}

/** A place on screen a room comes out of, in viewport coordinates, measured as the press lands. */
export interface Origin {
	left: number;
	top: number;
	width: number;
	height: number;
}

/**
 * Where each journey left from, by the page it left: the way back to that page lands on the same
 * card. Only a few pages are remembered, since only the last few can be gone back to quickly.
 */
const originsByPage = new Map<string, Origins>();
const pagesRemembered = 16;

function originsOf(path: string | undefined): Origins | undefined {
	return path === undefined ? undefined : originsByPage.get(path);
}

function rememberOrigins(path: string): Origins {
	const known = originsByPage.get(path) ?? new Map();
	originsByPage.delete(path);
	originsByPage.set(path, known);
	for (const old of originsByPage.keys()) {
		if (originsByPage.size <= pagesRemembered) {
			break;
		}
		originsByPage.delete(old);
	}
	return known;
}

let lastClick: EventTarget | null = null;
let homeScroll = 0;
let visualTransition = false;
let navigations = 0;
/** The page behind Now playing in the history stack, when that's known. */
let behindNowPlaying: string | null = null;
let landAtTop = false;
/** One navigation's borrowed lead: an id with no source of its own, and what stands in for it. */
let lent: { id: string; from: string } | null = null;
/** Where a borrowed lead left from, by page: the way back lands it on the same stand-in. */
const lentFrom = new Map<string, { id: string; from: string }>();

/**
 * Lends a lead to the next navigation: `id` leaves from wherever `from` is. An album card's menu
 * goes to its artist this way, from the card's own artist line, which only knows the album.
 */
export function lend(id: string, from: string): void {
	lent = { id, from };
}

/** Reports whether there's a page of ours to go back to. */
export function hasInAppHistory(): boolean {
	return navigations > 0;
}

/** Listens for what the journey needs to know before Kit does. Returns the cleanup. */
export function watchInput(): () => void {
	function onClick(event: MouseEvent): void {
		lastClick = event.target;
		markClick(performance.now());
	}
	function onPointerDown({ target }: PointerEvent): void {
		preloadFor(target instanceof Element ? target.closest('a') : null);
	}
	function onPopState(event: PopStateEvent): void {
		visualTransition = event.hasUAVisualTransition;
		markClick(performance.now());
	}
	document.addEventListener('click', onClick, { capture: true });
	document.addEventListener('pointerdown', onPointerDown, { capture: true, passive: true });
	window.addEventListener('popstate', onPopState, { capture: true });
	return () => {
		document.removeEventListener('click', onClick, { capture: true });
		document.removeEventListener('pointerdown', onPointerDown, { capture: true });
		window.removeEventListener('popstate', onPopState, { capture: true });
	};
}

/** A click is a hundred milliseconds of warning: the stage's cover starts moving now. */
function preloadFor(link: HTMLAnchorElement | null): void {
	const [, id] = /^\/album\/([^/]+)/.exec(link?.pathname ?? '') ?? [];
	if (id !== undefined) {
		void preload(library.entry(decodeURIComponent(id))?.artwork, 'stage');
	} else if (link?.pathname === '/now-playing') {
		void preload(library.playing?.album.artwork, 'stage');
	}
}

/**
 * The page on screen: the shell's own `main`. A ghost is a picture of a `main`, and it sits before
 * the real one in the shell, so a plain query would find the page being left.
 */
export function pageMain(): HTMLElement | null {
	return document.querySelector('.shell')?.querySelector<HTMLElement>(':scope > main') ?? null;
}

export function reducedMotion(): boolean {
	return matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function onScreen({ element }: MorphEntry): boolean {
	const { top, bottom } = element.getBoundingClientRect();
	return bottom > 0 && top < innerHeight;
}

/** Decides what a navigation does, or returns null when it shouldn't animate at all. */
export function beginJourney(navigation: OnNavigate): Journey | null {
	const { from, to, type } = navigation;
	navigations += 1;
	if (to?.route.id === '/now-playing') {
		// Arriving by going back means the page we left is ahead of us, not behind.
		const behind = navigation.type !== 'popstate' || (navigation.delta ?? 0) > 0;
		behindNowPlaying = behind ? (from?.url.pathname ?? null) : null;
	}
	const fromRoute = journeyOf(from?.route.id);
	const toRoute = journeyOf(to?.route.id);
	if (fromRoute === undefined || toRoute === undefined) {
		return null;
	}
	if (fromRoute === 'login') {
		// The door replaced itself: whatever's behind this entry isn't a page of ours.
		navigations = 0;
	}
	if (from?.url.pathname === to?.url.pathname) {
		return null;
	}
	// iOS already played its own swipe-back animation; a morph would be the second one.
	if (type === 'popstate' && visualTransition) {
		visualTransition = false;
		return null;
	}
	if (fromRoute === 'home') {
		homeScroll = scrollY;
	}
	const borrowed = lent;
	lent = null;
	const continues = albumOn(from) !== undefined && albumOn(from) === library.playing?.album.id;
	const choreography = reducedMotion() ? blend : choreographyFor(fromRoute, toRoute, { continues });
	const { travel } = choreography;
	if (travel === null) {
		return { choreography, ids: [], sources: new Map(), waits: new Map(), origin: null };
	}
	const origin =
		travel.subject === 'rooms'
			? originOf(type === 'link' ? lastClick : null, to?.url.pathname, toRoute)
			: null;

	const leads = leadsFor(travel, navigation);
	const ids = [...leads.keys()];
	const waits = new Map(
		[...leads].flatMap(([id, lead]) => {
			const wait = travel.waits?.[lead];
			return wait === undefined ? [] : [[id, wait] as const];
		})
	);
	if (ids.length === 0) {
		return { choreography, ids, sources: new Map(), waits, origin };
	}
	const scopes = type === 'link' ? clickedScopes() : [];
	const candidates = registry.of(borrowed === null ? ids : [...ids, borrowed.from]);
	const clicked =
		scopes.length === 0
			? undefined
			: ({ element }: MorphEntry) => scopes.some((scope) => scope.contains(element));
	const sources = pick(candidates, {
		ids,
		slots: travel.sourceSlots,
		clicked,
		visible: onScreen,
		eligible: travel.subject === 'rooms' ? inView : canTravel,
		// Forward through history has no click: go back to the card the album was opened from.
		origins: type === 'popstate' ? originsOf(from?.url.pathname) : undefined
	});
	if (borrowed !== null && ids.includes(borrowed.id) && !sources.has(borrowed.id)) {
		const stand = pick(candidates, {
			ids: [borrowed.from],
			slots: [...travel.sourceSlots, 'card'],
			visible: onScreen,
			eligible: canTravel
		}).get(borrowed.from);
		if (stand !== undefined) {
			sources.set(borrowed.id, stand);
			if (from !== null) {
				lentFrom.delete(from.url.pathname);
				lentFrom.set(from.url.pathname, borrowed);
				for (const old of lentFrom.keys()) {
					if (lentFrom.size <= pagesRemembered) {
						break;
					}
					lentFrom.delete(old);
				}
			}
		}
	}
	if (choreography.grain === 'opening' && travel.subject === 'pages' && from !== null) {
		const usual = travel.sourceSlots[0] ?? 'card';
		recordOrigins(candidates, ids, usual, sources, rememberOrigins(from.url.pathname));
	}
	return { choreography, ids, sources, waits, origin };
}

/** Says where each room is pressed in the nav: its item, the fox for You, the field for Search. */
function navPlace(room: RouteName): HTMLElement | null {
	return document.querySelector<HTMLElement>(`[data-room="${CSS.escape(room)}"]`);
}

function boxOf(element: Element | null): Origin | null {
	if (element === null) {
		return null;
	}
	const { left, top, width, height } = element.getBoundingClientRect();
	return width > 0 && height > 0 ? { left, top, width, height } : null;
}

/**
 * Finds what a room unfolds from: the link pressed, when one was (an item of the nav, or a link in
 * the page such as You's to Playlists on a phone), else the room's own place in the nav, as for Back.
 * A place that isn't drawn, Health's item on a narrow screen, is no place to come out of.
 */
function originOf(
	click: EventTarget | null,
	path: string | undefined,
	room: RouteName
): Origin | null {
	const link = click instanceof Element ? click.closest('a') : null;
	const pressed = link !== null && link.pathname === path;
	return (pressed ? boxOf(link) : null) ?? boxOf(navPlace(room));
}

/** The page a lead belongs to: an album's parts are an album page's, a name an artist's… */
function ownerOf(lead: Lead, target: RouteTarget | null): string | undefined {
	switch (lead) {
		case 'name':
			return artistOn(target);
		case 'mosaic':
		case 'listname':
			return playlistOn(target);
		default:
			return albumOn(target);
	}
}

/** Lists what travels on a journey, each id with the lead it is. */
function leadsFor(
	travel: NonNullable<Choreography['travel']>,
	{ from, to }: OnNavigate
): Map<string, Lead> {
	if (travel.subject === 'fox') {
		return new Map([['fox', 'cover']]);
	}
	if (travel.subject === 'rooms') {
		const into = journeyOf(to?.route.id);
		const playing = library.playing?.album.id;
		return new Map<string, Lead>([
			...(into === 'search'
				? [['field:search', 'field'] as const, ['words:search', 'title'] as const]
				: []),
			...(playing === undefined ? [] : [[`cover:${playing}`, 'cover'] as const])
		]);
	}
	const pages = (travel.leads ?? []).flatMap((leadOf: LeadOf) => {
		const [lead, side] = leadOf.split(':') as [Lead, 'from' | 'to'];
		const owner = ownerOf(lead, side === 'from' ? from : to);
		return owner === undefined ? [] : [[`${lead}:${owner}`, lead] as const];
	});
	if (travel.subject === 'playing') {
		// What's playing, after any lead of a page's own: the artist's name leads out of the moment.
		const id = library.playing?.album.id;
		const parts = id === undefined ? [] : [`cover:${id}`, 'song', ...(travel.controls ?? [])];
		return new Map([...pages, ...parts.map((each) => [each, 'cover'] as const)]);
	}
	return new Map(pages);
}

/** A hero scrolled off screen stays with its page: the island's cover travels instead. */
function canTravel(entry: MorphEntry): boolean {
	return entry.slot !== 'hero' || onScreen(entry);
}

/**
 * Out of the moment, the poster lands where the page holds its album in view: a card clear of the nav
 * and the island, or the album's own hero on screen. Anywhere else it goes into the island's thumb,
 * which is always there.
 */
function heldInView(entry: MorphEntry): boolean {
	return entry.slot === 'card' ? inTheClear(entry.element) : canTravel(entry);
}

/**
 * Between rooms, only what is in view on both travels: a cover clear of the nav and the island, or
 * the nav's own field. Anything else stays with its room and comes and goes with it.
 */
function inView(entry: MorphEntry): boolean {
	return entry.slot === 'nav' ? onScreen(entry) : inTheClear(entry.element);
}

/**
 * Where the click was: its link, then its row. A song's album link is beside its thumb, not around
 * it, and the thumb is what leaves.
 */
function clickedScopes(): Element[] {
	if (!(lastClick instanceof Element)) {
		return [];
	}
	return [lastClick.closest('a'), lastClick.closest('[data-track]')].filter(
		(scope): scope is Element => scope !== null
	);
}

/** Resolves the targets once the new page is in the DOM and the scroll is final. */
export function landJourney(
	journey: Journey,
	scrollToOrigin: boolean,
	path: string
): Map<string, MorphEntry> {
	const { travel, name, grain } = journey.choreography;
	if (landAtTop) {
		landAtTop = false;
		scrollTo({ top: 0, behavior: 'instant' });
	}
	if (travel === null) {
		return new Map();
	}
	const landingOnHome = name === 'album-to-home' || name === 'album-into-continue';
	if (landingOnHome && scrollToOrigin) {
		scrollTo({ top: homeScroll, behavior: 'instant' });
	}
	const { landIn } = travel;
	const home =
		landIn === undefined
			? null
			: (pageMain()?.querySelector(`[data-morph-key="${CSS.escape(landIn)}"]`) ?? null);
	const targets = pick(registry.of(journey.ids), {
		ids: journey.ids,
		slots: travel.targetSlots,
		// Where the page now holds what you were in wins over where it came from.
		clicked: home === null ? undefined : ({ element }) => home.contains(element),
		origins: originsOf(path),
		// With no origin, a card already in the clear wins, so the page stays where it was left.
		visible: ({ element }) => inTheClear(element),
		eligible:
			travel.subject === 'rooms' ? inView : travel.subject === 'playing' ? heldInView : canTravel
	});
	const borrowed = lentFrom.get(path);
	if (borrowed !== undefined && journey.ids.includes(borrowed.id) && !targets.has(borrowed.id)) {
		const stand = pick(registry.of([borrowed.from]), {
			ids: [borrowed.from],
			slots: travel.targetSlots,
			visible: ({ element }) => inTheClear(element)
		}).get(borrowed.from);
		if (stand !== undefined) {
			targets.set(borrowed.id, stand);
		}
	}
	const lead = targets.get(journey.ids.at(0) ?? '');
	// What's playing lands only where the page already shows it: nothing is scrolled for it.
	if (
		grain === 'gathering' &&
		travel.subject !== 'playing' &&
		lead !== undefined &&
		lead.slot !== 'hero'
	) {
		bringIntoView(lead.element);
	}
	return targets;
}

const clearTop = 90;

/** Finds where the viewport stops being clear of the island. */
function clearBottom(): number {
	const island = document.querySelector('aside');
	return (island?.getBoundingClientRect().top ?? innerHeight) - 16;
}

function inTheClear(element: HTMLElement): boolean {
	const { top, bottom } = element.getBoundingClientRect();
	return top >= clearTop && bottom <= clearBottom();
}

/** The most cards a deal sends out: a dozen is as many as can land in its time. */
const mostDealt = 12;

/** Reports whether a cover shows whole, not cut off by the unit it's in. */
function shownWhole(element: HTMLElement): boolean {
	const unit = element.closest('[data-morph-part]')?.getBoundingClientRect();
	const { left, right } = element.getBoundingClientRect();
	return unit !== undefined && left >= unit.left && right <= unit.right;
}

/**
 * Finds the covers a deal lands on, in reading order: whatever Home shows clear of the nav and the
 * island, one for each album. A card with no such place doesn't fly, and goes down with the pile.
 */
export function dealTargets(): Map<string, MorphEntry> {
	const clear = registry
		.list()
		.filter(
			({ element, kind, slot }) =>
				kind === 'cover' && slot === 'card' && inTheClear(element) && shownWhole(element)
		);
	const ids = [...new Set(clear.map(({ id }) => id))].slice(0, mostDealt);
	return pick(clear, { ids, slots: ['card'] });
}

/** Finds the nearest box that scrolls sideways around an element: a row of covers on a phone. */
function sidewaysScroller(element: HTMLElement): HTMLElement | null {
	for (let box = element.parentElement; box !== null; box = box.parentElement) {
		const { overflowX } = getComputedStyle(box);
		if ((overflowX === 'auto' || overflowX === 'scroll') && box.scrollWidth > box.clientWidth) {
			return box;
		}
	}
	return null;
}

/**
 * Scrolls instantly, under the outgoing page, so a card isn't behind the nav or the island, nor
 * off the side of a row that scrolls sideways.
 */
function bringIntoView(element: HTMLElement): void {
	const row = sidewaysScroller(element);
	if (row !== null) {
		const box = row.getBoundingClientRect();
		const { left, right } = element.getBoundingClientRect();
		if (left < box.left) {
			row.scrollBy({ left: left - box.left, behavior: 'instant' });
		} else if (right > box.right) {
			row.scrollBy({ left: Math.min(right - box.right, left - box.left), behavior: 'instant' });
		}
	}
	const { top, bottom } = element.getBoundingClientRect();
	const limit = clearBottom();
	if (top < clearTop) {
		scrollBy({ top: top - clearTop, behavior: 'instant' });
	} else if (bottom > limit) {
		const overshoot = Math.min(bottom - limit, top - clearTop);
		scrollBy({ top: overshoot, behavior: 'instant' });
	}
}

/**
 * Handles a click on a link up to the album. A link that can't go back is left to Kit, which
 * replaces Now playing with the album. Either way the page lands at the top, because the cover
 * was asked to settle into the hero and history may restore a scroll that hides it.
 */
export function goUpToAlbum(event: MouseEvent, albumPath: string): void {
	const opensElsewhere = event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
	if (!opensElsewhere && albumExit(behindNowPlaying, albumPath) === 'back') {
		event.preventDefault();
		landAtTop = true;
		history.back();
	}
}

/** Reads a motion from the tokens, at morph time. */
export function motionOf(spec: MotionSpec): Motion {
	const style = getComputedStyle(document.documentElement);
	return resolveMotion(spec, (token: MotionToken) => parseTime(style.getPropertyValue(token)));
}

/** Returns the center of an element in viewport coordinates. */
export function centerOf(element: Element): { x: number; y: number } {
	const { left, top, width, height } = element.getBoundingClientRect();
	return { x: left + width / 2, y: top + height / 2 };
}
