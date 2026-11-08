package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

// postListen records the start or the end of one of the caller's listens.
// Nothing in the body says whose.
func (s *Server) postListen(w http.ResponseWriter, r *http.Request) {
	var req api.ListenReport
	if err := readJSON(w, r, &req); err != nil || !validListen(&req) {
		writeProblem(w, r, http.StatusBadRequest,
			"request body must be a JSON listen: clientId, trackId and startedAt, with how it ended once it has")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	listen := store.NewListen{
		UserID: user.ID, TrackID: req.TrackID, ClientID: req.ClientID, Shuffle: req.Shuffle,
		StartedAt: req.StartedAt,
		Listened:  time.Duration(req.ListenedMs) * time.Millisecond,
		Ended:     store.Ending(req.Ended),
	}
	if now := s.now(); listen.StartedAt.After(now) {
		listen.StartedAt = now
	}
	switch err := s.library.RecordListen(r.Context(), &listen); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "no such track")
	case r.Context().Err() == nil: // A client that left keeps the listen until it hears a 204.
		s.internalError(w, r, "record listen", err)
	}
}

func validListen(req *api.ListenReport) bool {
	return req.TrackID != "" && !req.StartedAt.IsZero() && validPlayField(req.ClientID) &&
		len(req.Shuffle) <= maxPlayField && req.ListenedMs >= 0 && store.Ending(req.Ended).Valid()
}
