package server

import (
	"errors"
	"io/fs"
	"net/http"

	"github.com/simonhull/kitsune/internal/store"
	"github.com/simonhull/kitsune/internal/stream"
)

// getTrackStream serves a track's original file.
func (s *Server) getTrackStream(w http.ResponseWriter, r *http.Request) {
	file, err := s.library.TrackFile(r.Context(), r.PathValue("id"))
	if err != nil {
		s.trackLookupFailed(w, r, err)
		return
	}
	if err = stream.Serve(w, r, file); err != nil {
		s.trackLookupFailed(w, r, err)
	}
}

// trackLookupFailed answers a track that is not there, which includes one whose
// file left the disk since the last scan, or an error that is the server's.
func (s *Server) trackLookupFailed(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		writeProblem(w, r, http.StatusNotFound, "no such track")
		return
	}
	s.internalError(w, r, "stream track", err)
}
