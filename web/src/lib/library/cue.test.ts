import { describe, expect, it } from 'vitest';
import { cueOf } from './cue';
import type { Album, Track } from './source';

function tracks(count: number): Track[] {
	return Array.from({ length: count }, (_, index) => ({
		id: `t${String(index)}`,
		title: `T${String(index)}`,
		artists: [],
		durationMs: 1000,
		format: 'MP3',
		genres: [],
		etag: ''
	}));
}

function album(count: number): Album {
	return {
		id: 'a',
		title: 'A',
		artist: 'B',
		artists: [],
		trackCount: count,
		compilation: false,
		tracks: tracks(count),
		discs: [],
		durationMs: 1000,
		formats: ['MP3'],
		addedAt: '2026-10-12T18:00:00Z',
		genres: [],
		etag: ''
	};
}

describe('cueOf', () => {
	it('cues the first track and names what follows', () => {
		const cue = cueOf(album(6));
		expect(cue?.track.id).toBe('t0');
		expect(cue?.index).toBe(0);
		expect(cue?.trackCount).toBe(6);
		expect(cue?.upNext.map((track) => track.id)).toEqual(['t1', 't2', 't3']);
		expect(cue?.moreCount).toBe(2);
	});

	it('has nothing next on a single', () => {
		const cue = cueOf(album(1));
		expect(cue?.upNext).toEqual([]);
		expect(cue?.moreCount).toBe(0);
	});

	it('counts no more when everything is named', () => {
		expect(cueOf(album(4))?.moreCount).toBe(0);
	});

	it('follows the order and position it is given', () => {
		const source = album(6);
		const order = source.tracks.toReversed();
		const cue = cueOf(source, order, 1);
		expect(cue?.track.id).toBe('t4');
		expect(cue?.index).toBe(1);
		expect(cue?.trackCount).toBe(6);
		expect(cue?.upNext.map((track) => track.id)).toEqual(['t3', 't2', 't1']);
		expect(cue?.moreCount).toBe(1);
	});

	it('has nothing next at the end of the queue', () => {
		const cue = cueOf(album(3), album(3).tracks, 2);
		expect(cue?.upNext).toEqual([]);
		expect(cue?.moreCount).toBe(0);
	});

	it('cannot cue an empty album', () => {
		expect(cueOf(album(0))).toBeNull();
	});
});
