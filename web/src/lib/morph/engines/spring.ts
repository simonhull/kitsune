import type { OnNavigate } from '$app/navigation';
import { Channel } from '../channel.ts';
import {
	blendDuration,
	bloomOpenness,
	bloomStrength,
	choreographyFor,
	rippleDelay,
	rippleWeight,
	springParams,
	weighPart,
	type Choreography,
	type Part,
	type Passage,
	type Role,
	type SpringParams,
	type StageName
} from '../choreography.ts';
import { library } from '#lib/library/library.svelte.ts';
import { reportStart } from '../hud.svelte.ts';
import { beginJourney, centerOf, landJourney, motionOf, type Journey } from '../journey.ts';
import { registry, type MorphEntry } from '../registry.ts';
import {
	BarFlier,
	CoverFlier,
	drawTitle,
	measureBar,
	measureCover,
	measureTitle,
	TitleFlier,
	type BarShape,
	type CoverShape,
	type Flier,
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
/** A bloom cut short clears from wherever it had got to, quickly: something else is starting. */
const bloomLeaves: Part = {
	from: {},
	to: { opacity: 0 },
	delay: 0,
	motion: { token: '--morph-fast', bounce: 0, pace: 0.7 }
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

type Capture = CoverCapture | TitleCapture | BarCapture;

function capture(source: MorphEntry): Capture {
	const { element, kind } = source;
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
			picture.appendChild(whole);
		}
	}
	return picture;
}

function roleOf(unit: HTMLElement): Role {
	return unit.dataset['morphPart'] as Role;
}

function distance(unit: HTMLElement, from: { x: number; y: number }): number {
	const { x, y } = centerOf(unit);
	return Math.hypot(x - from.x, y - from.y);
}

/** Scales each unit's part by where it sits relative to the cover. */
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
		if (plain.ripple === undefined || away === undefined) {
			return { part: plain, delay: plain.delay };
		}
		const share = away / farthest;
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

function viewportCenter(): Point {
	return { x: innerWidth / 2, y: innerHeight / 2 };
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
	const landing = new Map<HTMLElement, Layer>();
	const ghosts: Ghost[] = [];
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

	function cloneRoute(path: string, part: Part, sources: Journey['sources']): Ghost | null {
		const main = surfaceFor('main');
		const under = surfaceFor('.ghost-layer');
		if (main === null || under === null) {
			return null;
		}
		for (const [id, { element }] of sources) {
			element.setAttribute(sourceAttribute, id);
			hide(element);
		}
		const { left, top, width, height } = main.getBoundingClientRect();
		const real = unitsNearFold(main);
		const clone = pictureOfPage(main, new Set(real));
		if (!(clone instanceof HTMLElement)) {
			return null;
		}
		for (const { element } of sources.values()) {
			element.removeAttribute(sourceAttribute);
		}

		clone.inert = true;
		clone.setAttribute('aria-hidden', 'true');
		clone.style.cssText = `position:absolute;left:${String(left)}px;top:${String(top)}px;width:${String(width)}px;height:${String(height)}px;z-index:auto`;
		keepColors(main, clone);
		under.append(clone);

		const cover = [...sources.values()].find((entry) => entry.kind === 'cover');
		const center = cover === undefined ? null : centerOf(cover.element);
		const units = unitsOf(clone);
		// A page with no units yet, still loading say, leaves as one.
		const movers = units.length > 0 ? units : [clone];
		const wave = shapeWave(movers, () => part, center);
		const layers = movers.map((unit, index) => {
			const realUnit = real.at(index);
			// A unit still arriving is at some pose: its ghost starts there, not at rest.
			const inherited = realUnit === undefined ? undefined : landing.get(realUnit);
			if (center !== null) {
				pivot(unit, center);
			}
			const shaped = wave.at(index);
			const layer = new Layer(unit, shaped?.part ?? part, shaped?.delay);
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

	/** Sends a cover past the viewer, with no target and no way of coming back. */
	function passBy(id: string, source: Capture, passage: Passage): void {
		const air = surfaceFor('.morph-layer');
		if (air === null || source.kind === 'title' || source.kind === 'bar') {
			return;
		}
		const flier = new CoverFlier(source.shape, null, source.picture, source.shadow);
		flier.pass(passage, viewportCenter(), springParams(motionOf(passage.motion)));
		fliers.set(id, flier);
		air.prepend(flier.element);
	}

	function launch(
		journey: Journey,
		captures: Map<string, Capture>,
		targets: Map<string, MorphEntry>
	) {
		const air = surfaceFor('.morph-layer');
		if (air === null) {
			return;
		}
		for (const [id, target] of targets) {
			hide(target.element);
			const flying = fliers.get(id);
			if (flying !== undefined) {
				flying.target = target;
				continue;
			}
			const source = captures.get(id);
			if (source === undefined) {
				show(target.element);
				inFlight.delete(id);
				continue;
			}
			const flier = createFlier(source, target);
			const arc = journey.choreography.way?.arc;
			if (arc !== undefined && flier instanceof CoverFlier) {
				const { duration, bounce } = motionOf(journey.choreography.motion);
				flier.curve(arc, springParams({ duration: duration * arc.sizing, bounce }));
			}
			fliers.set(id, flier);
			// Words stay in front of art, so a title never tucks behind the cover it travels with,
			// and the fox bar passes under the controls it crosses on the way.
			if (target.kind === 'cover' || target.kind === 'bar') {
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

	function arrive(journey: Journey, path: string, targets: Map<string, MorphEntry>): void {
		const main = surfaceFor('main');
		if (main === null) {
			return;
		}
		const { arriving, arrivingByKey, motion, way } = journey.choreography;
		const partOf = (unit: HTMLElement): Part | undefined =>
			arrivingByKey?.[unit.dataset['morphKey'] ?? ''] ?? arriving[roleOf(unit)];
		const units = unitsNearFold(main);
		const cover = [...targets.values()].find((entry) => entry.kind === 'cover');
		const center =
			way?.passes !== undefined
				? viewportCenter()
				: cover === undefined
					? null
					: centerOf(cover.element);
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
				const part: Part = arriving[roleOf(unit)] ?? { from: {}, to: {}, delay: 0, motion };
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

		const moving = units.filter((unit) => partOf(unit) !== undefined && !flying.has(unit));
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

	function moveStages(journey: Journey, frozen: Map<StageName, number>, held: boolean): void {
		for (const [name, part] of Object.entries(journey.choreography.stages) as [StageName, Part][]) {
			const element = surfaceFor(`[data-morph-stage="${name}"]`);
			if (element === null || (held && name === 'album')) {
				continue;
			}
			const stage = stages.get(name);
			if (stage === undefined) {
				stages.set(name, new Layer(element, part));
			} else {
				stage.retarget(part);
			}
			// Only a leaving stage is pinned: an arriving one scrolls with its page, as at rest.
			const top = frozen.get(name);
			if (part.to.opacity !== 0) {
				thaw(element);
			} else if (element.style.position === '' && top !== undefined) {
				freeze(element, top);
			}
		}
	}

	/**
	 * Leaving one album for another's Now playing: the stage would change color in a single
	 * frame. A picture of the old one stays to be covered, and the real one is free to change.
	 */
	function holdStage(stage: HTMLElement, part: Part): void {
		const clone = stage.cloneNode(true);
		if (!(clone instanceof HTMLElement)) {
			return;
		}
		clone.removeAttribute('data-morph-stage');
		freeze(clone, stage.getBoundingClientRect().top);
		stage.after(clone);
		ghosts.push({ path: '', element: clone, layers: [new Layer(clone, part)], units: new Map() });
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
		const out = spec.direction === 'out';
		if (bloom === null) {
			bloom = {
				element,
				cover,
				scale: new Channel(out ? spec.rest : spec.full, out ? spec.full : spec.rest),
				spec
			};
			element.dataset['morphBloom'] = 'open';
			// A full-viewport gradient is an expensive paint: keep it on its own layer in flight.
			element.style.willChange = 'transform, opacity';
		} else {
			bloom.scale.target = out ? spec.full : spec.rest;
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
		element.style.opacity = bloomStrength(
			bloomOpenness(scale.value, spec),
			spec.peak,
			spec.clears
		).toFixed(3);
	}

	/** Reports whether the bloom is still clearing, after what it came from has gone. */
	function bloomClearing(): boolean {
		const clears = bloom?.spec.clears;
		return (
			bloom !== null &&
			clears !== undefined &&
			bloomOpenness(bloom.scale.value, bloom.spec) < clears[1]
		);
	}

	/**
	 * A bloom still clearing leaves as it is, in the colors it has: the real one is about to change
	 * album, and starting it over from here would jump. A journey that interrupts it gets a new one.
	 */
	function letBloomGo(): void {
		const under = surfaceFor('.ghost-layer');
		if (bloom === null || under === null || !bloomClearing()) {
			return;
		}
		const { element } = bloom;
		const clone = element.cloneNode(true);
		if (clone instanceof HTMLElement) {
			const style = getComputedStyle(element);
			for (const name of ['--glow', '--field']) {
				clone.style.setProperty(name, style.getPropertyValue(name));
			}
			const leaving = document.createElement('div');
			leaving.style.cssText = 'position:fixed;inset:0';
			leaving.append(clone);
			under.append(leaving);
			const layer = new Layer(leaving, bloomLeaves);
			ghosts.push({ path: '', element: leaving, layers: [layer], units: new Map() });
		}
		closeBloom();
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
		for (const flier of fliers.values()) {
			flier.step(flight, seconds);
			flier.write();
		}
		island?.step(flight, seconds);
		island?.write();
		stepBloom(seconds);

		for (const [id, flier] of fliers) {
			if (flier.target === null && flier.leavesAlone !== true) {
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
			island !== null ||
			bloomClearing()
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
		arrive({ choreography, ids: [...targets.keys()], sources: new Map() }, dealPath, targets);
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
				ghost.element.remove();
				ghosts.splice(ghosts.indexOf(ghost), 1);
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
		if (stages.size === 0 && fliers.size === 0 && !bloomClearing()) {
			closeBloom();
		}
	}

	function run(): void {
		if (document.hidden) {
			putDown();
		} else if (frame === 0) {
			lastFrame = performance.now();
			frame = requestAnimationFrame(tick);
		}
	}

	/** Nobody is watching and no frames are coming: what's waiting for the end mustn't wait with it. */
	function putDown(): void {
		cancelAnimationFrame(frame);
		frame = 0;
		abandon();
		markMoving(false);
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

	/**
	 * Interrupted, a way back has no idea left to play: what's in the air finds its own way to settle
	 * from where it is, on the plain journey. Otherwise only the cover travels.
	 */
	function chooseWay(journey: Journey): Journey {
		if (journey.choreography.way === undefined) {
			return journey;
		}
		if (journey.ids.some((id) => fliers.has(id))) {
			return { ...journey, choreography: choreographyFor('album', 'home') };
		}
		const ids = journey.ids.filter((id) => id.startsWith('cover:'));
		const sources = new Map([...journey.sources].filter(([id]) => ids.includes(id)));
		return { ...journey, ids, sources };
	}

	async function fly(planned: Journey, navigation: OnNavigate): Promise<void> {
		const journey = chooseWay(planned);
		const { choreography, ids, sources } = journey;
		endDeal();
		letBloomGo();
		markMoving(true);
		const fromPath = navigation.from?.url.pathname ?? '';
		const toPath = navigation.to?.url.pathname ?? '';

		// Everything here runs synchronously in `onNavigate`, while the old page still exists.
		const frozen = new Map<StageName, number>();
		const albumStage = surfaceFor('[data-morph-stage="album"]');
		if (albumStage !== null) {
			frozen.set('album', albumStage.getBoundingClientRect().top);
		}
		const captures = new Map<string, Capture>();
		for (const [id, source] of sources) {
			if (!fliers.has(id)) {
				captures.set(id, capture(source));
			}
		}
		const fromAlbum = navigation.from?.params?.['id'];
		const heldStage = choreography.stages.album;
		const changesAlbum =
			choreography.name === 'album-to-now-playing' && fromAlbum !== library.playing?.album.id;
		if (changesAlbum && albumStage !== null && heldStage !== undefined) {
			holdStage(albumStage, heldStage);
		}
		const ghost = cloneRoute(fromPath, choreography.leaving, sources);
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
				ghost.element.remove();
				ghosts.splice(ghosts.indexOf(ghost), 1);
			}
			if (!active()) {
				markMoving(false);
			}
			return;
		}

		// DOM and scroll are final.
		const targets = landJourney(journey, navigation.type !== 'popstate');
		const passage = choreography.way?.passes;
		const passing = passage === undefined ? [] : ids.filter((id) => captures.has(id));
		for (const id of passing) {
			targets.delete(id);
		}
		for (const entry of registry.list()) {
			if (ids.includes(entry.id) && targets.get(entry.id) !== entry) {
				show(entry.element);
			}
		}
		for (const id of ids) {
			if (!targets.has(id) && !passing.includes(id)) {
				inFlight.delete(id);
				for (const element of ghost?.element.querySelectorAll(
					`[${sourceAttribute}="${CSS.escape(id)}"]`
				) ?? []) {
					show(element);
				}
			}
		}

		launch(journey, captures, targets);
		for (const id of passing) {
			const source = captures.get(id);
			if (source !== undefined && passage !== undefined) {
				passBy(id, source, passage);
			}
		}
		// Every target is known now: a card that mounts from here on isn't one of them.
		for (const id of ids) {
			inFlight.delete(id);
		}
		moveStages(journey, frozen, changesAlbum);
		arrive(journey, toPath, targets);
		if (choreography.island !== null) {
			landIsland(choreography.island);
		}
		if (choreography.islandForms !== undefined) {
			formIsland(choreography.islandForms);
		}
		openBloom(journey);
		run();
	}

	/** Blends a picture of everything into the new page, at final positions. */
	async function blendTo(changed: Promise<unknown>): Promise<void> {
		abandon();
		const shell = surfaceFor('.shell');
		const main = surfaceFor('main');
		const air = surfaceFor('.morph-layer');
		if (shell === null || main === null || air === null) {
			return;
		}
		markMoving(true);
		const picture = document.createElement('div');
		picture.style.cssText = `position:fixed;inset:0;overflow:clip;background:var(--ground)`;
		const clone = pictureOfPage(shell, new Set(unitsNearFold(main)));
		if (clone instanceof HTMLElement) {
			clone.inert = true;
			clone.style.marginTop = `${String(-scrollY)}px`;
			clone.querySelector('.morph-layer')?.remove();
			picture.append(clone);
		}
		air.append(picture);
		reportStart();
		await changed.catch(() => undefined);
		const fade = picture.animate(
			{ opacity: [1, 0] },
			{ duration: blendDuration, easing: 'linear' }
		);
		await fade.finished.catch(() => undefined);
		picture.remove();
		if (!active()) {
			markMoving(false);
		}
	}

	document.addEventListener('visibilitychange', () => {
		if (document.hidden && frame !== 0) {
			putDown();
		}
	});

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
