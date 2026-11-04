import type { OnNavigate } from '$app/navigation';
import { Channel } from '../channel.ts';
import {
	blendDuration,
	bloomOpenness,
	bloomStrength,
	springParams,
	type Choreography,
	type Part,
	type SpringParams
} from '../choreography.ts';
import type { Engine } from '../engine.ts';
import { reportStart } from '../hud.svelte.ts';
import { beginJourney, centerOf, landJourney, motionOf, type Journey } from '../journey.ts';
import { registry, type MorphEntry } from '../registry.ts';
import {
	CoverFlier,
	drawTitle,
	measureCover,
	measureTitle,
	TitleFlier,
	type CoverShape,
	type Flier,
	type TitleMeasure
} from './flier.ts';
import { Layer } from './layer.ts';

const hiddenAttribute = 'data-morph-hidden';
const sourceAttribute = 'data-morph-source';

/** The largest gap one frame may jump: a stalled main thread shouldn't teleport anything. */
const longestFrame = 1 / 20;

function hide(element: HTMLElement): void {
	element.setAttribute(hiddenAttribute, '');
}

function show(element: Element): void {
	element.removeAttribute(hiddenAttribute);
}

interface CoverCapture {
	kind: 'cover';
	shape: CoverShape;
	picture: Node;
	shadow: string;
}

interface TitleCapture {
	kind: 'title';
	measure: TitleMeasure;
	picture: HTMLElement;
}

type Capture = CoverCapture | TitleCapture;

function capture(source: MorphEntry): Capture {
	const { element, kind } = source;
	if (kind === 'title') {
		return { kind, measure: measureTitle(element), picture: drawTitle(element) };
	}
	return {
		kind,
		shape: measureCover(element),
		picture: pictureOf(element),
		shadow: getComputedStyle(element).boxShadow
	};
}

function pictureOf(element: HTMLElement): Node {
	return (element.firstElementChild ?? element).cloneNode(true);
}

/** The outgoing page, as an inert picture of itself. */
interface Ghost {
	path: string;
	layer: Layer;
}

/** The light around the cover, following it through the air. */
interface Bloom {
	element: HTMLElement;
	cover: CoverFlier;
	scale: Channel;
	spec: NonNullable<Choreography['bloom']>;
}

function pivot(element: HTMLElement, around: { x: number; y: number }): void {
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
export function createSpringEngine(): Engine {
	const fliers = new Map<string, Flier>();
	const landing = new Set<Layer>();
	const ghosts: Ghost[] = [];
	const inFlight = new Set<string>();
	let flight: SpringParams = springParams({ duration: 420, bounce: 0 });
	let page: Layer | null = null;
	let stage: Layer | null = null;
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
		// A page still arriving is at some pose: its ghost starts there, not at rest.
		const inherited = page;
		page?.release();
		page = null;

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
		under.append(clone);

		const layer = new Layer(clone, part);
		if (inherited !== null) {
			layer.adopt(inherited);
		}
		const cover = [...sources.values()].find((entry) => entry.kind === 'cover');
		if (cover !== undefined) {
			pivot(clone, centerOf(cover.element));
		}
		const ghost = { path, layer };
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
			// Covers fly above titles, whichever launched first.
			if (flier instanceof CoverFlier) {
				air.append(flier.element);
			} else {
				air.prepend(flier.element);
			}
		}
		// A flier whose page has gone has nowhere to land: it leaves with the ghost.
		for (const [id, flier] of fliers) {
			if (!targets.has(id)) {
				flier.target = null;
			}
		}
		flight = springParams(motionOf(journey.choreography.motion));
	}

	function createFlier(source: Capture, target: MorphEntry): Flier {
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
			growing ? pictureOf(target.element) : source.picture,
			growing ? getComputedStyle(target.element).boxShadow : source.shadow
		);
	}

	function arrive(journey: Journey, path: string, targets: Map<string, MorphEntry>): void {
		const main = surfaceFor('main');
		const { choreography } = journey;
		const returning = ghosts.findIndex((ghost) => ghost.path === path);
		const cover = [...targets.values()].find((entry) => entry.kind === 'cover');

		// Going back mid-morph: the real page takes over from its own ghost, wherever it had got to.
		const ghost = ghosts.at(returning);
		if (returning >= 0 && ghost !== undefined && main !== null) {
			ghosts.splice(returning, 1);
			const part = choreography.arriving.page ?? settle(choreography);
			page = new Layer(main, part);
			page.adopt(ghost.layer);
			page.retarget({ ...part, to: {} });
			if (cover !== undefined) {
				pivot(main, centerOf(cover.element));
			}
			ghost.layer.element.remove();
			return;
		}

		for (const [name, part] of Object.entries(choreography.arriving)) {
			const element = name === 'page' ? main : surfaceFor(`[data-morph-part="${name}"]`);
			if (element === null) {
				continue;
			}
			const layer = new Layer(element, part);
			if (name === 'page') {
				page = layer;
				if (cover !== undefined) {
					pivot(element, centerOf(cover.element));
				}
			} else {
				landing.add(layer);
			}
		}
	}

	function settle({ motion }: Choreography): Part {
		return { from: {}, to: {}, delay: 0, motion };
	}

	function moveStage(journey: Journey, leaving: boolean, frozenTop: number): void {
		const { stage: part } = journey.choreography;
		const element = surfaceFor('[data-morph-stage]');
		if (part === null || element === null) {
			return;
		}
		if (stage === null) {
			stage = new Layer(element, part);
		} else {
			stage.retarget(part);
		}
		// Only a leaving stage is pinned: an arriving one scrolls with its page, as it will at rest.
		if (!leaving) {
			thaw(element);
		} else if (element.style.position === '') {
			freeze(element, frozenTop);
		}
	}

	function openBloom(journey: Journey): void {
		const { bloom: spec, motion } = journey.choreography;
		const element = surfaceFor('[data-morph-bloom]');
		const cover = [...fliers.values()].find((flier) => flier instanceof CoverFlier);
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
			bloom = null;
		}
	}

	/** Reports whether anything still moving holds this element's final position. */
	function held(entry: MorphEntry | null): boolean {
		if (entry === null) {
			return false;
		}
		const pose = [...landing, ...(page === null ? [] : [page])];
		return pose.some((layer) => !layer.settled && layer.element.contains(entry.element));
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
		const layers = [...landing, ...ghosts.map((ghost) => ghost.layer)];
		if (page !== null) {
			layers.push(page);
		}
		if (stage !== null) {
			layers.push(stage);
		}
		for (const layer of layers) {
			layer.step(seconds);
			layer.write();
		}
		for (const flier of fliers.values()) {
			flier.step(flight, seconds);
			flier.write();
		}
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
		return fliers.size + landing.size + ghosts.length > 0 || page !== null || stage !== null;
	}

	/** Lets go of whatever has come to rest. */
	function retire(): void {
		for (const layer of landing) {
			if (layer.settled) {
				layer.release();
				landing.delete(layer);
			}
		}
		for (const ghost of [...ghosts]) {
			if (ghost.layer.settled) {
				ghost.layer.element.remove();
				ghosts.splice(ghosts.indexOf(ghost), 1);
			}
		}
		if (page?.settled === true) {
			page.release();
			page = null;
		}
		if (stage?.settled === true && fliers.size === 0) {
			stage.release();
			thaw(stage.element);
			stage = null;
			closeBloom();
		}
		if (stage === null && fliers.size === 0) {
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
		for (const layer of landing) {
			layer.release();
		}
		landing.clear();
		for (const ghost of ghosts.splice(0)) {
			ghost.layer.element.remove();
		}
		page?.release();
		page = null;
		if (stage !== null) {
			stage.release();
			thaw(stage.element);
			stage = null;
		}
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
		const stageElement = surfaceFor('[data-morph-stage]');
		const frozenTop = stageElement?.getBoundingClientRect().top ?? 0;
		const captures = new Map<string, Capture>();
		for (const [id, source] of sources) {
			if (!fliers.has(id)) {
				captures.set(id, capture(source));
			}
		}
		const ghost = cloneRoute(fromPath, choreography.leaving, sources);
		for (const id of ids) {
			inFlight.add(id);
		}
		startReported = false;

		try {
			await navigation.complete;
		} catch {
			// Superseded or failed: the newer navigation owns the stage now.
			if (ghost !== null) {
				ghost.layer.element.remove();
				ghosts.splice(ghosts.indexOf(ghost), 1);
			}
			return;
		}

		// DOM and scroll are final.
		const targets = landJourney(journey, navigation.type !== 'popstate');
		const leavingStage = choreography.name === 'album-to-home';
		for (const entry of registry.list()) {
			if (ids.includes(entry.id) && targets.get(entry.id) !== entry) {
				show(entry.element);
			}
		}
		for (const id of ids) {
			if (!targets.has(id)) {
				inFlight.delete(id);
				for (const element of ghost?.layer.element.querySelectorAll(
					`[${sourceAttribute}="${CSS.escape(id)}"]`
				) ?? []) {
					show(element);
				}
			}
		}

		launch(journey, captures, targets);
		moveStage(journey, leavingStage, frozenTop);
		arrive(journey, toPath, targets);
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

	return {
		name: 'spring',
		navigate(navigation) {
			const journey = beginJourney(navigation);
			if (journey === null) {
				return;
			}
			void (journey.choreography.travel === null ? blendTo(navigation) : fly(journey, navigation));
		}
	};
}
