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
		Pass: api.MetadataPass{Running: rep.Pass.Running, Done: rep.Pass.Done, Total: rep.Pass.Total},
		Artists: api.MetadataArtists{
			AlbumArtists: rep.AlbumArtists, WithMBID: rep.WithMBID, LookedUp: rep.LookedUp,
			WithArt: rep.WithArt, FoundNothing: rep.FoundNothing, WithoutMBID: rep.WithoutMBID,
		},
		NextRefresh: timeOrNil(rep.NextRefresh),
		Sources:     make([]api.MetadataSource, len(rep.Sources)),
	}
	if rep.Pass.Artist != "" {
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
