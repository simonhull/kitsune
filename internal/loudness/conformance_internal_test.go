package loudness

import (
	"math"
	"testing"
)

// The minimum requirements test signals of EBU Tech 3341 (version 4, 2023)
// and EBU Tech 3342, made here rather than read from the EBU's files. Cases 7
// and 8 of 3341 and 5 and 6 of 3342 are authentic program, which only the
// EBU's files hold. Levels are each channel's peak, in dBFS.

const testRate = 48_000

// tone is a stretch of a 1 kHz sine at a peak level, or silence at -Inf.
type tone struct {
	seconds float64
	dbfs    float64
}

// tones makes the signal of 3341's cases built from 1 kHz tones, one plane
// per channel, each channel at its own level relative to the tone's.
func tones(rate int, gains []float64, parts ...tone) [][]float64 {
	planes := make([][]float64, len(gains))
	var n int
	for _, p := range parts {
		frames := int(math.Round(p.seconds * float64(rate)))
		for i := range frames {
			for c, g := range gains {
				a := 0.0
				if !math.IsInf(p.dbfs, -1) {
					a = math.Pow(10, (p.dbfs+g)/20)
				}
				planes[c] = append(planes[c], a*math.Sin(2*math.Pi*1000*float64(n+i)/float64(rate)))
			}
		}
		n += frames
	}
	return planes
}

// stereo is the gains of a stereo tone: both channels at its level.
func stereo() []float64 { return []float64{0, 0} }

// measure meters planes at rate, pushed in uneven chunks, and returns the
// meter, finished, and its result.
func measure(t *testing.T, rate int, planes [][]float64) (*Meter, Result) {
	t.Helper()
	m, err := NewMeter(rate, len(planes))
	if err != nil {
		t.Fatal(err)
	}
	sizes := []int{1, 7, 4096, 333, 10_000, 1}
	for at, i := 0, 0; at < len(planes[0]); i++ {
		end := min(len(planes[0]), at+sizes[i%len(sizes)])
		chunk := make([][]float64, len(planes))
		for c := range planes {
			chunk[c] = planes[c][at:end]
		}
		if pushErr := m.Push(chunk...); pushErr != nil {
			t.Fatal(pushErr)
		}
		at = end
	}
	r, err := m.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return m, r
}

func within(t *testing.T, what string, got, want, below, above float64) {
	t.Helper()
	if got < want-below || got > want+above || math.IsNaN(got) {
		t.Errorf("%s = %.3f, want %.3f (-%.2f/+%.2f)", what, got, want, below, above)
	}
}

func TestTech3341Integrated(t *testing.T) {
	t.Parallel()
	five := []float64{-28 + 23, -28 + 23, -24 + 23, -30 + 23, -30 + 23} // Relative to -23 dBFS.
	cases := []struct {
		name   string
		gains  []float64
		parts  []tone
		want   float64
		steady bool // M and S read the same throughout.
	}{
		{"1", stereo(), []tone{{20, -23}}, -23, true},
		{"2", stereo(), []tone{{20, -33}}, -33, true},
		{"3", stereo(), []tone{{10, -36}, {60, -23}, {10, -36}}, -23, false},
		{"4", stereo(), []tone{{10, -72}, {10, -36}, {60, -23}, {10, -36}, {10, -72}}, -23, false},
		{"5", stereo(), []tone{{20, -26}, {20.1, -20}, {20, -26}}, -23, false},
		{"6", five, []tone{{20, -23}}, -23, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, r := measure(t, testRate, tones(testRate, tc.gains, tc.parts...))
			if r.Silent {
				t.Fatal("silent")
			}
			within(t, "I", r.Integrated, tc.want, 0.1, 0.1)
			if tc.steady {
				within(t, "max M", r.MaxMomentary, tc.want, 0.1, 0.1)
				within(t, "max S", r.MaxShortTerm, tc.want, 0.1, 0.1)
			}
		})
	}
}

// Case 9: S is constant after 3 s.
func TestTech3341ShortTermSteady(t *testing.T) {
	t.Parallel()
	parts := make([]tone, 0, 10)
	for range 5 {
		parts = append(parts, tone{1.34, -20}, tone{1.66, -30})
	}
	m, _ := measure(t, testRate, tones(testRate, stereo(), parts...))
	if len(m.windows) == 0 {
		t.Fatal("no short-term windows")
	}
	for i, w := range m.windows {
		within(t, "S", lufs(w), -23, 0.1, 0.1)
		if t.Failed() {
			t.Fatalf("window %d", i)
		}
	}
}

// Case 10: the loudest S of each segment.
func TestTech3341ShortTermMax(t *testing.T) {
	t.Parallel()
	for i := range 20 {
		_, r := measure(t, testRate, tones(testRate, stereo(),
			tone{float64(i) * 0.15, math.Inf(-1)}, tone{3, -23}, tone{1, math.Inf(-1)}))
		within(t, "max S", r.MaxShortTerm, -23, 0.1, 0.1)
	}
}

// Case 12: M is constant after 1 s. Cases 13 and 14 ask for the loudest M of
// tones placed every 20 ms, which a block every 100 ms, as BS.1770 sets, can
// only read within 1 LU: they are left out, since nothing here reads M.
func TestTech3341MomentarySteady(t *testing.T) {
	t.Parallel()
	parts := make([]tone, 0, 50)
	for range 25 {
		parts = append(parts, tone{0.18, -20}, tone{0.22, -30})
	}
	m, _ := measure(t, testRate, tones(testRate, stereo(), parts...))
	for i, b := range m.blocks {
		if (i+blockHops)*100 < 1000 { // The block ends before 1 s.
			continue
		}
		within(t, "M", lufs(b), -23, 0.1, 0.1)
		if t.Failed() {
			t.Fatalf("block %d", i)
		}
	}
}

// sine is cases 15 to 19: a stereo sine at testRate/divisor Hz of amplitude
// and phase in degrees, faded in and out over 10 ms.
func sine(divisor, amplitude, phase float64) [][]float64 {
	const rate = testRate
	n := rate // One second.
	fade := rate / 100
	plane := make([]float64, n)
	for i := range plane {
		ramp := min(1, float64(i)/float64(fade), float64(n-1-i)/float64(fade))
		plane[i] = ramp * amplitude * math.Sin(2*math.Pi*float64(i)/divisor+phase*math.Pi/180)
	}
	return [][]float64{plane, plane}
}

// burst is cases 20 to 23: a sine of rate/6 Hz at 0.5 holding one period of a
// sine of rate/4 Hz at 1, both starting at phase 0, made at 4 times the rate,
// low-passed and taken every 4th sample from offset. The rate is testRate.
func burst(offset int) [][]float64 {
	const rate = testRate
	const over = 4
	fast := float64(rate * over)
	n := rate * over / 2 // Half a second.
	start := n / 2
	start -= start % (6 * over) // At a zero of the slow sine, going up.
	period := 4 * over          // Samples of the fast sine's one period.
	hi := make([]float64, n)
	for i := range hi {
		switch {
		case i >= start && i < start+period:
			hi[i] = math.Sin(2 * math.Pi * float64(rate) / 4 * float64(i-start) / fast)
		case i >= start+period:
			hi[i] = 0.5 * math.Sin(2*math.Pi*float64(rate)/6*float64(i-start-period)/fast)
		default:
			hi[i] = 0.5 * math.Sin(2*math.Pi*float64(rate)/6*float64(i-start)/fast)
		}
		fade := float64(rate*over) / 100
		hi[i] *= min(1, float64(i)/fade, float64(n-1-i)/fade)
	}
	// A long windowed sinc at the low rate's Nyquist frequency.
	const taps = 255
	low := make([]float64, 0, n/over)
	for i := offset; i < n; i += over {
		var y, sum float64
		for k := -taps / 2; k <= taps/2; k++ {
			h := sinc(float64(k)/over) * kaiser(float64(k)/(taps/2+1))
			sum += h
			if j := i - k; j >= 0 && j < n {
				y += h * hi[j]
			}
		}
		low = append(low, y/sum)
	}
	return [][]float64{low, low}
}

func TestTech3341TruePeak(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		planes [][]float64
		want   float64
	}{
		{"15", sine(4, 0.5, 0), -6},
		{"16", sine(4, 0.5, 45), -6},
		{"17", sine(6, 0.5, 60), -6},
		{"18", sine(8, 0.5, 67.5), -6},
		{"19", sine(4, 1.41, 45), 3},
		{"20", burst(0), 0},
		{"21", burst(1), 0},
		{"22", burst(2), 0},
		{"23", burst(3), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, r := measure(t, testRate, tc.planes)
			within(t, "true peak", 20*math.Log10(r.TruePeak), tc.want, 0.4, 0.2)
		})
	}
}

func TestTech3342Range(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		parts []tone
		want  float64
	}{
		{"1", []tone{{20, -20}, {20, -30}}, 10},
		{"2", []tone{{20, -20}, {20, -15}}, 5},
		{"3", []tone{{20, -40}, {20, -20}}, 20},
		{"4", []tone{{20, -50}, {20, -35}, {20, -20}, {20, -35}, {20, -50}}, 15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, r := measure(t, testRate, tones(testRate, stereo(), tc.parts...))
			within(t, "LRA", r.Range, tc.want, 1, 1)
		})
	}
}
