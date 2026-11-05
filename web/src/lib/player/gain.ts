import type { Track } from '#lib/library/source.ts';

interface Measured {
	gain: number | undefined;
	peak: number | undefined;
}

/** Keeps the loudest sample at full scale: nothing may be pushed past what the file can hold. */
function limited({ peak }: Measured, gain: number): number {
	return peak === undefined || peak <= 0 ? gain : Math.min(gain, 20 * Math.log10(1 / peak));
}

/**
 * The gain to play a track at, in dB, or null when it has none. Album gain keeps an album's own
 * dynamics, so it applies in album order, and track gain applies shuffled; each falls back to the
 * other. A gain is limited by its own peak: mixing them would limit a level by a measure of another.
 */
export function appliedGain({ replayGain }: Track, ordered: boolean): number | null {
	const album = { gain: replayGain?.albumGain, peak: replayGain?.albumPeak };
	const track = { gain: replayGain?.trackGain, peak: replayGain?.trackPeak };
	const [wanted, fallback] = ordered ? [album, track] : [track, album];
	const measured = wanted.gain === undefined ? fallback : wanted;
	return measured.gain === undefined ? null : limited(measured, measured.gain);
}

/** Converts decibels to the multiplier a gain stage takes. */
export function linear(decibels: number): number {
	return 10 ** (decibels / 20);
}
