package server

import (
	"context"
	"encoding/json/v2"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// maxEditBody caps the request body of an edit or a scan.
	maxEditBody = 64 << 10
	// retryAfterSeconds is the Retry-After of an edit the library can't take now.
	retryAfterSeconds = "30"
	// The numbers a track or disc tag can hold in every format: M4A's are 16 bits.
	minNumber, maxNumber = 1, 65535
	// The years the API takes.
	minYear, maxYear = 1000, 9999
)

// patchTrack edits a track's title and numbers.
func (s *Server) patchTrack(w http.ResponseWriter, r *http.Request) {
	var req api.TrackEdit
	ifMatch, ok := s.readEdit(w, r, &req)
	if !ok {
		return
	}
	if !validTrackEdit(&req) {
		writeProblem(w, r, http.StatusBadRequest, "name at least one field; numbers run from 1 to 65535")
		return
	}
	id := r.PathValue("id")
	edit := library.Edit{Title: req.Title, TrackNo: req.TrackNo, DiscNo: req.DiscNo}
	//nolint:contextcheck // An edit outlives its request on purpose; see Config.Context.
	if err := s.scanner.EditTrack(s.editContext, id, ifMatch, &edit); err != nil {
		s.editFailed(w, r, err)
		return
	}
	s.writeTrack(w, afterEdit(r), id)
}

// patchAlbum edits the year and genres of every track of an album.
func (s *Server) patchAlbum(w http.ResponseWriter, r *http.Request) {
	var req api.AlbumEdit
	ifMatch, ok := s.readEdit(w, r, &req)
	if !ok {
		return
	}
	edit, ok := albumEdit(&req)
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, "name at least one field; a year runs from 1000 to 9999 and a genre is not blank")
		return
	}
	id := r.PathValue("id")
	//nolint:contextcheck // An edit outlives its request on purpose; see Config.Context.
	if err := s.scanner.EditAlbum(s.editContext, id, ifMatch, &edit); err != nil {
		s.editFailed(w, r, err)
		return
	}
	s.writeAlbum(w, afterEdit(r), id)
}

// afterEdit keeps r's values but not its cancellation: an edit that outlived its
// client ends with a read, and that read is not a failure worth logging.
func afterEdit(r *http.Request) *http.Request {
	return r.WithContext(context.WithoutCancel(r.Context()))
}

// readEdit checks that tag writes are on and reads If-Match and the JSON body
// into req. It writes the problem and reports false when it can't go on. Unknown
// members are refused, so a field this server doesn't edit never looks saved.
func (s *Server) readEdit(w http.ResponseWriter, r *http.Request, req any) (string, bool) {
	if !s.tagWrites {
		writeProblem(w, r, http.StatusForbidden, "tag writes are off: library.tag_writes")
		return "", false
	}
	ifMatch := r.Header.Get("If-Match")
	if ifMatch == "" || ifMatch == "*" {
		writeProblem(w, r, http.StatusPreconditionRequired, "send the etag you read as If-Match")
		return "", false
	}
	if err := readJSON(w, r, req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a JSON edit of the fields this route takes")
		return "", false
	}
	return ifMatch, true
}

// readJSON reads a JSON body into req, refusing other media types and unknown
// members.
func readJSON(w http.ResponseWriter, r *http.Request, req any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != api.JSONMediaType {
		return errors.ErrUnsupported
	}
	return json.UnmarshalRead(http.MaxBytesReader(w, r.Body, maxEditBody), req, json.RejectUnknownMembers(true)) //nolint:wrapcheck // The caller only tells the client the body is bad.
}

func validNumber(n *int) bool { return n == nil || (*n >= minNumber && *n <= maxNumber) }

func validTrackEdit(req *api.TrackEdit) bool {
	named := req.Title != nil || req.TrackNo != nil || req.DiscNo != nil
	return named && validNumber(req.TrackNo) && validNumber(req.DiscNo)
}

// albumEdit returns the edit req asks for, with its genres trimmed. It reports
// false for a request that names nothing or holds an invalid value.
func albumEdit(req *api.AlbumEdit) (library.Edit, bool) {
	if req.Year == nil && req.Genres == nil {
		return library.Edit{}, false
	}
	if req.Year != nil && (*req.Year < minYear || *req.Year > maxYear) {
		return library.Edit{}, false
	}
	edit := library.Edit{Year: req.Year}
	if req.Genres != nil {
		edit.Genres = make([]string, len(req.Genres))
	}
	for i, g := range req.Genres {
		if edit.Genres[i] = strings.TrimSpace(g); edit.Genres[i] == "" {
			return library.Edit{}, false
		}
	}
	return edit, true
}

// editFailed answers an edit that did not finish.
func (s *Server) editFailed(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, library.ErrBusy):
		w.Header().Set("Retry-After", retryAfterSeconds)
		writeProblem(w, r, http.StatusConflict, "a scan or another edit is running")
		return
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "no such track or album")
		return
	case errors.Is(err, library.ErrStale):
		writeProblem(w, r, http.StatusPreconditionFailed, "the files changed since you read them; read them again")
		return
	}
	stopped, ok := errors.AsType[*library.EditError](err)
	if !ok {
		s.internalError(w, r, "edit tags", err)
		return
	}
	status, detail := editStatus(stopped.Err), "internal error" // An unexpected cause stays in the log.
	if status != http.StatusInternalServerError {
		detail = stopped.Err.Error()
	}
	s.log.ErrorContext(r.Context(), "edit tags", "requestId", requestID(r.Context()), "err", err)
	body := api.EditProblem{Problem: newProblem(r, status, detail), Changed: stopped.Changed, Remaining: stopped.Remaining}
	writeBody(w, status, api.ProblemMediaType, &body)
}

// editStatus maps the cause of a stopped edit to its status.
func editStatus(err error) int {
	switch {
	case errors.Is(err, library.ErrUnsupportedEdit):
		return http.StatusUnprocessableEntity
	case errors.Is(err, library.ErrNotWritable):
		return http.StatusForbidden
	}
	return http.StatusInternalServerError
}
