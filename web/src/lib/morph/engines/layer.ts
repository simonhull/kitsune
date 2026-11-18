import { Channel, landsWithin } from '../channel.ts';
import { springParams, type Part, type Pose, type SpringParams } from '../choreography.ts';
import { motionOf } from '../journey.ts';

const rest = { opacity: 1, y: 0, scale: 1, rotate: 0 } as const;

function complete(pose: Pose): Required<Pose> {
	return { ...rest, ...pose };
}

/** A whole element moving as one: a page, a ghost, the stage. */
export class Layer {
	readonly element: HTMLElement;
	readonly opacity: Channel;
	readonly y: Channel;
	readonly scale: Channel;
	readonly rotate: Channel;
	#params: SpringParams;
	#fade: SpringParams;
	#wait: number;
	/**
	 * The point it moves around, as set when it started. Kept and written with every frame: a page
	 * that sets its own `style` attribute as it renders would otherwise wipe it in mid-flight.
	 */
	readonly #origin: string;

	constructor(element: HTMLElement, part: Part, delay = part.delay) {
		const from = complete(part.from);
		const to = complete(part.to);
		this.element = element;
		this.opacity = new Channel(from.opacity, to.opacity);
		this.y = new Channel(from.y, to.y);
		this.scale = new Channel(from.scale, to.scale);
		this.rotate = new Channel(from.rotate, to.rotate);
		this.#params = springParams(motionOf(part.motion));
		this.#fade = springParams(motionOf(part.fade ?? part.motion));
		this.#wait = delay / 1000;
		this.#origin = element.style.transformOrigin;
		element.style.willChange = 'transform, opacity';
		// A backdrop blur recomputes every frame anything under it moves; in flight it's unseen.
		element.style.backdropFilter = 'none';
		this.write();
	}

	/** Keeps going from wherever it is, toward a new pose. */
	retarget(part: Part): void {
		const to = complete(part.to);
		this.opacity.target = to.opacity;
		this.y.target = to.y;
		this.scale.target = to.scale;
		this.rotate.target = to.rotate;
		this.#params = springParams(motionOf(part.motion));
		this.#fade = springParams(motionOf(part.fade ?? part.motion));
		this.#wait = 0;
	}

	/** Continues another layer's motion on this element. */
	adopt(other: Layer): void {
		this.opacity.adopt(other.opacity);
		this.y.adopt(other.y);
		this.scale.adopt(other.scale);
		this.rotate.adopt(other.rotate);
		this.#wait = 0;
		this.write();
	}

	step(seconds: number): void {
		const moving = seconds - this.#wait;
		this.#wait = Math.max(0, this.#wait - seconds);
		if (moving <= 0) {
			return;
		}
		this.opacity.step(this.#fade, moving);
		for (const channel of [this.y, this.scale, this.rotate]) {
			channel.step(this.#params, moving);
		}
	}

	write(): void {
		const { style } = this.element;
		style.opacity = String(Math.min(1, Math.max(0, this.opacity.value)));
		if (this.#origin !== '') {
			style.transformOrigin = this.#origin;
		}
		style.transform = `translateY(${this.y.value.toFixed(2)}px) rotate(${this.rotate.value.toFixed(3)}deg) scale(${this.scale.value.toFixed(4)})`;
	}

	get settled(): boolean {
		return (
			this.#wait === 0 &&
			this.opacity.settled(0.01) &&
			this.y.settled(landsWithin) &&
			this.scale.settled(0.002) &&
			this.rotate.settled(0.2)
		);
	}

	/** Hands the element back to its stylesheet. */
	release(): void {
		const { style } = this.element;
		style.removeProperty('opacity');
		style.removeProperty('transform');
		style.removeProperty('transform-origin');
		style.removeProperty('will-change');
		style.removeProperty('backdrop-filter');
	}
}
