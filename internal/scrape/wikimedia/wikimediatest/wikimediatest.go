// Package wikimediatest is a stand-in for the Wikimedia sites, for tests and
// manual checks: Wikidata's API under /wikidata/w/api.php, Wikipedia's
// summaries under /wikipedia/{lang}/api/rest_v1/page/summary/{title},
// Commons' API under /commons/w/api.php, and its files under /upload/, drawn
// from their names. It can answer 429 with Retry-After, 503, a refusal, a bare
// file name in place of an address, slowly, and too much. Nothing in a test
// ever talks to the real sites.
package wikimediatest

import (
	"bytes"
	"cmp"
	"encoding/json/v2"
	"image"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/scrape/fanarttv/fanarttvtest"
	"github.com/simonhull/kitsune/internal/scrape/wikimedia"
)

// Item is a Wikidata item the stand-in has: its articles by language and its
// image's file name.
type Item struct {
	ID       string
	Articles map[string]string // Language → title.
	Image    string            // A Commons file name, or "".
}

// Article is a Wikipedia article the stand-in has.
type Article struct {
	Lang, Title string
	Extract     string
	// Disambiguation makes it a disambiguation page.
	Disambiguation bool
}

// File is a Commons file the stand-in has.
type File struct {
	Name                        string
	Width, Height               int    // 0 draws 300 × 400.
	Author, License, LicenseURL string // Author may hold HTML, as Commons' does.
}

// JSON members the stand-in writes more than once.
const (
	memberTitle = "title"
	memberValue = "value"
	memberType  = "type"
)

// Call is one request the stand-in answered.
type Call struct {
	Site, Path, Query string
	UserAgent         string
	At                time.Time
	Status            int
}

// Server is a running stand-in. Its methods are safe for concurrent use.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	items    map[string]Item
	articles map[string]Article // By lang + ":" + title.
	files    map[string]File
	failures []int
	retry    string
	bare     bool
	slow     time.Duration
	huge     bool
	calls    []Call
	mux      *http.ServeMux
}

// New starts a stand-in on the loopback network and stops it when the test ends.
func New(tb testing.TB) *Server {
	tb.Helper()
	s := NewInMemory()
	s.Server = httptest.NewServer(s.mux)
	tb.Cleanup(s.Close)
	return s
}

// NewInMemory returns a stand-in that answers only through [Server.RoundTrip],
// or [Server.Handler] for a server of its own.
func NewInMemory() *Server {
	s := &Server{items: map[string]Item{}, articles: map[string]Article{}, files: map[string]File{}, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /wikidata/w/api.php", s.wikidata)
	s.mux.HandleFunc("GET /wikipedia/{lang}/api/rest_v1/page/summary/{title}", s.summary)
	s.mux.HandleFunc("GET /commons/w/api.php", s.commons)
	s.mux.HandleFunc("GET /upload/{name}", s.upload)
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

// Options returns client options aimed at the stand-in, with no wait between
// requests and no retry delay.
func (s *Server) Options() wikimedia.Options {
	o := wikimedia.Options{BaseURL: s.root(), Version: "test", Interval: time.Nanosecond, Backoff: time.Nanosecond}
	if s.Server != nil {
		o.HTTP = s.Client()
	} else {
		o.HTTP = &http.Client{Transport: s}
	}
	return o
}

// Add holds an item, its articles and its file.
func (s *Server) Add(item Item, articles []Article, file *File) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[item.ID] = item
	for _, a := range articles {
		s.articles[a.Lang+":"+a.Title] = a
	}
	if file != nil {
		s.files[file.Name] = *file
	}
}

// Fail answers the next calls with statuses, in order. A 429 or a 503 carries
// retryAfter as Retry-After, unless it is "".
func (s *Server) Fail(retryAfter string, statuses ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, statuses...)
	s.retry = retryAfter
}

// BareNames answers a file's bare name in place of its address.
func (s *Server) BareNames(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bare = on
}

// Slow waits d before answering.
func (s *Server) Slow(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slow = d
}

// Huge pads every answer past any cap a client keeps.
func (s *Server) Huge(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.huge = on
}

// Calls returns every call answered so far, in order.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// hugeBody is more than any client takes.
const hugeBody = 20 << 20

// answer is how the stand-in answers one call.
type answer struct {
	status int // 0: as asked.
	retry  string
	slow   time.Duration
	huge   bool
}

// next records the call and says how to answer it.
func (s *Server) next(r *http.Request) answer {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := answer{retry: s.retry, slow: s.slow, huge: s.huge}
	if len(s.failures) > 0 {
		a.status, s.failures = s.failures[0], s.failures[1:]
	}
	site, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	s.calls = append(s.calls, Call{
		Site: site, Path: r.URL.Path, Query: r.URL.RawQuery, UserAgent: r.UserAgent(), At: time.Now(),
		Status: cmp.Or(a.status, http.StatusOK),
	})
	return a
}

// begin records the call and writes any failure, slowness or padding it asks
// for, reporting whether it answered.
func (s *Server) begin(w http.ResponseWriter, r *http.Request) bool {
	a := s.next(r)
	if a.slow > 0 {
		select {
		case <-time.After(a.slow):
		case <-r.Context().Done():
			return true
		}
	}
	switch {
	case a.huge && a.status == 0:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bytes.Repeat([]byte(" "), hugeBody)) // The client may have gone.
		return true
	case a.status == 0:
		return false
	case a.retry != "" && (a.status == http.StatusTooManyRequests || a.status == http.StatusServiceUnavailable):
		w.Header().Set("Retry-After", a.retry)
	}
	writeJSON(w, a.status, map[string]any{"error": map[string]string{"code": "failed", "info": http.StatusText(a.status)}})
	return true
}

func (s *Server) wikidata(w http.ResponseWriter, r *http.Request) {
	if s.begin(w, r) {
		return
	}
	q := r.URL.Query()
	switch q.Get("action") {
	case "wbgetentities":
		s.entities(w, q)
	case "wbgetclaims":
		s.claims(w, q)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
	}
}

func (s *Server) entities(w http.ResponseWriter, q url.Values) {
	id := q.Get("ids")
	s.mu.Lock()
	item, ok := s.items[id]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"entities": map[string]any{id: map[string]string{"id": id, "missing": ""}}})
		return
	}
	wanted := strings.Split(q.Get("sitefilter"), "|")
	links := map[string]any{}
	for lang, title := range item.Articles {
		if site := lang + "wiki"; slices.Contains(wanted, site) {
			links[site] = map[string]string{"site": site, memberTitle: title}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entities": map[string]any{id: map[string]any{"id": id, "sitelinks": links}}})
}

func (s *Server) claims(w http.ResponseWriter, q url.Values) {
	s.mu.Lock()
	item := s.items[q.Get("entity")]
	s.mu.Unlock()
	claims := map[string]any{}
	if item.Image != "" {
		claims["P18"] = []any{map[string]any{
			"rank":     "normal",
			"mainsnak": map[string]any{"property": "P18", "datavalue": map[string]any{memberValue: item.Image, memberType: "string"}},
		}}
	}
	writeJSON(w, http.StatusOK, map[string]any{"claims": claims})
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	if s.begin(w, r) {
		return
	}
	lang, title := r.PathValue("lang"), strings.ReplaceAll(r.PathValue("title"), "_", " ")
	s.mu.Lock()
	a, ok := s.articles[lang+":"+title]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{memberType: "https://mediawiki.org/wiki/HyperSwitch/errors/not_found"})
		return
	}
	kind := "standard"
	if a.Disambiguation {
		kind = "disambiguation"
	}
	page := "https://" + lang + ".wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(a.Title, " ", "_"))
	writeJSON(w, http.StatusOK, map[string]any{
		memberType: kind, memberTitle: a.Title, "lang": lang, "extract": a.Extract,
		"content_urls": map[string]any{"desktop": map[string]string{"page": page}},
	})
}

func (s *Server) commons(w http.ResponseWriter, r *http.Request) {
	if s.begin(w, r) {
		return
	}
	name := strings.TrimPrefix(r.URL.Query().Get("titles"), "File:")
	s.mu.Lock()
	f, ok := s.files[name]
	bare := s.bare
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"query": map[string]any{"pages": []any{map[string]any{memberTitle: "File:" + name, "missing": true}}}})
		return
	}
	sz := size(&f)
	address := s.root() + "/upload/" + url.PathEscape(name)
	if bare {
		address = name
	}
	info := map[string]any{
		"url": address, "width": sz.X, "height": sz.Y, "thumburl": address, "thumbwidth": sz.X, "thumbheight": sz.Y,
		"descriptionurl": "https://commons.wikimedia.org/wiki/File:" + url.PathEscape(name),
		"extmetadata": map[string]any{
			"Artist":           map[string]string{memberValue: f.Author},
			"LicenseShortName": map[string]string{memberValue: f.License},
			"LicenseUrl":       map[string]string{memberValue: f.LicenseURL},
		},
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": map[string]any{"pages": []any{map[string]any{memberTitle: "File:" + name, "imageinfo": []any{info}}}}})
}

// What a file with no size of its own is drawn at.
const (
	defaultWidth  = 300
	defaultHeight = 400
)

// size is the size f is drawn at.
func size(f *File) image.Point {
	if f.Width == 0 || f.Height == 0 {
		return image.Point{X: defaultWidth, Y: defaultHeight}
	}
	return image.Point{X: f.Width, Y: f.Height}
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	if s.begin(w, r) {
		return
	}
	s.mu.Lock()
	f, ok := s.files[r.PathValue("name")]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", fanarttvtest.MediaType(false))
	_, _ = w.Write(fanarttvtest.Draw("commons "+f.Name, size(&f), false)) // The client may have gone.
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, v) // The client may have gone.
}

// root is where the stand-in is reached.
func (s *Server) root() string {
	if s.Server != nil {
		return s.URL
	}
	return "http://wikimedia.test"
}
