package server

import (
	"errors"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

// maxPlayField is the longest source or client ID the API takes, in bytes.
const maxPlayField = 64

// postPlay records one of the caller's plays. Nothing in the body says whose.
func (s *Server) postPlay(w http.ResponseWriter, r *http.Request) {
	var req api.PlayReport
	if err := readJSON(w, r, &req); err != nil || !validPlay(&req) {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a JSON play: trackId, playedAt, source and clientId")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	play := store.NewPlay{UserID: user.ID, TrackID: req.TrackID, PlayedAt: req.PlayedAt, Source: req.Source, ClientID: req.ClientID}
	if now := s.now(); play.PlayedAt.After(now) {
		play.PlayedAt = now
	}
	switch err := s.library.RecordPlay(r.Context(), &play); {
	case err == nil:
		s.lastfm.Poke()
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "no such track")
	case r.Context().Err() == nil: // A client that left keeps the play until it hears a 204.
		s.internalError(w, r, "record play", err)
	}
}

func validPlay(req *api.PlayReport) bool {
	return req.TrackID != "" && !req.PlayedAt.IsZero() && validPlayField(req.Source) && validPlayField(req.ClientID)
}

func validPlayField(v string) bool { return v != "" && len(v) <= maxPlayField }
