package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// maxTrashIDs caps the songs or albums one Trash request names.
	maxTrashIDs = 1000
	// trashOffDetail is what the Trash is told while tag writes are off.
	trashOffDetail = "tag writes are off, and with them the Trash: library.tag_writes"
	// namedSongs is how many songs a refusal names before it counts the rest.
	namedSongs = 3
)

// getTrash lists the Trash.
func (s *Server) getTrash(w http.ResponseWriter, r *http.Request) {
	t, err := s.library.Trash(r.Context())
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "list the trash", err)
		}
		return
	}
	body := api.Trash{
		Count: t.Count, Bytes: t.Bytes, RetentionDays: library.TrashRetentionDays,
		Albums: make([]api.TrashAlbum, len(t.Albums)),
	}
	for i := range t.Albums {
		a := &t.Albums[i]
		out := api.TrashAlbum{
			ID: a.ID, Title: a.Title, Artist: a.Artist, Artwork: toAPIArtwork(a.Cover),
			Tracks: make([]api.TrashedTrack, len(a.Tracks)),
		}
		for j := range a.Tracks {
			tr := &a.Tracks[j]
			out.Tracks[j] = api.TrashedTrack{
				ID: tr.ID, Title: tr.Title, Size: tr.Size, TrashedAt: tr.TrashedAt, TrashedBy: tr.TrashedBy,
				PurgeAt: tr.TrashedAt.Add(library.TrashRetention),
			}
		}
		body.Albums[i] = out
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// postTrash moves songs, or whole albums, to the Trash.
func (s *Server) postTrash(w http.ResponseWriter, r *http.Request) {
	var req api.TrashRequest
	if !s.readTrash(w, r, &req) {
		return
	}
	if (len(req.Tracks) == 0) == (len(req.Albums) == 0) || len(req.Tracks) > maxTrashIDs || len(req.Albums) > maxTrashIDs {
		writeProblem(w, r, http.StatusBadRequest, fmt.Sprintf("name songs or albums, not both, and at most %d", maxTrashIDs))
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	var change library.TrashChange
	var err error
	//nolint:contextcheck // A trash outlives its request on purpose, as an edit does; see Config.Context.
	if req.Tracks != nil {
		change, err = s.scanner.TrashTracks(s.editContext, user.ID, req.Tracks)
	} else {
		change, err = s.scanner.TrashAlbums(s.editContext, user.ID, req.Albums)
	}
	if err != nil {
		s.trashFailed(w, r, "trash", err)
		return
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &api.TrashChange{Tracks: change.Tracks, Albums: change.Albums})
}

// postTrashRestore puts songs back from the Trash.
func (s *Server) postTrashRestore(w http.ResponseWriter, r *http.Request) {
	var req api.TrashRestore
	if !s.readTrash(w, r, &req) {
		return
	}
	if len(req.Tracks) == 0 || len(req.Tracks) > maxTrashIDs {
		writeProblem(w, r, http.StatusBadRequest, fmt.Sprintf("name from 1 to %d songs", maxTrashIDs))
		return
	}
	//nolint:contextcheck // As postTrash.
	change, err := s.scanner.RestoreTracks(s.editContext, req.Tracks)
	if err != nil {
		s.trashFailed(w, r, "restore", err)
		return
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &api.TrashChange{Tracks: change.Tracks, Albums: change.Albums})
}

// postTrashEmpty deletes songs in the Trash for good.
func (s *Server) postTrashEmpty(w http.ResponseWriter, r *http.Request) {
	var req api.TrashEmpty
	if !s.readTrash(w, r, &req) {
		return
	}
	if (len(req.Tracks) == 0) == !req.All || len(req.Tracks) > maxTrashIDs {
		writeProblem(w, r, http.StatusBadRequest, fmt.Sprintf("name from 1 to %d songs, or all", maxTrashIDs))
		return
	}
	ids := req.Tracks
	if req.All {
		ids = nil // Every song in the Trash.
	}
	//nolint:contextcheck // As postTrash.
	n, err := s.scanner.EmptyTrash(s.editContext, ids)
	if err != nil {
		s.trashFailed(w, r, "empty the trash", err)
		return
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &api.TrashEmptied{Deleted: n})
}

// readTrash checks that the Trash is on and reads the JSON body into req. It
// writes the problem and reports false when it can't go on.
func (s *Server) readTrash(w http.ResponseWriter, r *http.Request, req any) bool {
	if !s.scanner.TagWrites() {
		writeProblem(w, r, http.StatusConflict, trashOffDetail)
		return false
	}
	if err := readJSON(w, r, req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "request body must be JSON naming what to move, as this route takes it")
		return false
	}
	return true
}

// trashFailed answers a trash, restore or empty that didn't happen. A detail
// names songs, never a path: no library root reaches a client.
func (s *Server) trashFailed(w http.ResponseWriter, r *http.Request, what string, err error) {
	if taken, ok := errors.AsType[*library.PathTakenError](err); ok {
		writeProblem(w, r, http.StatusConflict, songList(taken.Titles)+" can't go back: a file is at its old path")
		return
	}
	if moved, ok := errors.AsType[*library.MoveError](err); ok {
		s.log.ErrorContext(r.Context(), what, "requestId", requestID(r.Context()), "err", err)
		writeProblem(w, r, http.StatusConflict, fmt.Sprintf("“%s” would not move, so nothing did; the server's log says why", moved.Title))
		return
	}
	switch {
	case errors.Is(err, library.ErrTagWritesOff): // Turned off since readTrash looked.
		writeProblem(w, r, http.StatusConflict, trashOffDetail)
	case errors.Is(err, library.ErrBusy):
		w.Header().Set("Retry-After", retryAfterSeconds)
		writeProblem(w, r, http.StatusConflict, "a scan or an edit is running")
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "a song or an album named isn't there")
	default:
		s.internalError(w, r, what, err)
	}
}

// songList names the songs in quotes, the first few, and counts the rest.
func songList(titles []string) string {
	quoted := make([]string, 0, namedSongs+1)
	for i, t := range titles {
		if i == namedSongs {
			quoted = append(quoted, fmt.Sprintf("%d more", len(titles)-namedSongs))
			break
		}
		quoted = append(quoted, "“"+t+"”")
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
}
