import type { MorphSlot } from './registry.ts';

/** The tokens in `tokens.css`, which stay the one place a morph's length is set. */
export type MotionToken = '--morph-fast' | '--morph' | '--morph-slow' | '--morph-drop';

/** A curve as designers say it: how long it feels, and how much it overshoots (0 to 1). */
export interface Motion {
	/** Perceived duration in milliseconds. */
	duration: number;
	bounce: number;
}

export interface MotionSpec {
	token: MotionToken;
	bounce: number;
	/** A share of the token's length, for what must finish inside it: exits are shorter. */
	pace?: number;
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
 * How a page answers the cover leaving it. This is about amplitude, not a queue: the units next
 * to the cover move most, the far ones barely move and only fade, and the spread in time is
 * under three frames. A uniform stagger would read as a metronome on something opened many
 * times a day, and it would hold back nothing useful anyway.
 */
export interface Ripple {
	/** The delay at the far edge of the page, in milliseconds. */
	spread: number;
	/** How much of the movement is left at the far edge, from 0 to 1. */
	floor: number;
}

/** The delay of a unit `share` of the way across the page, from 0 at the cover to 1. */
export function rippleDelay({ spread }: Ripple, share: number): number {
	return spread * share;
}

/** How much of the movement a unit keeps, `share` of the way across the page. */
export function rippleWeight({ floor }: Ripple, share: number): number {
	return floor + (1 - floor) * (1 - share) ** 2;
}

function weighPose(pose: Pose, weight: number): Pose {
	return {
		...pose,
		...(pose.scale === undefined ? {} : { scale: 1 + (pose.scale - 1) * weight }),
		...(pose.y === undefined ? {} : { y: pose.y * weight })
	};
}

/** Scales a part's movement, leaving its opacity alone: far units still fade. */
export function weighPart(part: Part, weight: number): Part {
	return { ...part, from: weighPose(part.from, weight), to: weighPose(part.to, weight) };
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
export type Role =
	'intro' | 'card' | 'art' | 'words' | 'panel' | 'line-1' | 'line-2' | 'details' | 'chrome';

export type StageName = 'album' | 'poster';

export type RouteName = 'login' | 'home' | 'album' | 'now-playing';

export type ChoreographyName =
	| 'home-to-album'
	| 'album-to-home'
	| 'home-to-now-playing'
	| 'now-playing-to-home'
	| 'album-to-now-playing'
	| 'now-playing-to-album'
	| 'login-to-home'
	| 'login-to-album'
	| 'blend';

interface SharedTravel {
	/** Whose things travel: the album at either end of the journey, what's playing, or the mark. */
	subject: 'from' | 'to' | 'playing' | 'fox';
	sourceSlots: readonly MorphSlot[];
	targetSlots: readonly MorphSlot[];
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
	/** The two full-bleed stages behind the page. */
	stages: Partial<Record<StageName, Part>>;
	bloom: Bloom | null;
	/** What's inside the island, when it changes shape on this journey. */
	island: Part | null;
	/** How the island first appears, on the one journey that starts without it. */
	islandForms?: Part;
}

const bloomShape = { rest: 0.12, full: 2.6, peak: 0.9 } as const;

// All critically damped: a tap carries no momentum, so nothing has a reason to overshoot.
const slow: MotionSpec = { token: '--morph-slow', bounce: 0 };
const base: MotionSpec = { token: '--morph', bounce: 0 };
const quick: MotionSpec = { token: '--morph-fast', bounce: 0 };
/** Exits go together and go fast: the user has already decided to leave. */
const exit: MotionSpec = { ...quick, pace: 0.7 };

// The app is one axis of depth: Home, then Album, then Now playing. Every journey is built from
// the same few verbs on that axis, so going deeper and coming back are opposite directions of
// one path. Going deeper, the art comes toward you, the page you're on falls back in a ripple
// from it, and what's next rises from below. Coming back is the same path the other way,
// shorter: the deep page sinks, and the shallower one comes forward around the art.

const fallBack: Part = {
	from: {},
	to: { opacity: 0, scale: 0.9 },
	delay: 0,
	motion: exit,
	ripple: { spread: 40, floor: 0.08 }
};

const sink: Part = { from: {}, to: { opacity: 0, y: 56 }, delay: 0, motion: exit };

/** Held back only until the deeper page has mostly gone, so two pages never compete. */
const comeForward: Part = {
	from: { opacity: 0, scale: 0.9 },
	to: {},
	delay: 90,
	motion: base,
	ripple: { spread: 0, floor: 0.08 }
};

/** A cover landing on the deck: it comes down onto the pile from just above it. */
export const drop: Part = {
	from: { opacity: 0, y: -28, scale: 1.08 },
	to: {},
	delay: 0,
	motion: { token: '--morph-drop', bounce: 0 }
};

/** The same landing with nothing moving, for reduced motion. */
export const crossfade: Part = { from: { opacity: 0 }, to: {}, delay: 0, motion: quick };

function rise(y: number, delay: number, motion: MotionSpec = base): Part {
	return { from: { opacity: 0, y }, to: {}, delay, motion };
}

function fadeOut(delay: number, motion: MotionSpec): Part {
	return { from: {}, to: { opacity: 0 }, delay, motion };
}

function fadeIn(delay: number, motion: MotionSpec): Part {
	return { from: { opacity: 0 }, to: {}, delay, motion };
}

/** The poster title: behind the cover, in its wake, one line after the other. */
function posterLine(delay: number): Part {
	return { from: { opacity: 0, y: 72, scale: 0.85 }, to: {}, delay, motion: slow };
}

const nowPlayingArrives: Choreography['arriving'] = {
	'line-1': posterLine(130),
	'line-2': posterLine(230),
	// One beat for everything else: order among these would mean nothing.
	details: rise(8, 240),
	panel: rise(24, 240),
	chrome: fadeIn(240, base)
};

// The island's old contents leave before the new ones arrive: it's changing size in both
// dimensions, and two rich layouts inside it at once would be chaos. Opening waits longest,
// because the fox bar has to pass the time label on its way to the scrubber.
const islandOpens = fadeIn(280, base);
const islandCloses = fadeIn(170, base);

const playing = {
	in: { subject: 'playing', targetSlots: ['poster'] },
	out: { subject: 'playing', sourceSlots: ['poster'] }
} as const;

/**
 * Home to Album. The cover lifts out of the grid and grows into the hero; its color breaks out
 * around it and floods the stage; the card's label swells into the headline. Home sinks away
 * behind all of it, and the tracks rise last, because they are what you came for.
 */
const homeToAlbum: Choreography = {
	name: 'home-to-album',
	motion: slow,
	travel: { subject: 'to', sourceSlots: ['card', 'deck'], targetSlots: ['hero'] },
	leaving: fallBack,
	arriving: { words: rise(20, 160), panel: rise(96, 160, slow) },
	stages: { album: { from: { opacity: 0, scale: 1.08 }, to: {}, delay: 120, motion: slow } },
	bloom: { direction: 'out', ...bloomShape },
	island: null
};

/**
 * Album to Home, the same journey played back and shorter: leaving is easier than arriving.
 * The hero drops into the card it came from, the color drains back into it, and Home comes
 * forward out of the depth it sank into.
 */
const albumToHome: Choreography = {
	name: 'album-to-home',
	motion: base,
	travel: { subject: 'from', sourceSlots: ['hero'], targetSlots: ['card', 'deck', 'island'] },
	leaving: sink,
	arriving: { intro: comeForward, card: comeForward },
	stages: { album: fadeOut(0, base) },
	bloom: { direction: 'in', ...bloomShape },
	island: null
};

/**
 * Home to Now playing: two levels down in one move. The island's cover rises to the center of
 * the stage and the song's name follows it up, the rest of the island gathers into the pill
 * controls, and the poster title climbs out from behind the cover, a line at a time.
 */
const homeToNowPlaying: Choreography = {
	name: 'home-to-now-playing',
	motion: slow,
	travel: { ...playing.in, sourceSlots: ['island'] },
	leaving: fallBack,
	arriving: nowPlayingArrives,
	stages: { poster: fadeIn(120, slow) },
	bloom: { direction: 'out', ...bloomShape },
	island: islandOpens
};

/** Now playing to Home: everything condenses back into the island, and Home comes forward. */
const nowPlayingToHome: Choreography = {
	name: 'now-playing-to-home',
	motion: base,
	travel: { ...playing.out, targetSlots: ['island'] },
	leaving: sink,
	arriving: {
		// Held until the poster title clears: two words that size can't share a stage.
		intro: { ...comeForward, delay: 140 },
		card: comeForward
	},
	stages: { poster: fadeOut(0, base) },
	bloom: { direction: 'in', ...bloomShape },
	island: islandCloses
};

/**
 * Album to Now playing: one level deeper. The stage is already this album's, so nothing blooms;
 * it stays and deepens. The hero moves to the center if it can; otherwise the island's cover
 * rises and the hero falls back with the page. The album's headline gives way to the song's.
 */
const albumToNowPlaying: Choreography = {
	name: 'album-to-now-playing',
	motion: slow,
	travel: { ...playing.in, sourceSlots: ['hero', 'island'] },
	leaving: fallBack,
	// Later than from Home: the album's headline is in the way until it has fallen back.
	arriving: { ...nowPlayingArrives, 'line-1': posterLine(200), 'line-2': posterLine(290) },
	stages: { poster: fadeIn(60, slow), album: fadeOut(460, quick) },
	bloom: null,
	island: islandOpens
};

/** Now playing to Album: the poster lifts off the stage it was deepening, and the album returns. */
const nowPlayingToAlbum: Choreography = {
	name: 'now-playing-to-album',
	motion: base,
	travel: { ...playing.out, targetSlots: ['hero', 'island'] },
	leaving: sink,
	arriving: {
		art: comeForward,
		// The headline waits for the poster title to clear: two words that size can't share a stage.
		words: { ...comeForward, ripple: undefined, delay: 140 },
		panel: rise(72, 90)
	},
	stages: { poster: fadeOut(0, base) },
	bloom: null,
	island: islandCloses
};

const throughTheDoor = { subject: 'fox', sourceSlots: ['door'], targetSlots: ['nav'] } as const;

/** One beat behind the page: the island closes the frame the fox has opened. */
const islandForms = rise(32, 220);

/**
 * Login to Home. The door is the shallowest place on the axis, so this is going deeper: the fox
 * leaves the form and takes its seat in the nav as the glass forms around it, the form falls back,
 * and Home rises under them with the island closing the frame from below. Login to Now playing
 * has no fox's journey to make: it blends, and so does anything back to the door.
 */
const loginToHome: Choreography = {
	name: 'login-to-home',
	motion: slow,
	travel: throughTheDoor,
	leaving: fallBack,
	arriving: { intro: rise(20, 160), card: rise(48, 160, slow) },
	stages: {},
	bloom: null,
	island: null,
	islandForms
};

/** Login to Album: the same door, returning after an expired session to the album left behind. */
const loginToAlbum: Choreography = {
	name: 'login-to-album',
	motion: slow,
	travel: throughTheDoor,
	leaving: fallBack,
	arriving: { art: rise(48, 160, slow), words: rise(20, 160), panel: rise(96, 160, slow) },
	stages: homeToAlbum.stages,
	bloom: null,
	island: null,
	islandForms
};

/** All of reduced motion: never a hard cut. */
export const blend: Choreography = {
	name: 'blend',
	motion: quick,
	travel: null,
	leaving: { from: {}, to: { opacity: 0 }, delay: 0, motion: quick },
	arriving: {},
	stages: {},
	bloom: null,
	island: null
};

const journeys: Partial<Record<`${RouteName}>${RouteName}`, Choreography>> = {
	'home>album': homeToAlbum,
	'album>home': albumToHome,
	'home>now-playing': homeToNowPlaying,
	'now-playing>home': nowPlayingToHome,
	'album>now-playing': albumToNowPlaying,
	'now-playing>album': nowPlayingToAlbum,
	'login>home': loginToHome,
	'login>album': loginToAlbum
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
	return journeys[`${from}>${to}`] ?? blend;
}

/** Reads a motion's duration from its token. */
export function resolveMotion(spec: MotionSpec, read: (token: MotionToken) => number): Motion {
	return { duration: read(spec.token) * (spec.pace ?? 1), bounce: spec.bounce };
}

/** Parses a CSS time such as `420ms` or `0.6s` to milliseconds. */
export function parseTime(value: string): number {
	const trimmed = value.trim();
	const number = Number.parseFloat(trimmed);
	return trimmed.endsWith('ms') ? number : number * 1000;
}
