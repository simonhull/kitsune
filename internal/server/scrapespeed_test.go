package server

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/matching/musicbrainztest"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv/fanarttvtest"
	"github.com/simonhull/kitsune/internal/scrape/wikimedia/wikimediatest"
	"github.com/simonhull/kitsune/internal/store"
)

// scrapeRuns is how many artists the artist pass's budget times, one each.
const scrapeRuns = 20

// fullArt gives the i-th scraped artist every image at its real size, as
// fanart.tv's rules and Commons' scaling make them: a 1000 px thumb, a
// 3840 × 2160 background, an 800 × 310 logo, a 1000 × 185 banner and a 1024 px
// portrait, which the pass doesn't fetch, since fanart.tv's thumb wins.
func (ss *scrapeServer) fullArt(t *testing.T, i int) {
	t.Helper()
	mbid := musicbrainztest.ID("scraped " + strconv.Itoa(i))
	ss.ft.Add(fanarttvtest.Artist{
		MBID:          mbid,
		Thumbs:        []fanarttvtest.Image{{ID: "1", Lang: "en", Width: 1000, Height: 1000}},
		Backgrounds4K: []fanarttvtest.Image{{ID: "2", Width: 3840, Height: 2160}},
		HDLogos:       []fanarttvtest.Image{{ID: "3", Lang: "en", Width: 800, Height: 310}},
		Banners:       []fanarttvtest.Image{{ID: "4", Width: 1000, Height: 185}},
	})
	name := "Scraped " + strconv.Itoa(i)
	ss.wm.Add(
		wikimediatest.Item{ID: "Q" + strconv.Itoa(900+i), Articles: map[string]string{"en": name}, Image: name + ".jpg"},
		[]wikimediatest.Article{{Lang: "en", Title: name, Extract: name + " are a band.\nThey formed in 2009.\nThey split in 2019."}},
		&wikimediatest.File{Name: name + ".jpg", Width: 1024, Height: 1365, Author: annName, License: "CC BY-SA 4.0"},
	)
	// Draw them now, so the pass times Kitsune and not the stand-ins drawing.
	base := ss.ft.URL + "/fanart/music/" + mbid + "/"
	for _, path := range []string{"artistthumb/1.jpg", "artist4kbackground/2.jpg", "hdmusiclogo/3.png", "musicbanner/4.jpg"} {
		fetchAll(t, ss.ft.Client(), base+path)
	}
	fetchAll(t, ss.wm.Client(), ss.wm.URL+"/upload/"+url.PathEscape(name+".jpg"))
}

func fetchAll(t *testing.T, c *http.Client, u string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, u, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body) // Only to draw it.
	_ = resp.Body.Close()
}

// TestScrapeArtistSpeed times the artist pass over one artist with every
// source on and everything to fetch, against stand-ins that answer at once:
// Kitsune's own cost per artist (its JSON, its choices, its renders and its
// transaction), without the gates' waits.
func TestScrapeArtistSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "scrape-artist")
	ss := newScrapeServer(t)
	took := make([]time.Duration, 0, scrapeRuns)
	for i := range scrapeRuns {
		id := ss.addArtist(t, i)
		ss.fullArt(t, i)
		start := time.Now()
		ss.Scraper.Pass(t.Context())
		took = append(took, time.Since(start))
		d, err := ss.Store.Artist(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if im := d.Images; im.Thumb == nil || im.Background == nil || im.Logo == nil || im.Banner == nil || d.Bio == nil {
			t.Fatalf("artist %d: images %+v, bio %v; want all four and a bio", i, im, d.Bio)
		}
		if im := d.Images; im.Background.Width != 3840 || im.Background.Source != store.ImageFromFanart {
			t.Fatalf("background %+v, want the 4K one", im.Background)
		}
	}
	holdSpeed(t, b, limit, took)
}

// fullAlbumArt gives the i-th test album everything the album pass fetches at
// its real size: a release group to look up, the Cover Art Archive's 1200 px
// front and back, and fanart.tv's 1000 px disc art, drawn now so the pass
// times Kitsune and not the stand-ins drawing.
func (ss *scrapeServer) fullAlbumArt(t *testing.T, i int) {
	t.Helper()
	release, group := releaseOf(i), musicbrainztest.ID("speed group "+strconv.Itoa(i))
	ss.mb.Add(matching.Release{ID: release, GroupID: group, Cover: true})
	ss.mb.AddBack(release)
	ss.ft.AddAlbum(fanarttvtest.Album{MBID: group, Discs: []fanarttvtest.Disc{{ID: "9", Width: 1000, Height: 1000, Disc: 1}}})
	base := ss.mb.URL + "/coverart/release/" + release + "/"
	for _, file := range []string{"1001-1200.jpg", "1002-1200.jpg"} {
		fetchAll(t, ss.mb.Client(), base+file)
	}
	fetchAll(t, ss.ft.Client(), ss.ft.URL+"/fanart/music/"+group+"/cdart/9.png")
}

// TestScrapeAlbumSpeed times the album pass over one album with every source
// on and everything to fetch, against stand-ins that answer at once: Kitsune's
// own cost per album (its JSON, its choices, three renders and its
// transaction, a library write), without the gates' waits.
func TestScrapeAlbumSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "scrape-album")
	ss := newScrapeServer(t)
	took := make([]time.Duration, 0, scrapeRuns)
	for i := range scrapeRuns {
		id := ss.addAlbum(t, 100+i, nil)
		ss.fullAlbumArt(t, 100+i)
		start := time.Now()
		ss.Scraper.Pass(t.Context())
		took = append(took, time.Since(start))
		a, err := ss.Store.Album(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if a.Extras.Cover == nil || a.Extras.Back == nil || len(a.Extras.Discs) != 1 {
			t.Fatalf("album %d: extras %+v, want the archive's front and back and fanart.tv's disc", i, a.Extras)
		}
	}
	holdSpeed(t, b, limit, took)
}
