package server

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/m3u"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// m3uMediaType is an exported playlist's.
	m3uMediaType = "audio/x-mpegurl; charset=utf-8"
	// mostSaveName is the longest a saved playlist's file name may be, in bytes.
	mostSaveName = 255
	// saveOffDetail is what a save is told while tag writes are off.
	saveOffDetail = "tag writes are off, and with them saving into the library: library.tag_writes"
)

// getPlaylistM3U answers a playlist the caller sees as an .m3u8 file, each
// song's path relative to its own library folder, so no root is ever named.
func (s *Server) getPlaylistM3U(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	name, songs, err := s.library.PlaylistExport(r.Context(), user.ID, r.PathValue("id"), s.genreAlbums())
	if err != nil {
		s.playlistFailed(w, r, "export playlist", err)
		return
	}
	out := make([]m3u.Song, len(songs))
	for i := range songs {
		out[i] = exportSong(&songs[i], filepath.ToSlash(songs[i].RelPath))
	}
	var body bytes.Buffer
	if err = m3u.Write(&body, out); err != nil {
		s.internalError(w, r, "write m3u", err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", m3uMediaType)
	h.Set("Content-Disposition", "attachment; filename*=UTF-8''"+attrEncode(m3u.FileName(name)))
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes()) // A client gone mid-body can't be told.
}

// exportSong is song as an exported file names it, at p.
func exportSong(song *store.ExportSong, p string) m3u.Song {
	return m3u.Song{Path: p, Duration: song.Duration, Artist: strings.Join(song.Artists, ", "), Title: song.Title}
}

// attrEncode percent-encodes s as RFC 8187 asks of a filename* value: every
// byte but its attr-chars.
func attrEncode(s string) string {
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		if c < 0x80 && (unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c)) || strings.IndexByte("!#$&+-.^_`|~", c) >= 0) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// postPlaylistSave saves a playlist the caller sees as an .m3u8 file in a
// library folder, each song's path relative to that folder; songs on another
// root can't be named that way and are left out.
func (s *Server) postPlaylistSave(w http.ResponseWriter, r *http.Request) {
	req, name, folder, ok := s.saveRequest(w, r)
	if !ok {
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	_, songs, err := s.library.PlaylistExport(r.Context(), user.ID, r.PathValue("id"), s.genreAlbums())
	if err != nil {
		s.playlistFailed(w, r, "export playlist", err)
		return
	}
	out := make([]m3u.Song, 0, len(songs))
	for i := range songs {
		if songs[i].Root == req.Root {
			out = append(out, exportSong(&songs[i], m3u.RelativeTo(folder, filepath.ToSlash(songs[i].RelPath))))
		}
	}
	var body bytes.Buffer
	if err = m3u.Write(&body, out); err != nil {
		s.internalError(w, r, "write m3u", err)
		return
	}
	//nolint:contextcheck // A save outlives its request on purpose, as an edit does; see Config.Context.
	if _, err = s.scanner.SavePlaylist(s.editContext, req.Root, filepath.FromSlash(folder), name, body.Bytes(), req.Replace); err != nil {
		s.saveFailed(w, r, req.Root, filepath.Join(filepath.FromSlash(folder), name), err)
		return
	}
	if folder == "." {
		folder = ""
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &api.PlaylistSaved{
		Name: name, Folder: folder, SongCount: len(out), LeftOut: len(songs) - len(out),
	})
}

// saveRequest reads a save's body and returns it with the file name it writes
// and its folder, slash-separated and cleaned, "." for the root. It writes the
// problem and reports false when it can't be a save.
func (s *Server) saveRequest(w http.ResponseWriter, r *http.Request) (req api.PlaylistSave, name, folder string, ok bool) { //nolint:nonamedreturns // Four results read best named.
	if err := readJSON(w, r, &req); err != nil || req.Root == "" {
		writeProblem(w, r, http.StatusBadRequest, "request body must name a library folder and a file name")
		return req, "", "", false
	}
	if !s.scanner.TagWrites() {
		writeProblem(w, r, http.StatusConflict, saveOffDetail)
		return req, "", "", false
	}
	if name, ok = saveName(req.Name); !ok {
		writeProblem(w, r, http.StatusUnprocessableEntity, "a file name is 1 to 255 bytes with no path separator, and doesn't start with a dot")
		return req, "", "", false
	}
	folder = path.Clean(strings.ReplaceAll(req.Folder, `\`, "/")) // "" is the root itself, ".".
	if path.IsAbs(folder) || folder == ".." || strings.HasPrefix(folder, "../") {
		writeProblem(w, r, http.StatusUnprocessableEntity, "that folder isn't in the library folder")
		return req, "", "", false
	}
	return req, name, folder, true
}

// saveName returns the file name a save writes: name trimmed, with ".m3u8"
// after it unless it ends in .m3u8 or .m3u. It reports false for one with a
// path separator or a control character, one that starts with a dot, which a
// scan would never read, or one too long.
func saveName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) ||
		strings.ContainsFunc(name, unicode.IsControl) {
		return "", false
	}
	if ext := strings.ToLower(path.Ext(name)); ext != ".m3u8" && ext != ".m3u" {
		name += ".m3u8"
	}
	return name, len(name) <= mostSaveName
}

// saveFailed answers a save that wrote nothing.
func (s *Server) saveFailed(w http.ResponseWriter, r *http.Request, root, rel string, err error) {
	if exists, ok := errors.AsType[*library.ExistsError](err); ok {
		s.answerExists(w, r, root, rel, exists)
		return
	}
	switch {
	case errors.Is(err, library.ErrTagWritesOff): // Turned off since the handler looked.
		writeProblem(w, r, http.StatusConflict, saveOffDetail)
	case errors.Is(err, library.ErrBusy):
		w.Header().Set("Retry-After", retryAfterSeconds)
		writeProblem(w, r, http.StatusConflict, "a scan or an edit is running")
	case errors.Is(err, library.ErrBadFolder):
		writeProblem(w, r, http.StatusUnprocessableEntity, "that folder isn't in a library folder")
	case r.Context().Err() == nil:
		s.internalError(w, r, "save playlist", err)
	}
}

// answerExists writes the 409 of a save whose file is there already, naming
// the file playlist read from it when there is one.
func (s *Server) answerExists(w http.ResponseWriter, r *http.Request, root, rel string, exists *library.ExistsError) {
	name := filepath.Base(rel)
	body := api.PlaylistExists{
		Problem: newProblem(r, http.StatusConflict, name+" is there already"),
		Exists:  api.ExistingFile{Name: name, FreeName: exists.FreeName, ModifiedAt: exists.ModifiedAt.UTC()},
	}
	if p, err := s.library.FilePlaylistOf(r.Context(), root, rel); err == nil {
		body.Exists.Playlist, body.Exists.PlaylistName, body.Exists.SongCount = p.ID, p.Name, new(p.Songs)
	} else if !errors.Is(err, store.ErrNotFound) && r.Context().Err() == nil {
		s.internalError(w, r, "read the file playlist", err)
		return
	}
	writeBody(w, http.StatusConflict, api.ProblemMediaType, &body)
}
