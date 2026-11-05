package server

import (
	"errors"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

// getArtists lists the artists who own an album, whole: a library has few enough.
func (s *Server) getArtists(w http.ResponseWriter, r *http.Request) {
	artists, err := s.library.Artists(r.Context())
	if err != nil {
		s.internalError(w, r, "list artists", err)
		return
	}
	body := api.ArtistList{Artists: make([]api.ArtistEntry, len(artists))}
	for i := range artists {
		body.Artists[i] = toAPIArtistEntry(&artists[i])
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// getArtist returns one artist with its albums.
func (s *Server) getArtist(w http.ResponseWriter, r *http.Request) {
	d, err := s.library.Artist(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no such artist")
		return
	}
	if err != nil {
		s.internalError(w, r, "get artist", err)
		return
	}
	body := api.ArtistDetail{
		ID: d.ID, Name: d.Name, AlbumCount: d.AlbumCount,
		Albums: toAPIAlbumEntries(d.Albums), AppearsOn: toAPIAlbumEntries(d.AppearsOn),
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

func toAPIArtistEntry(a *store.ArtistSummary) api.ArtistEntry {
	return api.ArtistEntry{ID: a.ID, Name: a.Name, AlbumCount: a.AlbumCount}
}

func toAPIAlbumEntries(albums []store.AlbumSummary) []api.AlbumEntry {
	out := make([]api.AlbumEntry, len(albums))
	for i := range albums {
		out[i] = toAPIAlbumEntry(&albums[i])
	}
	return out
}
