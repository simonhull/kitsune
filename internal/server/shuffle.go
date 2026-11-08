package server

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

// mostShuffleTracks caps the songs one rules request names, so a long query
// string can't take a read connection for long.
const mostShuffleTracks = 1000

// getShuffleRules answers the caller's shuffle rules touching some songs.
func (s *Server) getShuffleRules(w http.ResponseWriter, r *http.Request) {
	var tracks []string
	for _, value := range r.URL.Query()["tracks"] {
		for id := range strings.SplitSeq(value, ",") {
			if id != "" && !slices.Contains(tracks, id) {
				tracks = append(tracks, id)
			}
		}
	}
	if len(tracks) == 0 || len(tracks) > mostShuffleTracks {
		writeProblem(w, r, http.StatusBadRequest, "name from 1 to 1000 songs in tracks")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	rules, err := s.library.ShuffleRulesOf(r.Context(), user.ID, tracks)
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "read shuffle rules", err)
		}
		return
	}
	body := api.ShuffleRules{Runs: make([]api.ShuffleRun, len(rules.Runs)), Excluded: rules.Excluded}
	for i := range rules.Runs {
		body.Runs[i] = api.ShuffleRun{ID: rules.Runs[i].ID, Tracks: rules.Runs[i].Tracks}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// readShuffleTracks reads a body naming from 1 to [mostShuffleTracks] songs. It
// writes the problem and reports false when it can't.
func readShuffleTracks(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var req api.ShuffleTracks
	if err := readJSON(w, r, &req); err != nil || len(req.Tracks) == 0 || len(req.Tracks) > mostShuffleTracks ||
		slices.Contains(req.Tracks, "") {
		writeProblem(w, r, http.StatusBadRequest, "request body must name from 1 to 1000 songs in tracks")
		return nil, false
	}
	return req.Tracks, true
}

// putShuffleRun combines songs into one of the caller's runs.
func (s *Server) putShuffleRun(w http.ResponseWriter, r *http.Request) {
	tracks, ok := readShuffleTracks(w, r)
	if !ok {
		return
	}
	id, err := library.NewID(s.now())
	if err != nil {
		s.internalError(w, r, "new run ID", err)
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	run, err := s.library.SetRun(r.Context(), user.ID, id, tracks)
	switch {
	case errors.Is(err, store.ErrShortRun):
		writeProblem(w, r, http.StatusBadRequest, "a run takes two or more different songs")
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "no such song")
	case err != nil:
		if r.Context().Err() == nil {
			s.internalError(w, r, "set run", err)
		}
	default:
		writeBody(w, http.StatusOK, api.JSONMediaType, &api.ShuffleRun{ID: run.ID, Tracks: run.Tracks})
	}
}

// deleteShuffleRun undoes one of the caller's runs.
func (s *Server) deleteShuffleRun(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	err := s.library.DeleteRun(r.Context(), user.ID, r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "no such run")
	case err != nil:
		if r.Context().Err() == nil {
			s.internalError(w, r, "delete run", err)
		}
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// putShuffleExcluded leaves songs out of the caller's shuffle.
func (s *Server) putShuffleExcluded(w http.ResponseWriter, r *http.Request) {
	s.setExcluded(w, r, true)
}

// deleteShuffleExcluded lets songs back into the caller's shuffle.
func (s *Server) deleteShuffleExcluded(w http.ResponseWriter, r *http.Request) {
	s.setExcluded(w, r, false)
}

func (s *Server) setExcluded(w http.ResponseWriter, r *http.Request, on bool) {
	tracks, ok := readShuffleTracks(w, r)
	if !ok {
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	err := s.library.SetExcluded(r.Context(), user.ID, tracks, on)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "no such song")
	case err != nil:
		if r.Context().Err() == nil {
			s.internalError(w, r, "set excluded", err)
		}
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
