import { Channel } from '../channel.ts';
import type { SpringParams } from '../choreography.ts';
import type { MorphEntry } from '../registry.ts';

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
}

/** Measures a title in full, baseline probe included: once per target, never per frame. */
export function measureTitle(element: HTMLElement): TitleMeasure {
	const drawnSize = Number.parseFloat(getComputedStyle(element).fontSize);
	const drawnBaseline = baselineOffset(element);
	return { shape: placeTitle(element, drawnSize, drawnBaseline), drawnSize, drawnBaseline };
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
	picture.style.cssText = `${clipped}position:absolute;left:0;top:0;white-space:nowrap;transform-origin:0 0;will-change:transform,opacity;
		font-family:${style.fontFamily};font-size:${style.fontSize};font-weight:${style.fontWeight};
		letter-spacing:${style.letterSpacing};line-height:${style.lineHeight};color:${style.color}`;
	return picture;
}

/** Something in the air. Every flier ends by landing on, or fading out without, a target. */
export interface Flier {
	readonly element: HTMLElement;
	/** Where it's going; null when the page it was going to has gone. */
	target: MorphEntry | null;
	/** Reads the target's position, and nothing else. Runs before any writes, once per frame. */
	measure(): void;
	step(params: SpringParams, seconds: number): void;
	write(): void;
	readonly settled: boolean;
}

function unit(value: number, low: number, high: number): number {
	return high === low ? 1 : Math.min(1, Math.max(0, (value - low) / (high - low)));
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
	readonly #shadow = document.createElement('div');
	readonly #base: number;
	readonly #small: number;

	constructor(from: CoverShape, target: MorphEntry, picture: Node, shadow: string) {
		const to = measureCover(target.element);
		this.target = target;
		this.#x = new Channel(from.x, to.x);
		this.#y = new Channel(from.y, to.y);
		this.#width = new Channel(from.width, to.width);
		this.#height = new Channel(from.height, to.height);
		this.#rotation = new Channel(from.rotation, to.rotation);
		this.#radius = new Channel(from.radius, to.radius);
		this.#base = Math.max(from.width, to.width);
		this.#small = Math.min(from.width, to.width);

		const size = `${String(this.#base)}px`;
		this.element.style.cssText = `position:absolute;left:0;top:0;width:${size};height:${size};will-change:transform`;
		this.#shadow.style.cssText = `position:absolute;inset:0;box-shadow:${shadow};will-change:opacity`;
		this.#face.style.cssText = 'position:absolute;inset:0;overflow:hidden';
		this.#face.append(picture);
		this.element.append(this.#shadow, this.#face);
		this.write();
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
		}
	}

	measure(): void {
		if (this.#target === null) {
			return;
		}
		const to = placeCover(this.#target.element, this.#rotation0, this.#radius0);
		this.#x.target = to.x;
		this.#y.target = to.y;
		this.#width.target = to.width;
		this.#height.target = to.height;
		this.#rotation.target = to.rotation;
		this.#radius.target = to.radius;
	}

	step(params: SpringParams, seconds: number): void {
		for (const channel of this.#channels) {
			channel.step(params, seconds);
		}
	}

	get #channels(): Channel[] {
		return [this.#x, this.#y, this.#width, this.#height, this.#rotation, this.#radius];
	}

	write(): void {
		const scaleX = this.#width.value / this.#base;
		const scaleY = this.#height.value / this.#base;
		const left = this.#x.value - this.#base / 2;
		const top = this.#y.value - this.#base / 2;
		this.element.style.transform = `translate(${left.toFixed(2)}px, ${top.toFixed(2)}px) rotate(${this.#rotation.value.toFixed(3)}deg) scale(${scaleX.toFixed(4)}, ${scaleY.toFixed(4)})`;
		const radius = `${(Math.max(0, this.#radius.value) / Math.max(scaleX, 0.01)).toFixed(2)}px`;
		this.#face.style.borderRadius = radius;
		this.#shadow.style.borderRadius = radius;
		this.#shadow.style.opacity = unit(this.#width.value, this.#small, this.#base).toFixed(3);
	}

	/** Where the cover is right now, for whatever is following it. */
	get center(): { x: number; y: number } {
		return { x: this.#x.value, y: this.#y.value };
	}

	get settled(): boolean {
		return (
			this.#x.settled(0.4) &&
			this.#y.settled(0.4) &&
			this.#width.settled(0.4) &&
			this.#rotation.settled(0.05)
		);
	}
}

/** Where a bar is. Unlike a cover it changes proportion, so it's drawn at its true size. */
export interface BarShape {
	left: number;
	top: number;
	width: number;
	height: number;
	radius: number;
}

export function measureBar(element: HTMLElement): BarShape {
	const { left, top, width, height } = element.getBoundingClientRect();
	return { left, top, width, height, radius: radiusOf(element, width) };
}

/** A bar in flight: the island's hairline becoming the scrubber's fill, and back. */
export class BarFlier implements Flier {
	readonly element = document.createElement('div');
	target: MorphEntry | null;

	readonly #left: Channel;
	readonly #top: Channel;
	readonly #width: Channel;
	readonly #height: Channel;
	readonly #radius: Channel;

	constructor(from: BarShape, target: MorphEntry, color: string) {
		const to = measureBar(target.element);
		this.target = target;
		this.#left = new Channel(from.left, to.left);
		this.#top = new Channel(from.top, to.top);
		this.#width = new Channel(from.width, to.width);
		this.#height = new Channel(from.height, to.height);
		this.#radius = new Channel(from.radius, to.radius);
		this.element.style.cssText = `position:absolute;left:0;top:0;background:${color}`;
		this.write();
	}

	measure(): void {
		if (this.target === null) {
			return;
		}
		const { left, top, width, height } = this.target.element.getBoundingClientRect();
		this.#left.target = left;
		this.#top.target = top;
		this.#width.target = width;
		this.#height.target = height;
	}

	step(params: SpringParams, seconds: number): void {
		for (const channel of [this.#left, this.#top, this.#width, this.#height, this.#radius]) {
			channel.step(params, seconds);
		}
	}

	write(): void {
		const { style } = this.element;
		style.transform = `translate(${this.#left.value.toFixed(2)}px, ${this.#top.value.toFixed(2)}px)`;
		style.width = `${this.#width.value.toFixed(2)}px`;
		style.height = `${Math.max(0, this.#height.value).toFixed(2)}px`;
		style.borderRadius = `${Math.max(0, this.#radius.value).toFixed(2)}px`;
	}

	get settled(): boolean {
		return (
			this.#left.settled(0.4) &&
			this.#top.settled(0.4) &&
			this.#width.settled(0.4) &&
			this.#height.settled(0.2)
		);
	}
}

interface TitlePicture {
	element: HTMLElement;
	size: number;
	baseline: number;
}

/**
 * A title in flight, as two pictures of the same words: the small one at its true weight and
 * the large one at its. Which one shows follows the font size, not the clock, so the dissolve
 * plays itself backwards if the flight turns around.
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
		const source = { element: fromPicture, size: from.drawnSize, baseline: from.drawnBaseline };
		const landing = { element: toPicture, size: to.drawnSize, baseline: to.drawnBaseline };
		const growing = to.drawnSize >= from.drawnSize;
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

	measure(): void {
		if (this.#target === null) {
			return;
		}
		const shape = placeTitle(this.#target.element, this.#size0, this.#baseline0);
		this.#left.target = shape.left;
		this.#baseline.target = shape.baseline;
		this.#fontSize.target = shape.fontSize;
	}

	step(params: SpringParams, seconds: number): void {
		for (const channel of [this.#left, this.#baseline, this.#fontSize]) {
			channel.step(params, seconds);
		}
	}

	#place(picture: TitlePicture): void {
		const scale = this.#fontSize.value / picture.size;
		const top = this.#baseline.value - picture.baseline * scale;
		picture.element.style.transform = `translate(${this.#left.value.toFixed(2)}px, ${top.toFixed(2)}px) scale(${scale.toFixed(4)})`;
	}

	write(): void {
		this.#place(this.#small);
		this.#place(this.#large);
		const grown = unit(this.#fontSize.value, this.#small.size, this.#large.size);
		const large = unit(grown, 0.01, 0.1);
		this.#large.element.style.opacity = large.toFixed(3);
		this.#small.element.style.opacity = (1 - unit(grown, 0.04, 0.14)).toFixed(3);
	}

	get settled(): boolean {
		return this.#left.settled(0.4) && this.#baseline.settled(0.4) && this.#fontSize.settled(0.15);
	}
}
