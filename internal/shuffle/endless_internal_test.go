package shuffle

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

func TestFenwick(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		weights []float64
		target  float64
		want    int
	}{
		{"the first", []float64{1, 2, 3}, 0.5, 0},
		{"on a boundary goes on", []float64{1, 2, 3}, 1, 1},
		{"the last", []float64{1, 2, 3}, 5.9, 2},
		{"skips zeros", []float64{1, 0, 0, 3}, 1.5, 3},
		{"past the end stays in", []float64{1, 2}, 9, 1},
		{"seven", []float64{1, 1, 1, 1, 1, 1, 1}, 6.5, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFenwick(slices.Clone(tt.weights))
			if got := f.find(tt.target); got != tt.want {
				t.Fatalf("find(%v): got %d, want %d", tt.target, got, tt.want)
			}
		})
	}
	f := newFenwick([]float64{1, 2, 3, 4})
	f.set(2, 0)
	if got := f.total(); got != 7 {
		t.Fatalf("total after set: got %v, want 7", got)
	}
	if got := f.find(3.5); got != 3 {
		t.Fatalf("find after set: got %d, want 3", got)
	}
}

func TestFenwickDrawsInProportion(t *testing.T) {
	t.Parallel()
	weights := []float64{1, 2, 3, 4}
	f := newFenwick(slices.Clone(weights))
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a repeatable test draw.
	var counts [4]int
	const draws = 100_000
	for range draws {
		counts[f.find(rng.Float64()*f.total())]++
	}
	for i, w := range weights {
		if got, want := float64(counts[i])/draws, w/10; math.Abs(got-want) > 0.01 {
			t.Errorf("unit %d: drawn %.3f of the time, want %.3f", i, got, want)
		}
	}
}

// pages takes pages of size from e until it has total picks.
func pages(e *Endless, p *Pool, c Config, size, total int) []Pick {
	var out []Pick
	for len(out) < total {
		out = append(out, e.Next(p, c, min(size, total-len(out)))...)
	}
	return out
}

func TestEndlessDoesNotRepeatWithinACycle(t *testing.T) {
	t.Parallel()
	p := pool(t, grid(20, 5, 1), nil) // 100 units of equal weight
	for seed := range uint64(20) {
		// The cycle closes once CycleShare of the weight has played: 70 of 100 equal units.
		got := ids(pages(NewEndless(seed, 0), p, config(), 7, 69))
		if len(slices.Compact(slices.Sorted(slices.Values(got)))) != len(got) {
			t.Fatalf("seed %d: a repeat within 69 of 100 equal units: %v", seed, got)
		}
	}
}

func TestEndlessKeepsGoing(t *testing.T) {
	t.Parallel()
	p := pool(t, grid(10, 3, 1), nil)
	got := pages(NewEndless(1, 0), p, config(), PageSize, 300)
	counts := map[TrackID]int{}
	for _, id := range ids(got) {
		counts[id]++
	}
	if len(got) != 300 || len(counts) != 30 {
		t.Fatalf("got %d picks of %d units, want 300 of all 30", len(got), len(counts))
	}
	for id, n := range counts {
		if n < 5 || n > 15 {
			t.Errorf("unit %d came up %d times in 10 rounds of 30", id, n)
		}
	}
}

func TestEndlessSpacingHoldsAcrossPages(t *testing.T) {
	t.Parallel()
	p := pool(t, grid(12, 6, 2), nil)
	for seed := range uint64(20) {
		picks := pages(NewEndless(seed, 0), p, config(), 3, 200)
		if b := scaledBreaks(p, picks); b != 0 {
			t.Fatalf("seed %d: %d breaks in %v", seed, b, ids(picks))
		}
	}
}

func TestEndlessIsRepeatableAndResumes(t *testing.T) {
	t.Parallel()
	p := pool(t, grid(8, 6, 2), []Run{{1, 2}})
	c := Config{Settings: Settings{Ratings: 1, Flow: 1, Memory: true}, Similarity: ring{n: 8}, Now: now}
	whole := ids(pages(NewEndless(9, 5), p, c, 10, 120))
	if again := ids(pages(NewEndless(9, 5), p, c, 10, 120)); !slices.Equal(whole, again) {
		t.Fatalf("one seed, two streams:\n%v\n%v", whole, again)
	}
	e := NewEndless(9, 5)
	first := ids(pages(e, p, c, 10, 50))
	resumed := ResumeEndless(new(e.State()))
	if resumed.Position() != 50 {
		t.Fatalf("resumed at %d, want 50", resumed.Position())
	}
	rest := ids(pages(resumed, p, c, 10, 70))
	if got := slices.Concat(first, rest); !slices.Equal(got, whole) {
		t.Fatalf("a resumed stream differs:\n%v\n%v", got, whole)
	}
	if whole[0] != 5 {
		t.Fatalf("the stream starts with %d, want the chosen 5", whole[0])
	}
}

func TestEndlessForgetsAfterThirtyDays(t *testing.T) {
	t.Parallel()
	p := pool(t, grid(10, 1, 1), nil) // ten artists, one song each
	overlaps := func(later Config, more int) int {
		n := 0
		for seed := range uint64(20) {
			e := NewEndless(seed, 0)
			before := ids(e.Next(p, config(), 4))
			for _, id := range ids(e.Next(p, later, more)) {
				if slices.Contains(before, id) {
					n++
				}
			}
		}
		return n
	}
	soon, late := config(), config()
	soon.Now = now.Add(day)
	late.Now = now.Add(NoRepeatFor)
	if n := overlaps(soon, 3); n != 0 {
		t.Errorf("a day later, %d units came back", n)
	}
	if n := overlaps(late, 6); n == 0 {
		t.Error("thirty days later, nothing came back")
	}
}

func TestEndlessFollowsWeights(t *testing.T) {
	t.Parallel()
	tracks := grid(50, 4, 2)
	for i := range tracks {
		tracks[i].Stars = Stars(2 * (i%5 + 1))
	}
	p := pool(t, tracks, nil)
	c := Config{Settings: Settings{Ratings: 1}, Now: now}
	var plays [MaxStars/2 + 1]int
	for _, pk := range pages(NewEndless(3, 0), p, c, PageSize, 5000) {
		plays[int(pk.Reason.Stars)]++
	}
	for stars := 2; stars <= int(MaxStars/2); stars++ {
		if plays[stars] <= plays[stars-1] {
			t.Errorf("%d★ played no more than %d★: %v", stars, stars-1, plays)
		}
	}
}

func TestEndlessFlow(t *testing.T) {
	t.Parallel()
	p := pool(t, grid(30, 10, 2), nil)
	sim := ring{n: 30}
	mean := func(strength float64) float64 {
		c := Config{Settings: Settings{Ratings: DefaultRatings, Flow: strength}, Similarity: sim, Now: now}
		picks := pages(NewEndless(4, 0), p, c, PageSize, 3000)
		total := 0.0
		for i := 1; i < len(picks); i++ {
			total += sim.Artist(picks[i-1].Unit.artists[0], picks[i].Unit.artists[0])
		}
		return total / float64(len(picks)-1)
	}
	if off, on := mean(0), mean(1); on < off+0.05 {
		t.Fatalf("flow 1 gave neighbor similarity %.3f, flow 0 %.3f: want a clear lean", on, off)
	}
}

func TestEndlessSmallPools(t *testing.T) {
	t.Parallel()
	tracks := grid(1, 1, 1)
	tracks = append(tracks, skipped(2))
	p := pool(t, tracks, nil)
	tests := []struct {
		name  string
		p     *Pool
		first TrackID
		n     int
		want  []TrackID
	}{
		{"nothing from nothing", pool(t, nil, nil), 0, 5, nil},
		{"a chosen skipped song leads an empty pool", pool(t, []Track{skipped(2)}, nil), 2, 5, []TrackID{2}},
		{"one song repeats", p, 0, 3, []TrackID{1, 1, 1}},
		{"a chosen skipped song leads, then never again", p, 2, 3, []TrackID{2, 1, 1}},
		{"no page", p, 0, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ids(NewEndless(1, tt.first).Next(tt.p, config(), tt.n)); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
