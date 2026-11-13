package server

import (
	"encoding/json/v2"
	"errors"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store"
)

// getListening counts the caller's plays and rated tracks.
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
	writeBody(w, http.StatusOK, api.JSONMediaType, &api.Listening{Plays: n.Plays, Rated: n.Rated})
}

// putRating gives a track the caller's stars, ½ to 5 in halves. A rating that
// crosses the caller's love line queues a Last.fm love or unlove.
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
		s.lastfm.Poke() // A love or unlove may be waiting.
		s.publishMarks(user.ID, &api.EventMarks{Kind: trackKind, ID: id, Stars: rated.Stars()})
	}
	s.answerMark(w, r, err, "no such track", "rate")
}

// deleteRating clears the caller's rating of a track.
func (s *Server) deleteRating(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	id := r.PathValue("id")
	err := s.library.Unrate(r.Context(), user.ID, id)
	if err == nil {
		s.lastfm.Poke() // An unlove may be waiting.
		s.publishMarks(user.ID, &api.EventMarks{Kind: trackKind, ID: id, Stars: rating.None.Stars()})
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

// getHeartsSummary says what the caller kept of their hearts, for the You
// page's one-time card. All zeros means there is nothing left to turn into
// stars.
func (s *Server) getHeartsSummary(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	h, err := s.library.HeartsSummary(r.Context(), user.ID)
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "hearts summary", err)
		}
		return
	}
	out := api.HeartsSummary{
		TracksRated: h.Rated(), TracksUnrated: h.Unrated, Albums: h.Albums, Artists: h.Artists,
		ByStars: h.ByStars[:], Preview: make([]api.HeartedSong, len(h.Preview)),
	}
	for i, p := range h.Preview {
		out.Preview[i] = api.HeartedSong{ID: p.ID, Title: p.Title, Artist: p.Artist}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &out)
}

// badConvert is convertHearts' 400 detail.
const badConvert = "request body must be {\"stars\": ½ to 5 in halves, or null}"

// convertHearts turns the caller's hearts into stars, once: every hearted song
// without stars gets the body's, or stays unrated for null, and every heart
// goes. Nothing reaches Last.fm, since those songs were loved through their
// hearts. The caller's devices hear of each song rated.
func (s *Server) convertHearts(w http.ResponseWriter, r *http.Request) {
	var req api.HeartsConvert
	stars, err := rating.None, readJSON(w, r, &req)
	switch {
	case err != nil || len(req.Stars) == 0:
		err = errors.ErrUnsupported
	case req.Stars.Kind() != 'n': // null leaves them unrated.
		var n float64
		if err = json.Unmarshal(req.Stars, &n); err == nil {
			stars, err = rating.FromStars(n)
		}
	}
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, badConvert)
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	done, err := s.library.ConvertHearts(r.Context(), user.ID, stars)
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "convert hearts", err)
		}
		return
	}
	for _, id := range done.Rated {
		s.publishMarks(user.ID, &api.EventMarks{Kind: trackKind, ID: id, Stars: stars.Stars()})
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &api.HeartsConverted{
		Rated: len(done.Rated), Tracks: done.Tracks, Albums: done.Albums, Artists: done.Artists,
	})
}
