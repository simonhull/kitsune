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
	/** Degrees. Only a card landing on the deck turns: pages have no such term. */
	rotate?: number;
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

/**
 * How a page assembles in front of you around the thing you were in: the opposite of a ripple's
 * idea, for arriving. Every unit starts at once (an entrance never queues), but one far from where
 * the shared element lands comes from deeper than one beside it, which is all but there already.
 */
export interface Gather {
	/** How much of the depth is left beside the landing, from 0 to 1. */
	near: number;
}

/** How much of the depth a unit `share` of the way across the page comes from, 0 at the landing. */
export function gatherWeight({ near }: Gather, share: number): number {
	return near + (1 - near) * share;
}

/** Scales only the depth: every unit of a gathering rises the same way, and fades the same way. */
export function weighGather(part: Part, weight: number): Part {
	const { scale } = part.from;
	return scale === undefined
		? part
		: { ...part, from: { ...part.from, scale: 1 + (scale - 1) * weight } };
}

/** One thing that moves, from one pose to another, starting `delay` into the morph. */
export interface Part {
	from: Pose;
	to: Pose;
	/** Milliseconds after the morph starts. */
	delay: number;
	motion: MotionSpec;
	/** A curve of its own for the opacity, for something that's solid long before it arrives. */
	fade?: MotionSpec;
	ripple?: Ripple;
	gather?: Gather;
}

/** What a page's unit is, from `data-morph-part`. It decides how the unit arrives. */
export type Role =
	'intro' | 'card' | 'art' | 'words' | 'panel' | 'line-1' | 'line-2' | 'details' | 'chrome';

export type StageName = 'album' | 'poster';

export type RouteName =
	| 'login'
	| 'home'
	| 'albums'
	| 'album'
	| 'now-playing'
	| 'search'
	| 'artists'
	| 'artist'
	| 'health'
	| 'library'
	| 'server'
	| 'playlists'
	| 'playlist'
	| 'you';

export type ChoreographyName =
	| 'home-to-album'
	| 'library-to-album'
	| 'album-door'
	| 'album-into-continue'
	| 'album-to-home'
	| 'album-to-library'
	| 'album-to-albums'
	| 'album-to-artist'
	| 'album-to-playlist'
	| 'album-to-album'
	| 'name-door'
	| 'artist-to-artists'
	| 'artist-to-library'
	| 'artist-to-albums'
	| 'mosaic-door'
	| 'playlist-to-playlists'
	| 'home-to-now-playing'
	| 'now-playing-to-home'
	| 'album-to-now-playing'
	| 'now-playing-to-album'
	| 'login-to-home'
	| 'login-to-album'
	| 'deck-to-home'
	| 'blend';

/**
 * Which way a journey goes through the library's grain. Opening is into something the page you
 * leave holds (Home to an album); gathering is into something that holds it (an album to Home, or to
 * its artist); crossing is between two of one kind (an album to another). The grain picks the lead
 * and who gets out of whose way, never a direction on the screen: every journey is further in.
 */
export type Grain = 'opening' | 'gathering' | 'crossing';

/** What can travel between two pages: an album's parts, an artist's name, a playlist's mosaic… */
export type Lead =
	'cover' | 'title' | 'artist' | 'surface' | 'play' | 'name' | 'mosaic' | 'listname';

/** A lead and whose it is: the page's at the start of the journey, or at its end. */
export type LeadOf = `${Lead}:${'from' | 'to'}`;

interface SharedTravel {
	/** Whose things travel: the pages' own, what's playing, or the mark. */
	subject: 'pages' | 'playing' | 'fox';
	/** For `pages`, what travels. */
	leads?: readonly LeadOf[];
	sourceSlots: readonly MorphSlot[];
	targetSlots: readonly MorphSlot[];
	/**
	 * The unit (`data-morph-key`) whose target wins: where the page you go to now holds the one you
	 * leave, before where it came from.
	 */
	landIn?: string;
	/** Leads that leave this long after the rest, in milliseconds, for something in their way. */
	waits?: Partial<Record<Lead, number>>;
}

/** The light opening out of the cover: its scale at the cover and at full size, and its strength. */
export interface Bloom {
	rest: number;
	full: number;
	peak: number;
}

export interface Choreography {
	name: ChoreographyName;
	grain: Grain;
	/** The curve shared elements fly on. */
	motion: MotionSpec;
	travel: SharedTravel | null;
	/** Every unit of the outgoing page, unless its role has a part of its own. */
	leaving: Part;
	leavingByRole?: Partial<Record<Role, Part>>;
	/** The incoming page's units, by role. */
	arriving: Partial<Record<Role, Part>>;
	/** The few units that arrive their own way whatever their role, by `data-morph-key`. */
	arrivingByKey?: Readonly<Record<string, Part>>;
	/** The stages behind the page you go to: they come up over everything you leave. */
	stages: Partial<Record<StageName, Part>>;
	/** Pictures of the stages behind the page you leave, which give way under it. */
	held: Partial<Record<StageName, Part>>;
	bloom: Bloom | null;
	/** What's inside the island, when it changes shape on this journey. */
	island: Part | null;
	/** How the island first appears, on the one journey that starts without it. */
	islandForms?: Part;
}

const bloomShape: Bloom = { rest: 0.12, full: 2.6, peak: 0.9 };

// All critically damped: a tap carries no momentum, so nothing has a reason to overshoot.
const slow: MotionSpec = { token: '--morph-slow', bounce: 0 };
const base: MotionSpec = { token: '--morph', bounce: 0 };
const quick: MotionSpec = { token: '--morph-fast', bounce: 0 };
/** Exits go together and go fast: the user has already decided to leave. */
const exit: MotionSpec = { ...quick, pace: 0.7 };

// Every navigation is one step further in: depth is how far you've come, not where a page sits.
// The In-Front Rule makes that visible: the page you go to is always drawn in front of the page you
// leave, and the page you leave never moves away to show what is under it. What both pages show
// travels and becomes the new page's; what only the old page shows gives way under the new one;
// what only the new page shows arrives in front, built around what travelled. Opening into
// something the page held, its world floods out and the old page may still fall back, because the
// new stage covers it. Gathering into something that holds the page, the larger page assembles in
// front around where it now holds the one you were in. Gathering is quicker than opening.

const fallBack: Part = {
	from: {},
	to: { opacity: 0, scale: 0.9 },
	delay: 0,
	motion: exit,
	ripple: { spread: 40, floor: 0.08 }
};

/**
 * Give way: to nothing where it stands, under the page arriving. No travel and no scale, because
 * anything that moves away uncovers what's behind it, and that is a backstack.
 */
function giveWay(motion: MotionSpec = exit, delay = 0): Part {
	return { from: {}, to: { opacity: 0 }, delay, motion };
}

/**
 * Gather: the page assembles around where the shared element lands, every unit at once, from
 * `deep` of depth at the far edge and almost none beside the landing. `y` lifts every unit alike.
 */
function gather(deep: number, delay: number, y?: number): Part {
	return {
		from: { opacity: 0, scale: 1 - deep, ...(y === undefined ? {} : { y }) },
		to: {},
		delay,
		motion: base,
		gather: { near: 0.15 }
	};
}

/**
 * A cover thrown down on the deck, from well above and well off the table, still turning. It takes
 * `spin` degrees off its way to the tilt it lies at, and nothing about it lingers: at one every
 * 70 ms a placement would read as a queue.
 */
export function drop(spin: number): Part {
	return {
		from: { opacity: 0, y: -84, scale: 1.3, rotate: spin },
		to: {},
		delay: 0,
		motion: { token: '--morph-drop', bounce: 0 },
		// Solid almost at once: several land together, and a run of see-through cards is a dissolve.
		fade: { token: '--morph-drop', bounce: 0, pace: 0.3 }
	};
}

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

/**
 * A unit a journey has nothing for: it comes up in front where it stands, because a unit that
 * simply appeared over the page leaving would be a cut.
 */
export const plainArrival: Part = fadeIn(0, quick);

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

const albumParts = ['cover', 'title', 'artist'] as const;

function albumLeads(side: 'from' | 'to'): LeadOf[] {
	return albumParts.map((part): LeadOf => `${part}:${side}`);
}

/**
 * The album's door, from any cover a room or a record shows: a grid's cell, a shelf, a song's thumb.
 * The cover lifts out and grows into the hero; its color breaks out around it and floods the stage,
 * which comes up over the page falling back behind it; the card's label swells into the headline,
 * and the tracks rise last, because they are what you came for.
 */
const albumDoor: Choreography = {
	name: 'album-door',
	grain: 'opening',
	motion: slow,
	travel: {
		subject: 'pages',
		leads: albumLeads('to'),
		sourceSlots: ['card', 'deck', 'thumb'],
		targetSlots: ['hero']
	},
	leaving: fallBack,
	// The art only arrives on its own when no cover flew to it.
	arriving: { art: rise(48, 160, slow), words: rise(20, 160), panel: rise(96, 160, slow) },
	stages: { album: { from: { opacity: 0, scale: 1.08 }, to: {}, delay: 120, motion: slow } },
	held: {},
	bloom: bloomShape,
	island: null
};

const homeToAlbum: Choreography = { ...albumDoor, name: 'home-to-album' };
const libraryToAlbum: Choreography = { ...albumDoor, name: 'library-to-album' };

/** How an album gives way when a larger page gathers around it. */
const albumGivesWay: Pick<Choreography, 'leaving' | 'leavingByRole' | 'held'> = {
	leaving: giveWay(),
	// The songs sat lowest and widest: they stay longest to be covered.
	leavingByRole: { panel: giveWay(base) },
	held: { album: giveWay(base) }
};

/**
 * Album to Home, when the album is Continue's (playing, or paused there): Into Continue. The hero
 * is the Continue card. The album's stage reshapes, at its true size and radius, into the card, with
 * the cover and Play riding into their places on it, and Home gathers in front around it.
 */
const albumIntoContinue: Choreography = {
	name: 'album-into-continue',
	grain: 'gathering',
	motion: base,
	travel: {
		subject: 'pages',
		leads: ['surface:from', 'cover:from', 'play:from'],
		sourceSlots: ['hero'],
		targetSlots: ['card', 'island'],
		landIn: 'feature'
	},
	// Everything else on the album sat on the surface, and is gone well before it is small.
	leaving: giveWay(),
	arriving: { intro: gather(0.12, 90), card: gather(0.12, 90) },
	// The card's own words come last, onto a surface that has landed.
	arrivingByKey: { feature: fadeIn(200, base) },
	stages: {},
	// Only when nothing reshapes: a hero scrolled away leaves its stage behind.
	held: { album: giveWay(base) },
	bloom: null,
	island: null
};

/**
 * Album to Home otherwise: Set among the others. You were just there, so Home now holds it first in
 * Recently played: the cover and its words land there, Home rises in front around it, and the album
 * gives way where it stands. It lands where Home holds it, not where it came from.
 */
const albumToHome: Choreography = {
	name: 'album-to-home',
	grain: 'gathering',
	motion: base,
	travel: {
		subject: 'pages',
		leads: albumLeads('from'),
		sourceSlots: ['hero'],
		targetSlots: ['card', 'deck', 'island'],
		landIn: 'played'
	},
	...albumGivesWay,
	arriving: { intro: gather(0.08, 90, 24), card: gather(0.08, 90, 24) },
	stages: {},
	bloom: null,
	island: null
};

/**
 * Album to Library: the album stays where it is and gives way as the list rises in front of it, the
 * artist first and then the songs, and the cover takes its place among them. No color drains into
 * the cover: this is a list arriving, which is why it has no bloom.
 */
const albumToLibrary: Choreography = {
	name: 'album-to-library',
	grain: 'gathering',
	motion: base,
	travel: {
		subject: 'pages',
		leads: albumLeads('from'),
		sourceSlots: ['hero'],
		targetSlots: ['card']
	},
	leaving: giveWay(),
	arriving: { intro: rise(16, 60), card: rise(40, 90) },
	stages: {},
	held: { album: giveWay(base) },
	bloom: null,
	island: null
};

/**
 * Album to Albums: the grid grows from it. The cover and its words land in their cell, and the grid
 * assembles around it, all at once: the cells beside it all but there, the far ones from deeper.
 * The grid is built from the album, so the album was never put back.
 */
const albumToAlbums: Choreography = {
	name: 'album-to-albums',
	grain: 'gathering',
	motion: base,
	travel: {
		subject: 'pages',
		leads: albumLeads('from'),
		sourceSlots: ['hero'],
		targetSlots: ['card']
	},
	...albumGivesWay,
	arriving: { intro: gather(0.16, 60), card: gather(0.16, 60) },
	stages: {},
	bloom: null,
	island: null
};

/**
 * Album to Artist: the name takes the stage. The artist's name grows from its line under the
 * headline into the artist page's headline, a beat after the rest so it never crosses the album's
 * title on its way out, and the cover lands on its song in Top songs. The artist page rises in front.
 */
const albumToArtist: Choreography = {
	name: 'album-to-artist',
	grain: 'gathering',
	motion: base,
	travel: {
		subject: 'pages',
		leads: ['name:to', 'cover:from'],
		sourceSlots: ['hero'],
		targetSlots: ['hero', 'thumb', 'card'],
		waits: { name: 40 }
	},
	...albumGivesWay,
	// The shelves' covers are units of their own, and come with the songs.
	arriving: { words: rise(12, 90), panel: rise(24, 90), card: rise(24, 90) },
	stages: {},
	bloom: null,
	island: null
};

/**
 * Album to Playlist, only ever by Back: the playlist holds the album's song, so it gathers. The
 * cover lands on the song's thumb it was opened from, and the playlist rises in front around it.
 */
const albumToPlaylist: Choreography = {
	...albumToArtist,
	name: 'album-to-playlist',
	travel: {
		subject: 'pages',
		leads: ['cover:from'],
		sourceSlots: ['hero'],
		targetSlots: ['thumb']
	},
	arriving: { art: rise(12, 90), words: rise(12, 90), panel: rise(24, 90) }
};

/**
 * The name's door, from the Artists list, a Library heading, a search result or an album card's
 * menu: the name grows out of the list into the headline, the list falls back around it, and Top
 * songs rise last. No stage: an artist has no art.
 */
const nameDoor: Choreography = {
	name: 'name-door',
	grain: 'opening',
	motion: slow,
	travel: {
		subject: 'pages',
		leads: ['name:to'],
		sourceSlots: ['row', 'card', 'hero'],
		targetSlots: ['hero']
	},
	leaving: fallBack,
	arriving: { words: rise(20, 160), panel: rise(48, 160, slow), card: rise(48, 160, slow) },
	stages: {},
	held: {},
	bloom: null,
	island: null
};

/**
 * Artist to Artists: set in the roll. The headline settles into its line in the list, and the list
 * gathers around it, in front, while the artist gives way.
 */
const artistToArtists: Choreography = {
	name: 'artist-to-artists',
	grain: 'gathering',
	motion: base,
	travel: {
		subject: 'pages',
		leads: ['name:from'],
		sourceSlots: ['hero'],
		targetSlots: ['row']
	},
	leaving: giveWay(),
	leavingByRole: { panel: giveWay(base), card: giveWay(base) },
	arriving: { intro: gather(0.14, 60), card: gather(0.14, 60) },
	stages: {},
	held: {},
	bloom: null,
	island: null
};

/** Artist to Library: the headline settles into the artist's heading, and the list gathers round. */
const artistToLibrary: Choreography = { ...artistToArtists, name: 'artist-to-library' };

/**
 * Artist to Albums, back from an album card's Go to artist: the headline settles into the card's
 * artist line it grew from, and the grid gathers round it.
 */
const artistToAlbums: Choreography = {
	...artistToArtists,
	name: 'artist-to-albums',
	travel: { subject: 'pages', leads: ['name:from'], sourceSlots: ['hero'], targetSlots: ['card'] }
};

/**
 * The mosaic's door, Playlists to Playlist: the tile's mosaic grows into the head's and its name
 * into the headline, the wall falls back, and the songs rise last.
 */
const mosaicDoor: Choreography = {
	name: 'mosaic-door',
	grain: 'opening',
	motion: slow,
	travel: {
		subject: 'pages',
		leads: ['mosaic:to', 'listname:to'],
		sourceSlots: ['card'],
		targetSlots: ['hero']
	},
	leaving: fallBack,
	arriving: { art: rise(48, 160, slow), words: rise(20, 160), panel: rise(96, 160, slow) },
	stages: {},
	held: {},
	bloom: null,
	island: null
};

/**
 * Playlist to Playlists: set in the wall. The mosaic and the name settle into their tile, and the
 * wall gathers around it, in front, while the playlist gives way.
 */
const playlistToPlaylists: Choreography = {
	name: 'playlist-to-playlists',
	grain: 'gathering',
	motion: base,
	travel: {
		subject: 'pages',
		leads: ['mosaic:from', 'listname:from'],
		sourceSlots: ['hero'],
		targetSlots: ['card']
	},
	leaving: giveWay(),
	leavingByRole: { panel: giveWay(base) },
	arriving: { intro: gather(0.14, 60), card: gather(0.14, 60) },
	stages: {},
	held: {},
	bloom: null,
	island: null
};

/**
 * Home to Now playing: two levels down in one move. The island's cover rises to the center of
 * the stage and the song's name follows it up, the rest of the island gathers into the pill
 * controls, and the poster title climbs out from behind the cover, a line at a time.
 */
const homeToNowPlaying: Choreography = {
	name: 'home-to-now-playing',
	grain: 'opening',
	motion: slow,
	travel: { ...playing.in, sourceSlots: ['island'] },
	leaving: fallBack,
	arriving: nowPlayingArrives,
	stages: { poster: fadeIn(120, slow) },
	held: {},
	bloom: bloomShape,
	island: islandOpens
};

/**
 * Now playing to Home, until the player's own milestone gives it a journey of its own: everything
 * condenses back into the island while the moment gives way where it stands, and Home gathers in
 * front around the island. Nothing sinks and no light drains: that was the way in, played back.
 */
const nowPlayingToHome: Choreography = {
	name: 'now-playing-to-home',
	grain: 'gathering',
	motion: base,
	travel: { ...playing.out, targetSlots: ['island'] },
	leaving: giveWay(),
	arriving: {
		// Held until the poster title clears: two words that size can't share a stage.
		intro: gather(0.08, 140),
		card: gather(0.08, 90)
	},
	stages: {},
	held: { poster: giveWay(base) },
	bloom: null,
	island: islandCloses
};

/**
 * Album to Now playing: one level deeper. The stage is already this album's, so nothing blooms;
 * the poster comes up over it. The hero moves to the center if it can; otherwise the island's cover
 * rises and the hero falls back with the page. The album's headline gives way to the song's.
 */
const albumToNowPlaying: Choreography = {
	name: 'album-to-now-playing',
	grain: 'opening',
	motion: slow,
	travel: { ...playing.in, sourceSlots: ['hero', 'island'] },
	leaving: fallBack,
	// Later than from Home: the album's headline is in the way until it has fallen back.
	arriving: { ...nowPlayingArrives, 'line-1': posterLine(200), 'line-2': posterLine(290) },
	stages: { poster: fadeIn(60, slow) },
	held: { album: fadeOut(460, quick) },
	bloom: null,
	island: islandOpens
};

/**
 * Now playing to Album, until the player's own milestone: the poster gives way under the album,
 * whose stage comes up in front, and the album gathers around the cover landing in its hero.
 */
const nowPlayingToAlbum: Choreography = {
	name: 'now-playing-to-album',
	grain: 'gathering',
	motion: base,
	travel: { ...playing.out, targetSlots: ['hero', 'island'] },
	leaving: giveWay(),
	arriving: {
		art: gather(0.06, 90),
		// The headline waits for the poster title to clear: two words that size can't share a stage.
		words: { ...gather(0.06, 140), gather: undefined },
		panel: rise(72, 90)
	},
	stages: { album: fadeIn(0, base) },
	held: { poster: giveWay(base) },
	bloom: null,
	island: islandCloses
};

const throughTheDoor = { subject: 'fox', sourceSlots: ['door'], targetSlots: ['nav'] } as const;

/** One beat behind the page: the island closes the frame the fox has opened. */
const islandForms = rise(32, 220);

/**
 * Login to Home. The door is the shallowest place there is, so this is going deeper: the fox
 * leaves the form and takes its seat in the nav as the glass forms around it, the form falls back,
 * and Home rises in front of it with the island closing the frame from below. Login to Now playing
 * has no fox's journey to make: it blends, and so does anything back to the door.
 */
const loginToHome: Choreography = {
	name: 'login-to-home',
	grain: 'opening',
	motion: slow,
	travel: throughTheDoor,
	leaving: fallBack,
	arriving: { intro: rise(20, 160), card: rise(48, 160, slow) },
	stages: {},
	held: {},
	bloom: null,
	island: null,
	islandForms
};

/** Login to Album: the same door, returning after an expired session to the album left behind. */
const loginToAlbum: Choreography = {
	name: 'login-to-album',
	grain: 'opening',
	motion: slow,
	travel: throughTheDoor,
	leaving: fallBack,
	arriving: { art: rise(48, 160, slow), words: rise(20, 160), panel: rise(96, 160, slow) },
	stages: albumDoor.stages,
	held: {},
	bloom: null,
	island: null,
	islandForms
};

/** What fliers land on: already in place, and already there by the time the first one arrives. */
const landingPad = fadeIn(40, base);

/**
 * The first scan ends and the deck deals out into Home: one journey with no navigation. The cards
 * fly to the album tiles they are, on the short spring because a dozen must land in about 600 ms,
 * and the rest of Home rises under them as it does through the door. The greeting never moves: it
 * has been here all along. The fliers themselves aren't in here, because they're worked out from
 * what's on screen.
 */
export const deckToHome: Choreography = {
	name: 'deck-to-home',
	grain: 'opening',
	motion: quick,
	travel: null,
	leaving: { from: {}, to: { opacity: 0, scale: 0.86 }, delay: 0, motion: exit },
	arriving: { card: rise(48, 160, slow) },
	arrivingByKey: { feature: landingPad, recent: landingPad },
	stages: {},
	held: {},
	bloom: null,
	island: null,
	islandForms
};

/** The deck's count line, once there's a Home to say it instead. */
export const countFades = fadeOut(0, quick);

/** All of reduced motion, and every pair with no journey yet: never a hard cut. */
export const blend: Choreography = {
	name: 'blend',
	grain: 'crossing',
	motion: quick,
	travel: null,
	leaving: { from: {}, to: { opacity: 0 }, delay: 0, motion: quick },
	arriving: {},
	stages: {},
	held: {},
	bloom: null,
	island: null
};

const journeys: Partial<Record<`${RouteName}>${RouteName}`, Choreography>> = {
	'home>album': homeToAlbum,
	'library>album': libraryToAlbum,
	'albums>album': albumDoor,
	'artist>album': albumDoor,
	'playlist>album': albumDoor,
	'search>album': albumDoor,
	'album>home': albumToHome,
	'album>library': albumToLibrary,
	'album>albums': albumToAlbums,
	'album>artist': albumToArtist,
	'album>playlist': albumToPlaylist,
	'artists>artist': nameDoor,
	'library>artist': nameDoor,
	'albums>artist': nameDoor,
	'search>artist': nameDoor,
	'artist>artists': artistToArtists,
	'artist>library': artistToLibrary,
	'artist>albums': artistToAlbums,
	'playlists>playlist': mosaicDoor,
	'playlist>playlists': playlistToPlaylists,
	'home>now-playing': homeToNowPlaying,
	'now-playing>home': nowPlayingToHome,
	'album>now-playing': albumToNowPlaying,
	'now-playing>album': nowPlayingToAlbum,
	'login>home': loginToHome,
	'login>album': loginToAlbum
};

/** What decides a journey beyond its two routes: one pair can have two journeys. */
export interface JourneyContext {
	/** Whether the album being left is the one on Home's Continue card. */
	continues?: boolean;
}

/** Finds the choreography between two routes. */
export function choreographyFor(
	from: RouteName,
	to: RouteName,
	{ continues = false }: JourneyContext = {}
): Choreography {
	if (from === 'album' && to === 'home' && continues) {
		return albumIntoContinue;
	}
	return journeys[`${from}>${to}`] ?? blend;
}

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
