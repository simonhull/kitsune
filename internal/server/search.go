package server

import (
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

// getSearch finds artists, albums and tracks. [store.ParseSearch] caps the query
// here, before anything reaches the database: a long one would hold a connection
// for seconds, and a very long one would break it.
func (s *Server) getSearch(w http.ResponseWriter, r *http.Request) {
	found, err := s.library.Search(r.Context(), store.ParseSearch(r.URL.Query().Get("q")))
	if err != nil {
		// A client that typed on left this search behind; SQLite calls that
		// "interrupted", so only the request's context can tell it from a fault.
		if r.Context().Err() == nil {
			s.internalError(w, r, "search", err)
		}
		return
	}
	body := api.SearchResults{
		Artists: toAPIArtistLinks(found.Artists),
		Albums:  toAPIAlbumEntries(found.Albums),
		Tracks:  make([]api.TrackHit, len(found.Tracks)),
	}
	for i := range found.Tracks {
		body.Tracks[i] = toAPITrackHit(&found.Tracks[i])
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

func toAPITrackHit(t *store.TrackHit) api.TrackHit {
	return api.TrackHit{
		ID: t.ID, Title: t.Title, Artists: append([]string{}, t.Artists...),
		AlbumID: t.AlbumID, Album: t.Album, DurationMs: t.Duration.Milliseconds(), Artwork: toAPIArtwork(t.Cover),
	}
}
