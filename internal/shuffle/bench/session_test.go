package bench

import (
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/shuffle"
)

// songLength is every generated song's length.
const songLength = 4 * time.Minute

// listener decides how a person listens to a song: the share of it heard,
// and whether they moved on to the next.
type listener interface {
	listen(id shuffle.TrackID) (heard float64, skipped bool)
}

// neutral hears every song through.
type neutral struct{}

func (neutral) listen(shuffle.TrackID) (float64, bool) { return 1, false }

// skipper skips the songs it dislikes within seconds, while active.
type skipper struct {
	dislikes map[shuffle.TrackID]bool
	active   bool
}

func (s *skipper) listen(id shuffle.TrackID) (float64, bool) {
	if s.active && s.dislikes[id] {
		return 0.05, true // 12 seconds: an early skip
	}
	return 1, false
}

// halfListener moves on halfway through the songs in its set.
type halfListener struct{ set map[shuffle.TrackID]bool }

func (h halfListener) listen(id shuffle.TrackID) (float64, bool) {
	if h.set[id] {
		return 0.5, true
	}
	return 1, false
}

// session is one person's endless shuffle over a library, with their
// listening fed back into the songs' history between pages, as the store
// will: a page a day.
type session struct {
	t      testing.TB
	lib    *library
	tracks []shuffle.Track // the library's, with this person's history
	early  map[shuffle.TrackID][]time.Time
	c      shuffle.Config
	e      *shuffle.Endless
	who    listener
	pool   *shuffle.Pool // the last page's
	pages  int           // pages a day; 0 is one
}

// newSession starts a session over lib at now.
func newSession(tb testing.TB, lib *library, s shuffle.Settings, sim shuffle.Similarity, who listener, seed uint64) *session {
	tb.Helper()
	return &session{
		t: tb, lib: lib, tracks: slices.Clone(lib.tracks),
		early: map[shuffle.TrackID][]time.Time{},
		c:     shuffle.Config{Settings: s, Similarity: sim, Now: now},
		e:     shuffle.NewEndless(seed, 0), who: who,
	}
}

// play listens to picks units, page by page, and returns them.
func (s *session) play(picks int) []shuffle.Pick {
	var out []shuffle.Pick
	for len(out) < picks {
		s.remember()
		pool, err := shuffle.NewPool(s.tracks, s.lib.runs)
		if err != nil {
			s.t.Fatalf("NewPool: %v", err)
		}
		s.pool = pool
		page := s.e.Next(pool, s.c, min(shuffle.PageSize, picks-len(out)))
		for i := range page {
			for _, id := range page[i].Unit.Tracks() {
				s.listen(id)
			}
		}
		out = append(out, page...)
		s.c.Now = s.c.Now.Add(day / time.Duration(max(1, s.pages))) // a page a day, or as many as asked
	}
	return out
}

// listen plays song id to the listener and records what happened.
func (s *session) listen(id shuffle.TrackID) {
	heard, skipped := s.who.listen(id)
	h := &s.tracks[id-1].History
	h.ShuffleStarts++
	h.Listens++
	h.Heard += heard
	if skipped {
		h.ShuffleSkips++
		if time.Duration(heard*float64(songLength)) < shuffle.EarlySkip {
			s.early[id] = append(s.early[id], s.c.Now)
		}
	}
	h.LastPlayed = s.c.Now // a skip counts: the song came up
	s.c.Now = s.c.Now.Add(time.Duration(heard * float64(songLength)))
}

// remember counts each song's early skips within shuffle.SkipMemoryFor of
// now, as the store's query will.
func (s *session) remember() {
	for id, times := range s.early {
		times = slices.DeleteFunc(times, func(at time.Time) bool { return s.c.Now.Sub(at) >= shuffle.SkipMemoryFor })
		s.early[id] = times
		s.tracks[id-1].History.EarlySkips = len(times)
	}
}

// reasons explains the units of the last page's pool that hold one of ids
// and have come up in shuffle.
func (s *session) reasons(ids map[shuffle.TrackID]bool) []shuffle.Reason {
	var out []shuffle.Reason
	units := s.pool.Units()
	for i := range units {
		if slices.ContainsFunc(units[i].Tracks(), func(id shuffle.TrackID) bool {
			return ids[id] && s.tracks[id-1].History.ShuffleStarts > 0
		}) {
			out = append(out, shuffle.Explain(&units[i], s.c.Settings, s.c.Now))
		}
	}
	return out
}
