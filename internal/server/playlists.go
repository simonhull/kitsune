package server

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// mostHolding caps the songs GET /me/playlists?holding= asks about, so a
	// long query string can't take a read connection for long.
	mostHolding = 1000
	// mostAddAlbums caps the albums one add names.
	mostAddAlbums = 1000
	// noPlaylist is what a request about a playlist the caller doesn't have is
	// told, whether it is another person's or never was.
	noPlaylist = "no such playlist"
	// noEntry is what a request naming an entry the playlist doesn't hold is told.
	noEntry = "no such entry"
)

// getPlaylists lists the caller's playlists, each saying how many of the songs
// holding names it holds.
func (s *Server) getPlaylists(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	holding := holdingOf(q)
	if len(holding) > mostHolding {
		writeProblem(w, r, http.StatusBadRequest, fmt.Sprintf("holding names at most %d songs", mostHolding))
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	list, err := s.library.Playlists(r.Context(), user.ID, holding, s.genreAlbums())
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "list playlists", err)
		}
		return
	}
	body := api.Playlists{Playlists: make([]api.Playlist, len(list))}
	for i := range list {
		body.Playlists[i] = toAPIPlaylist(&list[i], q.Has("holding"))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// holdingOf returns the songs a list of playlists is asked about, each once:
// nil when it isn't asked, and empty, never nil, when it is asked of none.
func holdingOf(q url.Values) []string {
	if !q.Has("holding") {
		return nil
	}
	holding := []string{} // Asking what playlists hold of nothing still leaves smart ones out.
	for _, value := range q["holding"] {
		for id := range strings.SplitSeq(value, ",") {
			if id != "" && !slices.Contains(holding, id) {
				holding = append(holding, id)
			}
		}
	}
	return holding
}

// postPlaylist makes one of the caller's playlists, with songs in it or none.
func (s *Server) postPlaylist(w http.ResponseWriter, r *http.Request) {
	var req api.PlaylistCreate
	if err := readJSON(w, r, &req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a playlist's name, with songs or albums to put in it")
		return
	}
	name, ok := playlistName(w, r, req.Name)
	if !ok {
		return
	}
	if len(req.Rules) > 0 {
		s.postSmartPlaylist(w, r, &req, name)
		return
	}
	if !addSized(w, r, req.Albums, req.Tracks, true) {
		return
	}
	id, err := library.NewID(s.now())
	if err != nil {
		s.internalError(w, r, "new playlist ID", err)
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	added, err := s.library.CreatePlaylist(r.Context(), user.ID, id, name, &store.PlaylistAdd{Albums: req.Albums, Tracks: req.Tracks})
	if err != nil {
		s.addFailed(w, r, "create playlist", err)
		return
	}
	s.answerAdded(w, r, user.ID, id, &added, http.StatusCreated)
}

// getPlaylist answers one of the caller's playlists with its songs in order.
func (s *Server) getPlaylist(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	d, err := s.library.Playlist(r.Context(), user.ID, r.PathValue("id"), s.genreAlbums())
	if r.Context().Err() != nil {
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, noPlaylist)
		return
	}
	if err != nil {
		s.internalError(w, r, "read playlist", err)
		return
	}
	body := api.PlaylistDetail{
		Playlist: toAPIPlaylist(&d.Playlist, false),
		Entries:  make([]api.PlaylistEntry, len(d.Entries)),
		Albums:   toAPIPlaylistAlbums(d.Albums),
	}
	for i := range d.Entries {
		e := &d.Entries[i]
		t := &e.Track
		body.Entries[i] = api.PlaylistEntry{
			ID: entryID(e.ID), AlbumID: e.Album,
			Track: api.LibraryTrack{
				ID: t.ID, Title: t.Title, DurationMs: t.Duration.Milliseconds(),
				Artists: append([]string{}, t.Artists...), DiscNo: t.DiscNo, TrackNo: t.TrackNo,
				Stars: t.Rating.Stars(), TrackFields: smartFields(t.Extra),
			},
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeCompressed(w, r, &body)
}

// patchPlaylist renames one of the caller's playlists, or changes a smart
// one's rules, or both: the rules first, so a refusal changes nothing.
func (s *Server) patchPlaylist(w http.ResponseWriter, r *http.Request) {
	var req api.PlaylistChange
	if err := readJSON(w, r, &req); err != nil || (req.Name == nil && len(req.Rules) == 0) {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a playlist's new name, new rules, or both")
		return
	}
	var name string
	if req.Name != nil {
		var ok bool
		if name, ok = playlistName(w, r, *req.Name); !ok {
			return
		}
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	id := r.PathValue("id")
	if len(req.Rules) > 0 && !s.setRules(w, r, user.ID, id, req.Rules) {
		return
	}
	if req.Name != nil {
		if err := s.library.RenamePlaylist(r.Context(), user.ID, id, name); err != nil {
			s.playlistFailed(w, r, "rename playlist", err)
			return
		}
	}
	s.publishPlaylist(user.ID, id)
	s.answerPlaylist(w, r, user.ID, id, http.StatusOK)
}

// setRules gives the user's smart playlist id the rules in raw, and reports
// whether it did; when it didn't, it has answered.
func (s *Server) setRules(w http.ResponseWriter, r *http.Request, userID int64, id string, raw jsontext.Value) bool {
	rules, ok := smartRules(w, r, raw)
	if !ok {
		return false
	}
	if err := s.library.SetPlaylistRules(r.Context(), userID, id, &rules); err != nil {
		s.smartFailed(w, r, "set playlist rules", err)
		return false
	}
	return true
}

// deletePlaylist deletes one of the caller's playlists, never a song.
func (s *Server) deletePlaylist(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	id := r.PathValue("id")
	if err := s.library.DeletePlaylist(r.Context(), user.ID, id); err != nil {
		s.playlistFailed(w, r, "delete playlist", err)
		return
	}
	s.publishPlaylist(user.ID, id)
	w.WriteHeader(http.StatusNoContent)
}

// postPlaylistTracks puts songs in one of the caller's playlists.
func (s *Server) postPlaylistTracks(w http.ResponseWriter, r *http.Request) {
	var req api.PlaylistAdd
	if err := readJSON(w, r, &req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "request body must name songs or albums to add")
		return
	}
	if !addSized(w, r, req.Albums, req.Tracks, false) {
		return
	}
	add := &store.PlaylistAdd{Albums: req.Albums, Tracks: req.Tracks, Again: req.Again}
	if req.Before != "" {
		before, ok := parseEntryID(req.Before)
		if !ok {
			writeProblem(w, r, http.StatusNotFound, noEntry)
			return
		}
		add.Before = before
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	id := r.PathValue("id")
	added, err := s.library.AddToPlaylist(r.Context(), user.ID, id, add)
	if err != nil {
		s.addFailed(w, r, "add to playlist", err)
		return
	}
	s.answerAdded(w, r, user.ID, id, &added, http.StatusOK)
}

// deletePlaylistEntries takes entries out of one of the caller's playlists.
func (s *Server) deletePlaylistEntries(w http.ResponseWriter, r *http.Request) {
	var req api.PlaylistEntries
	if err := readJSON(w, r, &req); err != nil || len(req.Entries) == 0 || len(req.Entries) > store.MostPlaylistEntries {
		writeProblem(w, r, http.StatusBadRequest, fmt.Sprintf("request body must name from 1 to %d entries", store.MostPlaylistEntries))
		return
	}
	entries := make([]int64, 0, len(req.Entries))
	for _, e := range req.Entries {
		if n, ok := parseEntryID(e); ok {
			entries = append(entries, n) // One that can't be an entry isn't one: nothing to take out.
		}
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	id := r.PathValue("id")
	removed, err := s.library.RemoveFromPlaylist(r.Context(), user.ID, id, entries)
	if err != nil {
		s.playlistFailed(w, r, "take out of playlist", err)
		return
	}
	body := api.PlaylistEntries{Entries: make([]string, len(removed))}
	for i, e := range removed {
		body.Entries[i] = entryID(e)
	}
	if len(removed) > 0 {
		s.publishPlaylist(user.ID, id)
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// postPlaylistMove moves an entry of one of the caller's playlists.
func (s *Server) postPlaylistMove(w http.ResponseWriter, r *http.Request) {
	var req api.PlaylistMove
	if err := readJSON(w, r, &req); err != nil || req.Entry == "" || (req.Before == "") == (req.After == "") {
		writeProblem(w, r, http.StatusBadRequest, "request body must name an entry, and the one it goes before or after")
		return
	}
	entry, entryOK := parseEntryID(req.Entry)
	anchor, anchorOK := parseEntryID(req.Before + req.After)
	if !entryOK || !anchorOK {
		writeProblem(w, r, http.StatusNotFound, noEntry)
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	id := r.PathValue("id")
	if err := s.library.MovePlaylistEntry(r.Context(), user.ID, id, entry, anchor, req.After != ""); err != nil {
		s.playlistFailed(w, r, "move in playlist", err)
		return
	}
	s.publishPlaylist(user.ID, id)
	w.WriteHeader(http.StatusNoContent)
}

// playlistName returns name as a playlist takes it. It writes the problem and
// reports false when it can't be one.
func playlistName(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	trimmed, err := store.PlaylistName(name)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "a playlist's name is 1 to 200 characters")
		return "", false
	}
	return trimmed, true
}

// addSized reports whether an add names a number of songs and albums a
// playlist can take, none at all only when empty may, and writes the problem
// when it doesn't.
func addSized(w http.ResponseWriter, r *http.Request, albums, tracks []string, empty bool) bool {
	named := len(albums) + len(tracks)
	if (named == 0 && !empty) || len(albums) > mostAddAlbums || len(tracks) > store.MostPlaylistEntries ||
		slices.Contains(albums, "") || slices.Contains(tracks, "") {
		writeProblem(w, r, http.StatusBadRequest, fmt.Sprintf("name up to %d songs and %d albums", store.MostPlaylistEntries, mostAddAlbums))
		return false
	}
	return true
}

// addFailed answers an add or a create that put nothing in.
func (s *Server) addFailed(w http.ResponseWriter, r *http.Request, what string, err error) {
	if notLive, ok := errors.AsType[*store.NotLiveError](err); ok {
		body := api.PlaylistProblem{
			Problem: newProblem(r, http.StatusUnprocessableEntity, "some songs or albums aren't in the library"),
			Tracks:  append([]string{}, notLive.Tracks...), Albums: append([]string{}, notLive.Albums...),
		}
		writeBody(w, http.StatusUnprocessableEntity, api.ProblemMediaType, &body)
		return
	}
	if errors.Is(err, store.ErrPlaylistFull) {
		body := api.PlaylistProblem{
			Problem: newProblem(r, http.StatusUnprocessableEntity, fmt.Sprintf("a playlist holds at most %d songs", store.MostPlaylistEntries)),
			Tracks:  []string{}, Albums: []string{},
		}
		writeBody(w, http.StatusUnprocessableEntity, api.ProblemMediaType, &body)
		return
	}
	s.playlistFailed(w, r, what, err)
}

// playlistFailed answers a change to a playlist that didn't happen.
func (s *Server) playlistFailed(w http.ResponseWriter, r *http.Request, what string, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, noPlaylist)
	case errors.Is(err, store.ErrNoEntry):
		writeProblem(w, r, http.StatusNotFound, noEntry)
	case errors.Is(err, store.ErrSmartPlaylist):
		writeProblem(w, r, http.StatusConflict, "a smart playlist's rules choose its songs and their order")
	case errors.Is(err, store.ErrNotSmart):
		writeProblem(w, r, http.StatusConflict, "not a smart playlist")
	case r.Context().Err() == nil:
		s.internalError(w, r, what, err)
	}
}

// answerAdded writes what an add did, with the playlist after it, and tells the
// person's devices when it changed anything.
func (s *Server) answerAdded(w http.ResponseWriter, r *http.Request, userID int64, id string, added *store.PlaylistAdded, status int) {
	if len(added.Added) > 0 || status == http.StatusCreated {
		s.publishPlaylist(userID, id)
	}
	p, err := s.library.PlaylistSummary(r.Context(), userID, id, s.genreAlbums())
	if err != nil {
		s.playlistFailed(w, r, "read playlist", err)
		return
	}
	body := api.PlaylistAdded{Playlist: toAPIPlaylist(&p, false), Added: make([]api.AddedEntry, len(added.Added)), Skipped: added.Skipped}
	for i, e := range added.Added {
		body.Added[i] = api.AddedEntry{ID: entryID(e.Entry), Track: e.Track}
	}
	writeBody(w, status, api.JSONMediaType, &body)
}

// answerPlaylist writes the caller's playlist as a list shows it.
func (s *Server) answerPlaylist(w http.ResponseWriter, r *http.Request, userID int64, id string, status int) {
	p, err := s.library.PlaylistSummary(r.Context(), userID, id, s.genreAlbums())
	if err != nil {
		s.playlistFailed(w, r, "read playlist", err)
		return
	}
	body := toAPIPlaylist(&p, false)
	writeBody(w, status, api.JSONMediaType, &body)
}

// publishPlaylist tells the person's devices one of their playlists changed.
func (s *Server) publishPlaylist(userID int64, id string) {
	s.publish(events.Playlists, events.User(userID), &api.EventPlaylists{IDs: []string{id}})
}

func toAPIPlaylist(p *store.Playlist, holds bool) api.Playlist {
	out := api.Playlist{
		ID: p.ID, Name: p.Name, Source: p.Source, Smart: p.Rules != nil, SongCount: p.Songs, AlbumCount: p.Albums,
		DurationMs: p.Duration.Milliseconds(), Mosaic: toAPIPlaylistAlbums(p.Mosaic),
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, AddedAt: p.AddedAt,
	}
	if p.Rules != nil {
		out.Rules, _ = p.Rules.MarshalJSON() // Rules that parsed always encode.
	}

	if holds {
		out.Holds = new(p.Holds)
	}
	return out
}

func toAPIPlaylistAlbums(albums []store.PlaylistAlbum) []api.PlaylistAlbum {
	out := make([]api.PlaylistAlbum, len(albums))
	for i := range albums {
		a := &albums[i]
		out[i] = api.PlaylistAlbum{
			ID: a.ID, Title: a.Title, Artist: a.Artist, Year: a.Year, Compilation: a.Compilation, Artwork: toAPIArtwork(a.Cover),
		}
	}
	return out
}

// entryID is an entry's ID on the wire.
func entryID(id int64) string { return strconv.FormatInt(id, 10) }

// parseEntryID reads an entry's ID from the wire, reporting false for one that
// can't be an entry's.
func parseEntryID(s string) (int64, bool) {
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil && id > 0
}
