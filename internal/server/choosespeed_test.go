package server

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/matching/musicbrainztest"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv/fanarttvtest"
	"github.com/simonhull/kitsune/internal/store"
)

// pickerBackgrounds is how many backgrounds fanart.tv offers the artist the
// picker's budgets time: a well-covered artist's share.
const pickerBackgrounds = 12

// manyBackgrounds writes the i-th scraped artist with pickerBackgrounds
// 1920 × 1080 backgrounds at fanart.tv, their 400 px previews drawn now so a
// timing measures Kitsune and not the stand-in drawing, and returns the
// artist's public ID.
func (ss *scrapeServer) manyBackgrounds(t *testing.T, i int) string {
	t.Helper()
	id := ss.addArtist(t, i)
	mbid := musicbrainztest.ID("scraped " + strconv.Itoa(i))
	imgs := make([]fanarttvtest.Image, pickerBackgrounds)
	for n := range imgs {
		imgs[n] = fanarttvtest.Image{ID: strconv.Itoa(100 + n), Likes: n, Width: 1920, Height: 1080}
		fetchAll(t, ss.ft.Client(), ss.ft.URL+"/bigpreview/music/"+mbid+"/artistbackground/"+imgs[n].ID+".jpg")
	}
	ss.ft.Add(fanarttvtest.Artist{MBID: mbid, Backgrounds: imgs})
	return id
}

// TestChooseCandidatesSpeed times Choose artwork's list of an artist's
// backgrounds once it has been looked at within the day: what fanart.tv
// offers kept, every preview made.
func TestChooseCandidatesSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "choose-candidates")
	ss := newScrapeServer(t)
	token := ss.tokenFor(t, store.RoleAdmin)
	id := ss.manyBackgrounds(t, 1)
	if c := ss.candidatesOf(t, token, id, "background"); len(c.Candidates) != pickerBackgrounds {
		t.Fatalf("%d candidates, want %d", len(c.Candidates), pickerBackgrounds)
	}
	holdSpeed(t, b, limit, ss.timeGets(t, token, imagesPath(id)+"?kind=background"))
}

// TestChooseCandidatesFirstSpeed times the first look at an artist's
// backgrounds: fanart.tv asked, and twelve previews fetched and rendered,
// against a stand-in that answers at once: Kitsune's own cost, without the
// gates' waits.
func TestChooseCandidatesFirstSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "choose-candidates-first")
	ss := newScrapeServer(t)
	token := ss.tokenFor(t, store.RoleAdmin)
	took := make([]time.Duration, 0, scrapeRuns)
	for i := range scrapeRuns {
		id := ss.manyBackgrounds(t, i)
		start := time.Now()
		r := ss.send(t, call{method: http.MethodGet, path: imagesPath(id) + "?kind=background", header: bearer(token)})
		took = append(took, time.Since(start))
		if r.Status != http.StatusOK {
			t.Fatalf("status %d: %s", r.Status, r.Body)
		}
	}
	holdSpeed(t, b, limit, took)
}
