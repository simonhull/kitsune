import type { AlbumEntry } from '#lib/library/source.ts';
import { coverFor } from './cover.ts';

/** The colors an album brings to the stage. */
export interface AlbumPalette {
	/** The play button, the cued track. */
	accent: string;
	/** Text and icons on top of the accent. */
	onAccent: string;
	/** Soft text on top of the backdrop. */
	wash: string;
	/** The backdrop's darkest layer, behind everything. */
	stage: string;
	/** The backdrop's main field. */
	field: string;
	/** The backdrop's bright bloom. */
	glow: string;
	/** The backdrop's lower band. */
	band: string;
}

type Rgb = readonly [number, number, number];

const fox = '#f28c28';
const foxInk = '#1a0f05';
const ink = parse(foxInk);
const text = parse('#f5f1ec');
const white: Rgb = [255, 255, 255];
const card = parse('#151311');

/** What the page shows with no album known yet: the frame's own colors. */
export const neutralPalette: AlbumPalette = {
	accent: fox,
	onAccent: foxInk,
	wash: '#cbc4bc',
	stage: '#0a0908',
	field: '#151311',
	glow: '#151311',
	band: '#0a0908'
};

/** The contrast the palette holds everywhere, a little over AA so rounding can't take it away. */
const minContrast = 4.6;
const stageLimit = 0.012;
const fieldLimit = 0.04;
const glowLimit = 0.16;
/** The track panel's opacity over whatever is behind it, and the cued row's over the panel. */
const panelOpacity = 0.82;
const rowOpacity = 0.1;
const bandToStage = 0.6;
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

function darken(color: Rgb, limit: number): Rgb {
	return mixUntil(color, ink, (candidate) => luminance(candidate) <= limit);
}

function readsOn(surfaces: readonly Rgb[]): (color: Rgb) => boolean {
	return (color) => surfaces.every((surface) => contrast(color, surface) >= minContrast);
}

/** Lifts the accent until it reads on every surface it stands on, the cued row included. */
function liftAccent(accent: Rgb, surfaces: readonly Rgb[], panel: Rgb): Rgb {
	return mixUntil(accent, white, (candidate) =>
		readsOn([...surfaces, panel, snap(mix(panel, candidate, rowOpacity))])(candidate)
	);
}

/** Whether a cover has a color of its own: the server gives one with none the fox as its accent. */
export function ownColor(artwork: AlbumEntry['artwork']): boolean {
	return artwork === undefined || artwork.accent !== fox;
}

function fromArtwork(accentHex: string, groundHex: string): AlbumPalette {
	const ground = parse(groundHex);
	const stage = darken(ground, stageLimit);
	const field = darken(ground, fieldLimit);
	const band = snap(mix(field, stage, bandToStage));
	const tinted = accentHex !== fox;
	// A near-monochrome cover has no color to offer: its light comes from its ground.
	const glow = darken(tinted ? parse(accentHex) : ground, glowLimit);
	const panel = snap(mix(glow, card, panelOpacity));
	const accent = tinted ? liftAccent(parse(accentHex), [field, stage], panel) : parse(fox);
	const wash = mixUntil(mix(tinted ? accent : ground, white, 0.6), white, readsOn([glow, field]));
	const onAccent = contrast(ink, accent) >= contrast(text, accent) ? ink : text;
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
 * An album's colors. Pure: the same entry gives the same palette on every page and in every
 * flight. A cover's accent and ground come from the server; a generated cover brings its own, and
 * nothing known yet means the frame's.
 */
export function paletteFor(entry: Pick<AlbumEntry, 'id' | 'artwork'> | undefined): AlbumPalette {
	if (entry === undefined) {
		return neutralPalette;
	}
	const { artwork } = entry;
	return artwork === undefined
		? coverFor(entry.id).palette
		: fromArtwork(artwork.accent, artwork.ground);
}
