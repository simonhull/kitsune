package loudness

import (
	"errors"
	"math"
	"math/rand/v2"
	"testing"
)

// noiseRate is the rate of noise.
const noiseRate = 44_100

// noise is seconds of stereo noise shaped like music, loud and quiet passages,
// at noiseRate.
func noise(seconds float64, seed uint64) [][]float64 {
	const rate = noiseRate
	rng := rand.New(rand.NewPCG(seed, 1)) //nolint:gosec // G404: test noise, seeded to repeat.
	n := int(seconds * float64(rate))
	planes := [][]float64{make([]float64, n), make([]float64, n)}
	level := 0.2
	for i := range n {
		if i%(rate/2) == 0 {
			level = math.Pow(10, (-30+rng.Float64()*25)/20)
		}
		for c := range planes {
			planes[c][i] = level * (rng.Float64()*2 - 1)
		}
	}
	return planes
}

func TestChunkingChangesNothing(t *testing.T) {
	t.Parallel()
	planes := noise(12, 1)
	_, chunked := measure(t, 44_100, planes)
	m, err := NewMeter(44_100, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Push(planes...); err != nil {
		t.Fatal(err)
	}
	whole, err := m.Finish()
	if err != nil {
		t.Fatal(err)
	}
	// Only the order of a sum differs.
	within(t, "I", chunked.Integrated, whole.Integrated, 1e-9, 1e-9)
	within(t, "LRA", chunked.Range, whole.Range, 1e-9, 1e-9)
	if chunked.TruePeak != whole.TruePeak || chunked.SamplePeak != whole.SamplePeak || chunked.Blocks != whole.Blocks {
		t.Errorf("chunked peaks %v %v, whole %v %v", chunked.TruePeak, chunked.SamplePeak, whole.TruePeak, whole.SamplePeak)
	}
}

func TestRatesAgree(t *testing.T) {
	t.Parallel()
	for _, rate := range []int{22_050, 32_000, 44_100, 88_200, 96_000, 192_000} {
		_, r := measure(t, rate, tones(rate, stereo(), tone{10, -23}))
		within(t, "I", r.Integrated, -23, 0.1, 0.1)
		within(t, "true peak", 20*math.Log10(r.TruePeak), -23, 0.1, 0.1)
	}
}

func TestSilence(t *testing.T) {
	t.Parallel()
	_, r := measure(t, 48_000, [][]float64{make([]float64, 48_000*5)})
	if !r.Silent {
		t.Errorf("silence has loudness %v", r.Integrated)
	}
	if _, ok := r.Gain(); ok {
		t.Error("silence has a gain")
	}
	if _, silent := Pool(&r.Blocks); !silent {
		t.Error("pooled silence has loudness")
	}
}

func TestGain(t *testing.T) {
	t.Parallel()
	_, r := measure(t, 48_000, tones(48_000, stereo(), tone{10, -23}))
	g, ok := r.Gain()
	if !ok {
		t.Fatal("no gain")
	}
	within(t, "gain", g, 5, 0.1, 0.1)
}

func TestMeterRefuses(t *testing.T) {
	t.Parallel()
	if _, err := NewMeter(4_000, 2); !errors.Is(err, ErrFormat) {
		t.Errorf("4 kHz: %v", err)
	}
	if _, err := NewMeter(44_100, 0); !errors.Is(err, ErrFormat) {
		t.Errorf("no channels: %v", err)
	}
	m, err := NewMeter(44_100, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Push(make([]float64, 3)); !errors.Is(err, ErrChannels) {
		t.Errorf("one plane: %v", err)
	}
	if err = m.Push(make([]float64, 3), make([]float64, 2)); !errors.Is(err, ErrChannels) {
		t.Errorf("uneven planes: %v", err)
	}
	if _, err = m.Finish(); err != nil {
		t.Fatal(err)
	}
	if err = m.Push(make([]float64, 3), make([]float64, 3)); !errors.Is(err, ErrFinished) {
		t.Errorf("push after finish: %v", err)
	}
	if _, err = m.Finish(); !errors.Is(err, ErrFinished) {
		t.Errorf("finish twice: %v", err)
	}
}

// An album pooled from histograms reads as its tracks played one after
// another, and a track pooled alone as itself.
func TestPoolMatchesOneProgramme(t *testing.T) {
	t.Parallel()
	a, b := noise(30, 2), noise(20, 3)
	for c := range b {
		for i := range b[c] {
			b[c][i] *= 0.3 // A quieter track.
		}
	}
	_, ra := measure(t, 44_100, a)
	_, rb := measure(t, 44_100, b)
	joined := [][]float64{append(append([]float64{}, a[0]...), b[0]...), append(append([]float64{}, a[1]...), b[1]...)}
	_, whole := measure(t, 44_100, joined)
	var ha, hb Histogram
	for _, x := range []struct {
		in  *Histogram
		out *Histogram
	}{{&ra.Blocks, &ha}, {&rb.Blocks, &hb}} {
		data, err := x.in.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		if err = x.out.UnmarshalBinary(data); err != nil {
			t.Fatal(err)
		}
	}
	pooled, silent := Pool(&ha, &hb)
	if silent {
		t.Fatal("silent")
	}
	// Bins move a block by at most 0.05 LU, and joining adds the three blocks
	// across the seam.
	within(t, "album", pooled, whole.Integrated, 0.06, 0.06)
	alone, _ := Pool(&ha)
	within(t, "one track", alone, ra.Integrated, 0.05, 0.05)
	if math.Abs(pooled-(ra.Integrated+rb.Integrated)/2) < 0.5 {
		t.Errorf("album %.2f is the mean of %.2f and %.2f: it should weigh the longer track more",
			pooled, ra.Integrated, rb.Integrated)
	}
}

func TestHistogramRefusesGarbage(t *testing.T) {
	t.Parallel()
	for _, data := range [][]byte{nil, {9}, {histogramForm, 0}, {histogramForm, 1}, {histogramForm, 0xff, 0xff, 0xff}} {
		var h Histogram
		if err := h.UnmarshalBinary(data); !errors.Is(err, ErrHistogram) {
			t.Errorf("%v: %v", data, err)
		}
	}
}

func BenchmarkMeter(b *testing.B) {
	const rate, seconds = noiseRate, 10
	planes := noise(seconds, 4)
	b.SetBytes(int64(rate * seconds * 2 * 2)) // As 16-bit stereo.
	for b.Loop() {
		m, err := NewMeter(rate, 2)
		if err != nil {
			b.Fatal(err)
		}
		for at := 0; at < len(planes[0]); at += 4096 {
			end := min(at+4096, len(planes[0]))
			if err = m.Push(planes[0][at:end], planes[1][at:end]); err != nil {
				b.Fatal(err)
			}
		}
		if _, err = m.Finish(); err != nil {
			b.Fatal(err)
		}
	}
}
