package server

import (
	"net/http"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/scrape"
)

// getMetadata answers Health's Metadata card: how the artist pass and each of
// its sources stand, never a key.
func (s *Server) getMetadata(w http.ResponseWriter, r *http.Request) {
	if s.scraper == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "artist art and bios aren't set up on this server")
		return
	}
	rep, err := s.scraper.Report(r.Context())
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		s.internalError(w, r, "metadata report", err)
		return
	}
	body := api.MetadataStatus{
		Remote: rep.Remote, Language: rep.Language,
		Pass: api.MetadataPass{Running: rep.Pass.Running, Phase: api.PhaseArtists, Done: rep.Pass.Done, Total: rep.Pass.Total},
		Artists: api.MetadataArtists{
			AlbumArtists: rep.AlbumArtists, WithMBID: rep.WithMBID, LookedUp: rep.LookedUp,
			WithArt: rep.WithArt, FoundNothing: rep.FoundNothing, WithoutMBID: rep.WithoutMBID,
		},
		Albums: api.MetadataAlbums{
			Albums: rep.Albums.Albums, WithMBID: rep.Albums.WithMBID,
			FromArchive: rep.Albums.FromArchive, WithoutCover: rep.Albums.WithoutCover,
		},
		NextRefresh: timeOrNil(rep.NextRefresh),
		Sources:     make([]api.MetadataSource, len(rep.Sources)),
	}
	switch {
	case rep.Pass.Albums:
		body.Pass.Phase = api.PhaseAlbums
		if rep.Pass.Album != "" {
			body.Pass.Album = &api.AlbumLink{ID: rep.Pass.Album, Name: rep.Pass.Name}
		}
	case rep.Pass.Artist != "":
		body.Pass.Artist = &api.ArtistLink{ID: rep.Pass.Artist, Name: rep.Pass.Name}
	}
	for i := range rep.Sources {
		body.Sources[i] = toAPIMetadataSource(&rep.Sources[i])
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

func toAPIMetadataSource(src *scrape.SourceReport) api.MetadataSource {
	out := api.MetadataSource{
		Name: src.Name, Enabled: src.Enabled, State: string(src.State), Key: string(src.Key),
		ClientKey: string(src.ClientKey), Until: timeOrNil(src.Until),
	}
	if f := src.Failure; f != nil {
		out.Failure = &api.MetadataFailure{Words: f.Words, Status: f.Status, At: f.At.UTC()}
	}
	return out
}

// timeOrNil is t in UTC, or nil when it is zero.
func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return new(t.UTC())
}
