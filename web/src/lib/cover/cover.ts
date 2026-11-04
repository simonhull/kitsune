/** The colors one generated cover and everything derived from it share. */
export interface Palette {
	name: string;
	/** The cover's ground. */
	base: string;
	/** The motif's first shape. */
	primary: string;
	/** The motif's second shape. */
	secondary: string;
	/** Album accent: the play button, the current track. */
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

export const motifs = ['orb-horizon', 'split-block', 'hill-moon', 'ring-dot', 'diamond'] as const;

export type Motif = (typeof motifs)[number];

export const palettes: readonly Palette[] = [
	{
		name: 'teal',
		base: '#1F5C5E',
		primary: '#9FD3C7',
		secondary: '#12393B',
		accent: '#9FD3C7',
		onAccent: '#0B2526',
		wash: '#CDE6E0',
		stage: '#0C1E1F',
		field: '#1F5C5E',
		glow: '#9FD3C7',
		band: '#12393B'
	},
	{
		name: 'paper',
		base: '#E8DCC4',
		primary: '#C2462E',
		secondary: '#1C1A18',
		accent: '#E8DCC4',
		onAccent: '#1C1A18',
		wash: '#F0E6D4',
		stage: '#1E120D',
		field: '#6E2A1C',
		glow: '#C2462E',
		band: '#2A1712'
	},
	{
		name: 'violet',
		base: '#2B2147',
		primary: '#6E4BC2',
		secondary: '#F3D27A',
		accent: '#F3D27A',
		onAccent: '#2B2147',
		wash: '#E2DAF5',
		stage: '#120E22',
		field: '#2B2147',
		glow: '#6E4BC2',
		band: '#1B1530'
	},
	{
		name: 'navy',
		base: '#10243F',
		primary: '#5FA8E8',
		secondary: '#0B1A2E',
		accent: '#5FA8E8',
		onAccent: '#0B1A2E',
		wash: '#D5E6F6',
		stage: '#08121F',
		field: '#10243F',
		glow: '#5FA8E8',
		band: '#0B1A2E'
	},
	{
		name: 'rose',
		base: '#1C1A18',
		primary: '#E04F6A',
		secondary: '#7A2A3A',
		accent: '#E04F6A',
		onAccent: '#1C1A18',
		wash: '#F6DCE1',
		stage: '#140E0F',
		field: '#3A1A22',
		glow: '#E04F6A',
		band: '#1C1A18'
	},
	{
		name: 'amber',
		base: '#F1A93B',
		primary: '#FFF1D6',
		secondary: '#7A2E14',
		accent: '#F1A93B',
		onAccent: '#3A1608',
		wash: '#FBE7C8',
		stage: '#1E0F08',
		field: '#7A2E14',
		glow: '#F1A93B',
		band: '#4A1C0C'
	}
];

/** The number of distinct generated covers. */
export const coverCount = palettes.length * motifs.length;

const fnvOffset = 0x811c9dc5;
const fnvPrime = 0x01000193;

/** Hashes a string with 32-bit FNV-1a over its UTF-8 bytes. */
export function fnv1a(input: string): number {
	let hash = fnvOffset;
	for (const byte of new TextEncoder().encode(input)) {
		hash = Math.imul(hash ^ byte, fnvPrime) >>> 0;
	}
	return hash;
}

export interface Cover {
	palette: Palette;
	motif: Motif;
}

/** Picks the cover for an album. The same ID gives the same cover everywhere. */
export function coverFor(albumId: string): Cover {
	const index = fnv1a(albumId) % coverCount;
	const palette = palettes.at(index % palettes.length);
	const motif = motifs.at(Math.floor(index / palettes.length));
	if (palette === undefined || motif === undefined) {
		throw new RangeError(`cover index ${String(index)} is out of range`);
	}
	return { palette, motif };
}
