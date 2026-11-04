/** The tokens in `tokens.css`, which stay the one place a morph's length is set. */
export type MotionToken = '--morph-fast' | '--morph' | '--morph-slow';

/** A curve as designers say it: how long it feels, and how much it overshoots (0 to 1). */
export interface Motion {
	/** Perceived duration in milliseconds. */
	duration: number;
	bounce: number;
}

export interface MotionSpec {
	token: MotionToken;
	bounce: number;
}

export interface SpringParams {
	stiffness: number;
	damping: number;
}

/** Converts `(duration, bounce)` to a unit-mass spring, the way SwiftUI's `spring` does. */
export function springParams({ duration, bounce }: Motion): SpringParams {
	const omega = (2 * Math.PI) / (duration / 1000);
	return { stiffness: omega * omega, damping: 2 * omega * (1 - bounce) };
}

/** A spring's state relative to its target: 0 is at rest. */
export interface SpringState {
	offset: number;
	velocity: number;
}

/** Advances a spring by `seconds` in closed form, so any frame rate gives the same curve. */
export function stepSpring(
	{ stiffness, damping }: SpringParams,
	{ offset, velocity }: SpringState,
	seconds: number
): SpringState {
	const omega = Math.sqrt(stiffness);
	const zeta = damping / (2 * omega);
	if (zeta >= 1 - 1e-6) {
		const decay = Math.exp(-omega * seconds);
		const rate = velocity + omega * offset;
		return {
			offset: (offset + rate * seconds) * decay,
			velocity: (velocity - omega * rate * seconds) * decay
		};
	}
	const damped = omega * Math.sqrt(1 - zeta * zeta);
	const decay = Math.exp(-zeta * omega * seconds);
	const cosine = Math.cos(damped * seconds);
	const sine = Math.sin(damped * seconds);
	const rate = (velocity + zeta * omega * offset) / damped;
	const next = decay * (offset * cosine + rate * sine);
	return {
		offset: next,
		velocity:
			decay *
			(velocity * cosine - ((offset * omega * omega + zeta * omega * velocity) / damped) * sine)
	};
}

/** How close to the target counts as landed, as a fraction of the distance travelled. */
const settledWithin = 0.002;
const sampleStep = 1 / 240;

/** Returns how long, in milliseconds, a unit step takes to come to rest. */
export function settleTime(motion: Motion): number {
	const params = springParams(motion);
	let state: SpringState = { offset: 1, velocity: 0 };
	let lastOutside = 0;
	for (let t = sampleStep; t < 10; t += sampleStep) {
		state = stepSpring(params, state, sampleStep);
		if (Math.abs(state.offset) > settledWithin) {
			lastOutside = t;
		}
	}
	return Math.ceil((lastOutside + sampleStep) * 1000);
}

/** Emits the spring's unit step as a CSS `linear()` easing, sampled over its settle time. */
export function linearEasing(motion: Motion, samples = 48): string {
	const params = springParams(motion);
	const interval = settleTime(motion) / 1000 / samples;
	const points = ['0'];
	let state: SpringState = { offset: 1, velocity: 0 };
	for (let index = 1; index < samples; index += 1) {
		state = stepSpring(params, state, interval);
		points.push((1 - state.offset).toFixed(4));
	}
	points.push('1');
	return `linear(${points.join(', ')})`;
}

/** The parts of a moving thing a choreography sets. Unset means at rest. */
export interface Pose {
	opacity?: number;
	y?: number;
	scale?: number;
}

/**
 * A wave across a page's units, centered on the cover. Outward starts at the cover and spreads:
 * the way a page falls back. Inward starts at the edges and closes on it: the same fall, played
 * in reverse.
 */
export interface Ripple {
	order: 'outward' | 'inward';
	/** How fast the wave crosses the page, in pixels per millisecond. */
	speed: number;
	/** The most it can add to a part's delay, in milliseconds. */
	limit: number;
}

/** The delay a ripple adds at `distance` from the cover, on a page reaching out to `farthest`. */
export function rippleDelay(ripple: Ripple, distance: number, farthest: number): number {
	const steps = ripple.order === 'outward' ? distance : farthest - distance;
	return Math.min(ripple.limit, Math.max(0, steps) / ripple.speed);
}

/** One thing that moves, from one pose to another, starting `delay` into the morph. */
export interface Part {
	from: Pose;
	to: Pose;
	/** Milliseconds after the morph starts. */
	delay: number;
	motion: MotionSpec;
	ripple?: Ripple;
}

/** What a page's unit is, from `data-morph-part`. It decides how the unit arrives. */
export type Role = 'card' | 'art' | 'words' | 'panel';

export type RouteName = 'home' | 'album' | 'now-playing';

export type ChoreographyName = 'home-to-album' | 'album-to-home' | 'blend';

interface SharedTravel {
	/** Which end of the journey the album's ID comes from. */
	subject: 'from' | 'to';
	sourceSlots: readonly ('card' | 'hero')[];
	targetSlots: readonly ('card' | 'hero' | 'island')[];
}

/** The light around the cover: its scale at the cover and at full size, and its strength. */
export interface Bloom {
	direction: 'out' | 'in';
	rest: number;
	full: number;
	peak: number;
}

export interface Choreography {
	name: ChoreographyName;
	/** The curve shared elements fly on. */
	motion: MotionSpec;
	travel: SharedTravel | null;
	/** Every unit of the outgoing page. */
	leaving: Part;
	/** The incoming page's units, by role. */
	arriving: Partial<Record<Role, Part>>;
	/** The blurred album stage. */
	stage: Part | null;
	/** The light around the cover. */
	bloom: Bloom | null;
}

const bloomShape = { rest: 0.12, full: 2.6, peak: 0.9 } as const;

const slow: MotionSpec = { token: '--morph-slow', bounce: 0.08 };
const base: MotionSpec = { token: '--morph', bounce: 0.04 };
const quick: MotionSpec = { token: '--morph-fast', bounce: 0 };

/**
 * Home to Album. The cover lifts out of the grid and grows into the hero; its color breaks out
 * around it and floods the stage; the card's label swells into the headline. Home sinks away
 * behind all of it, and the tracks rise last, because they are what you came for.
 */
const homeToAlbum: Choreography = {
	name: 'home-to-album',
	motion: slow,
	travel: { subject: 'to', sourceSlots: ['card'], targetSlots: ['hero'] },
	leaving: {
		from: {},
		to: { opacity: 0, scale: 0.92 },
		delay: 0,
		motion: quick,
		ripple: { order: 'outward', speed: 3, limit: 170 }
	},
	arriving: {
		words: { from: { opacity: 0, y: 28 }, to: {}, delay: 200, motion: base },
		panel: { from: { opacity: 0, y: 120 }, to: {}, delay: 140, motion: slow }
	},
	stage: { from: { opacity: 0, scale: 1.08 }, to: {}, delay: 120, motion: slow },
	bloom: { direction: 'out', ...bloomShape }
};

/**
 * Album to Home, the same journey played back and shorter: leaving is easier than arriving.
 * The hero drops into the card it came from, the color drains back into it, and Home comes
 * forward out of the depth it sank into.
 */
const albumToHome: Choreography = {
	name: 'album-to-home',
	motion: base,
	travel: { subject: 'from', sourceSlots: ['hero'], targetSlots: ['card', 'island'] },
	leaving: { from: {}, to: { opacity: 0, y: 56 }, delay: 0, motion: quick },
	arriving: {
		// Held back until the album has mostly gone, so two pages never compete.
		card: {
			from: { opacity: 0, scale: 0.92 },
			to: {},
			delay: 130,
			motion: base,
			ripple: { order: 'inward', speed: 5, limit: 100 }
		}
	},
	stage: { from: {}, to: { opacity: 0 }, delay: 0, motion: base },
	bloom: { direction: 'in', ...bloomShape }
};

/** Everything without its own journey yet, and all of reduced motion: never a hard cut. */
export const blend: Choreography = {
	name: 'blend',
	motion: quick,
	travel: null,
	leaving: { from: {}, to: { opacity: 0 }, delay: 0, motion: quick },
	arriving: {},
	stage: null,
	bloom: null
};

/**
 * How strong the bloom is at a given openness, from 0 at the cover to 1 at full size. This is
 * the one bloom curve: it depends only on the bloom's size, never on the clock or the direction,
 * so a turn in mid-air can't make it jump.
 */
export function bloomStrength(open: number, peak: number): number {
	const clamped = Math.min(1, Math.max(0, open));
	return peak * Math.min(1, clamped / 0.2) * (1 - clamped ** 6);
}

/** Finds how open a bloom of this scale is. */
export function bloomOpenness(scale: number, { rest, full }: Bloom): number {
	return (scale - rest) / (full - rest);
}

/** The length of a blend, in milliseconds. */
export const blendDuration = 120;

/** Finds the choreography between two routes. */
export function choreographyFor(from: RouteName, to: RouteName): Choreography {
	if (from === 'home' && to === 'album') {
		return homeToAlbum;
	}
	if (from === 'album' && to === 'home') {
		return albumToHome;
	}
	return blend;
}

/** Reads a motion's duration from its token. */
export function resolveMotion(spec: MotionSpec, read: (token: MotionToken) => number): Motion {
	return { duration: read(spec.token), bounce: spec.bounce };
}

/** Parses a CSS time such as `420ms` or `0.6s` to milliseconds. */
export function parseTime(value: string): number {
	const trimmed = value.trim();
	const number = Number.parseFloat(trimmed);
	return trimmed.endsWith('ms') ? number : number * 1000;
}
