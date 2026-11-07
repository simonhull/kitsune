package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

// heartKinds are the contract's names for what a heart can be on.
var heartKinds = map[string]store.Kind{ //nolint:gochecknoglobals // A read-only table of the contract's enum.
	"tracks":  store.KindTrack,
	"albums":  store.KindAlbum,
	"artists": store.KindArtist,
}

// heartChange is [store.Store.Heart] or [store.Store.Unheart].
type heartChange func(ctx context.Context, userID int64, kind store.Kind, id string) error

// putHeart hearts an item for the caller. Nothing in the request says whose.
func (s *Server) putHeart(w http.ResponseWriter, r *http.Request) {
	s.changeHeart(w, r, s.library.Heart)
}

// deleteHeart takes the caller's heart off an item.
func (s *Server) deleteHeart(w http.ResponseWriter, r *http.Request) {
	s.changeHeart(w, r, s.library.Unheart)
}

// changeHeart applies change to the item the path names, for the caller.
func (s *Server) changeHeart(w http.ResponseWriter, r *http.Request, change heartChange) {
	kind, ok := heartKinds[r.PathValue("kind")]
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, "kind must be tracks, albums or artists")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	err := change(r.Context(), user.ID, kind, r.PathValue("id"))
	if err == nil && kind == store.KindTrack {
		s.lastfm.Poke() // A love or unlove may be waiting.
	}
	s.answerMark(w, r, err, "no such "+kind.String(), "change heart")
}

// putRating gives a track the caller's stars.
func (s *Server) putRating(w http.ResponseWriter, r *http.Request) {
	var req api.Rating
	if err := readJSON(w, r, &req); err != nil || req.Stars < store.MinStars || req.Stars > store.MaxStars {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a JSON rating: stars, from 1 to 5")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	err := s.library.Rate(r.Context(), user.ID, r.PathValue("id"), req.Stars)
	s.answerMark(w, r, err, "no such track", "rate")
}

// deleteRating clears the caller's rating of a track.
func (s *Server) deleteRating(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	err := s.library.Unrate(r.Context(), user.ID, r.PathValue("id"))
	s.answerMark(w, r, err, "no such track", "unrate")
}

// answerMark answers a change to a mark. A client that went away hears nothing:
// every change can be sent again, and it will.
func (s *Server) answerMark(w http.ResponseWriter, r *http.Request, err error, missing, what string) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, missing)
	case r.Context().Err() == nil:
		s.internalError(w, r, what, err)
	}
}
