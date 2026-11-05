package server

import (
	"errors"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
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
