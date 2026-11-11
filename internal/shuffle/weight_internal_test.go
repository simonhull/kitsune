package shuffle

import (
	"math"
	"strings"
	"testing"
	"time"
)

// now is the bench's and tests' fixed present.
var now = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) //nolint:gochecknoglobals // A fixed instant for tests.

// day is 24 hours.
const day = 24 * time.Hour

// close reports whether a and b agree to 1e-9.
func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// unitOf builds the single unit of tracks, linked as one run.
func unitOf(t *testing.T, tracks ...Track) *Unit {
	t.Helper()
	run := make(Run, len(tracks))
	for i := range tracks {
		run[i] = tracks[i].ID
	}
	p, err := NewPool(tracks, []Run{run})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	if p.Len() != 1 {
		t.Fatalf("got %d units, want 1", p.Len())
	}
	return &p.units[0]
}

func TestRatingCurve(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		ratings float64
		want    [MaxStars/2 + 1]float64 // by whole stars, unrated first
	}{
		{"ratings matter fully", 1, [...]float64{1, 0.2, 0.6, 1, 1.6, 2.4}},
		{"ratings don't matter", 0, [...]float64{1, 1, 1, 1, 1, 1}},
		{"the default is the square root", 0.5, [...]float64{
			1, math.Sqrt(0.2), math.Sqrt(0.6), 1, math.Sqrt(1.6), math.Sqrt(2.4),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := Settings{Ratings: tt.ratings}
			for stars, want := range tt.want {
				u := unitOf(t, Track{ID: 1, Stars: Stars(2 * stars)})
				if got := Explain(u, s, now).Rating; !near(got, want) {
					t.Errorf("%d★: got %v, want %v", stars, got, want)
				}
			}
		})
	}
}

func TestRunRatingIsTheMeanOfItsRatedSongs(t *testing.T) {
	t.Parallel()
	u := unitOf(t, Track{ID: 1}, Track{ID: 2, Stars: 8}, Track{ID: 3, Stars: 10})
	r := Explain(u, Settings{Ratings: 1}, now)
	if r.Stars != 4.5 {
		t.Errorf("Stars: got %v, want 4.5", r.Stars)
	}
	// Halfway between 1.6 and 2.4 in log space.
	if want := math.Sqrt(1.6 * 2.4); !near(r.Rating, want) {
		t.Errorf("Rating: got %v, want %v", r.Rating, want)
	}
}

// wholeFactor is the rating factor as it was before halves, on a mean of whole
// stars: a whole rating must weigh the same float it always did.
func wholeFactor(starSum, rated int, ratings float64) float64 {
	stars := float64(starSum) / float64(rated)
	lo := int(stars)
	w := math.Log(starCurve[lo-1])
	if lo < len(starCurve) {
		w += (stars - float64(lo)) * (math.Log(starCurve[lo]) - w)
	}
	return math.Exp(ratings * w)
}

func TestWholeRatingsWeighAsBefore(t *testing.T) {
	t.Parallel()
	for _, ratings := range []float64{0, 0.25, 0.5, 1, 2} {
		for _, whole := range [][]Stars{{1}, {2}, {3}, {4}, {5}, {4, 5}, {1, 2, 2}, {3, 5, 5, 4}, {1, 1, 1, 1, 1, 1, 2}} {
			tracks := make([]Track, len(whole))
			sum := 0
			for i, s := range whole {
				tracks[i] = Track{ID: TrackID(i + 1), Stars: 2 * s}
				sum += int(s)
			}
			r := Explain(unitOf(t, tracks...), Settings{Ratings: ratings}, now)
			if want := wholeFactor(sum, len(whole), min(ratings, 1)); r.Rating != want {
				t.Errorf("%v at Ratings %v: got %v, want exactly %v", whole, ratings, r.Rating, want)
			}
			if want := float64(sum) / float64(len(whole)); r.Stars != want {
				t.Errorf("%v: mean %v, want exactly %v", whole, r.Stars, want)
			}
		}
	}
}

func TestHalfRatings(t *testing.T) {
	t.Parallel()
	s := Settings{Ratings: 1}
	factor := func(halves Stars) float64 {
		t.Helper()
		return Explain(unitOf(t, Track{ID: 1, Stars: halves}), s, now).Rating
	}
	if got, want := factor(9), math.Sqrt(1.6*2.4); !near(got, want) || got <= factor(8) || got >= factor(10) {
		t.Errorf("4½★ = %v, want %v, between 4★ and 5★", got, want)
	}
	if got, want := factor(7), math.Sqrt(1.0*1.6); !near(got, want) {
		t.Errorf("3½★ = %v, want %v", got, want)
	}
	if got, want := factor(1), 0.2*math.Sqrt(0.2/0.6); !near(got, want) || got >= factor(2) {
		t.Errorf("½★ = %v, want %v, below 1★", got, want)
	}
	half := Explain(unitOf(t, Track{ID: 1, Stars: 1}), s, now)
	if half.Stars != 0.5 {
		t.Errorf("½★ reads as %v stars", half.Stars)
	}
	if got := half.String(); !strings.HasPrefix(got, "0.5★ ×0.12,") {
		t.Errorf("½★ String = %q", got)
	}
}

func TestHeart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ratings, want float64
	}{{0, 1}, {0.5, 1.25}, {1, 1.5625}}
	for _, tt := range tests {
		u := unitOf(t, Track{ID: 1}, Track{ID: 2, Hearted: true})
		if got := Explain(u, Settings{Ratings: tt.ratings}, now).Heart; !near(got, tt.want) {
			t.Errorf("Ratings %v: got %v, want %v", tt.ratings, got, tt.want)
		}
	}
}

func TestRecency(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ago  time.Duration // 0 for never
		want float64
	}{
		{"never played", 0, 1.2},
		{"just played", time.Second, 0.05},
		{"yesterday", day, math.Pow(1.0/14, 0.7)},
		{"a week ago", 7 * day, math.Pow(0.5, 0.7)},
		{"two weeks ago", 14 * day, 1},
		{"three months ago", 90 * day, 1},
		{"over half a year ago", 181 * day, 1.2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tr := Track{ID: 1}
			if tt.ago != 0 {
				tr.History.LastPlayed = now.Add(-tt.ago)
			}
			u := unitOf(t, tr)
			if got := Explain(u, DefaultSettings(), now).Recency; !near(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		history History
		memory  bool
		want    Reason // only SkipMemory, SkipRate and Listen are compared
	}{
		{"no history", History{}, true, Reason{SkipMemory: 1, SkipRate: 1, Listen: 1}},
		{"one early skip", History{EarlySkips: 1}, true, Reason{SkipMemory: 0.5, SkipRate: 1, Listen: 1}},
		{"two stack", History{EarlySkips: 2}, true, Reason{SkipMemory: 0.25, SkipRate: 1, Listen: 1}},
		{"three stack no further", History{EarlySkips: 3}, true, Reason{SkipMemory: 0.25, SkipRate: 1, Listen: 1}},
		{
			"one skip in one start is smoothed",
			History{ShuffleStarts: 1, ShuffleSkips: 1},
			true,
			Reason{SkipMemory: 1, SkipRate: 1 - 0.6/3, Listen: 1},
		},
		{
			"always skipped tends to 0.4",
			History{ShuffleStarts: 998, ShuffleSkips: 998},
			true,
			Reason{SkipMemory: 1, SkipRate: 1 - 0.6*0.998, Listen: 1},
		},
		{
			"always stopped halfway tends to 0.8",
			History{Listens: 98, Heard: 49},
			true,
			Reason{SkipMemory: 1, SkipRate: 1, Listen: 0.6 + 0.4*0.51},
		},
		{
			"memory off ignores it all",
			History{EarlySkips: 2, ShuffleStarts: 10, ShuffleSkips: 10, Listens: 10},
			false,
			Reason{SkipMemory: 1, SkipRate: 1, Listen: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u := unitOf(t, Track{ID: 1, History: tt.history})
			s := DefaultSettings()
			s.Memory = tt.memory
			got := Explain(u, s, now)
			if !near(got.SkipMemory, tt.want.SkipMemory) || !near(got.SkipRate, tt.want.SkipRate) || !near(got.Listen, tt.want.Listen) {
				t.Fatalf("got memory %v, rate %v, listen %v; want %v, %v, %v",
					got.SkipMemory, got.SkipRate, got.Listen, tt.want.SkipMemory, tt.want.SkipRate, tt.want.Listen)
			}
		})
	}
}

func TestWeightIsTheProduct(t *testing.T) {
	t.Parallel()
	u := unitOf(t, Track{
		ID: 1, Stars: MaxStars, Hearted: true,
		History: History{LastPlayed: now.Add(-7 * day), EarlySkips: 1, ShuffleStarts: 2, ShuffleSkips: 1, Listens: 2, Heard: 1.5},
	})
	s := Settings{Ratings: 1, Memory: true}
	r := Explain(u, s, now)
	want := 2.4 * 1.5625 * math.Pow(0.5, 0.7) * 0.5 * (1 - 0.6/4) * (0.6 + 0.4*3.5/4)
	if got := Weight(u, s, now); !near(got, want) || !near(r.Weight(), want) {
		t.Fatalf("Weight: got %v, want %v", got, want)
	}
	if got, want := r.String(), "5★ ×2.40, heart ×1.56, played 7d ago ×0.62, early skips ×0.50, skip rate ×0.85, listen time ×0.95"; got != want {
		t.Errorf("String:\n got %q\nwant %q", got, want)
	}
}

func TestSettingsAreClamped(t *testing.T) {
	t.Parallel()
	u := unitOf(t, Track{ID: 1, Stars: MaxStars})
	tests := []struct {
		ratings, want float64
	}{{-1, 1}, {2, 2.4}, {math.NaN(), math.Sqrt(2.4)}}
	for _, tt := range tests {
		if got := Explain(u, Settings{Ratings: tt.ratings}, now).Rating; !near(got, tt.want) {
			t.Errorf("Ratings %v: got %v, want %v", tt.ratings, got, tt.want)
		}
	}
}
