package server

import (
	"context"
	"encoding/json/v2"
	"errors"
	"mime"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// maxEditBody caps the request body of an edit or a scan. An edit of a large
	// album can repeat lyrics on every track.
	maxEditBody = 1 << 20
	// retryAfterSeconds is the Retry-After of an edit the library can't take now.
	retryAfterSeconds = "30"
)

// getAlbumTags answers with what a tag editor shows of an album.
func (s *Server) getAlbumTags(w http.ResponseWriter, r *http.Request) {
	tracks, etag, err := s.scanner.AlbumTags(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "no such album")
	case errors.Is(err, library.ErrStale):
		writeProblem(w, r, http.StatusConflict, "a file of this album is gone; scan the library")
	case err != nil:
		s.internalError(w, r, "read tags", err)
	default:
		body := api.AlbumTags{Etag: etag, Tracks: make([]api.TrackTags, len(tracks))}
		for i := range tracks {
			body.Tracks[i] = toAPITrackTags(&tracks[i])
		}
		writeBody(w, http.StatusOK, api.JSONMediaType, &body)
	}
}

func toAPITrackTags(t *library.TrackTags) api.TrackTags {
	out := api.TrackTags{
		ID: t.ID, File: t.File,
		Fields: make(map[string][]string, len(t.Fields)),
		Tags:   make([]api.RawTag, len(t.Tags)),
	}
	for name, values := range t.Fields {
		out.Fields[string(name)] = values
	}
	for _, name := range t.ReadOnly {
		out.ReadOnly = append(out.ReadOnly, string(name))
	}
	for i := range t.Tags {
		out.Tags[i] = api.RawTag{Key: t.Tags[i].Key, Values: t.Tags[i].Values}
	}
	return out
}

// postAlbumEdit edits the tags of an album's tracks.
func (s *Server) postAlbumEdit(w http.ResponseWriter, r *http.Request) {
	var req api.AlbumEdit
	ifMatch, ok := s.readEdit(w, r, &req)
	if !ok {
		return
	}
	edit, ok := albumEdit(&req)
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, "name at least one field, and each track once")
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

// albumEdit returns the edit req asks for. What a value may hold is the library's
// to judge, file by file. It reports false for a request that names nothing, or
// names a track twice or not at all, or both sets and clears a field.
func albumEdit(req *api.AlbumEdit) (library.AlbumEdit, bool) {
	all, ok := tagEdit(&req.All)
	if !ok {
		return library.AlbumEdit{}, false
	}
	edit := library.AlbumEdit{All: all, Tracks: make(map[string]library.Edit, len(req.Tracks))}
	named := len(all.Fields) > 0
	for i := range req.Tracks {
		tr := &req.Tracks[i]
		own, valid := tagEdit(&tr.TagEdit)
		if _, twice := edit.Tracks[tr.ID]; !valid || twice || tr.ID == "" {
			return library.AlbumEdit{}, false
		}
		edit.Tracks[tr.ID] = own
		named = named || len(own.Fields) > 0
	}
	return edit, named
}

// tagEdit returns the edit req asks for, reporting false for a field both set and
// cleared.
func tagEdit(req *api.TagEdit) (library.Edit, bool) {
	fields := make(map[library.Field][]string, len(req.Set)+len(req.Clear))
	for name, values := range req.Set {
		fields[library.Field(name)] = values
	}
	for _, name := range req.Clear {
		if _, set := req.Set[name]; set {
			return library.Edit{}, false
		}
		fields[library.Field(name)] = nil
	}
	return library.Edit{Fields: fields}, true
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
