// Package fanarttvtest is a stand-in fanart.tv for tests and manual checks: an
// artist's images under /v3.2/music/{mbid}, a release group's disc art under
// /v3.2/music/albums/{mbid}, the latest feed under /v3.2/music/latest, and the
// images themselves under /fanart/, drawn from their IDs. It can answer 429 with Retry-After, 503, a refused key, bare file
// names in place of addresses, slowly, and too much. Nothing in a test ever
// talks to the real fanart.tv.
package fanarttvtest

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/json/v2"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/scrape/fanarttv"
)

// Key is the project key the stand-in takes unless [Server.Want] says another.
const Key = "fanart-test-project-key"

// Image is one image the stand-in has.
type Image struct {
	ID    string
	Lang  string // "" is sent as "00", fanart.tv's no language.
	Likes int
	// Width and Height are the drawn image's; 0 draws a small one of its kind.
	Width, Height int
}

// Artist is what the stand-in has of an artist, by kind.
type Artist struct {
	MBID                               string
	Thumbs, Backgrounds4K, Backgrounds []Image
	HDLogos, Logos, Banners            []Image
}

// Album is what the stand-in has of a release group: its disc art.
type Album struct {
	MBID  string // The release group's.
	Discs []Disc
}

// Disc is one disc's art: a transparent PNG.
type Disc struct {
	Image

	Disc int // From 1; 0 sends no disc.
}

// artistName is the name the stand-in gives an artist it answers about.
const (
	artistName = "Artist"
	nameKey    = "name"
)

// kindDisc is fanart.tv's name for disc art.
const kindDisc = "cdart"

// fanart.tv's names for an artist's kinds of image.
const (
	kindThumb      = "artistthumb"
	kind4K         = "artist4kbackground"
	kindBackground = "artistbackground"
	kindHDLogo     = "hdmusiclogo"
	kindLogo       = "musiclogo"
	kindBanner     = "musicbanner"
)

// kindImages are an artist's images of one kind, and the size the stand-in
// draws one at when it gives none.
type kindImages struct {
	name  string
	imgs  []Image
	small wh
}

// wh is a width and a height.
type wh struct{ w, h int }

// kinds are a's images by kind, in fanart.tv's order.
func (a *Artist) kinds() []kindImages {
	return []kindImages{
		{name: kindThumb, imgs: a.Thumbs, small: wh{200, 200}},
		{name: kind4K, imgs: a.Backgrounds4K, small: wh{384, 216}},
		{name: kindBackground, imgs: a.Backgrounds, small: wh{192, 108}},
		{name: kindHDLogo, imgs: a.HDLogos, small: wh{160, 62}},
		{name: kindLogo, imgs: a.Logos, small: wh{160, 62}},
		{name: kindBanner, imgs: a.Banners, small: wh{200, 37}},
	}
}

// Call is one request the stand-in answered.
type Call struct {
	Path              string
	APIKey, ClientKey string
	UserAgent         string
	At                time.Time
	Status            int
}

// Server is a running stand-in. Its methods are safe for concurrent use.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	artists  map[string]Artist
	albums   map[string]Album
	latest   []string
	key      string
	failures []int
	retry    string
	bare     bool
	slow     time.Duration
	huge     bool
	calls    []Call
	mux      *http.ServeMux
	images   map[string][]byte // Drawn once each, so a speed test times Kitsune, not the stand-in.
}

// New starts a stand-in on the loopback network, holding artists, and stops
// it when the test ends.
func New(tb testing.TB, artists ...Artist) *Server {
	tb.Helper()
	s := NewInMemory(artists...)
	s.Server = httptest.NewServer(s.mux)
	tb.Cleanup(s.Close)
	return s
}

// NewInMemory returns a stand-in that answers only through [Server.RoundTrip],
// for a [testing/synctest] bubble, or [Server.Handler] for a server of its own.
func NewInMemory(artists ...Artist) *Server {
	s := &Server{artists: map[string]Artist{}, albums: map[string]Album{}, key: Key, mux: http.NewServeMux()}
	s.Add(artists...)
	s.mux.HandleFunc("GET /v3.2/music/latest", s.getLatest)
	s.mux.HandleFunc("GET /v3.2/music/albums/{mbid}", s.getAlbum)
	s.mux.HandleFunc("GET /v3.2/music/{mbid}", s.getArtist)
	s.mux.HandleFunc("GET /fanart/music/{mbid}/{kind}/{file}", s.getImage)
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
func (s *Server) Options() fanarttv.Options {
	o := fanarttv.Options{
		BaseURL: s.root(), AssetsURL: s.root(), Version: "test",
		Interval: time.Nanosecond, ImageInterval: time.Nanosecond, Backoff: time.Nanosecond,
	}
	if s.Server != nil {
		o.HTTP = s.Client()
	} else {
		o.HTTP = &http.Client{Transport: s}
	}
	return o
}

// Add holds more artists.
func (s *Server) Add(artists ...Artist) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range artists {
		s.artists[artists[i].MBID] = artists[i]
	}
}

// AddAlbum holds more release groups.
func (s *Server) AddAlbum(albums ...Album) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range albums {
		s.albums[albums[i].MBID] = albums[i]
	}
}

// SetLatest is what the latest feed lists.
func (s *Server) SetLatest(mbids ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest = mbids
}

// Want takes key as the project key from now on, refusing any other with a 401.
func (s *Server) Want(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.key = key
}

// Fail answers the next API and image calls with statuses, in order. A 429 or
// a 503 carries retryAfter as Retry-After, unless it is "".
func (s *Server) Fail(retryAfter string, statuses ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, statuses...)
	s.retry = retryAfter
}

// BareNames sends images' file names in place of their addresses, as fanart.tv
// did in November 2025.
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
const hugeBody = 40 << 20

// answer is how the stand-in answers one call.
type answer struct {
	status int // 0: as asked.
	retry  string
	slow   time.Duration
	huge   bool
}

// next records the call and says how to answer it. keyed calls need the key.
func (s *Server) next(r *http.Request, keyed bool) answer {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := answer{retry: s.retry, slow: s.slow, huge: s.huge}
	if len(s.failures) > 0 {
		a.status, s.failures = s.failures[0], s.failures[1:]
	}
	if keyed && a.status == 0 && r.Header.Get("Api-Key") != s.key {
		a.status = http.StatusUnauthorized
	}
	s.calls = append(s.calls, Call{
		Path: r.URL.Path, APIKey: r.Header.Get("Api-Key"), ClientKey: r.Header.Get("Client-Key"),
		UserAgent: r.UserAgent(), At: time.Now(), Status: cmp.Or(a.status, http.StatusOK),
	})
	return a
}

// begin records the call and writes any failure, slowness or refusal it asks
// for, reporting whether it answered.
func (s *Server) begin(w http.ResponseWriter, r *http.Request, keyed bool) bool {
	return s.next(r, keyed).write(w, r)
}

// write writes what a asks for in place of the answer, reporting whether it did.
func (a answer) write(w http.ResponseWriter, r *http.Request) bool {
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
	writeJSON(w, a.status, failed(http.StatusText(a.status)))
	return true
}

func (s *Server) getArtist(w http.ResponseWriter, r *http.Request) {
	if s.begin(w, r, true) {
		return
	}
	mbid := r.PathValue("mbid")
	s.mu.Lock()
	a, ok := s.artists[mbid]
	bare := s.bare
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, notFound())
		return
	}
	out := map[string]any{nameKey: artistName + " " + mbid[:8], "mbid_id": mbid}
	for _, k := range a.kinds() {
		if len(k.imgs) == 0 {
			continue
		}
		list := make([]map[string]string, len(k.imgs))
		for i, img := range k.imgs {
			file := img.ID + ext(k.name)
			address := s.root() + "/fanart/music/" + mbid + "/" + k.name + "/" + file
			if bare {
				address = file
			}
			list[i] = map[string]string{"id": img.ID, "url": address, "lang": cmp.Or(img.Lang, "00"), "likes": strconv.Itoa(img.Likes)}
		}
		out[k.name] = list
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getAlbum(w http.ResponseWriter, r *http.Request) {
	if s.begin(w, r, true) {
		return
	}
	mbid := r.PathValue("mbid")
	s.mu.Lock()
	a, ok := s.albums[mbid]
	bare := s.bare
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, notFound())
		return
	}
	discs := make([]map[string]string, len(a.Discs))
	for i, d := range a.Discs {
		address := s.root() + "/fanart/music/" + mbid + "/" + kindDisc + "/" + d.ID + ".png"
		if bare {
			address = d.ID + ".png"
		}
		discs[i] = map[string]string{"id": d.ID, "url": address, "likes": strconv.Itoa(d.Likes), "size": "1000"}
		if d.Disc > 0 {
			discs[i]["disc"] = strconv.Itoa(d.Disc)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		nameKey: artistName, "mbid_id": "00000000-0000-0000-0000-000000000000",
		"albums": map[string]any{mbid: map[string]any{"cdart": discs}},
	})
}

func (s *Server) getLatest(w http.ResponseWriter, r *http.Request) {
	if s.begin(w, r, true) {
		return
	}
	s.mu.Lock()
	list := make([]map[string]string, len(s.latest))
	for i, id := range s.latest {
		list[i] = map[string]string{"id": id, nameKey: artistName, "new_images": "1", "total_images": "3"}
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, list)
}

// ext is the file extension of a kind's images: logos and disc art are PNG.
func ext(kind string) string {
	if kind == kindHDLogo || kind == kindLogo || kind == kindDisc {
		return ".png"
	}
	return ".jpg"
}

func (s *Server) getImage(w http.ResponseWriter, r *http.Request) {
	if s.begin(w, r, false) {
		return
	}
	mbid, kind, file := r.PathValue("mbid"), r.PathValue("kind"), r.PathValue("file")
	id := strings.TrimSuffix(file, ext(kind))
	s.mu.Lock()
	a, ok := s.artists[mbid]
	album, isAlbum := s.albums[mbid]
	s.mu.Unlock()
	img, small, found := a.find(kind, id)
	if kind == kindDisc {
		img, small, found = album.find(id)
		ok = isAlbum
	}
	if !ok || !found {
		http.NotFound(w, r)
		return
	}
	size := image.Point{X: img.Width, Y: img.Height}
	if size.X == 0 || size.Y == 0 {
		size = small
	}
	alpha := ext(kind) == ".png"
	w.Header().Set("Content-Type", MediaType(alpha))
	_, _ = w.Write(s.drawn(mbid+kind+id, size, alpha)) //nolint:gosec // G705: an image drawn from a hash of the path, served as an image, never HTML.
}

// find returns the image of kind with id, and the size to draw it at when it
// gives none.
func (a *Artist) find(kind, id string) (Image, image.Point, bool) {
	for _, k := range a.kinds() {
		if k.name != kind {
			continue
		}
		if i := slices.IndexFunc(k.imgs, func(img Image) bool { return img.ID == id }); i >= 0 {
			return k.imgs[i], image.Point{X: k.small.w, Y: k.small.h}, true
		}
	}
	return Image{}, image.Point{}, false
}

// find returns the disc art with id, and the size to draw it at when it gives
// none.
func (a *Album) find(id string) (Image, image.Point, bool) {
	i := slices.IndexFunc(a.Discs, func(d Disc) bool { return d.ID == id })
	if i < 0 {
		return Image{}, image.Point{}, false
	}
	const side = 200
	return a.Discs[i].Image, image.Point{X: side, Y: side}, true
}

// inset is how far in from each edge, as a share of the side, a drawn image's
// mark starts: a quarter.
const inset = 4

// Draw returns an image of size in colors made from seed, as PNG with a
// transparent border when alpha is set, else as JPEG.
func Draw(seed string, size image.Point, alpha bool) []byte {
	sum := sha256.Sum256([]byte(seed))
	ink := color.RGBA{R: sum[0], G: sum[1], B: sum[2], A: math.MaxUint8}
	img := image.NewRGBA(image.Rectangle{Max: size})
	if !alpha {
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: sum[3], G: sum[4], B: sum[5], A: math.MaxUint8}), image.Point{}, draw.Src)
	}
	mark := image.Rect(size.X/inset, size.Y/inset, size.X-size.X/inset, size.Y-size.Y/inset)
	draw.Draw(img, mark, image.NewUniform(ink), image.Point{}, draw.Src)
	var b bytes.Buffer
	if alpha {
		_ = png.Encode(&b, img) // Into memory.
	} else {
		_ = jpeg.Encode(&b, img, nil) // Into memory.
	}
	return b.Bytes()
}

// MediaType is the media type of what [Draw] draws.
func MediaType(alpha bool) string {
	if alpha {
		return "image/png"
	}
	return "image/jpeg"
}

// drawn is [Draw]'s image, drawn once.
func (s *Server) drawn(seed string, size image.Point, alpha bool) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.images == nil {
		s.images = map[string][]byte{}
	}
	b, ok := s.images[seed]
	if !ok {
		b = Draw(seed, size, alpha)
		s.images[seed] = b
	}
	return b
}

// failed is fanart.tv's way of saying something went wrong, in message.
func failed(message string) map[string]string {
	return map[string]string{"status": "error", "error message": message}
}

// notFound is fanart.tv's way of saying it has none.
func notFound() map[string]string { return failed("Not found") }

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
	return "http://fanarttv.test"
}
