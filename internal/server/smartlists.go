package server

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/smartlist"
	"github.com/simonhull/kitsune/internal/store"
)

// mostSeed is the largest seed: any whole number a client holds exactly.
const mostSeed = 1<<53 - 1

// genreAlbums is how a smart playlist's genre rule reads the library's display
// genres.
func (s *Server) genreAlbums() store.GenreAlbums {
	if s.genres == nil {
		return nil
	}
	return s.genres.AlbumsWhere
}

// smartRules parses rules from a request. It writes the 422 and reports false
// when they aren't rules a smart playlist can have.
func smartRules(w http.ResponseWriter, r *http.Request, raw jsontext.Value) (smartlist.Rules, bool) {
	rules, err := smartlist.Parse(raw)
	if err != nil {
		// The error names what is wrong and never quotes a value.
		writeSmartProblem(w, r, err.Error(), nil)
		return smartlist.Rules{}, false
	}
	return rules, true
}

// writeSmartProblem writes a 422 about rules, with the playlists in a cycle when
// that is the trouble.
func writeSmartProblem(w http.ResponseWriter, r *http.Request, detail string, cycle []string) {
	body := api.SmartProblem{Problem: newProblem(r, http.StatusUnprocessableEntity, detail), Cycle: cycle}
	writeBody(w, http.StatusUnprocessableEntity, api.ProblemMediaType, &body)
}

// smartFailed answers a save of rules that didn't happen.
func (s *Server) smartFailed(w http.ResponseWriter, r *http.Request, what string, err error) {
	if cycle, ok := errors.AsType[*store.CycleError](err); ok {
		writeSmartProblem(w, r, "these rules name a playlist that names them back", append([]string{}, cycle.Playlists...))
		return
	}
	if errors.Is(err, store.ErrSmartTooDeep) {
		writeSmartProblem(w, r, fmt.Sprintf("playlist rules reach at most %d smart playlists deep", store.MostSmartDepth), nil)
		return
	}
	s.playlistFailed(w, r, what, err)
}

// smartFields is what a smart playlist's song carries for its columns: nil for
// a regular playlist's, which has no extra.
func smartFields(e *store.IndexExtra) *api.TrackFields {
	if e == nil {
		return nil
	}
	return &api.TrackFields{
		Year: e.Year, Format: e.Format, Bitrate: (e.Bitrate + bpsPerKbps/2) / bpsPerKbps, AddedAt: e.AddedAt,
		Plays: e.Plays, LastPlayedAt: e.LastPlayed,
	}
}

// bpsPerKbps turns a bitrate in bits per second into kbps.
const bpsPerKbps = 1000

// newSeed draws a seed for a smart playlist's random choices. It is no secret:
// it only orders the person's own songs.
func newSeed() int64 { return rand.Int64N(mostSeed + 1) } //nolint:gosec // Not for security: it orders songs.

// postSmartPlaylist makes one of the caller's smart playlists, named name.
func (s *Server) postSmartPlaylist(w http.ResponseWriter, r *http.Request, req *api.PlaylistCreate, name string) {
	if len(req.Albums) > 0 || len(req.Tracks) > 0 {
		writeSmartProblem(w, r, "a smart playlist's rules choose its songs: name rules or songs, not both", nil)
		return
	}
	rules, ok := smartRules(w, r, req.Rules)
	if !ok {
		return
	}
	seed := newSeed()
	if req.Seed != nil {
		seed = *req.Seed
	}
	id, err := library.NewID(s.now())
	if err != nil {
		s.internalError(w, r, "new playlist ID", err)
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	if err = s.library.CreateSmartPlaylist(r.Context(), user.ID, id, name, &rules, seed); err != nil {
		s.smartFailed(w, r, "create smart playlist", err)
		return
	}
	s.answerAdded(w, r, user.ID, id, &store.PlaylistAdded{Added: []store.AddedEntry{}, Skipped: []string{}}, http.StatusCreated)
}

// postPlaylistPreview answers what smart rules would choose now.
func (s *Server) postPlaylistPreview(w http.ResponseWriter, r *http.Request) {
	var req api.SmartPreviewRequest
	if err := readJSON(w, r, &req); err != nil || len(req.Rules) == 0 {
		writeProblem(w, r, http.StatusBadRequest, "request body must hold rules")
		return
	}
	rules, ok := smartRules(w, r, req.Rules)
	if !ok {
		return
	}
	var seed int64
	if req.Seed != nil {
		seed = *req.Seed
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	p, err := s.library.PreviewRules(r.Context(), user.ID, &rules, req.ID, seed, s.genreAlbums())
	if err != nil {
		s.playlistFailed(w, r, "preview rules", err)
		return
	}
	body := api.SmartPreview{
		SongCount: p.Songs, MatchCount: p.Matched, AlbumCount: p.Albums, DurationMs: p.Duration.Milliseconds(),
		Mosaic: toAPIPlaylistAlbums(p.Mosaic), Songs: make([]api.SmartSong, len(p.First)),
	}
	for i := range p.First {
		song := &p.First[i]
		body.Songs[i] = api.SmartSong{
			ID: song.ID, AlbumID: song.Album, Title: song.Title, Stars: song.Track.Rating.Stars(),
		}
		if e := song.Track.Extra; e != nil {
			body.Songs[i].Plays, body.Songs[i].LastPlayedAt = e.Plays, e.LastPlayed
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// postPlaylistSeed draws a new seed for one of the caller's smart playlists:
// Pick again.
func (s *Server) postPlaylistSeed(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	id := r.PathValue("id")
	if err := s.library.SetPlaylistSeed(r.Context(), user.ID, id, newSeed()); err != nil {
		s.playlistFailed(w, r, "new playlist seed", err)
		return
	}
	s.publishPlaylist(user.ID, id)
	s.answerPlaylist(w, r, user.ID, id, http.StatusOK)
}

// postPlaylistCopy copies one of the caller's playlists to a new regular one,
// holding its songs as they are now.
func (s *Server) postPlaylistCopy(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	newID, err := library.NewID(s.now())
	if err != nil {
		s.internalError(w, r, "new playlist ID", err)
		return
	}
	if err = s.library.CopyPlaylist(r.Context(), user.ID, r.PathValue("id"), newID, s.genreAlbums()); err != nil {
		s.addFailed(w, r, "copy playlist", err)
		return
	}
	s.publishPlaylist(user.ID, newID)
	s.answerPlaylist(w, r, user.ID, newID, http.StatusCreated)
}
