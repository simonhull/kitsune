// Package musicbrainztest is a stand-in MusicBrainz for tests and manual
// checks: release lookups and searches under /ws/2/, in MusicBrainz's own JSON,
// and the Cover Art Archive under /coverart/: a release's listing, and its
// front and back covers. Nothing in a test ever talks to the real
// MusicBrainz.
package musicbrainztest

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/json/v2"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/matching"
)

// Call is one request the stand-in answered.
type Call struct {
	Path, Query string // The path under /ws/2/, and the search's query, if any.
	UserAgent   string
	At          time.Time
	Status      int
}

// Server is a running stand-in. Its methods are safe for concurrent use.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	releases []matching.Release
	artists  map[string]Artist
	backs    map[string]bool // Releases with a back cover too.
	calls    []Call
	failures []int // Statuses for the next API calls, in order.
	retry    string
	mux      *http.ServeMux
}

// New starts a stand-in on the loopback network, holding releases, and stops
// it when the test ends.
func New(tb testing.TB, releases ...matching.Release) *Server {
	tb.Helper()
	s := NewInMemory(releases...)
	s.Server = httptest.NewServer(s.mux)
	tb.Cleanup(s.Close)
	return s
}

// NewInMemory returns a stand-in that answers only through [Server.RoundTrip],
// opening no listener, for a [testing/synctest] bubble, or [Server.Handler]
// for a server of its own.
func NewInMemory(releases ...matching.Release) *Server {
	s := &Server{releases: releases, artists: map[string]Artist{}, backs: map[string]bool{}, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /ws/2/release/{id}", s.lookup)
	s.mux.HandleFunc("GET /ws/2/artist/{id}", s.artist)
	s.mux.HandleFunc("GET /ws/2/release", s.search)
	s.mux.HandleFunc("GET /coverart/release/{id}", s.coverArt)
	s.mux.HandleFunc("GET /coverart/release/{id}/{size}", s.cover)
	return s
}

// Handler serves the stand-in.
func (s *Server) Handler() http.Handler { return s.mux }

// RoundTrip implements [http.RoundTripper], answering in memory.
func (s *Server) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// Options returns client options aimed at the stand-in: its listener when it has
// one, else memory, with no wait between requests and no retry delay.
func (s *Server) Options() matching.MusicBrainzOptions {
	o := matching.MusicBrainzOptions{Version: "test", Interval: time.Nanosecond, ArtInterval: time.Nanosecond, Backoff: time.Nanosecond}
	if s.Server != nil {
		o.BaseURL, o.CoverURL, o.HTTP = s.URL+"/ws/2/", s.URL+"/coverart/", s.Client()
	} else {
		o.BaseURL, o.CoverURL, o.HTTP = "http://musicbrainz.test/ws/2/", "http://musicbrainz.test/coverart/", &http.Client{Transport: s}
	}
	return o
}

// Add holds more releases.
func (s *Server) Add(releases ...matching.Release) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releases = append(s.releases, releases...)
}

// AddBack gives the releases ids a back cover at the Cover Art Archive, beside
// the front a release's Cover gives it.
func (s *Server) AddBack(ids ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		s.backs[id] = true
	}
}

// Artist is an artist the stand-in knows, with its links elsewhere.
type Artist struct {
	ID, Name string
	// Wikidata is its Wikidata item, such as Q2831, or "".
	Wikidata string
	// Wikipedia is an article linked straight to, by language: its title.
	Wikipedia map[string]string
	// Raw, when set, is answered in place of the artist's JSON.
	Raw string
}

// AddArtist holds artists, for artist/{mbid}?inc=url-rels.
func (s *Server) AddArtist(artists ...Artist) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range artists {
		s.artists[a.ID] = a
	}
}

// Fail answers the next API calls with statuses, in order, then goes back to
// answering. A 503 carries retryAfter as Retry-After, unless it is "".
func (s *Server) Fail(retryAfter string, statuses ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, statuses...)
	s.retry = retryAfter
}

// Calls returns every API call answered so far, in order.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// answer records the call and returns the status a failure asks for, or 0.
func (s *Server) answer(r *http.Request) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := 0
	if len(s.failures) > 0 {
		status, s.failures = s.failures[0], s.failures[1:]
	}
	s.calls = append(s.calls, Call{
		Path: strings.TrimPrefix(r.URL.Path, "/ws/2/"), Query: r.URL.Query().Get("query"),
		UserAgent: r.UserAgent(), At: time.Now(), Status: cmp.Or(status, http.StatusOK),
	})
	return status
}

// fail writes a failure's status, reporting whether there was one.
func (s *Server) fail(w http.ResponseWriter, r *http.Request) bool {
	status := s.answer(r)
	if status == 0 {
		return false
	}
	s.mu.Lock()
	retry := s.retry
	s.mu.Unlock()
	if status == http.StatusServiceUnavailable && retry != "" {
		w.Header().Set("Retry-After", retry)
	}
	http.Error(w, http.StatusText(status), status)
	return true
}

func (s *Server) find(id string) (matching.Release, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.releases, func(r matching.Release) bool { return r.ID == id })
	if i < 0 {
		return matching.Release{}, false
	}
	return s.releases[i], true
}

func (s *Server) lookup(w http.ResponseWriter, r *http.Request) {
	if s.fail(w, r) {
		return
	}
	release, ok := s.find(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not Found"})
		return
	}
	writeJSON(w, http.StatusOK, toJSON(&release, true))
}

// rel is one of an artist's URL relations.
type rel struct {
	Type string            `json:"type"`
	URL  map[string]string `json:"url"`
}

// resource is a URL relation's member holding the URL.
const resource = "resource"

func (s *Server) artist(w http.ResponseWriter, r *http.Request) {
	if s.fail(w, r) {
		return
	}
	s.mu.Lock()
	a, ok := s.artists[r.PathValue("id")]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not Found"})
		return
	}
	if a.Raw != "" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(a.Raw)) // The client may have gone.
		return
	}
	rels := []rel{}
	if a.Wikidata != "" {
		rels = append(rels, rel{Type: "wikidata", URL: map[string]string{resource: "https://www.wikidata.org/wiki/" + a.Wikidata}})
	}
	for _, lang := range slices.Sorted(maps.Keys(a.Wikipedia)) {
		page := "https://" + lang + ".wikipedia.org/wiki/" + a.Wikipedia[lang]
		rels = append(rels, rel{Type: "wikipedia", URL: map[string]string{resource: page}})
	}
	rels = append(rels, rel{Type: "official homepage", URL: map[string]string{resource: "https://example.com/"}})
	writeJSON(w, http.StatusOK, map[string]any{"id": a.ID, "name": a.Name, "relations": rels})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	if s.fail(w, r) {
		return
	}
	terms := parse(r.URL.Query().Get("query"))
	s.mu.Lock()
	var hits []mbRelease
	for i := range s.releases {
		if terms.match(&s.releases[i]) {
			hits = append(hits, toJSON(&s.releases[i], false))
		}
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"count": len(hits), "offset": 0, "releases": hits})
}

// coverSize is the side of a stand-in cover, in pixels, and horizon where its
// lower band starts.
const (
	coverSize = 250
	horizon   = coverSize * 5 / 8
)

// The Cover Art Archive's IDs for the stand-in's two images of a release.
const (
	frontID = "1001"
	backID  = "1002"
)

// coverArt is the archive's listing of a release: its front, as its Cover
// says, and its back, as [Server.AddBack] says, with thumbnails as the archive
// gives them, at the host asked.
func (s *Server) coverArt(w http.ResponseWriter, r *http.Request) {
	if s.fail(w, r) {
		return
	}
	id := r.PathValue("id")
	release, ok := s.find(id)
	s.mu.Lock()
	back := s.backs[id]
	s.mu.Unlock()
	if !ok || (!release.Cover && !back) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not Found"})
		return
	}
	root := "http://" + r.Host + "/coverart/release/" + id + "/"
	image := func(imageID string, front bool) map[string]any {
		return map[string]any{
			"id": imageID, "front": front, "back": !front, "approved": true,
			"image":      root + imageID + ".png",
			"thumbnails": map[string]string{"250": root + imageID + "-250.png", "1200": root + imageID + "-1200.png"},
		}
	}
	images := []any{}
	if release.Cover {
		images = append(images, image(frontID, true))
	}
	if back {
		images = append(images, image(backID, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"images": images, "release": "https://musicbrainz.org/release/" + id})
}

// cover draws a cover from the release's ID: a ground and a horizon, in colors
// of their own. A back cover (its image ID, or "back-…") is drawn from the ID
// and "back".
func (s *Server) cover(w http.ResponseWriter, r *http.Request) {
	id, size := r.PathValue("id"), r.PathValue("size")
	release, ok := s.find(id)
	s.mu.Lock()
	hasBack := s.backs[id]
	s.mu.Unlock()
	isBack := strings.HasPrefix(size, backID) || strings.HasPrefix(size, "back")
	if !ok || (isBack && !hasBack) || (!isBack && !release.Cover) {
		http.NotFound(w, r)
		return
	}
	seed := release.ID
	if isBack {
		seed += " back"
	}
	sum := sha256.Sum256([]byte(seed))
	sky := color.RGBA{R: sum[0], G: sum[1], B: sum[2], A: math.MaxUint8}
	ground := color.RGBA{R: sum[3], G: sum[4], B: sum[5], A: math.MaxUint8}
	img := image.NewRGBA(image.Rect(0, 0, coverSize, coverSize))
	draw.Draw(img, img.Bounds(), image.NewUniform(sky), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, horizon, coverSize, coverSize), image.NewUniform(ground), image.Point{}, draw.Src)
	var b bytes.Buffer
	_ = png.Encode(&b, img) // Into memory.
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=86400")
	_, _ = w.Write(b.Bytes()) // The client may have gone.
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, v) // The client may have gone.
}

// terms is what a search asked for, by field, each lower-cased; loose fields
// match any of their words.
type terms struct {
	fields map[string]string
	loose  map[string]bool
}

// parse reads the queries [matching.MusicBrainz] makes: field:"phrase" or
// field:(words), joined by AND.
func parse(q string) terms {
	t := terms{fields: map[string]string{}, loose: map[string]bool{}}
	for part := range strings.SplitSeq(q, " AND ") {
		field, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		if strings.HasPrefix(value, "(") {
			t.loose[field] = true
			value = strings.Trim(value, "()")
		}
		value = strings.Trim(value, `"`)
		value = strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\`, "").Replace(value)
		t.fields[field] = strings.ToLower(value)
	}
	return t
}

func (t *terms) match(r *matching.Release) bool {
	if len(t.fields) == 0 {
		return false
	}
	var artist strings.Builder
	for _, c := range r.Artists {
		artist.WriteString(c.Name + c.Join)
	}
	var catalogs strings.Builder
	for _, l := range r.Labels {
		catalogs.WriteString(l.CatalogNumber + " ")
	}
	have := map[string]string{"release": r.Title, "artist": artist.String(), "catno": catalogs.String(), "barcode": r.Barcode}
	for field, want := range t.fields {
		got := strings.ToLower(have[field])
		if t.loose[field] {
			if !slices.ContainsFunc(strings.Fields(want), func(w string) bool { return strings.Contains(got, w) }) {
				return false
			}
		} else if !strings.Contains(got, want) {
			return false
		}
	}
	return true
}

// MusicBrainz's JSON, as much of it as the client reads.
type (
	mbRelease struct {
		ID           string     `json:"id"`
		Title        string     `json:"title"`
		Status       string     `json:"status,omitempty"`
		Date         string     `json:"date,omitempty"`
		Country      string     `json:"country,omitempty"`
		Barcode      string     `json:"barcode"`
		Score        int        `json:"score,omitempty"`
		ArtistCredit []mbCredit `json:"artist-credit"`
		ReleaseGroup mbGroup    `json:"release-group"`
		LabelInfo    []mbLabel  `json:"label-info"`
		TrackCount   int        `json:"track-count,omitempty"`
		Media        []mbMedium `json:"media"`
		Genres       []mbGenre  `json:"genres,omitempty"`
		Cover        *mbCover   `json:"cover-art-archive,omitempty"`
	}
	mbGroup struct {
		ID               string `json:"id"`
		FirstReleaseDate string `json:"first-release-date,omitempty"`
	}
	mbCredit struct {
		Name       string   `json:"name"`
		JoinPhrase string   `json:"joinphrase"`
		Artist     mbArtist `json:"artist"`
	}
	mbArtist struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	mbLabel struct {
		CatalogNumber string   `json:"catalog-number,omitempty"`
		Label         mbArtist `json:"label"`
	}
	mbMedium struct {
		Position   int       `json:"position"`
		Format     string    `json:"format,omitempty"`
		Title      string    `json:"title"`
		TrackCount int       `json:"track-count"`
		Tracks     []mbTrack `json:"tracks,omitempty"`
	}
	mbTrack struct {
		ID           string      `json:"id"`
		Position     int         `json:"position"`
		Number       string      `json:"number"`
		Title        string      `json:"title"`
		Length       int64       `json:"length,omitempty"`
		ArtistCredit []mbCredit  `json:"artist-credit"`
		Recording    mbRecording `json:"recording"`
	}
	mbRecording struct {
		ID     string   `json:"id"`
		Title  string   `json:"title"`
		Length int64    `json:"length,omitempty"`
		ISRCs  []string `json:"isrcs,omitempty"`
	}
	mbGenre struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	mbCover struct {
		Front bool `json:"front"`
	}
)

// toJSON renders r as MusicBrainz would: with its tracks for a lookup, with
// track counts alone for a search.
func toJSON(r *matching.Release, lookup bool) mbRelease {
	out := mbRelease{
		ID: r.ID, Title: r.Title, Status: r.Status, Date: r.Date, Country: r.Country, Barcode: r.Barcode,
		ArtistCredit: credits(r.Artists), ReleaseGroup: mbGroup{ID: r.GroupID, FirstReleaseDate: r.Original},
		LabelInfo: []mbLabel{}, Media: []mbMedium{},
	}
	for _, l := range r.Labels {
		out.LabelInfo = append(out.LabelInfo, mbLabel{CatalogNumber: l.CatalogNumber, Label: mbArtist{ID: idOf("label " + l.Name), Name: l.Name}})
	}
	for i := range r.Media {
		m := &r.Media[i]
		medium := mbMedium{Position: m.Position, Format: m.Format, Title: m.Title, TrackCount: cmp.Or(len(m.Tracks), m.TrackCount)}
		if lookup {
			for j := range m.Tracks {
				t := &m.Tracks[j]
				ms := t.Length.Milliseconds()
				medium.Tracks = append(medium.Tracks, mbTrack{
					ID: t.ID, Position: t.Position, Number: strconv.Itoa(t.Position), Title: t.Title, Length: ms,
					ArtistCredit: credits(t.Artists), Recording: mbRecording{ID: t.RecordingID, Title: t.Title, Length: ms, ISRCs: t.ISRCs},
				})
			}
		}
		out.TrackCount += medium.TrackCount
		out.Media = append(out.Media, medium)
	}
	if lookup {
		out.TrackCount = 0
		for i, g := range r.Genres {
			out.Genres = append(out.Genres, mbGenre{Name: g, Count: len(r.Genres) - i})
		}
		out.Cover = &mbCover{Front: r.Cover}
	} else {
		out.Score = 100
	}
	return out
}

func credits(in []matching.Credit) []mbCredit {
	out := make([]mbCredit, len(in))
	for i, c := range in {
		out[i] = mbCredit{Name: c.Name, JoinPhrase: c.Join, Artist: mbArtist{ID: c.ID, Name: c.Name}}
	}
	return out
}

// idOf returns an MBID-shaped ID made from s, the same every time.
func idOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	const hex = "0123456789abcdef"
	var b strings.Builder
	for i, c := range sum[:16] {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			b.WriteByte('-')
		}
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0xf])
	}
	return b.String()
}

// ID returns an MBID-shaped ID made from s, the same every time, for tests
// that make their own releases.
func ID(s string) string { return idOf(s) }
