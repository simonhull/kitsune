import { Channel } from '../channel.ts';
import type { SpringParams } from '../choreography.ts';

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
	#box = { left: 0, top: 0 };
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

	/** Learns the shape to end on, once the island has its new state. */
	aim(): void {
		this.#restingRadius = Number.parseFloat(
			getComputedStyle(this.#island).getPropertyValue('--island-radius-px')
		);
	}

	measure(): void {
		const { left, top, width, height } = this.#island.getBoundingClientRect();
		this.#box = { left, top };
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
	}

	get settled(): boolean {
		return this.#channels.every((channel) => channel.settled(0.4));
	}

	release(): void {
		for (const property of ['inset', 'left', 'top', 'width', 'height', 'border-radius']) {
			this.#glass.style.removeProperty(property);
		}
	}
}
