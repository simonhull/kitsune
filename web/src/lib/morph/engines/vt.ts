import type { OnNavigate } from '$app/navigation';
import {
	blendDuration,
	linearEasing,
	settleTime,
	type Choreography,
	type Part,
	type Pose
} from '../choreography.ts';
import type { Engine } from '../engine.ts';
import { beginJourney, centerOf, landJourney, motionOf, type Journey } from '../journey.ts';
import type { MorphEntry } from '../registry.ts';

/**
 * Turns a registry id into a `view-transition-name`. Anything else, a digit first or a stray
 * character, aborts the whole transition.
 */
export function viewTransitionName(id: string): string {
	return `m-${id.replace(/[^a-zA-Z0-9_-]/g, '-')}`;
}

/** Stacking, bottom to top. Everything above the opaque root must carry the shell with it. */
const layers = {
	stage: 1,
	bloom: 2,
	page: 3,
	shell: 6,
	title: 7,
	cover: 8
} as const;

class Names {
	readonly #named = new Set<HTMLElement>();

	set(element: Element | null, name: string): void {
		if (element instanceof HTMLElement) {
			element.style.setProperty('view-transition-name', name);
			this.#named.add(element);
		}
	}

	clear(): void {
		for (const element of this.#named) {
			element.style.removeProperty('view-transition-name');
		}
		this.#named.clear();
	}
}

function transform({ y = 0, scale = 1 }: Pose): string {
	return `translateY(${String(y)}px) scale(${String(scale)})`;
}

function partRule(selector: string, keyframes: string, part: Part): string {
	const motion = motionOf(part.motion);
	const { opacity: fromOpacity = 1 } = part.from;
	const { opacity: toOpacity = 1 } = part.to;
	return `
@keyframes ${keyframes} {
	from { opacity: ${String(fromOpacity)}; transform: ${transform(part.from)}; }
	to { opacity: ${String(toOpacity)}; transform: ${transform(part.to)}; }
}
${selector} {
	animation: ${keyframes} ${String(settleTime(motion))}ms ${linearEasing(motion)} ${String(part.delay)}ms both;
	mix-blend-mode: normal;
}`;
}

function layer(name: string, zIndex: number): string {
	return `::view-transition-group(${name}) { z-index: ${String(zIndex)}; }`;
}

interface Shape {
	radius: string;
	shadow: string;
}

function shapeOf(element: HTMLElement): Shape {
	const { borderTopLeftRadius, boxShadow } = getComputedStyle(element);
	return { radius: borderTopLeftRadius, shadow: boxShadow };
}

/** How a shared element's box changes: the corners and the shadow, never scaled. */
function shapeRule(name: string, from: Shape, to: Shape, flight: string): string {
	return `
@keyframes ${name}-shape {
	from { border-radius: ${from.radius}; box-shadow: ${from.shadow}; }
	to { border-radius: ${to.radius}; box-shadow: ${to.shadow}; }
}
::view-transition-group(${name}) {
	animation-name: -ua-view-transition-group-anim-${name}, ${name}-shape;
	${flight}
	overflow: clip;
}`;
}

/** Fades the smaller snapshot within the first quarter of flight, mirrored on the way back. */
function imageRules(name: string, growing: boolean, settle: number, fit: string): string {
	const small = growing ? 'old' : 'new';
	const large = growing ? 'new' : 'old';
	const window = Math.round(settle * 0.25);
	const delay = growing ? 0 : settle - window;
	return `
@keyframes ${name}-fade { from { opacity: ${growing ? '1' : '0'}; } to { opacity: ${growing ? '0' : '1'}; } }
::view-transition-old(${name}), ::view-transition-new(${name}) {
	width: 100%;
	height: 100%;
	${fit}
	mix-blend-mode: normal;
}
::view-transition-${large}(${name}) { animation: none; opacity: 1; }
::view-transition-${small}(${name}) {
	animation: ${name}-fade ${String(window)}ms linear ${String(delay)}ms both;
}`;
}

interface Captured {
	width: number;
	shape: Shape;
}

function pairRules(entry: MorphEntry, source: Captured, flight: string, settle: number): string {
	const name = viewTransitionName(entry.id);
	const growing = entry.element.getBoundingClientRect().width >= source.width;
	if (entry.kind === 'cover') {
		return [
			layer(name, layers.cover),
			shapeRule(name, source.shape, shapeOf(entry.element), flight),
			imageRules(name, growing, settle, 'object-fit: cover;')
		].join('\n');
	}
	return [
		layer(name, layers.title),
		`::view-transition-group(${name}) { ${flight} }`,
		imageRules(name, growing, settle, 'object-fit: contain; object-position: left center;')
	].join('\n');
}

/**
 * Places the bloom. It's a disc in the middle of the viewport, because Chromium truncates the
 * snapshot of anything much larger: it's soft, so scaling it far past 1 costs nothing.
 */
function bloomPose(at: { x: number; y: number }, scale: number): string {
	const dx = at.x - innerWidth / 2;
	const dy = at.y - innerHeight / 2;
	return `translate(${String(dx)}px, ${String(dy)}px) scale(${String(scale)})`;
}

function bloomRules(
	choreography: Choreography,
	from: { x: number; y: number },
	to: { x: number; y: number }
): string {
	const { bloom, motion } = choreography;
	if (bloom === null) {
		return '';
	}
	const curve = motionOf(motion);
	const out = bloom.direction === 'out';
	const start = bloomPose(from, out ? bloom.rest : bloom.full);
	const end = bloomPose(to, out ? bloom.full : bloom.rest);
	return `
@keyframes m-bloom {
	0% { opacity: ${String(out ? 0 : bloom.peak)}; transform: ${start}; }
	25% { opacity: ${String(bloom.peak)}; }
	100% { opacity: 0; transform: ${end}; }
}
::view-transition-new(m-bloom) {
	animation: m-bloom ${String(settleTime(curve))}ms ${linearEasing(curve)} both;
	mix-blend-mode: normal;
}`;
}

function openBloom(): HTMLElement | null {
	const bloom = document.querySelector<HTMLElement>('[data-morph-bloom]');
	if (bloom !== null) {
		bloom.dataset['morphBloom'] = 'open';
	}
	return bloom;
}

function closeBloom(): void {
	const bloom = document.querySelector<HTMLElement>('[data-morph-bloom]');
	if (bloom !== null) {
		bloom.dataset['morphBloom'] = '';
	}
}

function originFor(element: Element, within: Element): string {
	const center = centerOf(element);
	const { left, top } = within.getBoundingClientRect();
	return `${String(center.x - left)}px ${String(center.y - top)}px`;
}

/** Builds the stylesheet for one flight. It exists only while that flight does. */
function flightStyles(
	journey: Journey,
	targets: Map<string, MorphEntry>,
	captured: Map<string, Captured>,
	names: Names,
	page: HTMLElement | null,
	origin: string
): string {
	const { choreography, sources } = journey;
	const curve = motionOf(choreography.motion);
	const settle = settleTime(curve);
	const flight = `animation-duration: ${String(settle)}ms; animation-timing-function: ${linearEasing(curve)};`;
	const rules: string[] = [];

	for (const id of sources.keys()) {
		const target = targets.get(id);
		const source = captured.get(id);
		const name = viewTransitionName(id);
		if (target === undefined || source === undefined) {
			// No card to go home to: it falls back with the page.
			rules.push(layer(name, layers.page));
			rules.push(partRule(`::view-transition-old(${name})`, `${name}-gone`, choreography.leaving));
			continue;
		}
		names.set(target.element, name);
		rules.push(pairRules(target, source, flight, settle));
		if (target.kind === 'cover') {
			target.element.setAttribute('data-morph-square', '');
		}
	}

	for (const [part, spec] of Object.entries(choreography.arriving)) {
		const element = part === 'page' ? page : document.querySelector(`[data-morph-part="${part}"]`);
		const name = `m-arriving-${part}`;
		names.set(element, name);
		// Chromium keeps a snapshot's backdrop blur, at full strength even while it's invisible.
		element?.setAttribute('data-morph-square', 'flat');
		rules.push(layer(name, layers.page));
		const pivot =
			part === 'page' && element !== null
				? `::view-transition-new(${name}) { transform-origin: ${originFor(firstOf(targets) ?? element, element)}; }`
				: '';
		rules.push(partRule(`::view-transition-new(${name})`, `${name}-in`, spec), pivot);
	}

	rules.push(
		`::view-transition-old(m-leaving) { transform-origin: ${origin}; }`,
		partRule('::view-transition-old(m-leaving)', 'm-leaving-out', choreography.leaving)
	);
	return rules.join('\n');
}

function firstOf(targets: Map<string, MorphEntry>): HTMLElement | undefined {
	return [...targets.values()].find((entry) => entry.kind === 'cover')?.element;
}

const constants = `
::view-transition { background: var(--ground); }
::view-transition-group(root) { z-index: 0; animation: none; }
::view-transition-old(root) { animation: none; opacity: 0; }
::view-transition-new(root) { animation: none; opacity: 1; }
${layer('m-leaving', layers.page)}
${layer('m-stage-old', layers.stage)}
${layer('m-stage-new', layers.stage)}
${layer('m-bloom', layers.bloom)}
${layer('m-nav', layers.shell)}
${layer('m-island', layers.shell)}
::view-transition-group(m-nav) {
	overflow: clip;
	border-radius: var(--radius-pill);
}
::view-transition-group(m-island) {
	overflow: clip;
	border-radius: var(--radius-28);
}
::view-transition-old(m-nav), ::view-transition-new(m-nav) {
	left: 50%;
	width: auto;
	height: 100%;
	object-fit: none;
	transform: translateX(-50%);
}`;

const blendStyles = `
::view-transition-old(root), ::view-transition-new(root) {
	animation-duration: ${String(blendDuration)}ms;
	animation-timing-function: linear;
}`;

/** The View Transitions engine: the browser snapshots both pages and flies the pictures. */
export function createVtEngine(): Engine {
	const names = new Names();
	const style = document.createElement('style');
	let current: ViewTransition | null = null;

	function cleanUp(transition: ViewTransition): void {
		if (current !== transition) {
			return;
		}
		current = null;
		names.clear();
		style.remove();
		closeBloom();
		for (const element of document.querySelectorAll('[data-morph-square]')) {
			element.removeAttribute('data-morph-square');
		}
	}

	function start(journey: Journey, navigation: OnNavigate, proceed: () => void): void {
		const { choreography, sources } = journey;
		const page = document.querySelector('main');
		const stage = document.querySelector('[data-morph-stage]');
		const stageOn = stage !== null && Number(getComputedStyle(stage).opacity) > 0;
		const sourceCover = [...sources.values()].find((entry) => entry.kind === 'cover');
		const pageOrigin =
			page !== null && sourceCover !== undefined
				? originFor(sourceCover.element, page)
				: 'center top';
		const sourceCenter = sourceCover === undefined ? null : centerOf(sourceCover.element);

		const captured = new Map<string, Captured>(
			[...sources].map(([id, { element }]) => [
				id,
				{ width: element.getBoundingClientRect().width, shape: shapeOf(element) }
			])
		);

		names.clear();
		style.textContent = constants;
		document.head.append(style);
		names.set(document.querySelector('nav'), 'm-nav');
		names.set(document.querySelector('aside'), 'm-island');
		names.set(page, 'm-leaving');
		if (stageOn) {
			names.set(stage, 'm-stage-old');
		}
		for (const [id, source] of sources) {
			names.set(source.element, viewTransitionName(id));
			if (source.kind === 'cover') {
				source.element.setAttribute('data-morph-square', '');
			}
		}

		const transition = document.startViewTransition(async () => {
			names.clear();
			proceed();
			await navigation.complete.catch(() => undefined);
			names.set(document.querySelector('nav'), 'm-nav');
			names.set(document.querySelector('aside'), 'm-island');

			const targets = landJourney(journey, navigation.type !== 'popstate');
			const targetCover = [...targets.values()].find((entry) => entry.kind === 'cover');
			const targetCenter = targetCover === undefined ? null : centerOf(targetCover.element);
			const rules = [constants, flightStyles(journey, targets, captured, names, page, pageOrigin)];
			const stageNow = document.querySelector('[data-morph-stage]');
			if (choreography.stage !== null) {
				if (stageOn) {
					rules.push(
						partRule('::view-transition-old(m-stage-old)', 'm-stage-out', choreography.stage)
					);
				} else {
					names.set(stageNow, 'm-stage-new');
					rules.push(
						partRule('::view-transition-new(m-stage-new)', 'm-stage-in', choreography.stage)
					);
				}
			}
			if (choreography.bloom !== null && sourceCenter !== null && targetCenter !== null) {
				names.set(openBloom(), 'm-bloom');
				rules.push(bloomRules(choreography, sourceCenter, targetCenter));
			}
			style.textContent = rules.join('\n');
		});

		current = transition;
		transition.ready.catch(() => undefined);
		void transition.finished.finally(() => {
			cleanUp(transition);
		});
	}

	function blendTo(navigation: OnNavigate, proceed: () => void): void {
		style.textContent = blendStyles;
		document.head.append(style);
		const transition = document.startViewTransition(async () => {
			proceed();
			await navigation.complete.catch(() => undefined);
		});
		current = transition;
		transition.ready.catch(() => undefined);
		void transition.finished.finally(() => {
			cleanUp(transition);
		});
	}

	return {
		name: 'vt',
		navigate(navigation) {
			const journey = beginJourney(navigation);
			if (journey === null || !('startViewTransition' in document)) {
				return;
			}
			return new Promise<void>((proceed) => {
				if (journey.choreography.travel === null) {
					blendTo(navigation, proceed);
				} else {
					start(journey, navigation, proceed);
				}
			});
		}
	};
}
