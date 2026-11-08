import type { AlbumEntry } from '#lib/library/source.ts';
import type { Theme } from '#lib/theme/theme.svelte.ts';
import { coverFor } from './cover.ts';

/** The colors an album brings to the stage. */
export interface AlbumPalette {
	/** The play button, the cued track. */
	accent: string;
	/** Text and icons on top of the accent. */
	onAccent: string;
	/** Soft text on top of the backdrop. */
	wash: string;
	/** The backdrop's deepest layer, behind everything: darkest on dark, palest on light. */
	stage: string;
	/** The backdrop's main field. */
	field: string;
	/** The backdrop's bloom: brightest on dark, most colored on light. */
	glow: string;
	/** The backdrop's lower band. */
	band: string;
}

type Rgb = readonly [number, number, number];

const fox = '#f28c28';
const foxInk = '#1a0f05';
const ink = parse(foxInk);
const white: Rgb = [255, 255, 255];

/**
 * How one theme turns a cover into a stage. Dark sinks the cover's colors toward black and lifts its
 * accent until it reads; light washes them toward paper and deepens the accent. The tokens' own
 * values are repeated here because the palette is worked out in numbers, not in CSS.
 */
interface StageRules {
	/** Where the stage's layers go: ink on dark, paper on light. */
	toward: Rgb;
	/** Whether a layer has gone far enough, by its luminance and the layer's limit. */
	far: (luminance: number, limit: number) => boolean;
	stage: number;
	field: number;
	glow: number;
	/** The card the track panel is made of. */
	card: Rgb;
	/** Where an accent goes until it reads: white on dark, ink on light. */
	readable: Rgb;
	/**
	 * Whether the cover's ground may be the accent too. Paper wants a deep color, and deepening a
	 * pale accent turns it gray, so light takes whichever of the two ends up more colorful.
	 */
	groundAccent: boolean;
	/** The accent of a cover with no color of its own. */
	frame: Rgb;
	/** The two choices of text on the accent. */
	onAccent: readonly [Rgb, Rgb];
	neutral: AlbumPalette;
}

const rules: Record<Theme, StageRules> = {
	dark: {
		toward: ink,
		far: (luminance, limit) => luminance <= limit,
		stage: 0.012,
		field: 0.04,
		glow: 0.16,
		card: parse('#151311'),
		readable: white,
		groundAccent: false,
		frame: parse(fox),
		onAccent: [ink, parse('#f5f1ec')],
		neutral: {
			accent: fox,
			onAccent: foxInk,
			wash: '#cbc4bc',
			stage: '#0a0908',
			field: '#151311',
			glow: '#151311',
			band: '#0a0908'
		}
	},
	light: {
		toward: parse('#fbf8f3'),
		far: (luminance, limit) => luminance >= limit,
		stage: 0.8,
		field: 0.66,
		glow: 0.45,
		card: parse('#fbf8f3'),
		readable: parse('#16120e'),
		groundAccent: true,
		frame: parse('#a64b00'),
		onAccent: [parse('#16120e'), parse('#fbf8f3')],
		neutral: {
			accent: '#a64b00',
			onAccent: '#fbf8f3',
			wash: '#3f3832',
			stage: '#f4efe8',
			field: '#fbf8f3',
			glow: '#fbf8f3',
			band: '#f4efe8'
		}
	}
};

/** What the page shows with no album known yet: the frame's own colors. */
export function neutralPalette(theme: Theme): AlbumPalette {
	return rules[theme].neutral;
}

/** The contrast the palette holds everywhere, a little over AA so rounding can't take it away. */
const minContrast = 4.6;
/** The track panel's opacity over whatever is behind it, and the cued row's over the panel. */
const panelOpacity = 0.82;
const rowOpacity = 0.1;
const bandToStage = 0.6;
/** How far the wash starts from its color toward the readable end. */
const washStart = 0.6;
const steps = 64;

function parse(hex: string): Rgb {
	const value = Number.parseInt(hex.slice(1), 16);
	return [(value >> 16) & 255, (value >> 8) & 255, value & 255];
}

function format(color: Rgb): string {
	const hex = color.map((channel) => Math.round(channel).toString(16).padStart(2, '0'));
	return `#${hex.join('')}`;
}

function channel(value: number): number {
	const unit = value / 255;
	return unit <= 0.03928 ? unit / 12.92 : ((unit + 0.055) / 1.055) ** 2.4;
}

/** The WCAG relative luminance. */
export function luminance([red, green, blue]: Rgb): number {
	return 0.2126 * channel(red) + 0.7152 * channel(green) + 0.0722 * channel(blue);
}

/** The WCAG contrast ratio. */
export function contrast(a: Rgb, b: Rgb): number {
	const [high, low] = [luminance(a), luminance(b)].sort((x, y) => y - x) as [number, number];
	return (high + 0.05) / (low + 0.05);
}

function mix(from: Rgb, to: Rgb, amount: number): Rgb {
	const [red, green, blue] = from.map(
		(value, index) => value + ((to.at(index) ?? value) - value) * amount
	) as [number, number, number];
	return [red, green, blue];
}

/** Rounds to what CSS will actually paint, so every check sees the final color. */
function snap(color: Rgb): Rgb {
	return parse(format(color));
}

/** Moves a color toward `to` only as far as it takes for `holds` to pass. */
function mixUntil(color: Rgb, to: Rgb, holds: (candidate: Rgb) => boolean): Rgb {
	for (let step = 0; step <= steps; step += 1) {
		const candidate = snap(mix(color, to, step / steps));
		if (holds(candidate)) {
			return candidate;
		}
	}
	return to;
}

/** How colorful a color is: the spread of its channels. */
function chroma(color: Rgb): number {
	return Math.max(...color) - Math.min(...color);
}

function readsOn(surfaces: readonly Rgb[]): (color: Rgb) => boolean {
	return (color) => surfaces.every((surface) => contrast(color, surface) >= minContrast);
}

/** Whether a cover has a color of its own: the server gives one with none the fox as its accent. */
export function ownColor(artwork: AlbumEntry['artwork']): boolean {
	return artwork === undefined || artwork.accent !== fox;
}

function fromArtwork(accentHex: string, groundHex: string, theme: Theme): AlbumPalette {
	const rule = rules[theme];
	const layer = (color: Rgb, limit: number): Rgb =>
		mixUntil(color, rule.toward, (candidate) => rule.far(luminance(candidate), limit));
	const ground = parse(groundHex);
	const stage = layer(ground, rule.stage);
	const field = layer(ground, rule.field);
	const band = snap(mix(field, stage, bandToStage));
	const tinted = accentHex !== fox;
	// A near-monochrome cover has no color to offer: its light comes from its ground.
	const glow = layer(tinted ? parse(accentHex) : ground, rule.glow);
	const panel = snap(mix(glow, rule.card, panelOpacity));
	// The accent moves until it reads on every surface it stands on, the cued row included.
	const reads = (candidate: Rgb): boolean =>
		readsOn([field, stage, panel, snap(mix(panel, candidate, rowOpacity))])(candidate);
	const sources = tinted
		? [parse(accentHex), ...(rule.groundAccent ? [ground] : [])]
		: [rule.frame];
	const accent = sources
		.map((source) => mixUntil(source, rule.readable, reads))
		.reduce((best, next) => (chroma(next) > chroma(best) ? next : best));
	const wash = mixUntil(
		mix(tinted ? accent : ground, rule.readable, washStart),
		rule.readable,
		readsOn([glow, field])
	);
	const [one, other] = rule.onAccent;
	const onAccent = contrast(one, accent) >= contrast(other, accent) ? one : other;
	return {
		accent: format(accent),
		onAccent: format(onAccent),
		wash: format(wash),
		stage: format(stage),
		field: format(field),
		glow: format(glow),
		band: format(band)
	};
}

/**
 * An album's colors in a theme. Pure: the same entry gives the same palette on every page and in
 * every flight. A cover's accent and ground come from the server; a generated cover brings its own
 * on dark, and its accent and ground go through the same rules on light. Nothing known yet means
 * the frame's.
 */
export function paletteFor(
	entry: Pick<AlbumEntry, 'id' | 'artwork'> | undefined,
	theme: Theme
): AlbumPalette {
	if (entry === undefined) {
		return neutralPalette(theme);
	}
	const { artwork } = entry;
	if (artwork !== undefined) {
		return fromArtwork(artwork.accent, artwork.ground, theme);
	}
	const { palette } = coverFor(entry.id);
	return theme === 'dark' ? palette : fromArtwork(palette.accent, palette.base, theme);
}
