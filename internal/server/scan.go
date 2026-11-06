package server

import (
	"errors"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

// postScan starts a scan and answers once it is claimed or refused. The client
// watches it through GET /library.
func (s *Server) postScan(w http.ResponseWriter, r *http.Request) {
	var req api.ScanRequest
	err := readJSON(w, r, &req)
	mode, ok := library.ParseMode(req.Mode)
	if err != nil || !ok {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a JSON object with a mode of quick or full")
		return
	}
	switch err = s.startScan(mode); {
	case errors.Is(err, library.ErrScanRunning):
		writeProblem(w, r, http.StatusConflict, "a scan is already running")
	case err != nil:
		writeProblem(w, r, http.StatusServiceUnavailable, "the server is stopping")
	default:
		body := toAPIScan(s.scanner.Progress(), true)
		writeBody(w, http.StatusAccepted, api.JSONMediaType, &body)
	}
}

// getIssues lists what scans had trouble with, with no library root in it.
func (s *Server) getIssues(w http.ResponseWriter, r *http.Request) {
	issues, err := s.library.ScanIssues(r.Context())
	if err != nil {
		s.internalError(w, r, "list scan issues", err)
		return
	}
	warned, err := s.library.WarnedTracks(r.Context())
	if err != nil {
		s.internalError(w, r, "list scan issues", err)
		return
	}
	body := api.ScanIssues{Issues: make([]api.ScanIssue, len(issues)), Warnings: make([]api.TrackWarned, len(warned))}
	for i := range issues {
		body.Issues[i] = toAPIScanIssue(&issues[i])
	}
	for i := range warned {
		body.Warnings[i] = toAPITrackWarned(&warned[i])
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

func toAPIScanIssue(is *store.ScanIssue) api.ScanIssue {
	return api.ScanIssue{Path: is.RelPath, Kind: string(is.Kind), Message: library.StripRoot(is.Message, is.Root)}
}

func toAPITrackWarned(t *store.WarnedTrack) api.TrackWarned {
	out := api.TrackWarned{TrackID: t.ID, Path: t.RelPath, Messages: make([]string, len(t.Warnings))}
	for i, w := range t.Warnings {
		out.Messages[i] = library.StripRoot(w, t.Root)
	}
	return out
}
