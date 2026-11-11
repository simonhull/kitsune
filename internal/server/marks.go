package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/rating"
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
	s.changeHeart(w, r, s.library.Heart, true)
}

// deleteHeart takes the caller's heart off an item.
func (s *Server) deleteHeart(w http.ResponseWriter, r *http.Request) {
	s.changeHeart(w, r, s.library.Unheart, false)
}

// changeHeart applies change, which leaves the item hearted or not, to the item
// the path names, for the caller, and tells the caller's devices.
func (s *Server) changeHeart(w http.ResponseWriter, r *http.Request, change heartChange, hearted bool) {
	name := r.PathValue("kind")
	kind, ok := heartKinds[name]
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, "kind must be tracks, albums or artists")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	id := r.PathValue("id")
	err := change(r.Context(), user.ID, kind, id)
	if err == nil {
		if kind == store.KindTrack {
			s.lastfm.Poke() // A love or unlove may be waiting.
		}
		s.publishMarks(user.ID, &api.EventMarks{Kind: name, ID: id, Hearted: &hearted})
	}
	s.answerMark(w, r, err, "no such "+kind.String(), "change heart")
}

// getListening counts the caller's plays, hearts and rated tracks.
func (s *Server) getListening(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	n, err := s.library.CountListening(r.Context(), user.ID)
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "count listening", err)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &api.Listening{Plays: n.Plays, Hearts: n.Hearts, Rated: n.Rated})
}

// putRating gives a track the caller's stars, ½ to 5 in halves.
func (s *Server) putRating(w http.ResponseWriter, r *http.Request) {
	var req api.Rating
	if err := readJSON(w, r, &req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, badRating)
		return
	}
	rated, err := rating.FromStars(req.Stars)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, badRating)
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	id := r.PathValue("id")
	err = s.library.Rate(r.Context(), user.ID, id, rated)
	if err == nil {
		s.publishMarks(user.ID, &api.EventMarks{Kind: trackKind, ID: id, Stars: new(rated.Stars())})
	}
	s.answerMark(w, r, err, "no such track", "rate")
}

// deleteRating clears the caller's rating of a track.
func (s *Server) deleteRating(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	id := r.PathValue("id")
	err := s.library.Unrate(r.Context(), user.ID, id)
	if err == nil {
		s.publishMarks(user.ID, &api.EventMarks{Kind: trackKind, ID: id, Stars: new(rating.None.Stars())})
	}
	s.answerMark(w, r, err, "no such track", "unrate")
}

// badRating is putRating's 400 detail.
const badRating = "request body must be a JSON rating: stars, from ½ to 5 in halves"

// trackKind is the contract's name for tracks, in a marks event.
const trackKind = "tracks"

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
