package server

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/loudness"
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
		body.Albums[i] = s.toAPIAlbumEntry(&page.Albums[i])
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
	s.writeAlbum(w, r, r.PathValue("id"))
}

// writeAlbum answers with the album id, and the caller's marks on it.
func (s *Server) writeAlbum(w http.ResponseWriter, r *http.Request, id string) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	body, err := s.albumDetail(r.Context(), user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no such album")
		return
	}
	if err != nil {
		s.internalError(w, r, "get album", err)
		return
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// albumDetail returns the wire form of the album id, with the user's marks.
// Genres live only in the raw tags, and marks in their own tables, so both come
// separately.
func (s *Server) albumDetail(ctx context.Context, userID int64, id string) (api.AlbumDetail, error) {
	d, err := s.library.Album(ctx, id)
	if err != nil {
		return api.AlbumDetail{}, fmt.Errorf("album detail: %w", err)
	}
	genres, err := s.library.AlbumTagValues(ctx, id, library.GenreKeys())
	if err != nil {
		return api.AlbumDetail{}, fmt.Errorf("album detail: %w", err)
	}
	marks, err := s.library.AlbumMarks(ctx, userID, id)
	if err != nil {
		return api.AlbumDetail{}, fmt.Errorf("album detail: %w", err)
	}
	out := toAPIAlbumDetail(&d, genres)
	out.DisplayGenres = append(out.DisplayGenres, s.genres.Of(id)...)
	out.Hearted = marks.Hearted
	for i := range out.Tracks {
		m := marks.Tracks[out.Tracks[i].ID]
		out.Tracks[i].Hearted, out.Tracks[i].Stars = m.Hearted, m.Stars
	}
	return out, nil
}

// getTrack returns one track and what it takes to edit it.
func (s *Server) getTrack(w http.ResponseWriter, r *http.Request) {
	s.writeTrack(w, r, r.PathValue("id"))
}

// writeTrack answers with the track id. It reads the whole album, which has the
// track's validators and the album's.
func (s *Server) writeTrack(w http.ResponseWriter, r *http.Request, id string) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	body, err := s.trackDetail(r.Context(), user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no such track")
		return
	}
	if err != nil {
		s.internalError(w, r, "get track", err)
		return
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

func (s *Server) trackDetail(ctx context.Context, userID int64, id string) (api.TrackDetail, error) {
	albumID, err := s.library.TrackAlbum(ctx, id)
	if err != nil {
		return api.TrackDetail{}, fmt.Errorf("track detail: %w", err)
	}
	album, err := s.albumDetail(ctx, userID, albumID)
	if err != nil {
		return api.TrackDetail{}, err
	}
	i := slices.IndexFunc(album.Tracks, func(t api.Track) bool { return t.ID == id })
	if i < 0 { // It left the album between the two reads.
		return api.TrackDetail{}, fmt.Errorf("track detail: %w", store.ErrNotFound)
	}
	return api.TrackDetail{Track: album.Tracks[i], AlbumID: album.ID, AlbumEtag: album.Etag}, nil
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
		Roots: len(s.scanner.Roots()), Issues: f.Issues, Scan: toAPIScan(s.scanner.Progress(), isAdmin(r.Context())),
		Fingerprints: toAPIFingerprints(s.scanner.Fingerprints()),
		Loudness:     toAPILoudness(s.scanner.Loudness()),
		IssueCounts: api.IssueCounts{
			Unreadable: f.Unreadable, Corrupted: f.Corrupted, Unsupported: f.Unsupported, Warnings: f.WarnedTracks,
		},
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// toAPILoudness is the loudness pass's progress, and what it has measured, on
// the wire.
func toAPILoudness(p library.LoudnessProgress) api.LoudnessStatus {
	return api.LoudnessStatus{
		Done: p.Done, Total: p.Total,
		Ungained: p.Counts.Ungained, Measured: p.Counts.Measured, Unmeasurable: p.Counts.Unmeasurable,
	}
}

// toAPIFingerprints is the fingerprint pass's progress on the wire.
func toAPIFingerprints(p library.FingerprintProgress) api.FingerprintStatus {
	return api.FingerprintStatus{Done: p.Done, Total: p.Total}
}

// getLibraryIndex lists every song, by artist and album. A client that goes away
// is no fault: a canceled query reads as an interrupted one.
func (s *Server) getLibraryIndex(w http.ResponseWriter, r *http.Request) {
	fields, err := parseIndexFields(r.URL.Query()["fields"])
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "fields must name the index's fields, joined by commas")
		return
	}
	albums, ok := indexAlbums(r.URL.Query())
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, "albums must be at most 500 album ids, joined by commas")
		return
	}
	artist := r.URL.Query().Get("artist")
	if artist != "" && albums != nil {
		writeProblem(w, r, http.StatusBadRequest, "artist and albums can't be asked for together")
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	opts := fields.options()
	opts.Albums, opts.Artist = albums, artist
	index, err := s.library.LibraryIndex(r.Context(), user.ID, &opts)
	if r.Context().Err() != nil {
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no such artist")
		return
	}
	if err != nil {
		s.internalError(w, r, "library index", err)
		return
	}
	writer := fields.writer(s.genres)
	body := toAPILibraryIndex(&index, &writer)
	writeCompressed(w, r, &body)
}

// maxIndexAlbums is the most albums one GET /library/index?albums= asks for.
const maxIndexAlbums = 500

// indexAlbums reads the albums parameter: ids joined by commas, in one
// parameter or several. It returns nil when there is none, which means every
// album, and false when it names too many.
func indexAlbums(q url.Values) ([]string, bool) {
	if !q.Has("albums") {
		return nil, true
	}
	out := []string{}
	for _, value := range q["albums"] {
		for id := range strings.SplitSeq(value, ",") {
			if id != "" {
				out = append(out, id)
			}
		}
	}
	return out, len(out) <= maxIndexAlbums
}

// toAPILibraryIndex returns the wire form of the library index, with what
// fields asks for.
func toAPILibraryIndex(index *store.LibraryIndex, fields *fieldWriter) api.LibraryIndex {
	out := api.LibraryIndex{Artists: make([]api.LibraryArtist, len(index.Artists))}
	for i := range index.Artists {
		a := &index.Artists[i]
		out.Artists[i] = api.LibraryArtist{
			ID: a.ID, Name: a.Name, SortName: a.SortName, Hearted: a.Hearted, Albums: make([]api.LibraryAlbum, len(a.Albums)),
		}
		for j := range a.Albums {
			out.Artists[i].Albums[j] = toAPILibraryAlbum(&a.Albums[j], fields)
			if index.Partial {
				out.Artists[i].Albums[j].After = &a.Albums[j].After
			}
		}
	}
	return out
}

func toAPILibraryAlbum(a *store.IndexAlbum, fields *fieldWriter) api.LibraryAlbum {
	out := api.LibraryAlbum{
		ID: a.ID, Title: a.Title, Artist: a.Artist, Compilation: a.Compilation, Hearted: a.Hearted, Year: a.Year,
		Artwork: toAPIArtwork(a.Cover), Tracks: make([]api.LibraryTrack, len(a.Tracks)),
	}
	if a.Discs != nil {
		out.Discs = toAPIDiscs(a.Discs)
	}
	for i := range a.Tracks {
		t := &a.Tracks[i]
		out.Tracks[i] = api.LibraryTrack{
			ID: t.ID, Title: t.Title, DurationMs: t.Duration.Milliseconds(),
			Artists: append([]string{}, t.Artists...), DiscNo: t.DiscNo, TrackNo: t.TrackNo,
			Hearted: t.Marks.Hearted, Stars: t.Marks.Stars,
		}
		fields.add(&out.Tracks[i], t)
	}
	return out
}

// toAPIScan returns the wire form of p. Only an administrator sees why a scan
// failed.
func toAPIScan(p library.Progress, admin bool) api.ScanStatus {
	out := api.ScanStatus{State: api.ScanIdle}
	if p.Running {
		out = api.ScanStatus{State: api.ScanRunning, Mode: p.Mode.String(), Read: p.Read, Total: p.Total, StartedAt: &p.StartedAt}
	}
	if l := p.Last; l != nil {
		st := &l.Stats
		out.Last = &api.ScanResult{
			Mode: l.Mode.String(), Outcome: api.ScanFinished, StartedAt: l.StartedAt, EndedAt: l.EndedAt,
			Seen: st.Seen, Read: st.Read, Added: st.Added, Updated: st.Updated, Moved: st.Moved,
			Missing: st.Missing, Deleted: st.Deleted, Issues: st.Issues, SkippedRoots: st.SkippedRoots,
		}
		if l.Err != "" {
			out.Last.Outcome = api.ScanFailed
		}
		if admin {
			out.Last.Error = l.Err
		}
	}
	return out
}

// toAPIDiscs returns the wire form of an album's discs, never nil.
func toAPIDiscs(discs []store.Disc) []api.Disc {
	out := make([]api.Disc, len(discs))
	for i, d := range discs {
		out[i] = api.Disc{No: d.No, Name: d.Name, Songs: d.Songs, DurationMs: d.Duration.Milliseconds()}
	}
	return out
}

// toAPIArtwork returns the wire form of a cover, or nil for none.
func toAPIArtwork(c *store.Cover) *api.Artwork {
	if c == nil {
		return nil
	}
	return &api.Artwork{Hash: hex.EncodeToString(c.Hash[:]), Accent: c.Accent, Ground: c.Ground, Width: c.Width, Height: c.Height}
}

func (s *Server) toAPIAlbumEntry(a *store.AlbumSummary) api.AlbumEntry {
	return api.AlbumEntry{
		ID: a.ID, Title: a.Title, SortName: a.SortName, Artist: a.Artist, SortArtist: a.SortArtist,
		Year: a.Year, TrackCount: a.TrackCount, Compilation: a.Compilation, Artwork: toAPIArtwork(a.Cover),
		AddedAt: a.AddedAt, DisplayGenres: append([]string{}, s.genres.Of(a.ID)...),
	}
}

// toAPIAlbumDetail builds the wire form of d, with genres by track ID.
func toAPIAlbumDetail(d *store.AlbumDetail, genres map[string][]string) api.AlbumDetail {
	out := api.AlbumDetail{
		ID: d.ID, Title: d.Title, SortName: d.SortName, Artist: d.Artist, Year: d.Year, TrackCount: d.TrackCount,
		Compilation: d.Compilation, Artwork: toAPIArtwork(d.Cover),
		Artists:       toAPIArtistLinks(d.Artists),
		Tracks:        make([]api.Track, len(d.Tracks)),
		Discs:         toAPIDiscs(d.Discs),
		DurationMs:    d.Duration.Milliseconds(),
		Formats:       append([]string{}, d.Formats...),
		AddedAt:       d.AddedAt,
		Genres:        []string{},
		DisplayGenres: []string{},
		Etag:          d.ETag,
	}
	for i := range d.Tracks {
		out.Tracks[i] = toAPITrack(&d.Tracks[i], genres[d.Tracks[i].ID])
		for _, g := range out.Tracks[i].Genres {
			if !slices.Contains(out.Genres, g) {
				out.Genres = append(out.Genres, g)
			}
		}
	}
	return out
}

func toAPITrack(t *store.Track, genres []string) api.Track {
	out := api.Track{
		ID: t.ID, Title: t.Title, Artists: append([]string{}, t.Artists...),
		DiscNo: t.DiscNo, TrackNo: t.TrackNo, DurationMs: t.Duration.Milliseconds(),
		Format: t.Format, SampleRate: t.SampleRate, BitDepth: t.BitDepth,
		TrackTotal: t.TrackTotal, DiscTotal: t.DiscTotal, DiscSubtitle: t.DiscSubtitle, Year: t.Year,
		Genres: append([]string{}, genres...), Etag: t.ETag,
	}
	out.ReplayGain = toAPIReplayGain(t)
	return out
}

// toAPIReplayGain is the gain a track plays at: its tags' when it has a gain
// tagged, else Kitsune's measure of its file, else none.
func toAPIReplayGain(t *store.Track) *api.ReplayGain {
	if rg := t.ReplayGain; rg.TrackGain != 0 || rg.AlbumGain != 0 {
		return &api.ReplayGain{
			TrackGain: rg.TrackGain, AlbumGain: rg.AlbumGain, TrackPeak: rg.TrackPeak, AlbumPeak: rg.AlbumPeak,
			Source: api.GainFromTags,
		}
	}
	m := t.Measured
	if !m.HasTrack {
		return nil
	}
	out := &api.ReplayGain{TrackGain: loudness.Gain(m.Track), TrackPeak: m.TrackPeak, Source: api.GainMeasured}
	if m.HasAlbum {
		out.AlbumGain, out.AlbumPeak = loudness.Gain(m.Album), m.AlbumPeak
	}
	return out
}
