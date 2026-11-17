import type { OnNavigate } from '$app/navigation';
import { Channel } from '../channel.ts';
import {
	blendDuration,
	bloomOpenness,
	bloomStrength,
	gatherWeight,
	plainArrival,
	rippleDelay,
	rippleWeight,
	springParams,
	weighGather,
	weighPart,
	type Choreography,
	type Part,
	type Role,
	type SpringParams,
	type StageName
} from '../choreography.ts';
import { reportStart } from '../hud.svelte.ts';
import {
	beginJourney,
	centerOf,
	landJourney,
	motionOf,
	pageMain,
	type Journey
} from '../journey.ts';
import { registry, type MorphEntry } from '../registry.ts';
import {
	BarFlier,
	CoverFlier,
	drawTitle,
	measureBar,
	measureCover,
	measureSurface,
	measureTitle,
	SurfaceFlier,
	TitleFlier,
	type BarShape,
	type CoverShape,
	type Flier,
	type SurfaceShape,
	type TitleMeasure
} from './flier.ts';
import { markMoving } from '../settled.ts';
import { IslandShape } from './island.ts';
import { Layer } from './layer.ts';

const hiddenAttribute = 'data-morph-hidden';
const sourceAttribute = 'data-morph-source';

/** What's inside the island leaves at once, before the glass has got anywhere. */
const islandLeaves: Part = {
	from: {},
	to: { opacity: 0 },
	delay: 0,
	motion: { token: '--morph-fast', bounce: 0 }
};
/** The largest gap one frame may jump: a stalled main thread shouldn't teleport anything. */
const longestFrame = 1 / 20;

function hide(element: HTMLElement): void {
	element.setAttribute(hiddenAttribute, '');
}

function show(element: Element): void {
	element.removeAttribute(hiddenAttribute);
}

interface Point {
	x: number;
	y: number;
}

interface CoverCapture {
	kind: 'cover' | 'control';
	shape: CoverShape;
	picture: Node;
	shadow: string;
}

interface TitleCapture {
	kind: 'title';
	measure: TitleMeasure;
	picture: HTMLElement;
}

interface BarCapture {
	kind: 'bar';
	shape: BarShape;
	color: string;
}

interface SurfaceCapture {
	kind: 'surface';
	shape: SurfaceShape;
	/** The element it was taken from, drawn as the flier leaves, and never held as a stage too. */
	element: HTMLElement;
}

type Capture = CoverCapture | TitleCapture | BarCapture | SurfaceCapture;

function capture(source: MorphEntry): Capture {
	const { element, kind } = source;
	if (kind === 'surface') {
		return { kind, shape: measureSurface(element), element };
	}
	if (kind === 'title') {
		return { kind, measure: measureTitle(element), picture: drawTitle(element) };
	}
	if (kind === 'bar') {
		const color = getComputedStyle(element).backgroundColor;
		return { kind, shape: measureBar(element), color };
	}
	return {
		kind,
		shape: measureCover(element),
		picture: pictureOf(source),
		shadow: getComputedStyle(element).boxShadow
	};
}

/** Draws what a flier shows: a cover's art, or a control as it stands. */
function pictureOf({ element, kind }: MorphEntry): Node {
	if (kind === 'cover') {
		const picture = (element.firstElementChild ?? element).cloneNode(true);
		if (picture instanceof Element) {
			settleImages(picture);
		}
		return picture;
	}
	const picture = element.cloneNode(true);
	if (picture instanceof HTMLElement) {
		picture.removeAttribute(hiddenAttribute);
		picture.style.cssText = 'position:absolute;inset:0;width:100%;height:100%;margin:0';
	}
	return picture;
}

/**
 * Makes a clone's images paint on its first frame, from what's already decoded. One still on its
 * way shows nothing where it stands, so its clone mustn't start a fetch of its own.
 */
function settleImages(picture: Element): void {
	for (const image of picture.querySelectorAll('img')) {
		if (image.hasAttribute('data-cover-pending')) {
			image.removeAttribute('src');
			image.remove();
		} else {
			image.loading = 'eager';
			image.decoding = 'sync';
		}
	}
}

/** The outgoing page, as an inert picture of itself. */
interface Ghost {
	path: string;
	element: HTMLElement;
	layers: Layer[];
	/** Each unit's layer by name, so the real page can take over from its ghost unit by unit. */
	units: Map<string, Layer>;
}

const partAttribute = 'data-morph-part';

/** Finds the parts of a page that move on their own. */
function unitsOf(root: Element): HTMLElement[] {
	return [...root.querySelectorAll<HTMLElement>(`[${partAttribute}]`)];
}

/**
 * How far past the viewport a unit still takes part in a morph, as a share of its height. Nobody
 * can see past this, and a long page holds hundreds of units whose pictures would each cost.
 */
const reach = 0.25;

function nearFold(unit: HTMLElement): boolean {
	const { top, bottom } = unit.getBoundingClientRect();
	const margin = innerHeight * reach;
	return bottom > -margin && top < innerHeight + margin;
}

/** Finds the units a morph moves: a ghost, an arrival and a ripple all work on this one set. */
function unitsNearFold(root: Element): HTMLElement[] {
	return unitsOf(root).filter(nearFold);
}

/**
 * Finds what a page shows outside its units: a rail, a heading no one gave a role. It has to come
 * and go with the page all the same, or it would cut: arriving it would appear over the page
 * leaving, and leaving it would vanish at once when the ghost goes. None of it holds a unit, so it
 * moves on its own without moving one twice.
 */
function looseOf(root: Element): HTMLElement[] {
	return [...root.children].flatMap((child) => {
		if (!(child instanceof HTMLElement) || child.hasAttribute(partAttribute)) {
			return [];
		}
		return child.querySelector(`[${partAttribute}]`) === null ? [child] : looseOf(child);
	});
}

/** Names units so a ghost and the page behind it agree, whichever cards each has mounted. */
function keysOf(units: readonly HTMLElement[]): string[] {
	const seen = new Map<string, number>();
	return units.map((unit) => {
		const { morphKey, morphPart = '' } = unit.dataset;
		const ordinal = seen.get(morphPart) ?? 0;
		seen.set(morphPart, ordinal + 1);
		return morphKey ?? `${morphPart}:${String(ordinal)}`;
	});
}

/** A far unit's stand-in: it keeps its place in the layout, and costs no images or layer. */
function standIn(unit: HTMLElement): Node {
	const stand = unit.cloneNode(false);
	if (stand instanceof HTMLElement) {
		stand.removeAttribute(partAttribute);
		stand.style.visibility = 'hidden';
		stand.style.height = `${String(unit.offsetHeight)}px`;
	}
	return stand;
}

/**
 * Holds what's stuck to the screen where it stands. A clone has no scroller to stick to, so it would
 * go back to the top of its block, somewhere off screen.
 */
function keepPinned(from: HTMLElement, picture: Node): void {
	if (picture instanceof HTMLElement && from.dataset['morphPinned'] !== undefined) {
		const top = from.getBoundingClientRect().top;
		const start = from.parentElement?.getBoundingClientRect().top ?? top;
		picture.style.position = 'relative';
		picture.style.top = `${String(top - start)}px`;
	}
}

/**
 * Clones a page for a ghost, in full only near the fold. `cloneNode` on the whole of it would fetch
 * every lazy image at once: an image ignores `loading` the moment it's cloned with a `src`.
 */
function pictureOfPage(root: Element, near: ReadonlySet<Element>): Node {
	const picture = root.cloneNode(false);
	for (const child of root.childNodes) {
		if (!(child instanceof HTMLElement)) {
			picture.appendChild(child.cloneNode(true));
		} else if (child.hasAttribute(partAttribute) && !near.has(child)) {
			picture.appendChild(standIn(child));
		} else if (!child.hasAttribute(partAttribute) && child.querySelector(`[${partAttribute}]`)) {
			picture.appendChild(pictureOfPage(child, near));
		} else {
			const whole = child.cloneNode(true);
			if (whole instanceof Element) {
				settleImages(whole);
			}
			keepPinned(child, whole);
			picture.appendChild(whole);
		}
	}
	return picture;
}

/**
 * Makes a picture of a page no `main` of its own. There is one page on screen, and anything that
 * asks for the page, a screen reader's landmarks or a query, must never find its ghost. The picture
 * keeps how the page sat among the stages, which its `main` had from the shell's styles.
 */
function noLandmark(picture: Node, page: HTMLElement): Node {
	if (!(picture instanceof HTMLElement)) {
		return picture;
	}
	const { position, zIndex } = getComputedStyle(page);
	const mains = picture.tagName === 'MAIN' ? [picture] : [...picture.querySelectorAll('main')];
	let root: HTMLElement = picture;
	for (const main of mains) {
		const plain = document.createElement('div');
		for (const { name, value } of main.attributes) {
			plain.setAttribute(name, value);
		}
		plain.style.position = position;
		plain.style.zIndex = zIndex;
		plain.append(...main.childNodes);
		main.replaceWith(plain);
		if (main === picture) {
			root = plain;
		}
	}
	return root;
}

function roleOf(unit: HTMLElement): Role {
	return unit.dataset['morphPart'] as Role;
}

function distance(unit: HTMLElement, from: { x: number; y: number }): number {
	const { x, y } = centerOf(unit);
	return Math.hypot(x - from.x, y - from.y);
}

/** Scales each unit's part by where it sits relative to the cover, or to where the lead lands. */
function shapeWave(
	units: HTMLElement[],
	part: (unit: HTMLElement) => Part,
	from: Point | null
): { part: Part; delay: number }[] {
	const reach = from === null ? [] : units.map((unit) => distance(unit, from));
	const farthest = Math.max(1, ...reach);
	return units.map((unit, index) => {
		const plain = part(unit);
		const away = reach.at(index);
		if (away === undefined) {
			return { part: plain, delay: plain.delay };
		}
		const share = away / farthest;
		if (plain.gather !== undefined) {
			// An entrance never queues: only how deep it comes from depends on where it is.
			return { part: weighGather(plain, gatherWeight(plain.gather, share)), delay: plain.delay };
		}
		if (plain.ripple === undefined) {
			return { part: plain, delay: plain.delay };
		}
		return {
			part: weighPart(plain, rippleWeight(plain.ripple, share)),
			delay: plain.delay + rippleDelay(plain.ripple, share)
		};
	});
}

/** Pins the album colors on a picture, because the shell's are about to change. */
function keepColors(from: HTMLElement, picture: HTMLElement): void {
	const style = getComputedStyle(from);
	for (const name of ['--accent', '--on-accent', '--wash', '--stage']) {
		picture.style.setProperty(name, style.getPropertyValue(name));
	}
}

/** The light around the cover, following it through the air. */
interface Bloom {
	element: HTMLElement;
	cover: CoverFlier;
	scale: Channel;
	spec: NonNullable<Choreography['bloom']>;
}

function pivot(element: HTMLElement, around: Point): void {
	element.style.transform = 'none';
	const { left, top } = element.getBoundingClientRect();
	element.style.transformOrigin = `${String(around.x - left)}px ${String(around.y - top)}px`;
}

/** Pins the stage to the viewport, so a scroll under the ghost doesn't drag it along. */
function freeze(stage: HTMLElement, top: number): void {
	stage.style.position = 'fixed';
	stage.style.top = `${String(top)}px`;
	stage.style.left = '0';
	stage.style.right = '0';
}

function thaw(stage: HTMLElement): void {
	for (const property of ['position', 'top', 'left', 'right']) {
		stage.style.removeProperty(property);
	}
}

/** One card of a deal: leaves `source` for `target`, `delay` milliseconds in. */
export interface DealtCard {
	source: MorphEntry;
	target: MorphEntry;
	delay: number;
}

/** Something that goes as a deal plays out, and stays gone until whoever owns it removes it. */
export interface Departure {
	element: HTMLElement;
	part: Part;
}

/** What the engine does for the shell. */
export interface Morphs {
	/** What the one `onNavigate` calls. */
	navigate: (navigation: OnNavigate) => void;
	/**
	 * Plays a journey with no navigation: Home is already mounted, under what's being dealt. Resolves
	 * once it's all at rest, or at once when a navigation cuts it short.
	 */
	deal: (
		choreography: Choreography,
		cards: readonly DealtCard[],
		departures: readonly Departure[]
	) => Promise<void>;
	/** Blends a picture of everything into whatever `change` leaves behind. */
	blend: (change: () => Promise<void>) => Promise<void>;
}

/** A deal has no route to name it by, and no ghost must ever answer to this. */
const dealPath = 'deal';

/** The spring engine: real elements, a persistent overlay, and physics that can turn around. */
export function createSpringEngine(): Morphs {
	const fliers = new Map<string, Flier>();
	/** Fliers that leave a beat after the rest, with the seconds each still has to wait. */
	const waiting = new Map<string, number>();
	const landing = new Map<HTMLElement, Layer>();
	const ghosts: Ghost[] = [];
	/** Pictures of the stages of pages left, giving way at the bottom of the ghost layer. */
	const heldStages = new Map<StageName, { ghost: Ghost; look: string }>();
	const inFlight = new Set<string>();
	let flight: SpringParams = springParams({ duration: 420, bounce: 0 });
	const stages = new Map<StageName, Layer>();
	let island: IslandShape | null = null;
	let bloom: Bloom | null = null;
	let frame = 0;
	let lastFrame = 0;
	let startReported = true;
	/** A deal's cards still on the pile, each counting down to its turn. */
	let pendingCards: (DealtCard & { wait: number })[] = [];
	/** Unlike `landing` these are never handed back: their elements leave with the deck. */
	let departing: Layer[] = [];
	let dealDone: (() => void) | null = null;

	// The moment a shared element mounts, before it can paint: this is what stops a flash.
	registry.subscribe((entry) => {
		if (inFlight.has(entry.id) && entry.slot !== 'island') {
			hide(entry.element);
		}
	});

	function surfaceFor(selector: string): HTMLElement | null {
		return document.querySelector<HTMLElement>(selector);
	}

	function cloneRoute(
		path: string,
		{ leaving, leavingByRole }: Choreography,
		sources: Journey['sources']
	): Ghost | null {
		const main = pageMain();
		const under = surfaceFor('.ghost-layer');
		if (main === null || under === null) {
			return null;
		}
		// A surface is the stage behind the page, not in it, and it only goes once its flier does.
		const setOff = [...sources].filter(([, { kind }]) => kind !== 'surface');
		for (const [id, { element }] of setOff) {
			element.setAttribute(sourceAttribute, id);
			hide(element);
		}
		const { left, top, width, height } = main.getBoundingClientRect();
		const real = unitsNearFold(main);
		const clone = noLandmark(pictureOfPage(main, new Set(real)), main);
		for (const [, { element }] of setOff) {
			element.removeAttribute(sourceAttribute);
			// The page itself is still on screen, in front of its ghost, until the next one replaces
			// it: what leaves it is gone from the ghost, never from the page.
			if (main.contains(element)) {
				show(element);
			}
		}
		if (!(clone instanceof HTMLElement)) {
			return null;
		}

		clone.inert = true;
		clone.setAttribute('aria-hidden', 'true');
		clone.dataset['morphGhost'] = 'page';
		clone.style.cssText = `position:absolute;left:${String(left)}px;top:${String(top)}px;width:${String(width)}px;height:${String(height)}px;z-index:auto`;
		keepColors(main, clone);
		under.append(clone);

		const cover = [...sources.values()].find((entry) => entry.kind === 'cover');
		const center = cover === undefined ? null : centerOf(cover.element);
		const units = unitsOf(clone);
		// A page with no units yet, still loading say, leaves as one.
		const movers = units.length > 0 ? [...units, ...looseOf(clone).filter(nearFold)] : [clone];
		const partOf = (unit: HTMLElement): Part => leavingByRole?.[roleOf(unit)] ?? leaving;
		const wave = shapeWave(movers, partOf, center);
		const layers = movers.map((unit, index) => {
			const realUnit = real.at(index);
			// A unit still arriving is at some pose: its ghost starts there, not at rest.
			const inherited = realUnit === undefined ? undefined : landing.get(realUnit);
			if (center !== null) {
				pivot(unit, center);
			}
			const shaped = wave.at(index);
			const layer = new Layer(unit, shaped?.part ?? leaving, shaped?.delay);
			if (inherited !== undefined) {
				layer.adopt(inherited);
			}
			return layer;
		});
		for (const unit of unitsOf(main)) {
			landing.delete(unit);
		}
		const keys = keysOf(units);
		const named = layers.flatMap((layer, index) => {
			const key = keys.at(index);
			return key === undefined ? [] : [[key, layer] as const];
		});
		const ghost = { path, element: clone, layers, units: new Map(named) };
		ghosts.push(ghost);
		return ghost;
	}

	function launch(
		journey: Journey,
		captures: Map<string, Capture>,
		targets: Map<string, MorphEntry>,
		ghost: Ghost | null
	) {
		const air = surfaceFor('.morph-layer');
		const under = surfaceFor('.ghost-layer');
		if (air === null || under === null) {
			return;
		}
		for (const [id, target] of targets) {
			hide(target.element);
			const flying = fliers.get(id);
			if (flying !== undefined) {
				// Turning round: it goes on from here, at once, whatever it was waiting for. Where it
				// was going, if that's still on screen, is the real thing again.
				const before = flying.target;
				if (before !== null && before.element !== target.element) {
					show(before.element);
				}
				flying.target = target;
				waiting.delete(id);
				continue;
			}
			const source = captures.get(id);
			if (source === undefined) {
				show(target.element);
				inFlight.delete(id);
				continue;
			}
			const flier = createFlier(source, target);
			fliers.set(id, flier);
			const wait = journey.waits.get(id);
			if (wait !== undefined) {
				waiting.set(id, wait / 1000);
			}
			if (flier instanceof SurfaceFlier) {
				// A surface is still the page you leave until it lands: under that page's own parts,
				// which sat on it and give way over it, and under the page arriving in front.
				under.insertBefore(flier.element, ghost?.element ?? null);
			} else if (target.kind === 'cover' || target.kind === 'bar') {
				// Words stay in front of art, so a title never tucks behind the cover it travels with,
				// and the fox bar passes under the controls it crosses on the way.
				air.prepend(flier.element);
			} else {
				air.append(flier.element);
			}
		}
		// A flier whose page has gone has nowhere to land: it leaves with the ghost. One heading
		// for the shell keeps going, whatever this journey is about: the island is still there.
		for (const [id, flier] of fliers) {
			if (!targets.has(id) && flier.target?.element.isConnected !== true) {
				flier.target = null;
			}
		}
		flight = springParams(motionOf(journey.choreography.motion));
	}

	function createFlier(source: Capture, target: MorphEntry): Flier {
		if (source.kind === 'surface') {
			return new SurfaceFlier(source.shape, source.element, target);
		}
		if (source.kind === 'bar') {
			const glass = surfaceFor('[data-island-glass]') ?? target.element;
			return new BarFlier(source.shape, target, source.color, glass);
		}
		if (source.kind === 'title') {
			return new TitleFlier(
				source.measure,
				source.picture,
				target,
				measureTitle(target.element),
				drawTitle(target.element)
			);
		}
		const flier = new CoverFlier(source.shape, target, source.picture, source.shadow);
		const want =
			target.element.querySelector<HTMLElement>('[data-cover-want]')?.dataset['coverWant'];
		if (want !== undefined && measureCover(target.element).width >= source.shape.width) {
			flier.sharpen(want);
		}
		return flier;
	}

	/**
	 * Brings the new page's units in. `everyUnit` gives a unit the journey says nothing about a plain
	 * arrival: it is drawn over the page leaving, so appearing at once would be a cut.
	 */
	function arrive(
		journey: Journey,
		path: string,
		targets: Map<string, MorphEntry>,
		everyUnit: boolean
	): void {
		const main = pageMain();
		if (main === null) {
			return;
		}
		const { arriving, arrivingByKey, motion } = journey.choreography;
		const partOf = (unit: HTMLElement): Part | undefined =>
			arrivingByKey?.[unit.dataset['morphKey'] ?? ''] ??
			arriving[roleOf(unit)] ??
			(everyUnit ? plainArrival : undefined);
		const units = unitsNearFold(main);
		// The page gathers around where the lead lands, and opens around the cover.
		const lead =
			targets.get(journey.ids.at(0) ?? '') ??
			[...targets.values()].find((entry) => entry.kind === 'cover');
		const center = lead === undefined ? null : centerOf(lead.element);
		const flying = new Set([...targets.values()].map((entry) => entry.element));

		// Going back mid-morph: the real page takes over from its own ghost, unit by unit.
		const returning = ghosts.findIndex((ghost) => ghost.path === path);
		const ghost = ghosts.at(returning);
		if (returning >= 0 && ghost !== undefined) {
			ghosts.splice(returning, 1);
			const keys = keysOf(units);
			for (const [index, unit] of units.entries()) {
				const from = ghost.units.get(keys.at(index) ?? '');
				if (from === undefined || flying.has(unit)) {
					continue;
				}
				const part: Part = partOf(unit) ?? { from: {}, to: {}, delay: 0, motion };
				if (center !== null) {
					pivot(unit, center);
				}
				const layer = new Layer(unit, part);
				layer.adopt(from);
				layer.retarget(part);
				landing.set(unit, layer);
			}
			ghost.element.remove();
			return;
		}

		const loose = everyUnit ? looseOf(main).filter(nearFold) : [];
		const moving = [...units, ...loose].filter(
			(unit) => partOf(unit) !== undefined && !flying.has(unit)
		);
		const wave = shapeWave(moving, (unit) => partOf(unit) as Part, center);
		for (const [index, unit] of moving.entries()) {
			const shaped = wave.at(index);
			if (shaped === undefined) {
				continue;
			}
			if (center !== null) {
				pivot(unit, center);
			}
			landing.set(unit, new Layer(unit, shaped.part, shaped.delay));
		}
	}

	/** How a stage looks: its album's colors, which a picture of it keeps. */
	function lookOf(stage: HTMLElement): string {
		const { style } = stage;
		return `${style.background}|${style.getPropertyValue('--field')}`;
	}

	/**
	 * Brings up the stages of the page arrived at, in front of everything left. Turning back to a
	 * stage just left, the real one carries on from wherever its picture had got to.
	 */
	function moveStages(journey: Journey): void {
		for (const [name, part] of Object.entries(journey.choreography.stages) as [StageName, Part][]) {
			const element = surfaceFor(`[data-morph-stage="${name}"]`);
			if (element === null) {
				continue;
			}
			const stage = stages.get(name);
			if (stage !== undefined) {
				stage.retarget(part);
				continue;
			}
			const layer = new Layer(element, part);
			const picture = heldStages.get(name);
			const from = picture?.ghost.layers.at(0);
			if (picture !== undefined && from !== undefined && picture.look === lookOf(element)) {
				layer.adopt(from);
				layer.retarget(part);
				dropGhost(picture.ghost);
			}
			stages.set(name, layer);
			thaw(element);
		}
	}

	/**
	 * Leaves a picture of each stage the page had, to give way at the bottom of the ghost layer:
	 * under the page left, and under the stage and the page arriving. The real stage is then free
	 * to be the next page's, whatever color that is. A stage reshaping into a surface isn't held.
	 */
	function holdStages({ held }: Choreography, reshaping: ReadonlySet<HTMLElement>): void {
		const under = surfaceFor('.ghost-layer');
		for (const [name, part] of Object.entries(held) as [StageName, Part][]) {
			const stage = surfaceFor(`[data-morph-stage="${name}"]`);
			if (under === null || stage === null || reshaping.has(stage)) {
				continue;
			}
			const moving = stages.get(name);
			// A stage nobody sees has nothing to give way.
			if (moving === undefined && Number(getComputedStyle(stage).opacity) < 0.01) {
				continue;
			}
			const clone = stage.cloneNode(true);
			if (!(clone instanceof HTMLElement)) {
				continue;
			}
			clone.removeAttribute('data-morph-stage');
			clone.removeAttribute(hiddenAttribute);
			keepColors(stage, clone);
			freeze(clone, stage.getBoundingClientRect().top);
			under.append(clone);
			const layer = new Layer(clone, part);
			if (moving !== undefined) {
				layer.adopt(moving);
				moving.release();
				thaw(stage);
				stages.delete(name);
			}
			const ghost: Ghost = { path: '', element: clone, layers: [layer], units: new Map() };
			ghosts.push(ghost);
			heldStages.set(name, { ghost, look: lookOf(stage) });
		}
	}

	function dropGhost(ghost: Ghost): void {
		ghost.element.remove();
		const index = ghosts.indexOf(ghost);
		if (index >= 0) {
			ghosts.splice(index, 1);
		}
		for (const [name, picture] of heldStages) {
			if (picture.ghost === ghost) {
				heldStages.delete(name);
			}
		}
	}

	/** Picks up the glass and leaves a picture of the island's old contents to fade. */
	function liftIsland(part: Part): void {
		const aside = surfaceFor('aside');
		const glass = surfaceFor('[data-island-glass]');
		const content = surfaceFor('[data-island-content]');
		const air = surfaceFor('.morph-layer');
		if (aside === null || glass === null || content === null || air === null) {
			return;
		}
		island ??= new IslandShape(aside, glass);

		const clone = content.cloneNode(true);
		if (!(clone instanceof HTMLElement)) {
			return;
		}
		// Inside a stand-in for the island, so `.expanded .content` still matches the picture.
		const { left, top, width, height } = aside.getBoundingClientRect();
		const stand = document.createElement('div');
		stand.className = aside.className;
		stand.inert = true;
		stand.style.cssText = `position:absolute;left:${String(left)}px;top:${String(top)}px;bottom:auto;width:${String(width)}px;height:${String(height)}px;transform:none`;
		keepColors(aside, stand);
		stand.append(clone);
		air.prepend(stand);
		island.carryFromHere(stand);
		const layer = new Layer(clone, part);
		const inherited = landing.get(content);
		if (inherited !== undefined) {
			layer.adopt(inherited);
		}
		landing.delete(content);
		ghosts.push({ path: '', element: stand, layers: [layer], units: new Map() });
	}

	/** The island's first appearance: glass and contents rise as one, under the frame. */
	function formIsland(part: Part): void {
		for (const selector of ['[data-island-glass]', '[data-island-content]']) {
			const element = surfaceFor(selector);
			if (element !== null) {
				landing.set(element, new Layer(element, part));
			}
		}
	}

	function landIsland(part: Part): void {
		island?.aim();
		island?.measure();
		island?.write();
		const content = surfaceFor('[data-island-content]');
		if (content !== null) {
			landing.set(content, new Layer(content, part));
			island?.carryHome(content);
			island?.write();
		}
	}

	function openBloom(journey: Journey): void {
		const { bloom: spec, motion } = journey.choreography;
		const element = surfaceFor('[data-morph-bloom]');
		const coverId = journey.ids.at(0) ?? '';
		const cover = fliers.get(coverId);
		if (spec === null || element === null || !(cover instanceof CoverFlier)) {
			return;
		}
		if (bloom === null) {
			bloom = { element, cover, scale: new Channel(spec.rest, spec.full), spec };
			element.dataset['morphBloom'] = 'open';
			// A full-viewport gradient is an expensive paint: keep it on its own layer in flight.
			element.style.willChange = 'transform, opacity';
		} else {
			// Light only ever opens out of a cover: it never drains back into one.
			bloom.scale.target = spec.full;
			bloom.spec = spec;
			bloom.cover = cover;
		}
		// The bloom rides the same curve as the cover it comes from.
		flight = springParams(motionOf(motion));
	}

	function stepBloom(seconds: number): void {
		if (bloom === null) {
			return;
		}
		const { scale, spec, element, cover } = bloom;
		scale.step(flight, seconds);
		const { x, y } = cover.center;
		element.style.transform = `translate(${(x - innerWidth / 2).toFixed(1)}px, ${(y - innerHeight / 2).toFixed(1)}px) scale(${scale.value.toFixed(3)})`;
		element.style.opacity = bloomStrength(bloomOpenness(scale.value, spec), spec.peak).toFixed(3);
	}

	function closeBloom(): void {
		if (bloom !== null) {
			bloom.element.dataset['morphBloom'] = '';
			bloom.element.style.removeProperty('transform');
			bloom.element.style.removeProperty('opacity');
			bloom.element.style.removeProperty('will-change');
			bloom = null;
		}
	}

	/** Reports whether anything still moving holds this element's final position. */
	function held(entry: MorphEntry | null): boolean {
		if (entry === null) {
			return false;
		}
		return [...landing.values()].some(
			(layer) => !layer.settled && layer.element.contains(entry.element)
		);
	}

	function land(id: string, flier: Flier): void {
		if (flier.target !== null) {
			show(flier.target.element);
		}
		flier.element.remove();
		fliers.delete(id);
		waiting.delete(id);
		inFlight.delete(id);
	}

	function tick(now: number): void {
		const seconds = Math.min((now - lastFrame) / 1000, longestFrame);
		lastFrame = now;
		if (!startReported) {
			startReported = true;
			reportStart();
		}

		dealCards(seconds);

		// All reads, then all writes: scrolling mid-flight still lands, without layout thrash.
		island?.measure();
		const carry = island?.offset ?? { x: 0, y: 0 };
		for (const flier of fliers.values()) {
			flier.measure(carry);
		}
		const layers = [
			...landing.values(),
			...ghosts.flatMap((ghost) => ghost.layers),
			...stages.values(),
			...departing
		];
		for (const layer of layers) {
			layer.step(seconds);
			layer.write();
		}
		for (const [id, flier] of fliers) {
			const wait = waiting.get(id);
			if (wait === undefined) {
				flier.step(flight, seconds);
			} else if (wait > seconds) {
				waiting.set(id, wait - seconds);
			} else {
				waiting.delete(id);
				flier.step(flight, seconds - wait);
			}
			flier.write();
		}
		island?.step(flight, seconds);
		island?.write();
		stepBloom(seconds);

		for (const [id, flier] of fliers) {
			if (flier.target === null) {
				flier.element.style.opacity = '0';
				land(id, flier);
			} else if (flier.settled && !held(flier.target)) {
				land(id, flier);
			}
		}
		retire();

		frame = active() ? requestAnimationFrame(tick) : 0;
		if (frame === 0) {
			markMoving(false);
			endDeal();
		}
	}

	function active(): boolean {
		return (
			fliers.size + landing.size + ghosts.length + stages.size + pendingCards.length > 0 ||
			departing.some((layer) => !layer.settled) ||
			island !== null
		);
	}

	/** Sends off every card whose turn has come. */
	function dealCards(seconds: number): void {
		pendingCards = pendingCards.filter((card) => {
			card.wait -= seconds;
			if (card.wait > 0) {
				return true;
			}
			launchCard(card);
			return false;
		});
	}

	function launchCard({ source, target }: DealtCard): void {
		const air = surfaceFor('.morph-layer');
		if (air === null || !source.element.isConnected || !target.element.isConnected) {
			show(target.element);
			return;
		}
		const flier = createFlier(capture(source), target);
		hide(source.element);
		fliers.set(source.id, flier);
		// The one dealt first stays on top, as it was on the pile.
		air.prepend(flier.element);
	}

	/** Lets go of a deal, finished or not: nothing still waiting may stay hidden, or go on waiting. */
	function endDeal(): void {
		for (const { target } of pendingCards) {
			show(target.element);
		}
		pendingCards = [];
		departing = [];
		dealDone?.();
		dealDone = null;
	}

	function deal(
		choreography: Choreography,
		cards: readonly DealtCard[],
		departures: readonly Departure[]
	): Promise<void> {
		for (const element of document.querySelectorAll(`[${hiddenAttribute}]`)) {
			show(element);
		}
		const targets = new Map(cards.map(({ source, target }) => [source.id, target]));
		for (const { element } of targets.values()) {
			hide(element);
		}
		arrive(
			{ choreography, ids: [...targets.keys()], sources: new Map(), waits: new Map() },
			dealPath,
			targets,
			false
		);
		if (choreography.islandForms !== undefined) {
			formIsland(choreography.islandForms);
		}
		departing = departures.map(({ element, part }) => new Layer(element, part));
		pendingCards = cards.map((card) => ({ ...card, wait: card.delay / 1000 }));
		flight = springParams(motionOf(choreography.motion));
		markMoving(true);
		run();
		return new Promise((resolve) => {
			dealDone = resolve;
		});
	}

	/** Lets go of whatever has come to rest. */
	function retire(): void {
		for (const [unit, layer] of landing) {
			if (layer.settled) {
				layer.release();
				landing.delete(unit);
			}
		}
		for (const ghost of [...ghosts]) {
			if (ghost.layers.every((layer) => layer.settled)) {
				dropGhost(ghost);
			}
		}
		for (const [name, stage] of stages) {
			if (stage.settled && fliers.size === 0) {
				stage.release();
				thaw(stage.element);
				stages.delete(name);
			}
		}
		if (island?.settled === true) {
			island.release();
			island = null;
		}
		if (stages.size === 0 && fliers.size === 0) {
			closeBloom();
		}
	}

	function run(): void {
		if (frame === 0) {
			lastFrame = performance.now();
			frame = requestAnimationFrame(tick);
		}
	}

	/** Puts everything down at once: for a blend, or a navigation that fell through. */
	function abandon(): void {
		endDeal();
		for (const [id, flier] of fliers) {
			land(id, flier);
		}
		for (const layer of landing.values()) {
			layer.release();
		}
		landing.clear();
		for (const ghost of ghosts.splice(0)) {
			ghost.element.remove();
		}
		heldStages.clear();
		waiting.clear();
		for (const stage of stages.values()) {
			stage.release();
			thaw(stage.element);
		}
		stages.clear();
		island?.release();
		island = null;
		closeBloom();
		inFlight.clear();
		for (const element of document.querySelectorAll(`[${hiddenAttribute}]`)) {
			show(element);
		}
	}

	async function fly(journey: Journey, navigation: OnNavigate): Promise<void> {
		const { choreography, ids, sources } = journey;
		endDeal();
		markMoving(true);
		const fromPath = navigation.from?.url.pathname ?? '';
		const toPath = navigation.to?.url.pathname ?? '';

		// Everything here runs synchronously in `onNavigate`, while the old page still exists.
		const captures = new Map<string, Capture>();
		for (const [id, source] of sources) {
			if (!fliers.has(id)) {
				captures.set(id, capture(source));
			}
		}
		const reshaping = new Set(
			[...captures.values()].flatMap((taken) => (taken.kind === 'surface' ? [taken.element] : []))
		);
		// From the bottom of the ghost layer up: the stages left, then the page left.
		holdStages(choreography, reshaping);
		const ghost = cloneRoute(fromPath, choreography, sources);
		if (choreography.island !== null) {
			liftIsland(islandLeaves);
		}
		for (const id of ids) {
			inFlight.add(id);
		}
		startReported = false;

		try {
			await navigation.complete;
		} catch {
			// Superseded or failed: the newer navigation owns the stage now.
			if (ghost !== null) {
				dropGhost(ghost);
			}
			if (!active()) {
				markMoving(false);
			}
			return;
		}

		// DOM and scroll are final. Turning round mid-flight, whatever is still in the air may have
		// somewhere to land on this page too, so it turns rather than vanish.
		const turning = [...fliers.keys()].filter((id) => !ids.includes(id));
		const targets = landJourney(
			{ ...journey, ids: [...ids, ...turning] },
			navigation.type !== 'popstate',
			toPath
		);
		for (const entry of registry.of(ids)) {
			if (targets.get(entry.id) !== entry) {
				show(entry.element);
			}
		}
		for (const id of ids) {
			if (!targets.has(id)) {
				inFlight.delete(id);
				for (const element of ghost?.element.querySelectorAll(
					`[${sourceAttribute}="${CSS.escape(id)}"]`
				) ?? []) {
					show(element);
				}
			}
		}

		launch(journey, captures, targets, ghost);
		// Every target is known now: a card that mounts from here on isn't one of them.
		for (const id of ids) {
			inFlight.delete(id);
		}
		moveStages(journey);
		arrive(journey, toPath, targets, true);
		if (choreography.island !== null) {
			landIsland(choreography.island);
		}
		if (choreography.islandForms !== undefined) {
			formIsland(choreography.islandForms);
		}
		openBloom(journey);
		run();
	}

	/**
	 * Blends into the new page, at final positions: it comes up in front of a picture of everything
	 * as it was, which goes as it comes, so even a blend never uncovers what was under it.
	 */
	async function blendTo(changed: Promise<unknown>): Promise<void> {
		abandon();
		const shell = surfaceFor('.shell');
		const main = pageMain();
		const under = surfaceFor('.ghost-layer');
		if (shell === null || main === null || under === null) {
			return;
		}
		markMoving(true);
		const picture = document.createElement('div');
		picture.dataset['morphGhost'] = 'everything';
		picture.style.cssText = `position:fixed;inset:0;overflow:clip;background:var(--ground)`;
		const clone = noLandmark(pictureOfPage(shell, new Set(unitsNearFold(main))), main);
		if (clone instanceof HTMLElement) {
			clone.inert = true;
			clone.style.marginTop = `${String(-scrollY)}px`;
			for (const layer of clone.querySelectorAll('.morph-layer, .ghost-layer')) {
				layer.remove();
			}
			picture.append(clone);
		}
		under.append(picture);
		reportStart();
		await changed.catch(() => undefined);
		const timing = { duration: blendDuration, easing: 'linear' };
		const arriving = [main, surfaceFor('.backdrop')].flatMap((element) =>
			element === null ? [] : [element.animate({ opacity: [0, 1] }, timing)]
		);
		const fade = picture.animate({ opacity: [1, 0] }, timing);
		await Promise.all([fade, ...arriving].map((motion) => motion.finished)).catch(() => undefined);
		picture.remove();
		if (!active()) {
			markMoving(false);
		}
	}

	function navigate(navigation: OnNavigate): void {
		const journey = beginJourney(navigation);
		if (journey === null) {
			return;
		}
		void (journey.choreography.travel === null
			? blendTo(navigation.complete)
			: fly(journey, navigation));
	}

	return { navigate, deal, blend: (change) => blendTo(change()) };
}
