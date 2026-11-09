/**
 * The alphabet rail's goo (plan 054, variant C · Snap): the pill under the lit letter is two edges,
 * each a `Channel`. When the letter changes, the edge heading there is at the new letter at once
 * and the trailing edge whips after it on a bouncy spring, so the pill stretches, squashes into
 * place and settles; a drop shed at the old letter shrinks away. Pure: no DOM, no clock of its own.
 * The component steps it from `requestAnimationFrame` and writes its `shape` as transforms.
 */
import { Channel } from '#lib/morph/channel.ts';
import { springParams, type Motion } from '#lib/morph/choreography.ts';

/** The springs, as the board tuned them. */
export const motions = {
	/** The leading edge snaps to its target; this only brings it back after the squash floor. */
	lead: { duration: 120, bounce: 0 },
	/** A finger on the rail is the motion: the lead is critically damped and quick. */
	held: { duration: 140, bounce: 0 },
	/** The trailing edge whips after the lead. Its bounce is the wobble. */
	trail: { duration: 300, bounce: 0.5 },
	/** Busy, or under a finger, the trailing edge tracks without wobbling. */
	trailSteady: { duration: 300, bounce: 0 },
	/** The drop shed at the old letter shrinks away. */
	drop: { duration: 280, bounce: 0 },
	/** The bubble's letter squishes in when it changes. */
	glyph: { duration: 220, bounce: 0.45 }
} as const satisfies Record<string, Motion>;

/** The pill is never longer than its rest length and this many slots. */
export const stretchSlots = 1.6;
/** The pill is never shorter than this share of its rest length. */
export const squashFloor = 0.62;
/** No edge passes its target by more than this share of a slot: there it stops and comes back. */
export const overshootSlots = 1 / 3;
/** The most the pill leans into its speed, in degrees. */
export const maxLean = 2;
/** Letters changing faster than this, in ms, are a sweep: the wobble waits for the last one. */
export const busyGapMs = 120;
/** The longest step taken at once: a frame later than this, the goo jumps rather than tunnels. */
export const maxStepSeconds = 1 / 20;
/** A letter is ink while the pill covers its middle by this many px. */
const inkMargin = 3;
/** How far the drop falls toward the new letter, in px, as it shrinks, and per second after. */
const dropFall = 6;
/** The bubble's letter starts at this scale when it changes. */
const glyphFrom = 0.82;

/** The rail's measures, in px: the height of a letter's slot and the pill's length at rest. */
export interface GooGeometry {
	step: number;
	pill: number;
}

/** What to draw: the pill's centre and length in px, its width as a scale, and its lean in degrees. */
export interface GooShape {
	center: number;
	length: number;
	width: number;
	lean: number;
	/** Shorter than its two round ends, the ends squash too, by this. */
	squash: number;
}

/** The drop shed at the old letter, while it shows: its centre in px and its scale. */
export interface DropShape {
	center: number;
	scale: number;
}

/** The letters the pill covers, by index, both ends included; `to < from` covers none. */
export interface InkRange {
	from: number;
	to: number;
}

/** What a retarget asks the component to do besides stepping. */
export interface Retargeted {
	/** Under reduced motion, the old pill's centre: it fades out there while the new one is in place. */
	ghost?: number;
}

interface Drop {
	center: number;
	direction: number;
	scale: Channel;
	age: number;
}

/** The slot centre of letter `index`, in px from the rail's top. */
export function slotCenter(index: number, step: number): number {
	return (index + 0.5) * step;
}

/**
 * The letters to change from one ink range to the next: each index whose ink differs, with whether
 * it's now ink. Only drawn letters are ever ink; a folded one keeps its slot but has no glyph.
 */
export function inkChanges(
	previous: InkRange,
	next: InkRange,
	drawn: (index: number) => boolean
): [index: number, ink: boolean][] {
	const inside = ({ from, to }: InkRange, index: number) =>
		index >= from && index <= to && drawn(index);
	const changes: [number, boolean][] = [];
	const low = Math.min(previous.from, next.from);
	const high = Math.max(previous.to, next.to);
	for (let index = low; index <= high; index++) {
		const was = inside(previous, index);
		const is = inside(next, index);
		if (was !== is) {
			changes.push([index, is]);
		}
	}
	return changes;
}

function clamp(value: number, low: number, high: number): number {
	return Math.min(high, Math.max(low, value));
}

export class RailGoo {
	#geometry: GooGeometry;
	readonly top: Channel;
	readonly bottom: Channel;
	/** The edge heading for the new letter, chosen again at every retarget from where the pill is. */
	lead: 'top' | 'bottom' = 'bottom';
	/** A finger holds the rail. */
	held = false;
	/** The person asked for reduced motion: the pill moves at once, without travel or lean. */
	reduced = false;
	/** The trailing edge's bounce, scaled down by how far this hop goes. */
	#calm = 1;
	/** When the last retarget came, in ms, and the gap before it. */
	#lastRetarget = Number.NEGATIVE_INFINITY;
	#gap = Number.POSITIVE_INFINITY;
	/** Seconds stepped since the last retarget. */
	#since = Number.POSITIVE_INFINITY;
	#drop: Drop | undefined;
	/** The bubble's letter's scale. */
	readonly glyph = new Channel(1);

	constructor(geometry: GooGeometry, center: number) {
		this.#geometry = geometry;
		this.top = new Channel(center - geometry.pill / 2);
		this.bottom = new Channel(center + geometry.pill / 2);
	}

	get geometry(): GooGeometry {
		return this.#geometry;
	}

	/** Letters are changing faster than a person reads them, and the last change was just now. */
	get busy(): boolean {
		return this.#gap < busyGapMs && this.#since * 1000 < busyGapMs;
	}

	/** Puts the pill at rest at `center`, in a rail measured `geometry`, without moving there. */
	place(center: number, geometry: GooGeometry = this.#geometry): void {
		this.#geometry = geometry;
		const half = geometry.pill / 2;
		for (const [edge, at] of [
			[this.top, center - half],
			[this.bottom, center + half]
		] as const) {
			edge.value = edge.target = at;
			edge.velocity = 0;
		}
		this.#drop = undefined;
	}

	/**
	 * Sends the pill to `center` at `now` (ms). The leading edge is there at once; the trailing
	 * edge's position and speed carry on, so a retarget mid-flight never restarts or reverses.
	 */
	retarget(center: number, now: number): Retargeted {
		this.#gap = now - this.#lastRetarget;
		this.#lastRetarget = now;
		this.#since = 0;
		const { step, pill } = this.#geometry;
		const from = this.center;
		if (this.reduced) {
			this.place(center);
			return from === center ? {} : { ghost: from };
		}
		this.lead = center >= from ? 'bottom' : 'top';
		// The bounce is for a hop of a letter or two: a long one lands with the same few px.
		this.#calm = Math.min(1, (2 * step) / Math.max(step, Math.abs(center - from)));
		this.top.target = center - pill / 2;
		this.bottom.target = center + pill / 2;
		const head = this.#head;
		head.value = head.target;
		head.velocity = 0;
		this.#stretch();
		if (Math.abs(center - from) >= step / 2 && !this.busy && !this.held) {
			this.#drop = {
				center: from,
				direction: Math.sign(center - from),
				scale: new Channel(1, 0),
				age: 0
			};
		}
		return {};
	}

	/** Squishes the bubble's letter in, as it changes under the finger. */
	squish(): void {
		if (!this.reduced) {
			this.glyph.value = glyphFrom;
			this.glyph.target = 1;
		}
	}

	/** Advances by `seconds`, clamped to `[0, maxStepSeconds]`: a late frame never runs backwards. */
	step(seconds: number): void {
		const dt = clamp(seconds, 0, maxStepSeconds);
		if (!(dt > 0)) {
			return;
		}
		this.#since += dt;
		const steady = this.busy || this.held;
		const trail = steady
			? motions.trailSteady
			: { duration: motions.trail.duration, bounce: motions.trail.bounce * this.#calm };
		this.#head.step(springParams(this.held ? motions.held : motions.lead), dt);
		this.#tail.step(springParams(trail), dt);
		this.#cap();
		if (this.#drop !== undefined) {
			this.#drop.scale.step(springParams(motions.drop), dt);
			this.#drop.age += dt;
			if (this.#drop.scale.value <= 0.02) {
				this.#drop = undefined;
			}
		}
		this.glyph.step(springParams(motions.glyph), dt);
	}

	/** The overshoot cap, then the stretch cap and the squash floor, which couple the two edges. */
	#cap(): void {
		const over = this.#geometry.step * overshootSlots;
		for (const edge of [this.top, this.bottom]) {
			const past = edge.value - edge.target;
			if (Math.abs(past) > over && Math.sign(past) === Math.sign(edge.velocity)) {
				edge.value = edge.target + Math.sign(past) * over;
				edge.velocity = 0;
			}
		}
		this.#stretch();
	}

	/**
	 * The stretch cap and the squash floor. Held at either, the edges stop growing apart (or
	 * together) but keep any speed that brings the pill back: the tail chasing a snapped head
	 * keeps its pace, so the curve doesn't depend on how often it's stepped.
	 */
	#stretch(): void {
		const { step, pill } = this.#geometry;
		const most = pill + stretchSlots * step;
		const least = pill * squashFloor;
		const length = this.bottom.value - this.top.value;
		const growing = this.bottom.velocity - this.top.velocity;
		if (length > most) {
			// Dragged along: the tail is held at the cap, and moves at least with the head.
			if (this.lead === 'bottom') {
				this.top.value = this.bottom.value - most;
			} else {
				this.bottom.value = this.top.value + most;
			}
			if (growing > 0) {
				this.#tail.velocity = this.#head.velocity;
			}
		} else if (length < least) {
			const middle = (this.top.value + this.bottom.value) / 2;
			this.top.value = middle - least / 2;
			this.bottom.value = middle + least / 2;
			if (growing < 0) {
				const shared = (this.top.velocity + this.bottom.velocity) / 2;
				this.top.velocity = this.bottom.velocity = shared;
			}
		}
	}

	get #head(): Channel {
		return this.lead === 'bottom' ? this.bottom : this.top;
	}

	get #tail(): Channel {
		return this.lead === 'bottom' ? this.top : this.bottom;
	}

	get center(): number {
		return (this.top.value + this.bottom.value) / 2;
	}

	/** The pill as it is now. It thins as it stretches, fattens as it squashes, leans into its speed. */
	shape(): GooShape {
		const { pill } = this.#geometry;
		const length = this.bottom.value - this.top.value;
		const speed = (this.top.velocity + this.bottom.velocity) / 2;
		return {
			center: this.center,
			length,
			width: clamp(Math.sqrt(pill / Math.max(length, Number.EPSILON)), 0.8, 1.08),
			lean: this.reduced ? 0 : clamp(-speed * 0.004, -maxLean, maxLean),
			squash: Math.min(1, length / pill)
		};
	}

	/** The drop shed at the old letter, falling toward the new one as it shrinks, while it shows. */
	dropShape(): DropShape | undefined {
		const drop = this.#drop;
		if (drop === undefined) {
			return undefined;
		}
		const scale = Math.max(0, drop.scale.value);
		const fall = dropFall * (1 - scale + drop.age);
		return { center: drop.center + drop.direction * fall, scale };
	}

	/** The letters whose middle the pill covers now. */
	inked(): InkRange {
		const { step } = this.#geometry;
		return {
			from: Math.ceil((this.top.value + inkMargin) / step - 0.5),
			to: Math.floor((this.bottom.value - inkMargin) / step - 0.5)
		};
	}

	/** Nothing is left to move: the loop can stop. */
	resting(): boolean {
		return (
			this.top.settled(0.05) &&
			this.bottom.settled(0.05) &&
			this.#drop === undefined &&
			this.glyph.settled(0.002)
		);
	}
}
