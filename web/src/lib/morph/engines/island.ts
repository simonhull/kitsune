import { Channel, landsWithin } from '../channel.ts';
import type { SpringParams } from '../choreography.ts';
import { carryOffset, type Point } from './path.ts';

/**
 * The island's glass in flight. The island's box goes straight to its final place, so that's
 * always the target; the glass is drawn wherever it has got to, at its true size and radius.
 * A shape really becoming another shape: nothing here is a scaled box.
 */
export class IslandShape {
	readonly #island: HTMLElement;
	readonly #glass: HTMLElement;
	readonly #left: Channel;
	readonly #top: Channel;
	readonly #width: Channel;
	readonly #height: Channel;
	readonly #radius: Channel;
	#box = { left: 0, top: 0, width: 0, height: 0 };
	/** What rides the glass, and where its middle was. `null` means the island's final place. */
	readonly #carried = new Map<HTMLElement, Point | null>();
	#restingRadius = 0;

	/** Starts the glass from where it is, before the page changes. */
	constructor(island: HTMLElement, glass: HTMLElement) {
		const { left, top, width, height } = glass.getBoundingClientRect();
		const radius = Math.min(
			Number.parseFloat(getComputedStyle(glass).borderTopLeftRadius),
			height / 2
		);
		this.#island = island;
		this.#glass = glass;
		this.#left = new Channel(left);
		this.#top = new Channel(top);
		this.#width = new Channel(width);
		this.#height = new Channel(height);
		this.#radius = new Channel(radius);
	}

	get #center(): Point {
		return {
			x: this.#left.value + this.#width.value / 2,
			y: this.#top.value + this.#height.value / 2
		};
	}

	/** Finds how far the glass is from its final place, which is how far its contents ride. */
	get offset(): Point {
		const { left, top, width, height } = this.#box;
		return carryOffset(this.#center, { x: left + width / 2, y: top + height / 2 });
	}

	/** Takes something that was in the island along with the glass, from where it is now. */
	carryFromHere(element: HTMLElement): void {
		this.#carried.set(element, this.#center);
	}

	/** Takes something bound for the island along with the glass, until they arrive together. */
	carryHome(element: HTMLElement): void {
		this.#carried.set(element, null);
	}

	/** Learns the shape to end on, once the island has its new state. */
	aim(): void {
		this.#restingRadius = Number.parseFloat(
			getComputedStyle(this.#island).getPropertyValue('--island-radius-px')
		);
	}

	measure(): void {
		const { left, top, width, height } = this.#island.getBoundingClientRect();
		this.#box = { left, top, width, height };
		const radius = this.#restingRadius;
		this.#left.target = left;
		this.#top.target = top;
		this.#width.target = width;
		this.#height.target = height;
		this.#radius.target = Math.min(radius, height / 2);
	}

	step(params: SpringParams, seconds: number): void {
		for (const channel of this.#channels) {
			channel.step(params, seconds);
		}
	}

	get #channels(): Channel[] {
		return [this.#left, this.#top, this.#width, this.#height, this.#radius];
	}

	write(): void {
		const { left, top } = this.#box;
		const { style } = this.#glass;
		style.inset = 'auto';
		style.left = `${(this.#left.value - left).toFixed(2)}px`;
		style.top = `${(this.#top.value - top).toFixed(2)}px`;
		style.width = `${this.#width.value.toFixed(2)}px`;
		style.height = `${this.#height.value.toFixed(2)}px`;
		style.borderRadius = `${Math.max(0, this.#radius.value).toFixed(2)}px`;

		const center = this.#center;
		const rest = { x: left + this.#box.width / 2, y: top + this.#box.height / 2 };
		for (const [element, anchor] of this.#carried) {
			if (!element.isConnected) {
				this.#carried.delete(element);
				continue;
			}
			const { x, y } = carryOffset(center, anchor ?? rest);
			// `translate`, not `transform`: a layer may own that one for the same element.
			element.style.translate = `${x.toFixed(2)}px ${y.toFixed(2)}px`;
		}
	}

	get settled(): boolean {
		return this.#channels.every((channel) => channel.settled(landsWithin));
	}

	release(): void {
		for (const property of ['inset', 'left', 'top', 'width', 'height', 'border-radius']) {
			this.#glass.style.removeProperty(property);
		}
		for (const element of this.#carried.keys()) {
			element.style.removeProperty('translate');
		}
		this.#carried.clear();
	}
}
