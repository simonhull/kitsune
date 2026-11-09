import { describe, expect, it } from 'vitest';
import type { Album, ReplayGain, Track } from '#lib/library/source.ts';
import { rowsOf } from './albumSongs.ts';

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
		hearted: false,
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
		hearted: false,
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
