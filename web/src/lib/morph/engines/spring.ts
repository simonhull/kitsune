import type { OnNavigate } from '$app/navigation';
import { Channel } from '../channel.ts';
import {
	blendDuration,
	bloomOpenness,
	bloomStrength,
	rippleDelay,
	rippleWeight,
	springParams,
	weighPart,
	type Choreography,
	type Part,
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
		return (element.firstElementChild ?? element).cloneNode(true);
	}
	const picture = element.cloneNode(true);
	if (picture instanceof HTMLElement) {
		picture.removeAttribute(hiddenAttribute);
		picture.style.cssText = 'position:absolute;inset:0;width:100%;height:100%;margin:0';
	}
	return picture;
}

/** The outgoing page, as an inert picture of itself. */
interface Ghost {
	path: string;
	element: HTMLElement;
	/** One layer per unit, in document order, so the real page can take over unit by unit. */
	layers: Layer[];
}

/** Finds the parts of a page that move on their own. */
function unitsOf(root: Element): HTMLElement[] {
	return [...root.querySelectorAll<HTMLElement>('[data-morph-part]')];
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

/** The spring engine: real elements, a persistent overlay, and physics that can turn around. */
export function createSpringEngine(): (navigation: OnNavigate) => void {
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
		const clone = main.cloneNode(true);
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
		const real = unitsOf(main);
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
		for (const unit of real) {
			landing.delete(unit);
		}
		const ghost = { path, element: clone, layers };
		ghosts.push(ghost);
		return ghost;
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
			return new BarFlier(source.shape, target, source.color);
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
		const to = measureCover(target.element);
		const growing = to.width >= source.shape.width;
		return new CoverFlier(
			source.shape,
			target,
			growing ? pictureOf(target) : source.picture,
			growing ? getComputedStyle(target.element).boxShadow : source.shadow
		);
	}

	function arrive(journey: Journey, path: string, targets: Map<string, MorphEntry>): void {
		const main = surfaceFor('main');
		if (main === null) {
			return;
		}
		const { arriving, motion } = journey.choreography;
		const units = unitsOf(main);
		const cover = [...targets.values()].find((entry) => entry.kind === 'cover');
		const center = cover === undefined ? null : centerOf(cover.element);
		const flying = new Set([...targets.values()].map((entry) => entry.element));

		// Going back mid-morph: the real page takes over from its own ghost, unit by unit.
		const returning = ghosts.findIndex((ghost) => ghost.path === path);
		const ghost = ghosts.at(returning);
		if (returning >= 0 && ghost !== undefined) {
			ghosts.splice(returning, 1);
			for (const [index, unit] of units.entries()) {
				const from = ghost.layers.at(index);
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

		const moving = units.filter(
			(unit) => arriving[roleOf(unit)] !== undefined && !flying.has(unit)
		);
		const wave = shapeWave(moving, (unit) => arriving[roleOf(unit)] as Part, center);
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
		ghosts.push({ path: '', element: clone, layers: [new Layer(clone, part)] });
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

		const { left, top, width, height } = content.getBoundingClientRect();
		const clone = content.cloneNode(true);
		if (!(clone instanceof HTMLElement)) {
			return;
		}
		clone.inert = true;
		clone.style.cssText = `position:absolute;left:${String(left)}px;top:${String(top)}px;width:${String(width)}px;height:${String(height)}px;margin:0`;
		air.prepend(clone);
		const layer = new Layer(clone, part);
		const inherited = landing.get(content);
		if (inherited !== undefined) {
			layer.adopt(inherited);
		}
		landing.delete(content);
		ghosts.push({ path: '', element: clone, layers: [layer] });
	}

	function landIsland(part: Part): void {
		island?.aim();
		island?.measure();
		island?.write();
		const content = surfaceFor('[data-island-content]');
		if (content !== null) {
			landing.set(content, new Layer(content, part));
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
		inFlight.delete(id);
	}

	function tick(now: number): void {
		const seconds = Math.min((now - lastFrame) / 1000, longestFrame);
		lastFrame = now;
		if (!startReported) {
			startReported = true;
			reportStart();
		}

		// All reads, then all writes: scrolling mid-flight still lands, without layout thrash.
		for (const flier of fliers.values()) {
			flier.measure();
		}
		island?.measure();
		const layers = [
			...landing.values(),
			...ghosts.flatMap((ghost) => ghost.layers),
			...stages.values()
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
			if (flier.target === null) {
				flier.element.style.opacity = '0';
				land(id, flier);
			} else if (flier.settled && !held(flier.target)) {
				land(id, flier);
			}
		}
		retire();

		frame = active() ? requestAnimationFrame(tick) : 0;
	}

	function active(): boolean {
		return fliers.size + landing.size + ghosts.length + stages.size > 0 || island !== null;
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

	async function fly(journey: Journey, navigation: OnNavigate): Promise<void> {
		const { choreography, ids, sources } = journey;
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
			return;
		}

		// DOM and scroll are final.
		const targets = landJourney(journey, navigation.type !== 'popstate');
		for (const entry of registry.list()) {
			if (ids.includes(entry.id) && targets.get(entry.id) !== entry) {
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

		launch(journey, captures, targets);
		moveStages(journey, frozen, changesAlbum);
		arrive(journey, toPath, targets);
		if (choreography.island !== null) {
			landIsland(choreography.island);
		}
		openBloom(journey);
		run();
	}

	/** Blends a picture of everything into the new page, at final positions. */
	async function blendTo(navigation: OnNavigate): Promise<void> {
		abandon();
		const shell = surfaceFor('.shell');
		const air = surfaceFor('.morph-layer');
		if (shell === null || air === null) {
			return;
		}
		const picture = document.createElement('div');
		picture.style.cssText = `position:fixed;inset:0;overflow:clip;background:var(--ground)`;
		const clone = shell.cloneNode(true);
		if (clone instanceof HTMLElement) {
			clone.inert = true;
			clone.style.marginTop = `${String(-scrollY)}px`;
			clone.querySelector('.morph-layer')?.remove();
			picture.append(clone);
		}
		air.append(picture);
		reportStart();
		await navigation.complete.catch(() => undefined);
		const fade = picture.animate(
			{ opacity: [1, 0] },
			{ duration: blendDuration, easing: 'linear' }
		);
		await fade.finished.catch(() => undefined);
		picture.remove();
	}

	return (navigation) => {
		const journey = beginJourney(navigation);
		if (journey === null) {
			return;
		}
		void (journey.choreography.travel === null ? blendTo(navigation) : fly(journey, navigation));
	};
}
