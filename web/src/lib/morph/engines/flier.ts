import { Channel, landsWithin } from '../channel.ts';
import {
	easeBetween,
	springParams,
	type Arc,
	type Passage,
	type SpringParams
} from '../choreography.ts';
import type { MorphEntry } from '../registry.ts';
import {
	barRise,
	dissolveOpacities,
	dissolveTarget,
	sharedWidth,
	unit,
	type Point
} from './path.ts';
import { scaleShadow, ShadowMix } from './shadow.ts';

/** Where a shared cover is and how it's shaped, in viewport coordinates. */
export interface CoverShape {
	x: number;
	y: number;
	width: number;
	height: number;
	rotation: number;
	radius: number;
}

/** Where a shared title is: its left edge, its baseline and its font size. */
export interface TitleShape {
	left: number;
	baseline: number;
	fontSize: number;
}

function rotationOf(element: HTMLElement): number {
	const { transform } = getComputedStyle(element);
	if (transform === 'none') {
		return 0;
	}
	const matrix = new DOMMatrixReadOnly(transform);
	return (Math.atan2(matrix.b, matrix.a) * 180) / Math.PI;
}

/** Measures a cover in full, styles included: once per target, never per frame. */
export function measureCover(element: HTMLElement): CoverShape {
	return placeCover(element, rotationOf(element), radiusOf(element, element.offsetWidth));
}

/** Finds a cover whose style is already known. The box ignores rotation; the center doesn't care. */
function placeCover(element: HTMLElement, rotation: number, radius: number): CoverShape {
	const { left, top, width, height } = element.getBoundingClientRect();
	const scale = rotation === 0 ? width / (element.offsetWidth || 1) : 1;
	return {
		x: left + width / 2,
		y: top + height / 2,
		width: element.offsetWidth * scale,
		height: element.offsetHeight * scale,
		rotation,
		radius: radius * scale
	};
}

/** Reads an element's corner radius in pixels, whichever unit it's written in. */
function radiusOf(element: HTMLElement, width: number): number {
	const { borderTopLeftRadius } = getComputedStyle(element);
	const radius = Number.parseFloat(borderTopLeftRadius);
	return borderTopLeftRadius.endsWith('%') ? (radius / 100) * width : radius;
}

/** Finds how far below the top of an element its first baseline sits, unscaled. */
function baselineOffset(element: HTMLElement): number {
	const probe = document.createElement('span');
	probe.style.cssText = 'display:inline-block;width:0;height:0;vertical-align:baseline';
	element.prepend(probe);
	const offset = probe.offsetTop - element.offsetTop;
	probe.remove();
	return offset;
}

export interface TitleMeasure {
	shape: TitleShape;
	/** The element's own font size and baseline offset, which its picture is drawn at. */
	drawnSize: number;
	drawnBaseline: number;
	/** The words' full width, and how much of it shows if the element truncates them. */
	drawnWidth: number;
	shownWidth: number;
	/** A long title can wrap at one end of its flight and not the other. */
	wrapped: boolean;
}

/** Measures a title in full, baseline probe included: once per target, never per frame. */
export function measureTitle(element: HTMLElement): TitleMeasure {
	const style = getComputedStyle(element);
	const drawnSize = Number.parseFloat(style.fontSize);
	const drawnBaseline = baselineOffset(element);
	return {
		shape: placeTitle(element, drawnSize, drawnBaseline),
		drawnSize,
		drawnBaseline,
		drawnWidth: element.scrollWidth,
		shownWidth: element.clientWidth,
		wrapped: wraps(element, style)
	};
}

/** Reports whether a title runs to more than one line where it sits. */
function wraps({ clientHeight }: HTMLElement, style: CSSStyleDeclaration): boolean {
	return style.whiteSpace !== 'nowrap' && clientHeight > Number.parseFloat(style.lineHeight) * 1.5;
}

/** Finds a title whose metrics are already known. */
function placeTitle(element: HTMLElement, drawnSize: number, drawnBaseline: number): TitleShape {
	const { left, top, height } = element.getBoundingClientRect();
	const scale = height / (element.offsetHeight || 1);
	return { left, baseline: top + drawnBaseline * scale, fontSize: drawnSize * scale };
}

/** A picture of a title at its true font, drawn once and only ever transformed. */
export function drawTitle(element: HTMLElement): HTMLElement {
	const style = getComputedStyle(element);
	const picture = document.createElement('span');
	picture.textContent = element.textContent;
	// A title truncated where it sits stays truncated in its picture, so the first frame matches.
	const clipped =
		element.scrollWidth > element.clientWidth
			? `max-width:${String(element.clientWidth)}px;overflow:hidden;text-overflow:ellipsis;`
			: '';
	// Same for one on two lines. The width rounds up, or a line that just fits would break again.
	const layout = wraps(element, style)
		? `display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:${style.webkitLineClamp};overflow:hidden;width:${String(element.clientWidth + 1)}px;padding-bottom:${style.paddingBottom};text-wrap:${style.textWrap};text-align:${style.textAlign};`
		: `${clipped}white-space:nowrap;`;
	picture.style.cssText = `${layout}position:absolute;left:0;top:0;transform-origin:0 0;will-change:transform,opacity;
		font-family:${style.fontFamily};font-size:${style.fontSize};font-weight:${style.fontWeight};
		letter-spacing:${style.letterSpacing};line-height:${style.lineHeight};color:${style.color}`;
	return picture;
}

const still: Point = { x: 0, y: 0 };

/** Finds how far a target is being carried, which is nothing outside the island's contents. */
function riding({ element }: MorphEntry, carry: Point): Point {
	return element.closest('[data-island-content]') === null ? still : carry;
}

/** Something in the air. Every flier ends by landing on, or fading out without, a target. */
export interface Flier {
	readonly element: HTMLElement;
	/** Where it's going; null when the page it was going to has gone. */
	target: MorphEntry | null;
	/** Whether it sees itself out with no target, instead of being taken away. */
	readonly leavesAlone?: boolean;
	/**
	 * Reads the target's position, and nothing else. Runs before any writes, once per frame.
	 * `carry` is how far the island's contents are riding from their place: a flier bound for
	 * them aims at that place, or it would chase the lag of the glass on top of its own.
	 */
	measure(carry: Point): void;
	step(params: SpringParams, seconds: number): void;
	write(): void;
	readonly settled: boolean;
}

/**
 * A cover in flight. Position, size and rotation are transforms; the corners and the shadow
 * are the shape's own, redrawn at their true size so they never scale with the box.
 */
export class CoverFlier implements Flier {
	readonly element = document.createElement('div');
	#target: MorphEntry | null = null;
	#rotation0 = 0;
	#radius0 = 0;

	readonly #x: Channel;
	readonly #y: Channel;
	readonly #width: Channel;
	readonly #height: Channel;
	readonly #rotation: Channel;
	readonly #radius: Channel;
	readonly #face = document.createElement('div');
	readonly #shadows = [document.createElement('div'), document.createElement('div')] as const;
	readonly #mix: ShadowMix;
	readonly #base: number;
	readonly #opacity = new Channel(1);
	#passing: Passing | null = null;
	#curve: Curve | null = null;

	constructor(from: CoverShape, target: MorphEntry | null, picture: Node, shadow: string) {
		const to = target === null ? from : measureCover(target.element);
		this.#base = Math.max(from.width, to.width);
		// Each end's shadow is drawn at the size it has at rest, whatever the box is scaled by.
		const scaled = scaleShadow(shadow, this.#base / from.width);
		this.#mix = new ShadowMix(scaled, target === null ? scaled : this.#restingShadow(target));
		this.target = target;
		this.#x = new Channel(from.x, to.x);
		this.#y = new Channel(from.y, to.y);
		this.#width = new Channel(from.width, to.width);
		this.#height = new Channel(from.height, to.height);
		this.#rotation = new Channel(from.rotation, to.rotation);
		this.#radius = new Channel(from.radius, to.radius);

		const size = `${String(this.#base)}px`;
		this.element.style.cssText = `position:absolute;left:0;top:0;width:${size};height:${size};will-change:transform`;
		this.#face.style.cssText = 'position:absolute;inset:0;overflow:hidden';
		this.#face.append(picture);
		for (const [index, layer] of this.#shadows.entries()) {
			layer.style.cssText = `position:absolute;inset:0;will-change:opacity`;
			layer.style.boxShadow = this.#mix.shadows.at(index) ?? 'none';
		}
		this.element.append(...this.#shadows, this.#face);
		this.write();
	}

	/** Sends the cover past the viewer, drifting toward `toward` on the way. */
	pass({ reach, gone }: Passage, toward: Point, params: SpringParams): void {
		this.#passing = {
			reach,
			gone,
			toward,
			params,
			from: { x: this.#x.value, y: this.#y.value, width: this.#width.value },
			radius: this.#radius.value,
			height: this.#height.value,
			depth: new Channel(0, 1)
		};
	}

	/**
	 * Bends the flight, which stays a straight spring underneath: `along` runs with it, and the bow
	 * and the tip are functions of that. Together they trace a curve that comes at the target from
	 * above, and a turn in mid-air just unwinds them on the way to wherever it's going now.
	 */
	curve({ lift, tip }: Arc, sizing: SpringParams): void {
		const bow = {
			x: (this.#x.target - this.#x.value) / 2,
			y: (this.#y.target - this.#y.value) / 2 - lift
		};
		this.#curve = { along: new Channel(0, 1), bow, tip, sizing };
	}

	get leavesAlone(): boolean {
		return this.#passing !== null || this.#curve !== null;
	}

	/**
	 * Works the shape out from the depth, and writes it to the channels with its speed, so a target
	 * that turns up in mid-air takes over from here at this speed.
	 */
	#stepPassing(
		{ reach, gone, toward, params, from, radius, height, depth }: Passing,
		seconds: number
	): void {
		depth.step(params, seconds);
		const grown = 1 / (1 - reach * depth.value);
		const carry = (channel: Channel, next: number): void => {
			channel.velocity = seconds > 0 ? (next - channel.value) / seconds : 0;
			channel.value = next;
			channel.target = next;
		};
		carry(this.#x, from.x + (toward.x - from.x) * depth.value);
		carry(this.#y, from.y + (toward.y - from.y) * depth.value);
		carry(this.#width, from.width * grown);
		carry(this.#height, height * grown);
		carry(this.#radius, radius * grown);
		this.#opacity.value = 1 - easeBetween(depth.value, gone);
	}

	#restingShadow({ element }: MorphEntry): string {
		const { boxShadow } = getComputedStyle(element);
		return scaleShadow(boxShadow, this.#base / element.offsetWidth);
	}

	/**
	 * Lays a sharper picture over the one in flight, once it has decoded. Never the other way: a
	 * flight can't wait for bytes, so it leaves on what's already on screen. It's the same picture,
	 * so no crossfade: the only change is that it stops being soft.
	 */
	sharpen(url: string): void {
		const image = new Image();
		image.src = url;
		image.style.cssText = 'position:absolute;inset:0;width:100%;height:100%;object-fit:cover';
		image
			.decode()
			.then(() => {
				this.#face.append(image);
			})
			.catch(() => undefined);
	}

	get target(): MorphEntry | null {
		return this.#target;
	}

	set target(entry: MorphEntry | null) {
		this.#target = entry;
		if (entry !== null) {
			const { rotation, radius } = measureCover(entry.element);
			this.#rotation0 = rotation;
			this.#radius0 = radius;
			this.#aimShadow(entry);
			this.#passing = null;
			this.#opacity.target = 1;
		} else if (this.#curve !== null) {
			this.#opacity.target = 0;
		}
	}

	#aimShadow(entry: MorphEntry): void {
		this.#mix.aim(this.#restingShadow(entry));
		for (const [index, layer] of this.#shadows.entries()) {
			layer.style.boxShadow = this.#mix.shadows.at(index) ?? 'none';
		}
	}

	measure(carry: Point): void {
		if (this.#target === null) {
			return;
		}
		const to = placeCover(this.#target.element, this.#rotation0, this.#radius0);
		const { x, y } = riding(this.#target, carry);
		this.#x.target = to.x - x;
		this.#y.target = to.y - y;
		this.#width.target = to.width;
		this.#height.target = to.height;
		this.#rotation.target = to.rotation;
		this.#radius.target = to.radius;
	}

	step(params: SpringParams, seconds: number): void {
		if (this.#passing !== null) {
			this.#stepPassing(this.#passing, seconds);
			return;
		}
		const sizing = this.#curve?.sizing ?? params;
		for (const channel of [this.#x, this.#y, this.#rotation]) {
			channel.step(params, seconds);
		}
		for (const channel of [this.#width, this.#height, this.#radius]) {
			channel.step(sizing, seconds);
		}
		this.#curve?.along.step(params, seconds);
		this.#opacity.step(params, seconds);
		this.#mix.step(params, seconds);
	}

	write(): void {
		const scaleX = this.#width.value / this.#base;
		const scaleY = this.#height.value / this.#base;
		const along = this.#curve?.along.value ?? 0;
		const bend = 2 * along * (1 - along);
		const left = this.#x.value + (this.#curve?.bow.x ?? 0) * bend - this.#base / 2;
		const top = this.#y.value + (this.#curve?.bow.y ?? 0) * bend - this.#base / 2;
		const tip =
			this.#curve === null
				? ''
				: `perspective(1100px) rotateX(${(this.#curve.tip * Math.sin(Math.PI * along)).toFixed(3)}deg) `;
		this.element.style.transform = `translate(${left.toFixed(2)}px, ${top.toFixed(2)}px) ${tip}rotate(${this.#rotation.value.toFixed(3)}deg) scale(${scaleX.toFixed(4)}, ${scaleY.toFixed(4)})`;
		this.element.style.opacity = this.#opacity.value.toFixed(3);
		const radius = `${(Math.max(0, this.#radius.value) / Math.max(scaleX, 0.01)).toFixed(2)}px`;
		this.#face.style.borderRadius = radius;
		for (const [index, layer] of this.#shadows.entries()) {
			layer.style.borderRadius = radius;
			layer.style.opacity = (this.#mix.opacities.at(index) ?? 0).toFixed(3);
		}
	}

	/** Where the cover is right now, for whatever is following it. */
	get center(): { x: number; y: number } {
		return { x: this.#x.value, y: this.#y.value };
	}

	get settled(): boolean {
		if (this.#passing !== null || (this.#curve !== null && this.#target === null)) {
			return this.#opacity.value < 0.01;
		}
		return (
			(this.#curve?.along.settled(0.002) ?? true) &&
			this.#x.settled(landsWithin) &&
			this.#y.settled(landsWithin) &&
			this.#width.settled(landsWithin) &&
			this.#rotation.settled(0.2) &&
			this.#opacity.settled(0.01)
		);
	}
}

/** A cover going past the viewer: all of its shape follows from `depth`. */
interface Passing {
	reach: number;
	gone: readonly [number, number];
	toward: Point;
	params: SpringParams;
	from: Point & { width: number };
	radius: number;
	height: number;
	depth: Channel;
}

/** A cover coming round to its place: all of the bend follows from `along`. */
interface Curve {
	along: Channel;
	bow: Point;
	tip: number;
	sizing: SpringParams;
}

/** Where a bar is. Unlike a cover it changes proportion, so it's drawn at its true size. */
export interface BarShape extends BarEnd {
	top: number;
	/** Where the glass's floor was when this was measured. */
	glassBottom: number;
}

export function measureBar(element: HTMLElement): BarShape {
	const { left, top, width, height } = element.getBoundingClientRect();
	const glass = document.querySelector('[data-island-glass]')?.getBoundingClientRect();
	return {
		left,
		top,
		width,
		height,
		radius: radiusOf(element, width),
		glassBottom: glass?.bottom ?? top
	};
}

/** The bar's resting state at the scrubber, which is what it lifts into. */
interface BarEnd {
	left: number;
	width: number;
	height: number;
	radius: number;
}

/**
 * A bar in flight: the island's hairline becoming the scrubber's fill, and back. It runs along
 * the glass's floor, under the controls crossing it the other way, and lifts into the track only
 * at the end. Its height is measured from the glass, so it rides whatever the glass is doing.
 */
export class BarFlier implements Flier {
	readonly element = document.createElement('div');
	#target: MorphEntry | null = null;

	readonly #glass: HTMLElement;
	readonly #along: Channel;
	readonly #left: Channel;
	readonly #width: Channel;
	readonly #floorHeight: number;
	readonly #trackHeight: number;
	readonly #floorRadius: number;
	readonly #trackRadius: number;
	/** How far the scrubber sits above the glass's floor. */
	readonly #trackRise: number;
	#floor = 0;

	constructor(from: BarShape, target: MorphEntry, color: string, glass: HTMLElement) {
		const to = measureBar(target.element);
		const opening = target.slot === 'poster';
		const [hairline, track] = opening ? [from, to] : [to, from];
		const island = glass.parentElement?.getBoundingClientRect().bottom ?? to.top;
		this.#glass = glass;
		this.#along = new Channel(opening ? 0 : 1, opening ? 1 : 0);
		this.#left = new Channel(from.left, to.left);
		this.#width = new Channel(from.width, to.width);
		this.#floorHeight = hairline.height;
		this.#trackHeight = track.height;
		this.#floorRadius = hairline.radius;
		this.#trackRadius = track.radius;
		this.#trackRise = (opening ? island : from.glassBottom) - track.top;
		this.#floor = from.glassBottom;
		this.target = target;
		this.element.style.cssText = `position:absolute;left:0;top:0;background:${color}`;
		this.write();
	}

	get target(): MorphEntry | null {
		return this.#target;
	}

	set target(entry: MorphEntry | null) {
		this.#target = entry;
		if (entry !== null) {
			this.#along.target = entry.slot === 'poster' ? 1 : 0;
		}
	}

	measure(carry: Point): void {
		this.#floor = this.#glass.getBoundingClientRect().bottom;
		if (this.#target === null) {
			return;
		}
		const { left, width } = this.#target.element.getBoundingClientRect();
		this.#left.target = left - riding(this.#target, carry).x;
		this.#width.target = width;
	}

	step(params: SpringParams, seconds: number): void {
		for (const channel of [this.#along, this.#left, this.#width]) {
			channel.step(params, seconds);
		}
	}

	write(): void {
		const along = this.#along.value;
		const height = barRise(along, this.#floorHeight, this.#trackHeight);
		const rise = barRise(along, this.#floorHeight + 1, this.#trackRise);
		const { style } = this.element;
		style.transform = `translate(${this.#left.value.toFixed(2)}px, ${(this.#floor - rise).toFixed(2)}px)`;
		style.width = `${this.#width.value.toFixed(2)}px`;
		style.height = `${height.toFixed(2)}px`;
		style.borderRadius = `${barRise(along, this.#floorRadius, this.#trackRadius).toFixed(2)}px`;
	}

	get settled(): boolean {
		return (
			this.#along.settled(0.01) &&
			this.#left.settled(landsWithin) &&
			this.#width.settled(landsWithin)
		);
	}
}

interface TitlePicture {
	element: HTMLElement;
	size: number;
	baseline: number;
	width: number;
	shown: number;
}

/** A dissolve short enough to read as one event: about 80 ms from one picture to the other. */
const dissolveSpring = springParams({ duration: 60, bounce: 0 });

function pictureFrom(element: HTMLElement, measure: TitleMeasure): TitlePicture {
	return {
		element,
		size: measure.drawnSize,
		baseline: measure.drawnBaseline,
		width: measure.drawnWidth,
		shown: measure.shownWidth
	};
}

/**
 * A title in flight, as two pictures of the same words: the small one at its true weight and
 * the large one at its. Both share one width, so the letters line up as one dissolves into the
 * other. When to dissolve follows the font size, not the clock, so it plays itself backwards if
 * the flight turns around.
 */
export class TitleFlier implements Flier {
	readonly element = document.createElement('div');
	#target: MorphEntry | null = null;
	#size0 = 0;
	#baseline0 = 0;

	readonly #left: Channel;
	readonly #baseline: Channel;
	readonly #fontSize: Channel;
	readonly #small: TitlePicture;
	readonly #large: TitlePicture;
	readonly #dissolve: Channel;
	/** The two ends break differently, so their widths mean nothing to each other. */
	readonly #reflows: boolean;

	constructor(
		from: TitleMeasure,
		fromPicture: HTMLElement,
		target: MorphEntry,
		to: TitleMeasure,
		toPicture: HTMLElement
	) {
		this.target = target;
		this.#left = new Channel(from.shape.left, to.shape.left);
		this.#baseline = new Channel(from.shape.baseline, to.shape.baseline);
		this.#fontSize = new Channel(from.shape.fontSize, to.shape.fontSize);
		const source = pictureFrom(fromPicture, from);
		const landing = pictureFrom(toPicture, to);
		const growing = to.drawnSize >= from.drawnSize;
		this.#dissolve = new Channel(growing ? 0 : 1);
		this.#reflows = from.wrapped !== to.wrapped;
		this.#small = growing ? source : landing;
		this.#large = growing ? landing : source;
		this.element.style.cssText = 'position:absolute;left:0;top:0';
		this.element.append(fromPicture, toPicture);
		this.write();
	}

	get target(): MorphEntry | null {
		return this.#target;
	}

	set target(entry: MorphEntry | null) {
		this.#target = entry;
		if (entry !== null) {
			const { drawnSize, drawnBaseline } = measureTitle(entry.element);
			this.#size0 = drawnSize;
			this.#baseline0 = drawnBaseline;
		}
	}

	measure(carry: Point): void {
		if (this.#target === null) {
			return;
		}
		const shape = placeTitle(this.#target.element, this.#size0, this.#baseline0);
		const { x, y } = riding(this.#target, carry);
		this.#left.target = shape.left - x;
		this.#baseline.target = shape.baseline - y;
		this.#fontSize.target = shape.fontSize;
	}

	get #grown(): number {
		return unit(this.#fontSize.value, this.#small.size, this.#large.size);
	}

	step(params: SpringParams, seconds: number): void {
		for (const channel of [this.#left, this.#baseline, this.#fontSize]) {
			channel.step(params, seconds);
		}
		const middle = (this.#small.size + this.#large.size) / 2;
		this.#dissolve.target = dissolveTarget(this.#grown, this.#fontSize.target > middle);
		this.#dissolve.step(dissolveSpring, seconds);
	}

	#place(picture: TitlePicture): void {
		const scale = this.#fontSize.value / picture.size;
		const top = this.#baseline.value - picture.baseline * scale;
		const { style } = picture.element;
		if (this.#reflows) {
			style.transform = `translate(${this.#left.value.toFixed(2)}px, ${top.toFixed(2)}px) scale(${scale.toFixed(4)})`;
			return;
		}
		const width = sharedWidth(this.#small.width, this.#large.width, this.#grown);
		const stretch = width / picture.width;
		style.transform = `translate(${this.#left.value.toFixed(2)}px, ${top.toFixed(2)}px) scale(${stretch.toFixed(4)}, ${scale.toFixed(4)})`;
		// A truncated end clips the other picture too, or its extra letters show past the cut.
		const shown = sharedWidth(this.#small.shown, this.#large.shown, this.#grown) / stretch;
		const cut = picture.width - shown;
		style.clipPath = cut > 0.5 ? `inset(-50% ${cut.toFixed(2)}px -50% 0)` : 'none';
	}

	write(): void {
		this.#place(this.#small);
		this.#place(this.#large);
		const { small, large } = dissolveOpacities(this.#dissolve.value);
		this.#small.element.style.opacity = small.toFixed(3);
		this.#large.element.style.opacity = large.toFixed(3);
	}

	get settled(): boolean {
		return (
			this.#left.settled(landsWithin) &&
			this.#baseline.settled(landsWithin) &&
			this.#fontSize.settled(landsWithin / 2) &&
			this.#dissolve.settled(0.02)
		);
	}
}
