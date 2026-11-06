import { describe, expect, it } from 'vitest';
import type { ReplayGain, Track } from '#lib/library/source.ts';
import { appliedGain, linear } from './gain';

function track(replayGain?: ReplayGain): Track {
	return {
		id: 't',
		title: 'T',
		artists: [],
		durationMs: 1000,
		format: 'FLAC',
		genres: [],
		etag: '',
		...(replayGain === undefined ? {} : { replayGain })
	};
}

describe('appliedGain', () => {
	it('uses album gain in album order and track gain shuffled', () => {
		const both = track({ albumGain: -4, trackGain: -7 });
		expect(appliedGain(both, true)).toBe(-4);
		expect(appliedGain(both, false)).toBe(-7);
	});

	it('falls back to the other gain', () => {
		expect(appliedGain(track({ trackGain: -7 }), true)).toBe(-7);
		expect(appliedGain(track({ albumGain: -4 }), false)).toBe(-4);
	});

	it('has nothing to apply on an untagged track', () => {
		expect(appliedGain(track(), true)).toBeNull();
		expect(appliedGain(track({}), false)).toBeNull();
	});

	it('gives null for a peak with no gain', () => {
		expect(appliedGain(track({ trackPeak: 0.9, albumPeak: 0.9 }), true)).toBeNull();
	});

	it('limits a gain so the peak stays at full scale', () => {
		const applied = appliedGain(track({ trackGain: 6, trackPeak: 0.8 }), false) ?? 0;
		expect(applied).toBeCloseTo(1.938, 3);
		expect(0.8 * linear(applied)).toBeCloseTo(1, 10);
	});

	it('leaves a gain under the limit alone', () => {
		expect(appliedGain(track({ trackGain: 1, trackPeak: 0.8 }), false)).toBe(1);
	});

	it('pulls a peak above full scale down, even from a positive gain', () => {
		expect(appliedGain(track({ trackGain: 2, trackPeak: 1.2 }), false)).toBeCloseTo(-1.584, 3);
	});

	it('does not limit without a peak', () => {
		expect(appliedGain(track({ trackGain: 9 }), false)).toBe(9);
	});

	it("limits by the gain's own peak, not the other's", () => {
		const mixed = track({ albumGain: 3, albumPeak: 0.5, trackGain: 3, trackPeak: 1 });
		expect(appliedGain(mixed, true)).toBe(3);
		expect(appliedGain(mixed, false)).toBe(0);
		expect(appliedGain(track({ albumGain: 3, trackPeak: 1 }), true)).toBe(3);
	});

	it('ignores a nonsense peak', () => {
		expect(appliedGain(track({ trackGain: 3, trackPeak: 0 }), false)).toBe(3);
	});
});

describe('linear', () => {
	it('converts decibels', () => {
		expect(linear(0)).toBe(1);
		expect(linear(-20)).toBeCloseTo(0.1, 10);
		expect(linear(6)).toBeCloseTo(1.995, 3);
	});
});
