package server

import (
	"encoding/json/jsontext"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/prefs"
	"github.com/simonhull/kitsune/internal/store"
)

// getPref answers the caller's value for a preference. One the caller never set
// is a 404, and so is one that no longer passes its key's rule: the client
// starts again from its defaults.
func (s *Server) getPref(w http.ResponseWriter, r *http.Request) {
	key, err := prefs.Lookup(r.PathValue("key"))
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "no such preference")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	stored, err := s.library.Pref(r.Context(), user.ID, key.Name())
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "not set")
		return
	case err != nil:
		if r.Context().Err() == nil {
			s.internalError(w, r, "read pref", err)
		}
		return
	}
	value, err := key.Check(stored)
	if err != nil {
		s.log.WarnContext(r.Context(), "stored preference fails its rule",
			"requestId", requestID(r.Context()), "key", key.Name(), "err", err)
		writeProblem(w, r, http.StatusNotFound, "not set")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, jsontext.Value(value))
}

// putPref replaces the caller's value for a preference.
func (s *Server) putPref(w http.ResponseWriter, r *http.Request) {
	key, err := prefs.Lookup(r.PathValue("key"))
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "no such preference")
		return
	}
	value, err := readPref(w, r, key)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a JSON value this preference takes")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	if err = s.library.SetPref(r.Context(), user.ID, key.Name(), value); err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "set pref", err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readPref reads a request body of at most [prefs.MaxValue] bytes of JSON, and
// returns it as key keeps it.
func readPref(w http.ResponseWriter, r *http.Request, key prefs.Key) ([]byte, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != api.JSONMediaType {
		return nil, errors.ErrUnsupported
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, prefs.MaxValue))
	if err != nil {
		return nil, err //nolint:wrapcheck // The caller only tells the client the body is bad.
	}
	return key.Check(body) //nolint:wrapcheck // As above.
}
