import { Channel } from '../channel.ts';
import type { SpringParams } from '../choreography.ts';

/** Rescales every length in a computed `box-shadow`, leaving its colors alone. */
export function scaleShadow(shadow: string, factor: number): string {
	return shadow.replaceAll(/(-?[\d.]+)px/g, (_, length: string) => {
		return `${(Number(length) * factor).toFixed(2)}px`;
	});
}

/**
 * A shadow in flight: two real shadows and how far it has got from one to the other. Each end
 * shows exactly what's there at rest, so nothing pops on the first or last frame, and a turn in
 * mid-air fades back the way it came.
 */
export class ShadowMix {
	/** The two shadows. Index 1 is the one `weight` counts towards. */
	readonly shadows: [string, string];
	readonly weight = new Channel(0);

	constructor(from: string, to: string) {
		this.shadows = [from, to];
		this.weight.target = 1;
	}

	/** Turns towards a shadow. A third one takes the place of whichever shows least. */
	aim(shadow: string): void {
		const [first, second] = this.shadows;
		if (shadow === second) {
			this.weight.target = 1;
		} else if (shadow === first) {
			this.weight.target = 0;
		} else if (this.weight.value < 0.5) {
			this.shadows[1] = shadow;
			this.weight.target = 1;
		} else {
			this.shadows[0] = shadow;
			this.weight.target = 0;
		}
	}

	step(params: SpringParams, seconds: number): void {
		this.weight.step(params, seconds);
	}

	/** Reports how much of each shadow shows, as opacities. */
	get opacities(): [number, number] {
		const weight = Math.min(1, Math.max(0, this.weight.value));
		return [1 - weight, weight];
	}
}
