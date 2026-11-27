package server

import (
	"crypto/sha256"
	"encoding/json/v2"
	"fmt"
	"image"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv/fanarttvtest"
	"github.com/simonhull/kitsune/internal/store"
)

// Choose artwork's routes, under /admin/artists/{id}.
func imagesPath(id string) string               { return "/admin/artists/" + id + "/images" }
func kindPath(id, kind string) string           { return imagesPath(id) + "/" + kind }
func uploadPath(id, kind string) string         { return kindPath(id, kind) + "/upload" }
func artistActionPath(id, action string) string { return "/admin/artists/" + id + "/" + action }

// noSuchArtist is an artist ID no test writes.
const noSuchArtist = "00000000000000000000000000"

// candidatesOf lists the artist's candidates of kind.
func (ss *scrapeServer) candidatesOf(t *testing.T, token, id, kind string) api.ArtistImageCandidates {
	t.Helper()
	r := ss.send(t, call{method: http.MethodGet, path: imagesPath(id) + "?kind=" + kind, header: bearer(token)})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d: %s", r.Status, r.Body)
	}
	for _, key := range []string{fanartProjectKey, fanartPersonalKey} {
		if strings.Contains(string(r.Body), key) {
			t.Errorf("the candidates hold a key: %s", r.Body)
		}
	}
	var c api.ArtistImageCandidates
	if err := json.Unmarshal(r.Body, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// noContent checks r is a 204.
func (r response) noContent(t *testing.T) {
	t.Helper()
	if r.Status != http.StatusNoContent {
		t.Errorf("status %d: %s, want 204", r.Status, r.Body)
	}
}

// accepted checks r is a 202.
func (r response) accepted(t *testing.T) {
	t.Helper()
	if r.Status != http.StatusAccepted {
		t.Errorf("status %d: %s, want 202", r.Status, r.Body)
	}
}

// bareArtist writes an album by an artist with no MusicBrainz ID, and returns
// the artist's public ID.
func bareArtist(t *testing.T, ts *testServer) string {
	t.Helper()
	const name, key = "Bare", "bare"
	ulid := fmt.Sprintf("%026d", 700)
	w := store.TrackWrite{
		ULID: fmt.Sprintf("01B%023d", 1), Root: musicBase, RelPath: "bare/1.flac", Size: 1000,
		MTime: time.UnixMilli(1_761_000_000_000), Digest: new(sha256.Sum256([]byte(key))),
		Format: flacFormat, Codec: flacFormat, Duration: 200 * time.Second, Title: scrapedSong, SortTitle: scrapedSong,
		Album:   store.AlbumWrite{GroupKey: key, ULID: fmt.Sprintf("02B%023d", 1), Artist: name, Name: name, SortName: name},
		Artists: []store.ArtistWrite{{ULID: ulid, MatchKey: key, Name: name}},
	}
	if err := ts.Store.WriteBatch(t.Context(), &store.Batch{Tracks: []store.TrackWrite{w}}); err != nil {
		t.Fatal(err)
	}
	return ulid
}

// pinBody is a PUT's body naming candidate.
func pinBody(candidate string) string { return `{"candidate":"` + candidate + `"}` }

// uploadHeader is an administrator's headers for an upload of mediaType.
func uploadHeader(token, mediaType string) http.Header {
	h := bearer(token)
	h.Set("Content-Type", mediaType)
	return h
}

func TestChooseArtworkLists(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	id := ss.addArtist(t, 1)
	ss.Scraper.Pass(t.Context())
	c := ss.candidatesOf(t, ss.tokenFor(t, store.RoleAdmin), id, "thumb")
	if len(c.Candidates) != 1 || c.Candidates[0].ID != "wikimedia:Scraped 1.jpg" || !c.Candidates[0].Chosen || c.Candidates[0].Credit == nil {
		t.Fatalf("thumbs = %+v, want Commons' portrait, chosen, credited", c)
	}
}

// pin pins the candidate as the artist's kind, and returns the image shown.
func (ss *scrapeServer) pin(t *testing.T, token, id, kind, candidate string) api.ExtraImage {
	t.Helper()
	r := ss.send(t, call{method: http.MethodPut, path: kindPath(id, kind), body: pinBody(candidate), header: jsonHeader(token)})
	if r.Status != http.StatusOK {
		t.Fatalf("pin: %d %s", r.Status, r.Body)
	}
	var img api.ExtraImage
	if err := json.Unmarshal(r.Body, &img); err != nil {
		t.Fatal(err)
	}
	return img
}

func TestChooseArtworkPins(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	id := ss.addArtist(t, 1)
	ss.Scraper.Pass(t.Context())
	token := ss.tokenFor(t, store.RoleAdmin)
	tap := ss.Events.Subscribe(events.Viewer{UserID: 99}, "")
	defer ss.Events.Unsubscribe(tap)
	<-tap.C() // Ready.
	if img := ss.pin(t, token, id, "background", "fanart:7"); img.Source != "pinned" || img.PinnedFrom != "fanart" {
		t.Fatalf("pinned %+v, want fanart.tv's, pinned", img)
	}
	if heard := drainTap(tap); !slices.ContainsFunc(heard, func(h string) bool { return strings.HasPrefix(h, "artists") && strings.Contains(h, id) }) {
		t.Errorf("events = %q, want an artists event for the artist", heard)
	}
	bg := ss.candidatesOf(t, token, id, "background")
	if !bg.Pinned || !bg.Candidates[0].Pinned || bg.Candidates[0].PinnedBy == "" || bg.Candidates[0].PinnedAt == nil {
		t.Errorf("backgrounds = %+v, want the pin first, with who and when", bg)
	}
	ss.send(t, call{method: http.MethodDelete, path: kindPath(id, "background"), header: jsonHeader(token)}).noContent(t)
	if bg = ss.candidatesOf(t, token, id, "background"); bg.Pinned {
		t.Errorf("after Automatic: %+v", bg)
	}
}

func TestChooseArtworkUploads(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	id := ss.addArtist(t, 1)
	token := ss.tokenFor(t, store.RoleAdmin)
	data := fanarttvtest.Draw("mine", image.Point{X: 640, Y: 360}, false)
	r := ss.send(t, call{method: http.MethodPost, path: uploadPath(id, "background"), body: string(data), header: uploadHeader(token, "image/jpeg")})
	if r.Status != http.StatusOK {
		t.Fatalf("upload: %d %s", r.Status, r.Body)
	}
	if c := ss.candidatesOf(t, token, id, "background"); c.Candidates[0].Source != "upload" || !c.Candidates[0].Pinned {
		t.Errorf("backgrounds = %+v, want the upload pinned first", c)
	}
	ss.send(t, call{method: http.MethodPost, path: uploadPath(id, "background"), body: "not an image", header: uploadHeader(token, "image/png")}).problem(t, http.StatusUnprocessableEntity)
	ss.send(t, call{method: http.MethodPost, path: uploadPath(id, "background"), body: "GIF89a", header: uploadHeader(token, "image/gif"), invalid: true}).problem(t, http.StatusUnsupportedMediaType)
	huge := strings.Repeat("x", maxUpload+1)
	ss.send(t, call{method: http.MethodPost, path: uploadPath(id, "background"), body: huge, header: uploadHeader(token, "image/webp")}).problem(t, http.StatusRequestEntityTooLarge)
	ss.send(t, call{method: http.MethodPost, path: uploadPath(noSuchArtist, "thumb"), body: string(data), header: uploadHeader(token, "image/jpeg")}).problem(t, http.StatusNotFound)
	ss.send(t, call{method: http.MethodPost, path: uploadPath(id, "cover"), body: string(data), header: uploadHeader(token, "image/jpeg"), invalid: true}).problem(t, http.StatusBadRequest)
}

func TestChooseArtworkRefusals(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	id := ss.addArtist(t, 1)
	token := ss.tokenFor(t, store.RoleAdmin)
	ss.send(t, call{method: http.MethodGet, path: imagesPath(id) + "?kind=cover", header: bearer(token), invalid: true}).problem(t, http.StatusBadRequest)
	ss.send(t, call{method: http.MethodGet, path: imagesPath(noSuchArtist) + "?kind=logo", header: bearer(token)}).problem(t, http.StatusNotFound)
	ss.send(t, call{method: http.MethodPut, path: kindPath(id, "logo"), body: pinBody("fanart:404"), header: jsonHeader(token)}).problem(t, http.StatusUnprocessableEntity)
	ss.send(t, call{method: http.MethodPut, path: kindPath(id, "logo"), body: `{}`, header: jsonHeader(token), invalid: true}).problem(t, http.StatusBadRequest)
	ss.send(t, call{method: http.MethodPut, path: kindPath(noSuchArtist, "logo"), body: pinBody("fanart:7"), header: jsonHeader(token)}).problem(t, http.StatusNotFound)
	ss.send(t, call{method: http.MethodDelete, path: kindPath(id, "cover"), header: jsonHeader(token), invalid: true}).problem(t, http.StatusBadRequest)
	ss.send(t, call{method: http.MethodDelete, path: kindPath(noSuchArtist, "logo"), header: jsonHeader(token)}).problem(t, http.StatusNotFound)
	// A source that won't give the image it listed.
	ss.candidatesOf(t, token, id, "background")
	ss.ft.Fail("", http.StatusServiceUnavailable, http.StatusServiceUnavailable, http.StatusServiceUnavailable, http.StatusServiceUnavailable)
	ss.send(t, call{method: http.MethodPut, path: kindPath(id, "background"), body: pinBody("fanart:7"), header: jsonHeader(token)}).problem(t, http.StatusServiceUnavailable)
}

func TestLookAgainAndNotThisArtist(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	id := ss.addArtist(t, 1)
	ss.Scraper.Pass(t.Context())
	token := ss.tokenFor(t, store.RoleAdmin)
	ss.send(t, call{method: http.MethodPost, path: artistActionPath(id, "refresh"), header: jsonHeader(token)}).accepted(t)
	ss.send(t, call{method: http.MethodPost, path: artistActionPath(id, "reject"), header: jsonHeader(token)}).noContent(t)
	var d api.ArtistDetail
	r := ss.send(t, call{method: http.MethodGet, path: "/artists/" + id, header: bearer(token)})
	if err := json.Unmarshal(r.Body, &d); err != nil || d.Images != (api.ArtistImages{}) || d.Bio != nil {
		t.Errorf("after Not this artist: %+v, %v", d, err)
	}
	if c := ss.candidatesOf(t, token, id, "thumb"); !c.Held || c.Sources[0].State != "held" {
		t.Errorf("candidates = %+v, want the artist held", c)
	}
	ss.send(t, call{method: http.MethodPost, path: artistActionPath(id, "refresh"), header: jsonHeader(token)}).problem(t, http.StatusConflict)
	ss.send(t, call{method: http.MethodPost, path: artistActionPath(noSuchArtist, "refresh"), header: jsonHeader(token)}).problem(t, http.StatusNotFound)
	ss.send(t, call{method: http.MethodPost, path: artistActionPath(noSuchArtist, "reject"), header: jsonHeader(token)}).problem(t, http.StatusNotFound)
}

func TestNotThisArtistNeedsAnMBID(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	ss.addArtist(t, 1)
	bare := bareArtist(t, ss.testServer)
	ss.send(t, call{method: http.MethodPost, path: artistActionPath(bare, "reject"), header: jsonHeader(ss.tokenFor(t, store.RoleAdmin))}).problem(t, http.StatusConflict)
}

func TestChooseArtworkIsForAdministrators(t *testing.T) {
	t.Parallel()
	ss := newScrapeServer(t)
	id := ss.addArtist(t, 1)
	user := ss.tokenFor(t, store.RoleUser)
	for _, c := range []call{
		{method: http.MethodGet, path: imagesPath(id) + "?kind=thumb"},
		{method: http.MethodPut, path: kindPath(id, "thumb"), body: pinBody("fanart:7")},
		{method: http.MethodDelete, path: kindPath(id, "thumb")},
		{method: http.MethodPost, path: uploadPath(id, "thumb"), body: "x"},
		{method: http.MethodPost, path: artistActionPath(id, "refresh")},
		{method: http.MethodPost, path: artistActionPath(id, "reject")},
	} {
		c.header = jsonHeader(user)
		if c.method == http.MethodPost && strings.HasSuffix(c.path, "/upload") {
			c.header = uploadHeader(user, "image/png")
		}
		ss.send(t, c).problem(t, http.StatusForbidden)
		c.header, c.invalid = nil, true
		ss.send(t, c).problem(t, http.StatusUnauthorized)
	}
}

func TestChooseArtworkWithoutThePass(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.tokenFor(t, store.RoleAdmin)
	for _, c := range []call{
		{method: http.MethodGet, path: imagesPath(noSuchArtist) + "?kind=thumb", header: bearer(token)},
		{method: http.MethodPut, path: kindPath(noSuchArtist, "thumb"), body: pinBody("fanart:7"), header: jsonHeader(token)},
		{method: http.MethodDelete, path: kindPath(noSuchArtist, "thumb"), header: jsonHeader(token)},
		{method: http.MethodPost, path: uploadPath(noSuchArtist, "thumb"), body: "x", header: uploadHeader(token, "image/png")},
		{method: http.MethodPost, path: artistActionPath(noSuchArtist, "refresh"), header: jsonHeader(token)},
		{method: http.MethodPost, path: artistActionPath(noSuchArtist, "reject"), header: jsonHeader(token)},
	} {
		ts.send(t, c).problem(t, http.StatusServiceUnavailable)
	}
}
