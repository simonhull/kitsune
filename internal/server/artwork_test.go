package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/artwork"
)

const (
	artworkPath   = "/artwork/"
	immutableCC   = "private, max-age=31536000, immutable"
	ifNoneMatch   = "If-None-Match"
	unknownHash   = "0000000000000000000000000000000000000000000000000000000000000000"
	artworkSquare = 700
)

// coverPNG returns a size x size PNG.
func coverPNG(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.Set(x, y, color.RGBA{R: uint8(x & 0xFF), G: uint8(y & 0xFF), B: 0x80, A: 0xFF})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// artServer is a test server with a cache holding one cover per entry of sizes.
type artServer struct {
	*testServer

	token string
	cache *artwork.Cache
}

func newArtServer(t *testing.T) *artServer {
	t.Helper()
	cache := artwork.NewCache(t.TempDir())
	ts := newTestServer(t, testServerOptions{Artwork: cache})
	return &artServer{testServer: ts, token: ts.sessionFor(t).token, cache: cache}
}

// put stores a size x size cover and returns its hash in URL form.
func (as *artServer) put(t *testing.T, size int) string {
	t.Helper()
	img, err := as.cache.Put(t.Context(), coverPNG(t, size))
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(img.Hash[:])
}

func (as *artServer) get(t *testing.T, path string, header http.Header) response {
	t.Helper()
	if header == nil {
		header = bearer(as.token)
	}
	return as.send(t, call{method: http.MethodGet, path: path, header: header})
}

func jpegSize(t *testing.T, body []byte) [2]int {
	t.Helper()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("response is not a JPEG: %v", err)
	}
	return [2]int{cfg.Width, cfg.Height}
}

func TestArtworkRenditions(t *testing.T) {
	t.Parallel()
	as := newArtServer(t)
	hash := as.put(t, artworkSquare) // Rungs 128, 512 and 1024 (at 700).
	tests := []struct {
		name  string
		query string
		want  int
		etag  string
	}{
		{"default is 512", "", 512, "512"},
		{"128", "?size=128", 128, "128"},
		{"512", "?size=512", 512, "512"},
		{"1024 is the source size", "?size=1024", artworkSquare, "1024"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := as.get(t, artworkPath+hash+tt.query, nil)
			if resp.Status != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", resp.Status, resp.Body)
			}
			if got := jpegSize(t, resp.Body); got != [2]int{tt.want, tt.want} {
				t.Errorf("size = %v, want %dx%d", got, tt.want, tt.want)
			}
			if got := resp.Header.Get("Content-Type"); got != "image/jpeg" {
				t.Errorf("Content-Type = %q, want image/jpeg", got)
			}
			if got := resp.Header.Get("Cache-Control"); got != immutableCC {
				t.Errorf("Cache-Control = %q, want %q", got, immutableCC)
			}
			if got, want := resp.Header.Get("ETag"), `"`+hash+"-"+tt.etag+`"`; got != want {
				t.Errorf("ETag = %q, want %q", got, want)
			}
			if got := resp.Header.Get("Content-Length"); got != strconv.Itoa(len(resp.Body)) {
				t.Errorf("Content-Length = %q, want %d", got, len(resp.Body))
			}
			if got := resp.Header.Get("Last-Modified"); got != "" {
				t.Errorf("Last-Modified = %q, want none", got)
			}
		})
	}
}

func TestArtworkSmallSourceIsNotUpscaled(t *testing.T) {
	t.Parallel()
	as := newArtServer(t)
	hash := as.put(t, 100) // Only a 128 rung, holding 100 px.
	for _, q := range []string{"?size=128", "?size=512", "?size=1024", ""} {
		resp := as.get(t, artworkPath+hash+q, nil)
		if resp.Status != http.StatusOK {
			t.Fatalf("%q: status = %d: %s", q, resp.Status, resp.Body)
		}
		if got := jpegSize(t, resp.Body); got != [2]int{100, 100} {
			t.Errorf("%q: size = %v, want 100x100", q, got)
		}
	}
}

func TestArtworkConditionalRequest(t *testing.T) {
	t.Parallel()
	as := newArtServer(t)
	hash := as.put(t, artworkSquare)
	path := artworkPath + hash + "?size=128"
	etag := as.get(t, path, nil).Header.Get("ETag")

	h := bearer(as.token)
	h.Set(ifNoneMatch, etag)
	resp := as.get(t, path, h)
	if resp.Status != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", resp.Status)
	}
	if len(resp.Body) != 0 {
		t.Errorf("a 304 carries %d body bytes", len(resp.Body))
	}
	if got := resp.Header.Get("ETag"); got != etag {
		t.Errorf("304 ETag = %q, want %q", got, etag)
	}
	if got := resp.Header.Get("Cache-Control"); got != immutableCC {
		t.Errorf("304 Cache-Control = %q, want %q", got, immutableCC)
	}

	// The tag names a size: another size's tag does not match.
	h.Set(ifNoneMatch, `"`+hash+`-512"`)
	if resp = as.get(t, path, h); resp.Status != http.StatusOK {
		t.Errorf("status with another size's ETag = %d, want 200", resp.Status)
	}
}

func TestArtworkIgnoresRange(t *testing.T) {
	t.Parallel()
	as := newArtServer(t)
	hash := as.put(t, artworkSquare)
	h := bearer(as.token)
	h.Set("Range", "bytes=0-9")
	// The contract does not declare Range, so the request is invalid by it.
	resp := as.send(t, call{method: http.MethodGet, path: artworkPath + hash + "?size=128", header: h, invalid: true})
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Status)
	}
	if len(resp.Body) <= 10 {
		t.Errorf("body is %d bytes: the range was honored", len(resp.Body))
	}
	if got := resp.Header.Get("Content-Range"); got != "" {
		t.Errorf("Content-Range = %q, want none", got)
	}
	if got := resp.Header.Get("Accept-Ranges"); got == "bytes" {
		t.Error("Accept-Ranges: bytes advertises ranges that are not served")
	}
}

func TestArtworkAcceptsTheSessionCookie(t *testing.T) {
	t.Parallel()
	as := newArtServer(t)
	hash := as.put(t, 300)
	resp := as.get(t, artworkPath+hash, cookieOnly(as.token))
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.Status)
	}
}

func TestArtworkNeedsCredentials(t *testing.T) {
	t.Parallel()
	as := newArtServer(t)
	hash := as.put(t, 300)
	resp := as.send(t, call{method: http.MethodGet, path: artworkPath + hash, invalid: true})
	resp.problem(t, http.StatusUnauthorized)
	if strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Error("a 401 is cacheable forever")
	}
}

func TestArtworkUnknownHashIsNotFound(t *testing.T) {
	t.Parallel()
	as := newArtServer(t)
	resp := as.get(t, artworkPath+unknownHash, nil)
	resp.problem(t, http.StatusNotFound)
	if strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Error("a 404 carries immutable")
	}
	if resp.Header.Get("ETag") != "" {
		t.Errorf("a 404 carries ETag %q", resp.Header.Get("ETag"))
	}
}

func TestArtworkDeletedIsNotFound(t *testing.T) {
	t.Parallel()
	as := newArtServer(t)
	hash := as.put(t, 300)
	raw, err := hex.DecodeString(hash)
	if err != nil {
		t.Fatal(err)
	}
	if err = as.cache.Delete([32]byte(raw)); err != nil {
		t.Fatal(err)
	}
	as.get(t, artworkPath+hash, nil).problem(t, http.StatusNotFound)
}

func TestArtworkWithoutACacheIsNotFound(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	// Even a request the contract rejects is a 404 when there is no cache.
	for _, path := range []string{artworkPath + unknownHash, artworkPath + "nothex", artworkPath + unknownHash + "?size=7"} {
		resp := ts.send(t, call{method: http.MethodGet, path: path, header: bearer(token), invalid: path != artworkPath+unknownHash})
		resp.problem(t, http.StatusNotFound)
	}
}

func TestArtworkRejectsBadRequests(t *testing.T) {
	t.Parallel()
	as := newArtServer(t)
	hash := as.put(t, 300)
	tests := []struct{ name, path string }{
		{"short hash", artworkPath + hash[:63]},
		{"long hash", artworkPath + hash + "0"},
		{"upper case hash", artworkPath + strings.ToUpper(hash)},
		{"non-hex hash", artworkPath + strings.Repeat("g", 64)},
		{"size zero", artworkPath + hash + "?size=0"},
		{"size off the ladder", artworkPath + hash + "?size=256"},
		{"size too big", artworkPath + hash + "?size=2048"},
		{"size not a number", artworkPath + hash + "?size=big"},
		{"empty size", artworkPath + hash + "?size="},
		{"negative size", artworkPath + hash + "?size=-128"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := as.send(t, call{method: http.MethodGet, path: tt.path, header: bearer(as.token), invalid: true})
			resp.problem(t, http.StatusBadRequest)
		})
	}
}

func TestArtworkHashIsHexOfSHA256(t *testing.T) {
	t.Parallel()
	as := newArtServer(t)
	data := coverPNG(t, 200)
	if _, err := as.cache.Put(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	resp := as.get(t, artworkPath+hex.EncodeToString(sum[:]), nil)
	if resp.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.Status)
	}
}
