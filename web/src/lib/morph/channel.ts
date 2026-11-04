import { stepSpring, type SpringParams } from './choreography.ts';

/**
 * One animated number. It always knows where it is and how fast it's going, so changing the
 * target mid-flight carries on from here, at this speed: a snap can't be expressed.
 */
export class Channel {
	value: number;
	velocity = 0;
	target: number;

	constructor(value: number, target = value) {
		this.value = value;
		this.target = target;
	}

	step(params: SpringParams, seconds: number): void {
		const { offset, velocity } = stepSpring(
			params,
			{ offset: this.value - this.target, velocity: this.velocity },
			seconds
		);
		this.value = this.target + offset;
		this.velocity = velocity;
	}

	/** Reports whether it has come to rest within `tolerance`, in its own units. */
	settled(tolerance: number): boolean {
		return (
			Math.abs(this.value - this.target) < tolerance && Math.abs(this.velocity) < tolerance * 10
		);
	}

	/** Takes over another channel's position and speed. */
	adopt(other: Channel): void {
		this.value = other.value;
		this.velocity = other.velocity;
	}
}
