import { describe, expect, it } from 'vitest';
import type { AlbumEntry } from '#lib/library/source.ts';
import { contrast, luminance, neutralPalette, paletteFor } from './palette';

const fox = '#f28c28';

function entry(accent: string, ground: string): AlbumEntry {
	return {
		id: 'a',
		title: 'A',
		artist: 'B',
		trackCount: 1,
		compilation: false,
		artwork: { hash: 'h', accent, ground, width: 600, height: 600 }
	};
}

function rgb(hex: string): [number, number, number] {
	const value = Number.parseInt(hex.slice(1), 16);
	return [(value >> 16) & 255, (value >> 8) & 255, value & 255];
}

function over(top: string, opacity: number, under: string): [number, number, number] {
	const [a, b, c] = rgb(top);
	const [x, y, z] = rgb(under);
	return [
		a * opacity + x * (1 - opacity),
		b * opacity + y * (1 - opacity),
		c * opacity + z * (1 - opacity)
	];
}

const worstCases = [
	['near black', '#101010', '#050505'],
	['near white', '#fefefe', '#6c6c6c'],
	['saturated red', '#ff0000', '#7a0000'],
	['saturated blue', '#0000ff', '#00007a'],
	['saturated yellow', '#ffff00', '#5c5c00'],
	['dark on dark', '#3a1a22', '#1c1a18'],
	['fox fallback on gray', fox, '#6c6c7b'],
	['fox fallback on black', fox, '#000000'],
	['fox fallback on a light ground', fox, '#74705a']
] as const;

describe('paletteFor on dark', () => {
	it('is pure', () => {
		expect(paletteFor(entry('#c83838', '#741f1f'), 'dark')).toEqual(
			paletteFor(entry('#c83838', '#741f1f'), 'dark')
		);
	});

	it('uses the frame until something is known', () => {
		expect(paletteFor(undefined, 'dark')).toBe(neutralPalette('dark'));
		expect(paletteFor(undefined, 'light')).toBe(neutralPalette('light'));
	});

	it('leaves a generated cover its own colors', () => {
		const palette = paletteFor({ id: 'glasshouse-66' }, 'dark');
		expect(palette.accent).toMatch(/^#[0-9A-F]{6}$/);
		expect(palette.accent).not.toBe(neutralPalette('dark').accent);
	});

	it('keeps fox for a cover with no color, and takes the light from its ground', () => {
		const palette = paletteFor(entry(fox, '#3c5a78'), 'dark');
		expect(palette.accent).toBe(fox);
		const [red, , blue] = rgb(palette.glow);
		expect(blue).toBeGreaterThan(red);
	});

	it.each(worstCases)('holds on %s', (_, accent, ground) => {
		const palette = paletteFor(entry(accent, ground), 'dark');
		const { stage, field, band, glow, wash, onAccent } = palette;

		expect(luminance(rgb(stage))).toBeLessThanOrEqual(0.012);
		expect(luminance(rgb(field))).toBeLessThanOrEqual(0.04);
		expect(luminance(rgb(band))).toBeLessThanOrEqual(0.04);
		expect(luminance(rgb(glow))).toBeLessThanOrEqual(0.16);

		const panel = over('#151311', 0.82, glow);
		const row = over(
			palette.accent,
			0.1,
			`#${panel.map((v) => Math.round(v).toString(16).padStart(2, '0')).join('')}`
		);
		for (const surface of [rgb(field), rgb(stage), panel, row]) {
			expect(contrast(rgb(palette.accent), surface)).toBeGreaterThanOrEqual(4.5);
		}
		expect(contrast(rgb(wash), rgb(glow))).toBeGreaterThanOrEqual(4.6);
		expect(contrast(rgb(wash), rgb(field))).toBeGreaterThanOrEqual(4.6);
		expect(contrast(rgb(onAccent), rgb(palette.accent))).toBeGreaterThanOrEqual(4.5);
	});

	it('holds across the whole color space', () => {
		const levels = [0, 64, 128, 192, 255];
		for (const r of levels) {
			for (const g of levels) {
				for (const b of levels) {
					const hex = `#${[r, g, b].map((v) => v.toString(16).padStart(2, '0')).join('')}`;
					const { accent, field, stage, wash, glow } = paletteFor(entry(hex, '#4a3a2a'), 'dark');
					expect(contrast(rgb(accent), rgb(field))).toBeGreaterThanOrEqual(4.6);
					expect(contrast(rgb(accent), rgb(stage))).toBeGreaterThanOrEqual(4.6);
					expect(contrast(rgb(wash), rgb(glow))).toBeGreaterThanOrEqual(4.6);
				}
			}
		}
	});
});

describe('paletteFor on light', () => {
	const paper = '#fbf8f3';

	it('washes a cover toward paper and deepens its accent', () => {
		const dark = paletteFor(entry('#8fcdbb', '#1a5a5c'), 'dark');
		const light = paletteFor(entry('#8fcdbb', '#1a5a5c'), 'light');
		expect(luminance(rgb(light.stage))).toBeGreaterThan(luminance(rgb(dark.stage)));
		expect(luminance(rgb(light.accent))).toBeLessThan(luminance(rgb(dark.accent)));
	});

	it('gives a cover with no color the fox as words, deepened as far as its stage needs', () => {
		const accent = rgb(paletteFor(entry(fox, '#3c5a78'), 'light').accent);
		const [red, , blue] = accent;
		expect(red).toBeGreaterThan(blue);
		expect(luminance(accent)).toBeLessThanOrEqual(luminance(rgb('#a64b00')));
	});

	it('runs a generated cover through the same rules', () => {
		const palette = paletteFor({ id: 'glasshouse-66' }, 'light');
		expect(luminance(rgb(palette.stage))).toBeGreaterThanOrEqual(0.8);
	});

	it.each(worstCases)('holds on %s', (_, accent, ground) => {
		const palette = paletteFor(entry(accent, ground), 'light');
		const { stage, field, band, glow, wash, onAccent } = palette;

		expect(luminance(rgb(stage))).toBeGreaterThanOrEqual(0.8);
		expect(luminance(rgb(field))).toBeGreaterThanOrEqual(0.66);
		expect(luminance(rgb(band))).toBeGreaterThanOrEqual(0.66);
		expect(luminance(rgb(glow))).toBeGreaterThanOrEqual(0.45);

		const panel = over(paper, 0.82, glow);
		const row = over(
			palette.accent,
			0.1,
			`#${panel.map((v) => Math.round(v).toString(16).padStart(2, '0')).join('')}`
		);
		for (const surface of [rgb(field), rgb(stage), panel, row]) {
			expect(contrast(rgb(palette.accent), surface)).toBeGreaterThanOrEqual(4.5);
		}
		expect(contrast(rgb(wash), rgb(glow))).toBeGreaterThanOrEqual(4.6);
		expect(contrast(rgb(wash), rgb(field))).toBeGreaterThanOrEqual(4.6);
		expect(contrast(rgb(onAccent), rgb(palette.accent))).toBeGreaterThanOrEqual(4.5);
	});

	it('holds across the whole color space', () => {
		const levels = [0, 64, 128, 192, 255];
		for (const r of levels) {
			for (const g of levels) {
				for (const b of levels) {
					const hex = `#${[r, g, b].map((v) => v.toString(16).padStart(2, '0')).join('')}`;
					const { accent, field, stage, wash, glow } = paletteFor(entry(hex, hex), 'light');
					expect(contrast(rgb(accent), rgb(field))).toBeGreaterThanOrEqual(4.6);
					expect(contrast(rgb(accent), rgb(stage))).toBeGreaterThanOrEqual(4.6);
					expect(contrast(rgb(wash), rgb(glow))).toBeGreaterThanOrEqual(4.6);
				}
			}
		}
	});
});
