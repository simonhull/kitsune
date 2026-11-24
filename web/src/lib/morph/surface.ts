import { tick } from 'svelte';
import { clipInset, cornerOf, edgeRect, floorRect, sameRect, type Rect } from './bud.ts';
import { Channel, landsWithin } from './channel.ts';
import {
	linearEasing,
	settleTime,
	springParams,
	type MotionSpec,
	type SpringParams
} from './choreography.ts';
import { motionOf, pageMain, reducedMotion } from './journey.ts';

// A surface in the page (M4, Bud): a menu, a confirm, a picker, a toast, the selection bar, an
// editor. It is one box at its true size and radius, born at the control that opened it and grown
// into itself; its words never move: they sit where they rest, and the box clips them, so a box that
// grows reveals them and a box that reshapes cuts them. It becomes the next surface by reshaping
// the same box, and it leaves by giving way where it stands, never back into its control: that
// would be its arrival, reversed.

const fast: MotionSpec = { token: '--morph-fast', bounce: 0 };
const base: MotionSpec = { token: '--morph', bounce: 0 };
/** Leaving goes fast: the person has already decided. The same exit spring as a page's. */
const exit: MotionSpec = { ...fast, pace: 0.7 };
/** Reduced motion: every surface is a plain blend at its final place, its shape set at once. */
const blendMs = 120;

/** How a surface moves. */
export interface SurfaceOptions {
	/** A window-sized surface (an editor): it buds on `--morph`, its words @90. */
	big?: boolean;
	/** A toast: it reshapes on the fast spring, its new words @90. */
	quick?: boolean;
	/** It has no control left on screen (a toast, the selection bar): it is born at the floor. */
	floor?: boolean;
	/** It comes and goes where it stands and never grows (the folder picker's backdrop, dimming). */
	still?: boolean;
	/** Where it gives way: under the page, which is laid again in front of it (an editor). */
	under?: 'page';
	/**
	 * Names it across instances: one opened again while its last self is still giving way takes over
	 * from the box and the opacity that one had reached, rather than being born twice.
	 */
	key?: string;
}

/** The properties a surface's look is drawn with, which its box takes while it moves. */
const lookProperties = [
	'background-color',
	'background-image',
	'box-shadow',
	...['top', 'right', 'bottom', 'left'].flatMap((side) =>
		['width', 'style', 'color'].map((part) => `border-${side}-${part}`)
	)
];

/** What a surface's own element is told while its box draws it: no look of its own. */
const bare: [string, string][] = [
	['background-color', 'transparent'],
	['background-image', 'none'],
	['border-color', 'transparent'],
	['box-shadow', 'none']
];

/** The properties the engine writes on a surface's own element, all handed back at rest. */
const written = ['clip-path', 'opacity', 'position', ...bare.map(([name]) => name)];

/**
 * One number on its way: on a spring after a wait, or in a straight line for reduced motion. Like a
 * `Channel` it always knows where it is and how fast it's going, so a turn goes on from here.
 */
class Track {
	readonly channel: Channel;
	#spring: SpringParams | null = null;
	#wait = 0;

	constructor(value: number) {
		this.channel = new Channel(value);
	}

	get value(): number {
		return this.channel.value;
	}

	/** Sets it here, at rest. */
	set(value: number): void {
		this.channel.value = value;
		this.channel.target = value;
		this.channel.velocity = 0;
		this.#wait = 0;
	}

	/** Heads for `target` on `spring` after `delay` ms, or as a blend. */
	to(target: number, spring: SpringParams | 'blend', delay = 0): void {
		this.channel.target = target;
		this.#wait = delay / 1000;
		if (spring === 'blend') {
			this.#spring = null;
			this.channel.velocity = 0;
		} else {
			this.#spring = spring;
		}
	}

	/** Stops where it is, as a surface giving way where it stands. */
	hold(): void {
		this.channel.target = this.channel.value;
		this.channel.velocity = 0;
		this.#wait = 0;
	}

	step(seconds: number): void {
		const moving = seconds - this.#wait;
		this.#wait = Math.max(0, this.#wait - seconds);
		if (moving <= 0) {
			return;
		}
		if (this.#spring !== null) {
			this.channel.step(this.#spring, moving);
			return;
		}
		const { value, target } = this.channel;
		const reach = (moving * 1000) / blendMs;
		this.channel.value =
			Math.abs(target - value) <= reach ? target : value + Math.sign(target - value) * reach;
	}

	settled(tolerance: number): boolean {
		return this.#wait === 0 && this.channel.settled(tolerance);
	}
}

/** A box's five numbers. */
class Shape {
	readonly left: Track;
	readonly top: Track;
	readonly width: Track;
	readonly height: Track;
	readonly radius: Track;

	constructor(rect: Rect) {
		this.left = new Track(rect.left);
		this.top = new Track(rect.top);
		this.width = new Track(rect.width);
		this.height = new Track(rect.height);
		this.radius = new Track(rect.radius);
	}

	get #tracks(): Track[] {
		return [this.left, this.top, this.width, this.height, this.radius];
	}

	get rect(): Rect {
		return {
			left: this.left.value,
			top: this.top.value,
			width: Math.max(0, this.width.value),
			height: Math.max(0, this.height.value),
			radius: Math.max(0, this.radius.value)
		};
	}

	set(rect: Rect): void {
		this.left.set(rect.left);
		this.top.set(rect.top);
		this.width.set(rect.width);
		this.height.set(rect.height);
		this.radius.set(rect.radius);
	}

	to(rect: Rect, spring: SpringParams): void {
		this.left.to(rect.left, spring);
		this.top.to(rect.top, spring);
		this.width.to(rect.width, spring);
		this.height.to(rect.height, spring);
		this.radius.to(rect.radius, spring);
	}

	hold(): void {
		for (const track of this.#tracks) {
			track.hold();
		}
	}

	adopt(other: Shape): void {
		const tracks = other.#tracks;
		for (const [index, track] of this.#tracks.entries()) {
			const from = tracks.at(index);
			if (from !== undefined) {
				track.channel.adopt(from.channel);
			}
		}
	}

	step(seconds: number): void {
		for (const track of this.#tracks) {
			track.step(seconds);
		}
	}

	get settled(): boolean {
		return this.#tracks.every((track) => track.settled(landsWithin));
	}
}

/** A surface's words, live or a picture of words it held before, and where they rest. */
interface Words {
	element: HTMLElement;
	rect: Rect;
	opacity: Track;
	/** A picture: the engine's own, removed when it has gone. */
	picture: boolean;
}

/** What a surface looks like at rest, read once, so its box can draw it however it moves. */
interface Look {
	css: [string, string][];
	zIndex: string;
	radius: number;
}

function lookOf(style: CSSStyleDeclaration): Look {
	return {
		css: lookProperties.map((name) => [name, style.getPropertyValue(name)]),
		zIndex: style.zIndex,
		radius: Number.parseFloat(style.borderTopLeftRadius) || 0
	};
}

function rectOf(element: Element, radius: number): Rect {
	const { left, top, width, height } = element.getBoundingClientRect();
	return { left, top, width, height, radius: cornerOf(radius, width, height) };
}

/** Takes the spring of a motion from the tokens, once per change. */
function spring(spec: MotionSpec): SpringParams {
	return springParams(motionOf(spec));
}

/**
 * Draws a surface's words as they stand, to give way while the live element goes on to its next
 * face, or is gone: nothing in it can be focused, pressed or read twice by a screen reader.
 */
function pictureOf(element: HTMLElement, rect: Rect, origin: Origin): HTMLElement {
	const picture = element.cloneNode(true);
	if (!(picture instanceof HTMLElement)) {
		return document.createElement('div');
	}
	for (const named of picture.querySelectorAll('[id], [name]')) {
		named.removeAttribute('id');
		named.removeAttribute('name');
	}
	picture.removeAttribute('id');
	picture.removeAttribute('data-testid');
	for (const tested of picture.querySelectorAll('[data-testid]')) {
		tested.removeAttribute('data-testid');
	}
	picture.inert = true;
	picture.setAttribute('aria-hidden', 'true');
	const { style } = picture;
	for (const [name, value] of bare) {
		style.setProperty(name, value);
	}
	style.position = 'fixed';
	style.inset = 'auto';
	style.left = `${(rect.left - origin.x).toFixed(2)}px`;
	style.top = `${(rect.top - origin.y).toFixed(2)}px`;
	style.width = `${rect.width.toFixed(2)}px`;
	style.height = `${rect.height.toFixed(2)}px`;
	style.margin = '0';
	style.transform = 'none';
	style.translate = 'none';
	style.transition = 'none';
	style.animation = 'none';
	style.pointerEvents = 'none';
	style.maxHeight = 'none';
	return picture;
}

/** Where a fixed box's coordinates start: the viewport, unless something above it holds it. */
interface Origin {
	x: number;
	y: number;
}

const viewport: Origin = { x: 0, y: 0 };

/** Surfaces giving way, by key, so one opened again can take over where it had got to. */
const leavingByKey = new Map<string, Surface>();

/**
 * A surface in flight. It owns nothing at rest: the element is exactly as its stylesheet draws it,
 * and only while it moves does a box of the engine's own draw its look, at its true size and radius,
 * while the element's words are clipped to that box.
 */
export class Surface {
	readonly #options: SurfaceOptions;
	#element: HTMLElement | null = null;
	#parent: HTMLElement | null = null;
	#look: Look | null = null;
	#rest: Rect | null = null;
	#origin: Origin = viewport;
	#box: HTMLElement | null = null;
	#shape: Shape | null = null;
	readonly #boxOpacity = new Track(1);
	/** What it gives way in: its card and its words, as one. */
	#layer: HTMLElement | null = null;
	readonly #layerOpacity = new Track(1);
	#words: Words[] = [];
	#live: Words | null = null;
	#leaving = false;
	#resize: ResizeObserver | null = null;
	/** The words changing: a picture holds the old ones until the new shape is known. */
	#holding = false;

	constructor(options: SurfaceOptions = {}) {
		this.#options = options;
	}

	/** Buds from `from`, or from the control just pressed, or the floor. */
	open(element: HTMLElement, from: Rect | null): void {
		this.#element = element;
		this.#parent = element.parentElement;
		const key = this.#options.key;
		const gone = key === undefined ? undefined : leavingByKey.get(key);
		const was = gone === undefined ? undefined : gone.#handOver();
		this.#begin();
		const rest = this.#rest;
		const shape = this.#shape;
		const live = this.#live;
		if (rest === null || shape === null || live === null) {
			return;
		}
		if (reducedMotion()) {
			shape.set(rest);
			this.#boxOpacity.set(was?.box ?? 0);
			this.#boxOpacity.to(1, 'blend');
			live.opacity.set(was?.words ?? 0);
			live.opacity.to(1, 'blend');
			this.write();
			schedule(this);
			return;
		}
		const { big = false, floor = false } = this.#options;
		if (was?.shape === undefined || was.shape === null) {
			shape.set(
				this.#options.still === true
					? rest
					: floor
						? floorRect(rest)
						: (from ?? pressedRect() ?? edgeRect(rest))
			);
			this.#boxOpacity.set(0);
			live.opacity.set(0);
			live.opacity.to(1, spring(big ? base : fast), big ? 90 : 30);
		} else {
			// Opened again while it gave way: it grows on from the box it had, at the opacity it had.
			shape.adopt(was.shape);
			this.#boxOpacity.set(was.box);
			live.opacity.set(was.words);
			live.opacity.to(1, spring(big ? base : fast));
		}
		shape.to(rest, spring(big ? base : fast));
		this.#boxOpacity.to(1, spring(exit));
		this.write();
		schedule(this);
	}

	/**
	 * Takes an element already at rest on its page (Health's Metadata card) as this surface, without
	 * budding, so a new face can reshape it in place with `hold` and `reshape`. As an attachment: it
	 * leaves with its page, so going takes away whatever it was drawing at once.
	 */
	readonly rests = (element: HTMLElement): (() => void) => {
		this.#element = element;
		this.#parent = element.parentElement;
		return () => {
			this.#resize?.disconnect();
			this.#dispose();
			this.#element = null;
		};
	};

	/** Gives up a surface giving way to its next self: where its box had got to, and its opacities. */
	#handOver(): { shape: Shape | null; box: number; words: number } {
		const layer = clamp(this.#layerOpacity.value);
		const handed = {
			shape: this.#shape,
			box: this.#boxOpacity.value * layer,
			words: this.#shown * layer
		};
		this.#dispose();
		return handed;
	}

	/** How much of its words still show, live or pictured. */
	get #shown(): number {
		return Math.max(0, ...this.#words.map((words) => words.opacity.value));
	}

	/**
	 * The words are about to change (a menu item into its confirm, Undo's toast into what it did): a
	 * picture holds the old words where they stand, and the box stays where it is. Call before the
	 * change, then `reshape` once the new face is in place.
	 */
	hold(): void {
		const element = this.#element;
		if (element === null || this.#leaving || !element.isConnected) {
			return;
		}
		if (this.#shape === null) {
			this.#begin();
		}
		const live = this.#live;
		const rest = this.#rest;
		const box = this.#box;
		if (live === null || rest === null || box === null) {
			return;
		}
		const picture = pictureOf(element, rest, this.#origin);
		element.before(picture);
		picture.scrollTop = element.scrollTop;
		const opacity = new Track(live.opacity.value);
		opacity.to(0, reducedMotion() ? 'blend' : spring(exit));
		this.#words.push({ element: picture, rect: rest, opacity, picture: true });
		live.opacity.set(0);
		this.#holding = true;
		this.write();
	}

	/** The new face is in place: the box reshapes to it, and its words come up once the old are gone. */
	reshape(): void {
		const element = this.#element;
		const shape = this.#shape;
		const live = this.#live;
		if (
			element === null ||
			shape === null ||
			live === null ||
			this.#look === null ||
			this.#leaving
		) {
			return;
		}
		this.#holding = false;
		const next = this.#relook(element);
		const rest = rectOf(element, next.radius);
		this.#rest = rest;
		live.rect = rest;
		const { quick = false } = this.#options;
		if (reducedMotion()) {
			shape.set(rest);
			live.opacity.to(1, 'blend');
		} else {
			shape.to(rest, spring(quick ? fast : base));
			live.opacity.to(1, spring(fast), quick ? 90 : 140);
		}
		this.#boxOpacity.to(1, reducedMotion() ? 'blend' : spring(exit));
		this.write();
		schedule(this);
	}

	/**
	 * Reads the look of the face it now shows (a confirm's fox edge and misprint after a menu's
	 * plain one), which the box turns into on the reshape's own spring: the shadows interpolate.
	 */
	#relook(element: HTMLElement): Look {
		const box = this.#box;
		for (const [name] of bare) {
			element.style.removeProperty(name);
		}
		const look = lookOf(getComputedStyle(element));
		for (const [name, value] of bare) {
			element.style.setProperty(name, value);
		}
		const before = this.#look;
		this.#look = look;
		if (box === null || before === null || reducedMotion()) {
			for (const [name, value] of look.css) {
				box?.style.setProperty(name, value);
			}
			return look;
		}
		const changed = look.css.filter(
			([name, value]) => before.css.find(([was]) => was === name)?.[1] !== value
		);
		if (changed.length > 0) {
			const motion = motionOf(this.#options.quick === true ? fast : base);
			const curve = `${String(settleTime(motion))}ms ${linearEasing(motion)}`;
			box.style.transition = changed.map(([name]) => `${name} ${curve}`).join(', ');
			for (const [name, value] of changed) {
				box.style.setProperty(name, value);
			}
		}
		return look;
	}

	/** Gives way where it stands, under whatever comes next. The element itself is already gone. */
	leave(): void {
		const element = this.#element;
		const look = this.#look ?? null;
		if (element === null || this.#leaving) {
			return;
		}
		this.#leaving = true;
		this.#resize?.disconnect();
		const rest = this.#rest ?? rectOf(element, look?.radius ?? 0);
		const under = this.#options.under === 'page' ? pageMain() : null;
		const parent = this.#parent?.isConnected === true ? this.#parent : null;
		if (under !== null || parent === null) {
			this.#origin = viewport;
		}
		if (this.#shape === null) {
			this.#shape = new Shape(rest);
			this.#boxOpacity.set(1);
		}
		if (this.#box === null && look !== null) {
			this.#box = boxFor(look);
		}
		const live = this.#live;
		this.#words = this.#words.filter((words) => words.picture);
		if (live !== null) {
			const picture = pictureOf(element, live.rect, this.#origin);
			const opacity = new Track(live.opacity.value);
			this.#words.push({ element: picture, rect: live.rect, opacity, picture: true });
			this.#live = null;
		}
		const nodes = [this.#box, ...this.#words.map((words) => words.element)].filter(
			(node): node is HTMLElement => node !== null
		);
		// It gives way as one layer, its card and its words together, as the page's own units do.
		const layer = document.createElement('div');
		layer.setAttribute('aria-hidden', 'true');
		layer.dataset['surfaceLeaving'] = '';
		layer.style.cssText = 'position:fixed;inset:0;pointer-events:none;will-change:opacity';
		const zIndex = under === null ? (look?.zIndex ?? 'auto') : '1';
		if (zIndex !== 'auto') {
			layer.style.zIndex = zIndex;
		}
		layer.append(...nodes);
		this.#layer = layer;
		if (under !== null) {
			// Under the page, which is laid again in front of it: the In-Front Rule.
			under.before(layer);
		} else if (parent !== null) {
			// First among its own: anything opened after it stands in front of it.
			parent.prepend(layer);
		} else {
			(document.querySelector('.shell') ?? document.body).prepend(layer);
		}
		const reduced = reducedMotion();
		this.#shape.hold();
		this.#boxOpacity.hold();
		for (const words of this.#words) {
			words.opacity.hold();
		}
		this.#layerOpacity.to(0, reduced ? 'blend' : spring(this.#options.big === true ? fast : exit));
		const key = this.#options.key;
		if (key !== undefined) {
			leavingByKey.set(key, this);
		}
		this.write();
		schedule(this);
	}

	/** Starts a flight: the box takes the element's look, and the element keeps only its words. */
	#begin(): void {
		const element = this.#element;
		if (element === null) {
			return;
		}
		const style = getComputedStyle(element);
		const look = lookOf(style);
		const statics = style.position === 'static';
		const rest = rectOf(element, look.radius);
		const box = boxFor(look);
		element.before(box);
		const at = box.getBoundingClientRect();
		this.#origin = { x: at.left, y: at.top };
		this.#look = look;
		this.#rest = rest;
		this.#box = box;
		this.#shape = new Shape(rest);
		for (const [name, value] of bare) {
			element.style.setProperty(name, value);
		}
		if (statics) {
			// In front of its box, which a positioned element is and a static one isn't.
			element.style.position = 'relative';
		}
		const opacity = new Track(1);
		this.#live = { element, rect: rest, opacity, picture: false };
		this.#words = [this.#live];
		this.#resize ??= new ResizeObserver(() => {
			this.#resized();
		});
		this.#resize.observe(element);
	}

	/** Its words grew or shrank as it moved (a list arriving): the box heads for the new shape. */
	#resized(): void {
		const element = this.#element;
		const shape = this.#shape;
		const live = this.#live;
		const look = this.#look;
		if (element === null || shape === null || live === null || look === null) {
			return;
		}
		if (this.#leaving || this.#holding || !element.isConnected) {
			return;
		}
		const rest = rectOf(element, look.radius);
		if (this.#rest !== null && sameRect(rest, this.#rest)) {
			return;
		}
		this.#rest = rest;
		live.rect = rest;
		if (reducedMotion()) {
			shape.set(rest);
		} else {
			shape.to(rest, spring(this.#options.big === true ? base : fast));
		}
		schedule(this);
	}

	step(seconds: number): void {
		this.#shape?.step(seconds);
		this.#boxOpacity.step(seconds);
		this.#layerOpacity.step(seconds);
		for (const words of this.#words) {
			words.opacity.step(seconds);
		}
	}

	get settled(): boolean {
		return (
			!this.#holding &&
			(this.#shape?.settled ?? true) &&
			this.#boxOpacity.settled(0.01) &&
			this.#layerOpacity.settled(0.01) &&
			this.#words.every((words) => words.opacity.settled(0.01))
		);
	}

	/** Draws where everything has got to. */
	write(): void {
		const shape = this.#shape;
		const box = this.#box;
		if (shape === null || box === null) {
			return;
		}
		const rect = shape.rect;
		const { style } = box;
		style.transform = `translate(${(rect.left - this.#origin.x).toFixed(2)}px, ${(rect.top - this.#origin.y).toFixed(2)}px)`;
		style.width = `${rect.width.toFixed(2)}px`;
		style.height = `${rect.height.toFixed(2)}px`;
		style.borderRadius = `${cornerOf(rect.radius, rect.width, rect.height).toFixed(2)}px`;
		style.opacity = clamp(this.#boxOpacity.value).toFixed(3);
		if (this.#layer !== null) {
			this.#layer.style.opacity = clamp(this.#layerOpacity.value).toFixed(3);
		}
		for (const words of this.#words) {
			words.element.style.clipPath = clipInset(rect, words.rect);
			words.element.style.opacity = clamp(words.opacity.value).toFixed(3);
		}
	}

	/** Called by the ticker once everything has come to rest. */
	finish(): void {
		if (this.#leaving) {
			this.#dispose();
			return;
		}
		if (!this.settled) {
			return;
		}
		this.#box?.remove();
		this.#box = null;
		this.#shape = null;
		for (const words of this.#words) {
			if (words.picture) {
				words.element.remove();
			}
		}
		this.#words = this.#live === null ? [] : [this.#live];
		this.#resize?.disconnect();
		const element = this.#element;
		if (element !== null) {
			for (const name of written) {
				element.style.removeProperty(name);
			}
		}
	}

	/** Takes away everything it drew, at once. */
	#dispose(): void {
		unschedule(this);
		this.#layer?.remove();
		this.#layer = null;
		this.#box?.remove();
		for (const words of this.#words) {
			if (words.picture) {
				words.element.remove();
			}
		}
		this.#words = [];
		this.#box = null;
		this.#shape = null;
		const key = this.#options.key;
		if (key !== undefined && leavingByKey.get(key) === this) {
			leavingByKey.delete(key);
		}
	}
}

/**
 * A place a surface comes and goes in: each element mounted there is a surface of its own, so one
 * replaced (a second toast) gives way under the next, which is born in front of it. As an
 * attachment it buds once the element is placed (after the flush that mounted it, which may move it),
 * and gives way where it stands when it unmounts.
 */
export class SurfaceSlot {
	readonly #options: SurfaceOptions;
	#current: Surface | null = null;

	constructor(options: SurfaceOptions = {}) {
		this.#options = options;
	}

	readonly attach = (element: HTMLElement): (() => void) => {
		const surface = new Surface(this.#options);
		this.#current = surface;
		// Unseen until it buds: its place may still move in the flush that mounted it.
		element.style.opacity = '0';
		let gone = false;
		void tick().then(() => {
			if (!gone) {
				surface.open(element, null);
			}
		});
		return () => {
			gone = true;
			surface.leave();
			if (this.#current === surface) {
				this.#current = null;
			}
		};
	};

	/** The surface now in the slot: see `Surface.hold`. */
	hold(): void {
		this.#current?.hold();
	}

	/** See `Surface.reshape`. */
	reshape(): void {
		this.#current?.reshape();
	}
}

function clamp(value: number): number {
	return Math.min(1, Math.max(0, value));
}

/** The engine's box: a surface's look at its true size and radius, under its words. */
function boxFor(look: Look): HTMLElement {
	const box = document.createElement('div');
	box.setAttribute('aria-hidden', 'true');
	box.dataset['surfaceBox'] = '';
	const { style } = box;
	style.cssText =
		'position:fixed;left:0;top:0;margin:0;box-sizing:border-box;pointer-events:none;contain:layout style;will-change:transform,opacity';
	for (const [name, value] of look.css) {
		style.setProperty(name, value);
	}
	if (look.zIndex !== 'auto') {
		style.zIndex = look.zIndex;
	}
	return box;
}

// ---- One frame loop for every surface ----

const flying = new Set<Surface>();
let frame = 0;
let last = 0;

function schedule(surface: Surface): void {
	flying.add(surface);
	if (frame === 0) {
		last = performance.now();
		frame = requestAnimationFrame(onFrame);
	}
}

function unschedule(surface: Surface): void {
	flying.delete(surface);
}

function onFrame(now: number): void {
	// A frame held up by a long task mustn't throw a surface to its end: at most a short step.
	const seconds = Math.min(0.05, Math.max(0, (now - last) / 1000));
	last = now;
	for (const surface of flying) {
		surface.step(seconds);
		surface.write();
		if (surface.settled) {
			flying.delete(surface);
			surface.finish();
		}
	}
	frame = flying.size > 0 ? requestAnimationFrame(onFrame) : 0;
}

// ---- The control that was pressed ----

/** What counts as a control a surface can come out of. */
const controls = 'button, a[href], [role="menuitem"], [role="button"], summary, label';

/** How long a press stays the cause of what opens. An editor's code may take a moment to come. */
const pressLasts = 1500;

interface Press {
	element: Element | null;
	rect: Rect | null;
	at: number;
}

let lastPress: Press | null = null;

/**
 * Whether a control's place must be read as it's pressed: it is about to be replaced (a menu's
 * item, a button a confirm stands in place of) or it opens something. Any other press is read only
 * if a surface asks, so a press on Play costs a `closest` and nothing more.
 */
function readsNow(control: Element): boolean {
	return (
		control.closest('[data-surface]') !== null ||
		control.hasAttribute('aria-haspopup') ||
		control.hasAttribute('aria-expanded') ||
		control.hasAttribute('data-buds')
	);
}

function controlRect(control: Element): Rect {
	return rectOf(control, Number.parseFloat(getComputedStyle(control).borderTopLeftRadius) || 0);
}

/** Listens for presses, whose controls surfaces are born at. Returns the cleanup. */
export function watchPresses(): () => void {
	function onClick(event: MouseEvent): void {
		const control = event.target instanceof Element ? event.target.closest(controls) : null;
		if (control === null) {
			return;
		}
		lastPress = {
			element: control,
			rect: readsNow(control) ? controlRect(control) : null,
			at: performance.now()
		};
	}
	function onContextMenu(event: MouseEvent): void {
		// A right-click has no control: what it opens is born at the pointer, a point.
		lastPress = {
			element: null,
			rect: { left: event.clientX, top: event.clientY, width: 1, height: 1, radius: 0 },
			at: performance.now()
		};
	}
	document.addEventListener('click', onClick, { capture: true, passive: true });
	document.addEventListener('contextmenu', onContextMenu, { capture: true, passive: true });
	return () => {
		document.removeEventListener('click', onClick, { capture: true });
		document.removeEventListener('contextmenu', onContextMenu, { capture: true });
	};
}

/** Tells the next surface where it comes from, when no click says: a key that opened it. */
export function pressFrom(control: Element): void {
	lastPress = { element: control, rect: controlRect(control), at: performance.now() };
}

/** The control just pressed, at its true size and radius, or null when nothing was. */
export function pressedRect(): Rect | null {
	const press = lastPress;
	// A link pressed went somewhere: nothing on the page it opens came out of it.
	if (
		press === null ||
		performance.now() - press.at > pressLasts ||
		press.element?.matches('a[href]') === true
	) {
		return null;
	}
	if (press.rect !== null) {
		return press.rect;
	}
	if (press.element?.isConnected !== true) {
		return null;
	}
	const rect = controlRect(press.element);
	return rect.width > 0 ? rect : null;
}

/** The middle of the control just pressed: what a page laid again gathers around. */
export function pressedPoint(): { x: number; y: number } | null {
	const rect = pressedRect();
	return rect === null ? null : { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
}

// ---- Words re-pressed in place ----

/** Plays a part of a surface's words as a spring, through the compositor. */
function animateOn(
	element: HTMLElement,
	keyframes: Keyframe[],
	spec: MotionSpec | 'blend'
): Animation {
	if (spec === 'blend') {
		return element.animate(keyframes, { duration: blendMs, easing: 'linear', fill: 'both' });
	}
	const motion = motionOf(spec);
	return element.animate(keyframes, {
		duration: settleTime(motion),
		easing: linearEasing(motion),
		fill: 'both'
	});
}

/**
 * Words that change in place (the selection bar's count): the old give way where they stand and the
 * new rise 6 px into their place on the fast spring. Never a rolling number.
 */
export class Repress {
	/** Call before the words change: a picture of them gives way, and the new come up once drawn. */
	hold(element: HTMLElement): void {
		if (!element.isConnected) {
			return;
		}
		const picture = element.cloneNode(true);
		if (!(picture instanceof HTMLElement)) {
			return;
		}
		picture.setAttribute('aria-hidden', 'true');
		picture.inert = true;
		Object.assign(picture.style, {
			position: 'absolute',
			left: `${String(element.offsetLeft)}px`,
			top: `${String(element.offsetTop)}px`,
			margin: '0',
			pointerEvents: 'none'
		});
		element.after(picture);
		const reduced = reducedMotion();
		animateOn(picture, [{ opacity: 1 }, { opacity: 0 }], reduced ? 'blend' : exit).onfinish =
			() => {
				picture.remove();
			};
		// After the change is drawn into the element, in the same flush.
		void Promise.resolve().then(() => {
			const rise = animateOn(
				element,
				reduced
					? [{ opacity: 0 }, { opacity: 1 }]
					: [
							{ opacity: 0, translate: '0 6px' },
							{ opacity: 1, translate: '0 0' }
						],
				reduced ? 'blend' : fast
			);
			rise.onfinish = () => {
				rise.cancel();
			};
		});
	}
}

/**
 * What a surface in place stood for (a page's buttons, after its confirm) comes back in front of the
 * surface giving way where it stood, fading up on the fast spring: never uncovered by it. On a first
 * mount, with nothing giving way beside it, it does nothing. An attachment.
 */
export function layInFront(element: HTMLElement): void {
	queueMicrotask(() => {
		const parent = element.parentElement;
		if (parent?.querySelector(':scope > [data-surface-leaving]') == null) {
			return;
		}
		const statics = getComputedStyle(element).position === 'static';
		if (statics) {
			element.style.position = 'relative';
		}
		const reduced = reducedMotion();
		const arrive = animateOn(element, [{ opacity: 0 }, { opacity: 1 }], reduced ? 'blend' : fast);
		arrive.onfinish = () => {
			arrive.cancel();
			if (statics) {
				element.style.removeProperty('position');
			}
		};
	});
}
