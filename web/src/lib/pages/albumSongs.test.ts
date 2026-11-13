import { describe, expect, it } from 'vitest';
import type { Album, ReplayGain, Track } from '#lib/library/source.ts';
import { discGroups, rowsOf } from './albumSongs.ts';

function song(no: number, replayGain?: ReplayGain): Track {
	return {
		id: `t${String(no)}`,
		title: `Song ${String(no)}`,
		artists: ['Ann'],
		trackNo: no,
		durationMs: 60_000,
		format: 'FLAC',
		genres: [],
		etag: `t${String(no)}:0`,
		...(replayGain === undefined ? {} : { replayGain })
	};
}

function album(tracks: Track[]): Album {
	return {
		id: 'a1',
		title: 'Glasshouse',
		artist: 'Ann',
		trackCount: tracks.length,
		compilation: false,
		artists: [],
		tracks,
		durationMs: 60_000 * tracks.length,
		formats: ['FLAC'],
		addedAt: '2026-11-01T00:00:00Z',
		genres: [],
		etag: 'a1',
		discs: [{ no: 1, songs: tracks.length, durationMs: 60_000 * tracks.length }]
	};
}

describe('rowsOf', () => {
	it('says where each song’s gain comes from, as the index does', () => {
		const rows = rowsOf(
			album([
				song(1, { trackGain: -6.2, source: 'tags' }),
				song(2, { trackGain: 2, albumGain: 2, source: 'measured' }),
				song(3, { albumGain: -7, source: 'tags' }),
				song(4)
			])
		);
		expect(rows.tracks.map((t) => [t.trackGain, t.trackGainSource])).toEqual([
			[-6.2, 'tags'],
			[2, 'measured'],
			[undefined, undefined],
			[undefined, undefined]
		]);
	});
});

describe('discGroups', () => {
	const on = (discNo: number | undefined, no: number): Track => ({
		...song(no),
		...(discNo === undefined ? {} : { discNo })
	});

	it('keeps an album with no disc numbers as one group with no heading', () => {
		const tracks = [on(undefined, 1), on(undefined, 2), on(undefined, 3)];
		expect(discGroups({ tracks, discs: [{ no: 1, songs: 3, durationMs: 180_000 }] })).toEqual([
			{ disc: undefined, indexes: [0, 1, 2] }
		]);
	});

	it('heads each disc of an album whose songs all have one', () => {
		const one = { no: 1, songs: 2, durationMs: 120_000 };
		const two = { no: 2, songs: 1, durationMs: 60_000 };
		const tracks = [on(1, 1), on(1, 2), on(2, 1)];
		expect(discGroups({ tracks, discs: [one, two] })).toEqual([
			{ disc: one, indexes: [0, 1] },
			{ disc: two, indexes: [2] }
		]);
	});

	it('gives the songs with no disc number their own group, last, beside a disc', () => {
		const one = { no: 1, songs: 2, durationMs: 120_000 };
		const none = { songs: 3, durationMs: 180_000 };
		const tracks = [on(1, 1), on(1, 2), on(undefined, 37), on(undefined, 1), on(undefined, 2)];
		expect(discGroups({ tracks, discs: [one, none] })).toEqual([
			{ disc: one, indexes: [0, 1] },
			{ disc: none, indexes: [2, 3, 4] }
		]);
	});
});
