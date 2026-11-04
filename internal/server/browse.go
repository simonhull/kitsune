package server

import (
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

// getAlbums lists albums a page at a time.
func (s *Server) getAlbums(w http.ResponseWriter, r *http.Request) {
	q, err := parseAlbumsQuery(r.URL.Query())
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.library.Albums(r.Context(), q.sort, q.limit, q.cursor)
	if errors.Is(err, store.ErrBadCursor) {
		writeProblem(w, r, http.StatusBadRequest, "cursor is not valid for this sort")
		return
	}
	if err != nil {
		s.internalError(w, r, "list albums", err)
		return
	}
	body := api.AlbumPage{Albums: make([]api.AlbumEntry, len(page.Albums)), NextCursor: page.Next}
	for i := range page.Albums {
		body.Albums[i] = toAPIAlbumEntry(&page.Albums[i])
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// albumsQuery is a validated GET /albums query.
type albumsQuery struct {
	sort   store.AlbumSort
	limit  int
	cursor string
}

var (
	errBadAlbumSort  = errors.New("sort must be added or name")
	errBadAlbumLimit = errors.New("limit must be a number from 1 to 200")
)

// parseAlbumsQuery validates the sort and limit and applies their defaults. An
// empty value is an error, not a default.
func parseAlbumsQuery(v url.Values) (albumsQuery, error) {
	q := albumsQuery{sort: store.SortAdded, limit: store.DefaultAlbumLimit, cursor: v.Get("cursor")}
	if v.Has("sort") {
		switch q.sort = store.AlbumSort(v.Get("sort")); q.sort {
		case store.SortAdded, store.SortName:
		default:
			return albumsQuery{}, errBadAlbumSort
		}
	}
	if v.Has("limit") {
		n, err := strconv.Atoi(v.Get("limit"))
		if err != nil || n < 1 || n > store.MaxAlbumLimit {
			return albumsQuery{}, errBadAlbumLimit
		}
		q.limit = n
	}
	return q, nil
}

// getAlbum returns one album with its tracks.
func (s *Server) getAlbum(w http.ResponseWriter, r *http.Request) {
	d, err := s.library.Album(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no such album")
		return
	}
	if err != nil {
		s.internalError(w, r, "get album", err)
		return
	}
	body := toAPIAlbumDetail(&d)
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// getLibrary reports what the library holds.
func (s *Server) getLibrary(w http.ResponseWriter, r *http.Request) {
	f, err := s.library.Facts(r.Context())
	if err != nil {
		s.internalError(w, r, "library facts", err)
		return
	}
	body := api.LibraryFacts{
		Albums: f.Albums, Tracks: f.Tracks, DurationMs: f.Duration.Milliseconds(),
		Roots: s.roots, Issues: f.Issues,
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// toAPIArtwork returns the wire form of a cover, or nil for none.
func toAPIArtwork(c *store.Cover) *api.Artwork {
	if c == nil {
		return nil
	}
	return &api.Artwork{Hash: hex.EncodeToString(c.Hash[:]), Accent: c.Accent, Ground: c.Ground, Width: c.Width, Height: c.Height}
}

func toAPIAlbumEntry(a *store.AlbumSummary) api.AlbumEntry {
	return api.AlbumEntry{
		ID: a.ID, Title: a.Title, Artist: a.Artist, Year: a.Year, TrackCount: a.TrackCount,
		Compilation: a.Compilation, Artwork: toAPIArtwork(a.Cover),
	}
}

func toAPIAlbumDetail(d *store.AlbumDetail) api.AlbumDetail {
	out := api.AlbumDetail{
		ID: d.ID, Title: d.Title, Artist: d.Artist, Year: d.Year, TrackCount: d.TrackCount,
		Compilation: d.Compilation, Artwork: toAPIArtwork(d.Cover),
		Tracks:     make([]api.Track, len(d.Tracks)),
		DurationMs: d.Duration.Milliseconds(),
		Formats:    append([]string{}, d.Formats...),
		AddedAt:    d.AddedAt,
	}
	for i := range d.Tracks {
		out.Tracks[i] = toAPITrack(&d.Tracks[i])
	}
	return out
}

func toAPITrack(t *store.Track) api.Track {
	out := api.Track{
		ID: t.ID, Title: t.Title, Artists: append([]string{}, t.Artists...),
		DiscNo: t.DiscNo, TrackNo: t.TrackNo, DurationMs: t.Duration.Milliseconds(),
		Format: t.Format, SampleRate: t.SampleRate, BitDepth: t.BitDepth,
	}
	if rg := t.ReplayGain; rg != (store.ReplayGain{}) {
		out.ReplayGain = &api.ReplayGain{TrackGain: rg.TrackGain, AlbumGain: rg.AlbumGain, TrackPeak: rg.TrackPeak, AlbumPeak: rg.AlbumPeak}
	}
	return out
}
