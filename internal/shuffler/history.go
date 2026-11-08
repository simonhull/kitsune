package shuffler

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/shuffle"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// historyFor is how long a person's history is kept before it is read
	// again, so the skip memory's week moves on. Their own listens and plays
	// drop it at once ([Service.Forget]).
	historyFor = 10 * time.Minute
	// sittingGap is how long between two listens starts a new sitting.
	sittingGap = 45 * time.Minute
	// coListenFor is how far back co-listening looks.
	coListenFor = 180 * 24 * time.Hour
	// leastSittings is how many sittings an artist must be in before
	// co-listening says anything about it: one or two say nothing yet.
	leastSittings = 3
)

// history is what a person's listening says, kept between their shuffles.
type history struct {
	read  time.Time
	songs map[int64]store.SongListening
	co    coListening
}

// readHistory reads the user's listening, as at now.
func readHistory(ctx context.Context, st *store.Store, userID int64, now time.Time) (*history, error) {
	songs, err := st.ShuffleListening(ctx, userID, shuffle.EarlySkip, now.Add(-shuffle.SkipMemoryFor))
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	sittings, err := st.ShuffleSittings(ctx, userID, now.Add(-coListenFor))
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	return &history{read: now, songs: songs, co: countCoListening(sittings)}, nil
}

// of is a song's history, as the engine takes it.
func (h *history) of(id int64) shuffle.History {
	l, ok := h.songs[id]
	if !ok {
		return shuffle.History{}
	}
	return shuffle.History{
		LastPlayed: l.LastAt, EarlySkips: l.EarlySkips,
		ShuffleStarts: l.Starts, ShuffleSkips: l.Skips,
		Listens: l.Listens, Heard: min(l.Heard, float64(l.Listens)),
	}
}

// coListening counts which artists a person plays in the same sitting: a run
// of listens they chose, with no long gap between them.
type coListening struct {
	sittings map[int64]int    // by artist: the sittings it is in
	pairs    map[[2]int64]int // by pair, the lower ID first: the sittings with both
}

// countCoListening counts sittings, oldest first.
func countCoListening(sittings []store.Sitting) coListening {
	co := coListening{sittings: make(map[int64]int), pairs: make(map[[2]int64]int)}
	var in []int64 // the artists of the sitting under way
	var last time.Time
	flush := func() {
		for i, a := range in {
			co.sittings[a]++
			for _, b := range in[i+1:] {
				co.pairs[pair(a, b)]++
			}
		}
		in = in[:0]
	}
	for _, s := range sittings {
		if s.At.Sub(last) > sittingGap {
			flush()
		}
		last = s.At
		if !slices.Contains(in, s.Artist) {
			in = append(in, s.Artist)
		}
	}
	flush()
	return co
}

func pair(a, b int64) [2]int64 {
	if b < a {
		a, b = b, a
	}
	return [2]int64{a, b}
}

// score is how often a and b share a sitting, from 0 to 1 (a cosine of their
// sittings), and whether it is known: both must be in enough sittings.
func (co *coListening) score(a, b int64) (float64, bool) {
	sa, sb := co.sittings[a], co.sittings[b]
	if sa < leastSittings || sb < leastSittings {
		return 0, false
	}
	return float64(co.pairs[pair(a, b)]) / math.Sqrt(float64(sa)*float64(sb)), true
}

// known reports whether co-listening says anything about anyone.
func (co *coListening) known() bool {
	n := 0
	for _, s := range co.sittings {
		if s >= leastSittings {
			n++
		}
	}
	return n >= 2 //nolint:mnd // A pair takes two.
}

// similarity scores two artists for flow: the mean of what is known of them,
// their genres' overlap and how often the person plays them together. With
// nothing known it says so, and flow leaves the pick alone.
type similarity struct {
	genres map[shuffle.ArtistID]map[string]bool
	co     *coListening
}

// Artist implements [shuffle.Similarity]. NaN means nothing is known.
func (s *similarity) Artist(a, b shuffle.ArtistID) float64 {
	sum, n := 0.0, 0
	if ga, gb := s.genres[a], s.genres[b]; len(ga) > 0 && len(gb) > 0 {
		sum += jaccard(ga, gb)
		n++
	}
	if c, ok := s.co.score(int64(a), int64(b)); ok {
		sum += c
		n++
	}
	if n == 0 {
		return math.NaN()
	}
	return sum / float64(n)
}

// jaccard is the share of the genres either has that both have.
func jaccard(a, b map[string]bool) float64 {
	both := 0
	for g := range a {
		if b[g] {
			both++
		}
	}
	return float64(both) / float64(len(a)+len(b)-both)
}
