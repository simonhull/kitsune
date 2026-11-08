package shuffler

import (
	"math"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/shuffle"
	"github.com/simonhull/kitsune/internal/store"
)

// sittings plays artists one after another, a minute apart, with a long gap
// before each new sitting.
func sittings(groups ...[]int64) []store.Sitting {
	var out []store.Sitting
	at := time.Unix(0, 0)
	for _, group := range groups {
		at = at.Add(2 * time.Hour)
		for _, artist := range group {
			at = at.Add(time.Minute)
			out = append(out, store.Sitting{At: at, Artist: artist})
		}
	}
	return out
}

func TestCoListeningCountsSittings(t *testing.T) {
	t.Parallel()
	co := countCoListening(sittings([]int64{1, 2, 1}, []int64{1, 2}, []int64{1, 3}, []int64{2, 3}, []int64{3}))
	if co.sittings[1] != 3 || co.sittings[2] != 3 || co.sittings[3] != 3 {
		t.Errorf("sittings = %v, want 3 each: one artist twice in a sitting is once", co.sittings)
	}
	if co.pairs[pair(2, 1)] != 2 || co.pairs[pair(1, 3)] != 1 {
		t.Errorf("pairs = %v", co.pairs)
	}
	if got, ok := co.score(1, 2); !ok || math.Abs(got-2.0/3) > 1e-9 {
		t.Errorf("score(1, 2) = %v, %v, want 2/3", got, ok)
	}
	if _, ok := co.score(1, 4); ok {
		t.Error("an artist never heard has a score")
	}
}

func TestCoListeningNeedsSittings(t *testing.T) {
	t.Parallel()
	if co := countCoListening(sittings([]int64{1, 2}, []int64{1, 2}, []int64{1, 2})); !co.known() {
		t.Error("three sittings of two artists know nothing")
	}
	if few := countCoListening(sittings([]int64{1, 2}, []int64{1, 2})); few.known() {
		t.Error("two sittings are taken for knowledge")
	}
}

func TestSimilarityUsesWhatIsKnown(t *testing.T) {
	t.Parallel()
	co := countCoListening(sittings([]int64{1, 2}, []int64{1, 2}, []int64{1, 2}, []int64{3}, []int64{3}, []int64{3}))
	sim := &similarity{
		genres: map[shuffle.ArtistID]map[string]bool{
			1: {"Dream pop": true, "Shoegaze": true}, 2: {"Shoegaze": true}, 4: {"Jazz": true},
		},
		co: &co,
	}
	for _, tt := range []struct {
		a, b shuffle.ArtistID
		want float64
	}{
		{1, 2, (0.5 + 1) / 2}, // genres half alike, always heard together
		{1, 3, 0},             // no genres for 3; heard, never with 1
		{1, 4, 0},             // genres apart, 4 never heard
		{2, 5, math.NaN()},    // nothing known of 5
	} {
		got := sim.Artist(tt.a, tt.b)
		if math.IsNaN(tt.want) != math.IsNaN(got) || (!math.IsNaN(got) && math.Abs(got-tt.want) > 1e-9) {
			t.Errorf("Artist(%d, %d) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestHistoryHeardNeverPassesListens(t *testing.T) {
	t.Parallel()
	h := &history{songs: map[int64]store.SongListening{7: {Listens: 2, Heard: 2.0000001, Starts: 1, Skips: 1}}}
	got := h.of(7)
	if got.Heard != 2 || got.ShuffleStarts != 1 || got.ShuffleSkips != 1 {
		t.Errorf("of(7) = %+v", got)
	}
	if (h.of(8) != shuffle.History{}) {
		t.Error("a song never heard has a history")
	}
}

func TestGivenFrom(t *testing.T) {
	t.Parallel()
	st := &state{}
	st.remember(given{From: 0, To: 3, Picks: []givenPick{{At: 0}, {At: 1}, {At: 2}}})
	st.remember(given{From: 3, To: 6, Picks: []givenPick{{At: 3}, {At: 5}}}) // 4 had gone
	for _, tt := range []struct {
		after    int64
		ok       bool
		from, at int64
	}{{0, true, 0, 0}, {2, true, 2, 2}, {4, true, 4, 5}, {6, false, 0, 0}} {
		g, ok := st.givenFrom(tt.after)
		if ok != tt.ok || (ok && (g.From != tt.from || g.Picks[0].At != tt.at)) {
			t.Errorf("givenFrom(%d) = %+v, %v", tt.after, g, ok)
		}
	}
	for range keptPages {
		st.remember(given{From: 9, To: 10})
	}
	if len(st.Given) != keptPages {
		t.Errorf("kept %d pages, want %d", len(st.Given), keptPages)
	}
}
