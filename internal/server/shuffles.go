package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/shuffler"
	"github.com/simonhull/kitsune/internal/store"
)

// postShuffle starts one of the caller's shuffles.
func (s *Server) postShuffle(w http.ResponseWriter, r *http.Request) {
	var req api.ShuffleStart
	if err := readJSON(w, r, &req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a JSON shuffle: scope and mode")
		return
	}
	id, err := library.NewID(s.now())
	if err != nil {
		s.internalError(w, r, "new shuffle ID", err)
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	page, err := s.shuffles.Start(r.Context(), user.ID, &shuffler.Start{
		ID: id, Mode: shuffler.Mode(req.Mode), First: req.First, Seed: req.Seed,
		Scope: shuffler.Scope{Kind: shuffler.Kind(req.Scope.Kind), ID: req.Scope.ID, Tracks: req.Scope.Tracks},
	})
	if s.shuffleFailed(w, r, err) {
		return
	}
	s.publish(events.Shuffle, events.User(user.ID), &api.EventShuffle{ID: page.Session, To: page.To})
	writeBody(w, http.StatusCreated, api.JSONMediaType, new(toAPIShufflePage(&page)))
}

// getShuffleNext answers one of the caller's shuffles after a position.
func (s *Server) getShuffleNext(w http.ResponseWriter, r *http.Request) {
	after, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if err != nil || after < 0 {
		writeProblem(w, r, http.StatusBadRequest, "after must be a position, from 0")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	page, err := s.shuffles.Next(r.Context(), user.ID, r.PathValue("id"), after)
	if s.shuffleFailed(w, r, err) {
		return
	}
	if page.Moved {
		s.publish(events.Shuffle, events.User(user.ID), &api.EventShuffle{ID: page.Session, To: page.To})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, new(toAPIShufflePage(&page)))
}

// shuffleFailed writes the problem for err, if there is one, and reports
// whether it did.
func (s *Server) shuffleFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, shuffler.ErrBadScope):
		writeProblem(w, r, http.StatusBadRequest, "scope must be library, hearted or rated alone, an album or artist with its id, or 1 to 10000 tracks; mode finite or endless")
	case errors.Is(err, shuffler.ErrBadAfter):
		writeProblem(w, r, http.StatusBadRequest, "the shuffle hasn't got that far")
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "no such shuffle, album or artist")
	case r.Context().Err() == nil:
		s.internalError(w, r, "shuffle", err)
	}
	return true
}

func toAPIShufflePage(p *shuffler.Page) api.ShufflePage {
	out := api.ShufflePage{
		ID: p.Session, Mode: string(p.Mode), From: p.From, To: p.To, Songs: p.Songs, Done: p.Done,
		Picks: make([]api.ShufflePick, len(p.Picks)),
	}
	for i := range p.Picks {
		pick := &p.Picks[i]
		songs := make([]api.TrackHit, len(pick.Songs))
		for j := range pick.Songs {
			songs[j] = toAPITrackHit(&pick.Songs[j])
		}
		out.Picks[i] = api.ShufflePick{Songs: songs, Why: toAPIWhy(&pick.Why, pick.After)}
	}
	return out
}

func toAPIWhy(w *shuffler.Why, after *store.ArtistLink) api.ShuffleWhy {
	out := api.ShuffleWhy{
		Stars: w.Stars, Rating: w.Rating, Hearted: w.Hearted, Heart: w.Heart,
		Played: w.Played, SinceDays: w.SinceDays, Recency: w.Recency, Memory: w.Memory,
		EarlySkips: w.EarlySkips, SkipMemory: w.SkipMemory, Starts: w.Starts, Skips: w.Skips, SkipRate: w.SkipRate,
		Listens: w.Listens, Heard: w.Heard, Listen: w.Listen, Flow: w.Flow,
	}
	if after != nil {
		out.After = &api.ArtistLink{ID: after.ID, Name: after.Name}
		out.Similarity = w.Similarity
	}
	return out
}
