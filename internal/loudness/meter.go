package loudness

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// Reference is the loudness ReplayGain 2.0 plays everything at, in LUFS.
const Reference = -18.0

// Gating and windows, by BS.1770-4 and EBU Tech 3342.
const (
	absoluteGate  = -70.0 // LUFS
	relativeGate  = -10.0 // LU under the absolute-gated loudness
	rangeGate     = -20.0 // LU, for loudness range
	hopsPerSecond = 10    // Blocks start every 100 ms.
	blockHops     = 4     // A block is 400 ms.
	windowHops    = 30    // A short-term window is 3 s.
	lowPercentile = 0.10  // Loudness range is the spread from here...
	topPercentile = 0.95  // ...to here.
	offset        = -0.691
)

// Decibels: power is 10 dB a decade, and amplitude 20.
const (
	decade            = 10
	powerDecibels     = 10
	amplitudeDecibels = 20
)

// Errors a [Meter] returns.
var (
	ErrFormat   = errors.New("loudness: unsupported audio format")
	ErrChannels = errors.New("loudness: wrong channel count")
	ErrFinished = errors.New("loudness: meter already finished")
)

// Result is what a [Meter] measured. Loudness is in LUFS and LU, and peaks are
// linear, 1 being full scale.
type Result struct {
	// Integrated is the gated loudness of the whole. It means nothing when
	// Silent is set: no block was above the gates.
	Integrated float64
	Silent     bool
	// Range is the loudness range of EBU Tech 3342, 0 for audio too short or
	// quiet to have one.
	Range float64
	// MaxMomentary and MaxShortTerm are the loudest 400 ms block and 3 s
	// window, read every 100 ms; -Inf with none.
	MaxMomentary, MaxShortTerm float64
	TruePeak, SamplePeak       float64
	// Blocks is the gated blocks, for pooling into an album's loudness.
	Blocks Histogram
}

// Gain is the ReplayGain 2.0 gain of audio with this result, in dB. It
// reports false for silence, which no gain can level.
func (r *Result) Gain() (float64, bool) {
	if r.Silent {
		return 0, false
	}
	return Gain(r.Integrated), true
}

// Gain is the ReplayGain 2.0 gain, in dB, for audio of integrated loudness
// LUFS: what brings it to [Reference].
func Gain(integrated float64) float64 { return Reference - integrated }

// hop is the K-weighted energy of 100 ms of audio, its channels weighted and
// summed, and how many sample frames it holds. Rates that don't divide by 10
// give hops one frame apart.
type hop struct {
	energy float64
	frames int
}

// Meter measures one track's loudness and peaks from its samples. Create one
// with [NewMeter]. It is not safe for concurrent use.
type Meter struct {
	rate     int
	weights  []float64
	filters  []kFilter
	peaks    []peakReader
	sample   float64 // The sample peak.
	current  hop     // The hop being filled.
	hopIndex int     // Which hop current is, from 0.
	hopEnd   int     // Frames into the track where current ends.
	frames   int     // Frames pushed so far.
	recent   [windowHops]hop
	blocks   []float64 // Every 400 ms block's mean square, one per hop.
	windows  []float64 // Every 3 s window's mean square, one per hop.
	finished bool
}

// NewMeter returns a meter for audio at rate Hz with channels channels. The
// channels are weighted by BS.1770-4 for the layouts FLAC and WAV use: with 5
// (L, R, C, Ls, Rs) or 6 (L, R, C, LFE, Ls, Rs) the surrounds weigh 1.41 and
// the LFE nothing; every other channel weighs 1.
//
// # Errors
//
// It returns [ErrFormat] for a rate below 8 kHz or above 768 kHz, or a
// channel count outside 1 to 8.
func NewMeter(rate, channels int) (*Meter, error) {
	if rate < 8_000 || rate > 768_000 || channels < 1 || channels > 8 {
		return nil, fmt.Errorf("%w: %d Hz, %d channels", ErrFormat, rate, channels)
	}
	m := &Meter{
		rate: rate, weights: channelWeights(channels),
		filters: make([]kFilter, channels), peaks: make([]peakReader, channels),
	}
	phases := newInterpolator(oversampling(rate))
	for c := range channels {
		m.filters[c] = newKFilter(float64(rate))
		m.peaks[c] = newPeakReader(phases)
	}
	m.hopEnd = m.hopBoundary(1)
	return m, nil
}

// channelWeights returns BS.1770-4's weight for each of channels.
func channelWeights(channels int) []float64 {
	const (
		surround     = 1.41
		fiveChannels = 5 // L, R, C, Ls, Rs
		fivePointOne = 6 // L, R, C, LFE, Ls, Rs
	)
	weights := make([]float64, channels)
	for c := range weights {
		weights[c] = 1
	}
	switch channels {
	case fiveChannels:
		weights[3], weights[4] = surround, surround
	case fivePointOne:
		weights[3], weights[4], weights[5] = 0, surround, surround
	}
	return weights
}

// Push measures the next samples: one slice per channel, all as long, each
// sample scaled to full scale at 1.
//
// # Errors
//
// It returns [ErrChannels] when planes doesn't hold one slice per channel, all
// as long, and [ErrFinished] after [Meter.Finish].
func (m *Meter) Push(planes ...[]float64) error {
	if m.finished {
		return ErrFinished
	}
	n, err := m.frameCount(planes)
	if err != nil {
		return err
	}
	for c, p := range planes {
		m.peaks[c].read(p)
		for _, x := range p {
			m.sample = max(m.sample, math.Abs(x))
		}
	}
	for start := 0; start < n; {
		end := min(n, start+m.hopEnd-m.frames)
		for c, p := range planes {
			if w := m.weights[c]; w != 0 {
				m.current.energy += w * m.filters[c].energy(p[start:end])
			}
		}
		m.current.frames += end - start
		m.frames += end - start
		start = end
		if m.frames == m.hopEnd {
			m.endHop()
		}
	}
	return nil
}

// Finish returns what the meter measured. Audio after the last whole 100 ms
// counts toward the peaks only. A meter can be finished once.
//
// # Errors
//
// It returns [ErrFinished] when called again.
func (m *Meter) Finish() (Result, error) {
	if m.finished {
		return Result{}, ErrFinished
	}
	m.finished = true
	r := Result{SamplePeak: m.sample, TruePeak: m.sample}
	for c := range m.peaks {
		m.peaks[c].flush()
		r.TruePeak = max(r.TruePeak, m.peaks[c].peak)
	}
	r.MaxMomentary, r.MaxShortTerm = maxLoudness(m.blocks), maxLoudness(m.windows)
	for _, ms := range m.blocks {
		r.Blocks.add(ms)
	}
	r.Integrated, r.Silent = integrated(m.blocks)
	r.Range = loudnessRange(m.windows)
	return r, nil
}

// hopBoundary is the frame where hop i begins.
func (m *Meter) hopBoundary(i int) int { return i * m.rate / hopsPerSecond }

// frameCount checks planes holds one slice per channel, all as long, and
// returns their length.
func (m *Meter) frameCount(planes [][]float64) (int, error) {
	if len(planes) != len(m.filters) {
		return 0, fmt.Errorf("%w: %d, want %d", ErrChannels, len(planes), len(m.filters))
	}
	n := len(planes[0])
	for _, p := range planes {
		if len(p) != n {
			return 0, fmt.Errorf("%w: channels of %d and %d samples", ErrChannels, n, len(p))
		}
	}
	return n, nil
}

// endHop files the hop just filled, and the block and window it ends.
func (m *Meter) endHop() {
	m.recent[m.hopIndex%windowHops] = m.current
	m.hopIndex++
	m.current = hop{}
	m.hopEnd = m.hopBoundary(m.hopIndex + 1)
	if m.hopIndex >= blockHops {
		m.blocks = append(m.blocks, m.meanSquare(blockHops))
	}
	if m.hopIndex >= windowHops {
		m.windows = append(m.windows, m.meanSquare(windowHops))
	}
}

// meanSquare is the mean square of the last hops hops.
func (m *Meter) meanSquare(hops int) float64 {
	var energy float64
	var frames int
	for i := m.hopIndex - hops; i < m.hopIndex; i++ {
		h := m.recent[i%windowHops]
		energy += h.energy
		frames += h.frames
	}
	return energy / float64(frames)
}

// lufs is the loudness of a mean square.
func lufs(meanSquare float64) float64 { return offset + powerDecibels*math.Log10(meanSquare) }

// meanSquareOf is the mean square of a loudness: the inverse of lufs.
func meanSquareOf(loudness float64) float64 { return ratio(loudness - offset) }

// ratio is the power ratio of a level difference in dB.
func ratio(db float64) float64 { return math.Pow(decade, db/powerDecibels) }

// maxLoudness is the loudness of the loudest of mean squares, -Inf with none.
func maxLoudness(meanSquares []float64) float64 {
	if len(meanSquares) == 0 {
		return math.Inf(-1)
	}
	return lufs(slices.Max(meanSquares))
}

// integrated gates blocks' mean squares and returns the loudness of what
// passes, and true when nothing does.
func integrated(blocks []float64) (float64, bool) {
	floor := meanSquareOf(absoluteGate)
	ungated, ok := mean(blocks, floor)
	if !ok {
		return 0, true
	}
	gated, ok := mean(blocks, max(floor, ungated*ratio(relativeGate)))
	if !ok {
		return 0, true
	}
	return lufs(gated), false
}

// mean is the mean of the values above floor, and false when there are none.
func mean(values []float64, floor float64) (float64, bool) {
	var sum float64
	var n int
	for _, v := range values {
		if v > floor {
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

// loudnessRange is EBU Tech 3342's loudness range of short-term windows' mean
// squares.
func loudnessRange(windows []float64) float64 {
	floor := meanSquareOf(absoluteGate)
	ungated, ok := mean(windows, floor)
	if !ok {
		return 0
	}
	gate := max(floor, ungated*ratio(rangeGate))
	var levels []float64
	for _, w := range windows {
		if w > gate {
			levels = append(levels, lufs(w))
		}
	}
	if len(levels) == 0 {
		return 0
	}
	slices.Sort(levels)
	return percentile(levels, topPercentile) - percentile(levels, lowPercentile)
}

// percentile is the p-th of sorted values, by the nearest rank.
func percentile(sorted []float64, p float64) float64 {
	i := int(math.Round(p * float64(len(sorted)-1)))
	return sorted[min(max(i, 0), len(sorted)-1)]
}
