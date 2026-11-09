package loudness

import "math"

// The true peak is read between the samples by interpolating them with a
// windowed sinc: phaseTaps samples around each point, under a Kaiser window
// of kaiserBeta. BS.1770-4 Annex 2's example filter has 12 taps a phase, and
// under-reads content near the Nyquist frequency; measured on music and
// noise, the reading stops rising at about 32.
const (
	phaseTaps  = 32
	halfTaps   = phaseTaps / 2
	kaiserBeta = 8
)

// oversampling returns how many points a sample period is read at, by
// BS.1770-4 Annex 2: 4 below 96 kHz, 2 below 192 kHz, and 1 above.
func oversampling(rate int) int {
	const (
		double, quadruple = 2, 4
		high, highest     = 96_000, 192_000
	)
	switch {
	case rate < high:
		return quadruple
	case rate < highest:
		return double
	}
	return 1
}

// interpolator holds the filter that reads the points between samples: for
// each point but the sample itself, phaseTaps coefficients, oldest sample
// first.
type interpolator [][phaseTaps]float64

// newInterpolator designs the filter for factor points a sample period. Each
// phase is scaled to a gain of 1 at DC, so a constant reads as itself.
func newInterpolator(factor int) interpolator {
	phases := make(interpolator, factor-1)
	half := float64(halfTaps)
	for p := range phases {
		offset := float64(p+1) / float64(factor)
		var sum float64
		for k := range phaseTaps {
			// The point lies between the samples half-1 and half of the window.
			u := half - 1 - float64(k) + offset
			phases[p][k] = sinc(u) * kaiser(u/half)
			sum += phases[p][k]
		}
		for k := range phaseTaps {
			phases[p][k] /= sum
		}
	}
	return phases
}

// sinc is the normalized sinc function.
func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

// kaiser is the Kaiser window at x, from -1 to 1.
func kaiser(x float64) float64 {
	if x <= -1 || x >= 1 {
		return 0
	}
	return besselI0(kaiserBeta*math.Sqrt(1-x*x)) / besselI0(kaiserBeta)
}

// besselI0 is the modified Bessel function of the first kind, of order 0, by
// its power series, which converges fast for the arguments a window takes.
func besselI0(x float64) float64 {
	sum, term := 1.0, 1.0
	for k := 1.0; term > sum*1e-17; k++ {
		half := x / (k + k)
		term *= half * half
		sum += term
	}
	return sum
}

// quadruplePhases is how many points between samples 4 times oversampling
// reads.
const quadruplePhases = 3

// segment is how many samples the peak reader bounds at a time: see
// [peakReader.read].
const segment = 64

// peakReader follows one channel's true peak. It keeps the last phaseTaps
// samples twice over, so the window is always one slice.
type peakReader struct {
	phases interpolator
	gain   float64 // The most any phase can amplify: the largest sum of its coefficients' magnitudes.
	window [phaseTaps + phaseTaps]float64
	at     int // Where the next sample goes.
	peak   float64
}

// newPeakReader returns a reader that reads between samples with phases.
func newPeakReader(phases interpolator) peakReader {
	r := peakReader{phases: phases}
	for p := range phases {
		var sum float64
		for _, c := range phases[p] {
			sum += math.Abs(c)
		}
		r.gain = max(r.gain, sum)
	}
	return r
}

// read takes samples in and keeps the largest magnitude read between them. A
// point between samples is at most gain times the largest sample in its
// window, so a segment that can't reach the peak so far is only remembered,
// not read: loud passages are read, and the rest costs little.
func (r *peakReader) read(samples []float64) {
	if len(r.phases) == 0 {
		return // At 192 kHz and above the samples are the points.
	}
	for len(samples) > 0 {
		seg := samples[:min(segment, len(samples))]
		samples = samples[len(seg):]
		// The windows reach back into the samples already in the window.
		var loudest float64
		for _, x := range r.window[:phaseTaps] {
			loudest = max(loudest, math.Abs(x))
		}
		for _, x := range seg {
			loudest = max(loudest, math.Abs(x))
		}
		if loudest*r.gain <= r.peak {
			r.remember(seg)
		} else {
			r.interpolate(seg)
		}
	}
}

// remember puts samples in the window without reading between them.
func (r *peakReader) remember(samples []float64) {
	for _, x := range samples {
		r.window[r.at] = x
		r.window[r.at+phaseTaps] = x
		r.at++
		if r.at == phaseTaps {
			r.at = 0
		}
	}
}

// interpolate puts samples in the window and reads between each and the one
// before.
func (r *peakReader) interpolate(samples []float64) {
	peak := r.peak
	for _, x := range samples {
		r.window[r.at] = x
		r.window[r.at+phaseTaps] = x
		r.at++
		if r.at == phaseTaps {
			r.at = 0
		}
		w := (*[phaseTaps]float64)(r.window[r.at : r.at+phaseTaps])
		if len(r.phases) == quadruplePhases { // The usual case, in one pass over the window.
			c1, c2, c3 := &r.phases[0], &r.phases[1], &r.phases[2]
			var y1, y2, y3 float64
			for k := range phaseTaps {
				y1 += c1[k] * w[k]
				y2 += c2[k] * w[k]
				y3 += c3[k] * w[k]
			}
			peak = max(peak, math.Abs(y1), math.Abs(y2), math.Abs(y3))
			continue
		}
		for p := range r.phases {
			c := &r.phases[p]
			var y float64
			for k := range phaseTaps {
				y += c[k] * w[k]
			}
			peak = max(peak, math.Abs(y))
		}
	}
	r.peak = peak
}

// flush reads the points after the last sample, as if silence followed.
func (r *peakReader) flush() {
	var silence [halfTaps]float64
	r.interpolate(silence[:])
}
