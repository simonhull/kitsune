package loudness

import "math"

// The K-weighting filter of BS.1770-4 is a high shelf (the head's effect)
// followed by a high-pass (the revised low-frequency B-curve). BS.1770 gives
// their coefficients at 48 kHz only; these are the analog parameters they come
// from, so that the bilinear transform below gives the same filter at any rate
// and exactly BS.1770's coefficients at 48 kHz.
const (
	shelfHz   = 1681.974450955533
	shelfGain = 3.999843853973347 // dB
	shelfQ    = 0.7071752369554196
	shelfBand = 0.4996667741545416 // How much of the shelf's gain its mid band takes.
	passHz    = 38.13547087602444
	passQ     = 0.5003270373238773
)

// biquad is a second-order section, normalized so that a0 is 1.
type biquad struct{ b0, b1, b2, a1, a2 float64 }

// newKFilter returns a channel's K-weighting filter at rate Hz.
func newKFilter(rate float64) kFilter {
	return kFilter{shelf: highShelf(rate), pass: highPass(rate)}
}

// highShelf is the first section of the K-weighting filter at rate Hz.
func highShelf(rate float64) biquad {
	k := math.Tan(math.Pi * shelfHz / rate)
	vh := math.Pow(decade, shelfGain/amplitudeDecibels)
	vb := math.Pow(vh, shelfBand)
	a0 := 1 + k/shelfQ + k*k
	return biquad{
		b0: (vh + vb*k/shelfQ + k*k) / a0,
		b1: 2 * (k*k - vh) / a0,
		b2: (vh - vb*k/shelfQ + k*k) / a0,
		a1: 2 * (k*k - 1) / a0,
		a2: (1 - k/shelfQ + k*k) / a0,
	}
}

// highPass is the second section of the K-weighting filter at rate Hz.
func highPass(rate float64) biquad {
	k := math.Tan(math.Pi * passHz / rate)
	a0 := 1 + k/passQ + k*k
	return biquad{b0: 1, b1: -2, b2: 1, a1: 2 * (k*k - 1) / a0, a2: (1 - k/passQ + k*k) / a0}
}

// kFilter is one channel's K-weighting: both sections, with their state, in
// transposed direct form II.
type kFilter struct {
	shelf, pass biquad
	s1, s2      float64 // The shelf's state.
	p1, p2      float64 // The high-pass's state.
}

// energy filters samples and returns the sum of their squares, filtered.
func (f *kFilter) energy(samples []float64) float64 {
	sh, ps := f.shelf, f.pass
	s1, s2, p1, p2 := f.s1, f.s2, f.p1, f.p2
	var sum float64
	for _, x := range samples {
		y := sh.b0*x + s1
		s1 = sh.b1*x - sh.a1*y + s2
		s2 = sh.b2*x - sh.a2*y
		z := ps.b0*y + p1
		p1 = ps.b1*y - ps.a1*z + p2
		p2 = ps.b2*y - ps.a2*z
		sum += z * z
	}
	f.s1, f.s2, f.p1, f.p2 = s1, s2, p1, p2
	return sum
}
