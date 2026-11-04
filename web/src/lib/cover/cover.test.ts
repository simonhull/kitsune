import { describe, expect, it } from 'vitest';
import { coverCount, coverFor, fnv1a, motifs, palettes } from './cover';

describe('fnv1a', () => {
	it('matches the published test vectors', () => {
		expect(fnv1a('')).toBe(0x811c9dc5);
		expect(fnv1a('a')).toBe(0xe40c292c);
		expect(fnv1a('foobar')).toBe(0xbf9cf968);
	});

	it('hashes UTF-8 bytes, not UTF-16 units', () => {
		expect(fnv1a('é')).toBe(fnv1a('é'));
		expect(fnv1a('é')).not.toBe(fnv1a('Ã'));
	});
});

describe('coverFor', () => {
	it('gives the same ID the same cover', () => {
		expect(coverFor('glasshouse-66')).toEqual(coverFor('glasshouse-66'));
	});

	it('lands the board albums on the board covers', () => {
		const cover = coverFor('glasshouse-66');
		expect(cover.palette.name).toBe('teal');
		expect(cover.motif).toBe('orb-horizon');
	});

	it('reaches all 30 combinations', () => {
		const seen = new Set<string>();
		for (let index = 0; index < 2000; index += 1) {
			const { palette, motif } = coverFor(`album-${String(index)}`);
			seen.add(`${palette.name}/${motif}`);
		}
		expect(coverCount).toBe(30);
		expect(seen.size).toBe(palettes.length * motifs.length);
	});
});
