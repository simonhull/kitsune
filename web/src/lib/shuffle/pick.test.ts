import { describe, expect, it } from 'vitest';
import type { LibraryIndex } from '#lib/library/source.ts';
import { candidatesOf, pickUnit, unitsOf, weightOf, type Candidate } from './pick.ts';

function song(id: string, extra: Partial<Candidate> = {}): Candidate {
	return { id, albumId: 'a', hearted: false, ...extra };
}

const ids = (units: ReturnType<typeof unitsOf>): string[][] =>
	units.map(({ songs }) => songs.map(({ id }) => id));

describe('unitsOf', () => {
	it('never offers an excluded song on its own', () => {
		expect(ids(unitsOf([song('a'), song('b', { mark: { excluded: true } }), song('c')]))).toEqual([
			['a'],
			['c']
		]);
	});

	it('plays a run whole from its first song, whichever song is in scope, reaching outside it', () => {
		const intro = song('intro', { albumId: 'x', mark: { run: 'r', position: 1, excluded: true } });
		const main = song('main', { mark: { run: 'r', position: 2 } });
		expect(ids(unitsOf([main, song('d')], [intro, main]))).toEqual([['intro', 'main'], ['d']]);
	});

	it('drops a run every song of which is excluded', () => {
		const a = song('a', { mark: { run: 'r', position: 1, excluded: true } });
		const b = song('b', { mark: { run: 'r', position: 2, excluded: true } });
		expect(unitsOf([a, b])).toEqual([]);
	});
});

describe('weightOf', () => {
	it('follows the server’s rating curve and heart', () => {
		expect(weightOf({ songs: [song('a')] }, 0.5)).toBeCloseTo(1);
		expect(weightOf({ songs: [song('a', { stars: 5 })] }, 1)).toBeCloseTo(2.4);
		expect(weightOf({ songs: [song('a', { stars: 1 })] }, 1)).toBeCloseTo(0.2);
		expect(weightOf({ songs: [song('a', { stars: 5 })] }, 0)).toBeCloseTo(1);
		expect(weightOf({ songs: [song('a', { hearted: true })] }, 0.5)).toBeCloseTo(1.25);
		expect(weightOf({ songs: [song('a', { stars: 4 }), song('b', { stars: 5 })] }, 1)).toBeCloseTo(
			Math.exp((Math.log(1.6) + Math.log(2.4)) / 2)
		);
	});
});

describe('pickUnit', () => {
	it('draws in proportion to weight, and never an excluded song', () => {
		const units = unitsOf([
			song('five', { stars: 5 }),
			song('one', { stars: 1 }),
			song('no', { mark: { excluded: true } })
		]);
		let state = 1;
		const random = (): number => {
			state = (state * 16807) % 2147483647;
			return state / 2147483647;
		};
		const counts = new Map<string, number>();
		for (let i = 0; i < 4000; i++) {
			const id = pickUnit(units, 1, random)?.songs[0]?.id ?? '';
			counts.set(id, (counts.get(id) ?? 0) + 1);
		}
		expect(counts.get('no')).toBeUndefined();
		const ratio = (counts.get('five') ?? 0) / (counts.get('one') ?? 1);
		expect(ratio).toBeGreaterThan(9);
		expect(ratio).toBeLessThan(15);
		expect(pickUnit([], 0.5)).toBeUndefined();
	});
});

describe('candidatesOf', () => {
	it('reads the index with the marks as they stand, keeping what the filter keeps', () => {
		const index: LibraryIndex = {
			artists: [
				{
					name: 'A',
					sortName: 'A',
					hearted: false,
					albums: [
						{
							id: 'al',
							title: 'Al',
							artist: 'A',
							compilation: false,
							hearted: false,
							tracks: [
								{
									id: 't1',
									title: 'T1',
									durationMs: 1,
									artists: [],
									hearted: false,
									shuffle: { excluded: true }
								},
								{ id: 't2', title: 'T2', durationMs: 1, artists: [], hearted: false }
							]
						}
					]
				}
			]
		};
		const { inScope, everyone } = candidatesOf(
			index,
			(id) => ({ hearted: id === 't2', stars: 4 }),
			(_artist, _album, track) => track.id === 't2'
		);
		expect(inScope).toEqual([
			{ id: 't2', albumId: 'al', hearted: true, stars: 4, mark: undefined }
		]);
		expect(everyone.map(({ id }) => id)).toEqual(['t1', 't2']);
		expect(everyone[0]?.mark).toEqual({ excluded: true });
	});
});
