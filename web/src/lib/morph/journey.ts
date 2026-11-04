import type { OnNavigate } from '$app/navigation';
import {
	blend,
	choreographyFor,
	parseTime,
	resolveMotion,
	type Choreography,
	type Motion,
	type MotionSpec,
	type MotionToken,
	type RouteName
} from './choreography.ts';
import { library } from '#lib/library/library.svelte.ts';
import { markClick } from './hud.svelte.ts';
import { pick, recordOrigins, type Origins } from './pairs.ts';
import { registry, type MorphEntry } from './registry.ts';

/** One navigation's plan. */
export interface Journey {
	choreography: Choreography;
	/** The shared ids of this navigation; empty when nothing travels. */
	ids: string[];
	sources: Map<string, MorphEntry>;
}

const routes = new Map<string, RouteName>([
	['/', 'home'],
	['/album/[id]', 'album'],
	['/now-playing', 'now-playing']
]);

const origins: Origins = new Map();

const playerParts = ['control:previous', 'control:pause', 'control:next', 'progress'];

let lastClick: EventTarget | null = null;
let homeScroll = 0;
let visualTransition = false;
let navigations = 0;

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
	function onPopState(event: PopStateEvent): void {
		visualTransition = event.hasUAVisualTransition;
		markClick(performance.now());
	}
	document.addEventListener('click', onClick, { capture: true });
	window.addEventListener('popstate', onPopState, { capture: true });
	return () => {
		document.removeEventListener('click', onClick, { capture: true });
		window.removeEventListener('popstate', onPopState, { capture: true });
	};
}

function reducedMotion(): boolean {
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
	const fromRoute = routes.get(from?.route.id ?? '');
	const toRoute = routes.get(to?.route.id ?? '');
	if (fromRoute === undefined || toRoute === undefined) {
		return null;
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

	const choreography = reducedMotion() ? blend : choreographyFor(fromRoute, toRoute);
	const { travel } = choreography;
	if (travel === null) {
		return { choreography, ids: [], sources: new Map() };
	}

	const ids = idsFor(travel.subject, navigation);
	if (ids.length === 0) {
		return { choreography, ids, sources: new Map() };
	}
	const scope = type === 'link' ? clickedScope() : null;
	const candidates = registry.list();
	const sources = pick(candidates, {
		ids,
		slots: travel.sourceSlots,
		clicked: scope === null ? undefined : ({ element }) => scope.contains(element),
		visible: onScreen,
		eligible: canTravel,
		// Forward through history has no click: go back to the card the album was opened from.
		origins: type === 'popstate' ? origins : undefined
	});
	if (choreography.name === 'home-to-album') {
		recordOrigins(candidates, ids, 'card', sources, origins);
	}
	return { choreography, ids, sources };
}

/** Lists what travels on a journey. */
function idsFor(subject: 'from' | 'to' | 'playing', { from, to }: OnNavigate): string[] {
	if (subject === 'playing') {
		const id = library.playing?.album.id;
		return id === undefined ? [] : [`cover:${id}`, 'song', ...playerParts];
	}
	const id = (subject === 'to' ? to : from)?.params?.['id'];
	return id === undefined ? [] : [`cover:${id}`, `title:${id}`];
}

/** A hero scrolled off screen stays with its page: the island's cover travels instead. */
function canTravel(entry: MorphEntry): boolean {
	return entry.slot !== 'hero' || onScreen(entry);
}

function clickedScope(): Element | null {
	return lastClick instanceof Element ? lastClick.closest('a') : null;
}

/** Resolves the targets once the new page is in the DOM and the scroll is final. */
export function landJourney(journey: Journey, scrollToOrigin: boolean): Map<string, MorphEntry> {
	const { travel, name } = journey.choreography;
	if (travel === null) {
		return new Map();
	}
	const landingOnHome = name === 'album-to-home';
	if (landingOnHome && scrollToOrigin) {
		scrollTo({ top: homeScroll, behavior: 'instant' });
	}
	const targets = pick(registry.list(), {
		ids: journey.ids,
		slots: travel.targetSlots,
		origins,
		visible: onScreen,
		eligible: canTravel
	});
	if (landingOnHome) {
		const cover = [...targets.values()].find((entry) => entry.kind === 'cover');
		if (cover !== undefined && cover.slot === 'card') {
			bringIntoView(cover.element);
		}
	}
	return targets;
}

/** Scrolls instantly, under the outgoing page, so a card isn't behind the nav or the island. */
function bringIntoView(element: HTMLElement): void {
	const { top, bottom } = element.getBoundingClientRect();
	const clearTop = 90;
	const island = document.querySelector('aside');
	const clearBottom = (island?.getBoundingClientRect().top ?? innerHeight) - 16;
	if (top < clearTop) {
		scrollBy({ top: top - clearTop, behavior: 'instant' });
	} else if (bottom > clearBottom) {
		const overshoot = Math.min(bottom - clearBottom, top - clearTop);
		scrollBy({ top: overshoot, behavior: 'instant' });
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
