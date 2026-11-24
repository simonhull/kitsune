package scrape_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/scrape/fanarttv/fanarttvtest"
	"github.com/simonhull/kitsune/internal/scrape/wikimedia/wikimediatest"
)

// TestNoErrorQuotesAKey answers every way a source can fail and holds the
// log, every stored error and the report to naming no key.
func TestNoErrorQuotesAKey(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		set  func(*fanarttvtest.Server, *wikimediatest.Server)
	}{
		{"busy", func(f *fanarttvtest.Server, w *wikimediatest.Server) {
			f.Fail("0", http.StatusTooManyRequests, 429, 429, 429)
			w.Fail("0", 429, 429, 429, 429)
		}},
		{"down", func(f *fanarttvtest.Server, w *wikimediatest.Server) {
			f.Fail("", 503, 503, 503, 503)
			w.Fail("", 503, 503, 503, 503)
		}},
		{"refused", func(f *fanarttvtest.Server, w *wikimediatest.Server) {
			f.Want("another")
			w.Fail("", http.StatusForbidden)
		}},
		{"bare names", func(f *fanarttvtest.Server, w *wikimediatest.Server) { f.BareNames(true); w.BareNames(true) }},
		{"slow", func(f *fanarttvtest.Server, w *wikimediatest.Server) { f.Slow(time.Second); w.Slow(time.Second) }},
		{"huge", func(f *fanarttvtest.Server, w *wikimediatest.Server) { f.Huge(true); w.Huge(true) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t)
			id := r.addArtist(t, 1, false)
			r.standInArt(1)
			c.set(r.ft, r.wm)
			r.s.Pass(t.Context())
			words := []string{r.log.String()}
			for _, st := range r.states(t, id) {
				words = append(words, st.Error)
			}
			rep, err := r.s.Report(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			for _, src := range rep.Sources {
				if src.Failure != nil {
					words = append(words, src.Failure.Words)
				}
			}
			for _, w := range words {
				if strings.Contains(w, fanarttvtest.Key) || strings.Contains(w, personalKey) {
					t.Errorf("%q quotes a key", w)
				}
			}
		})
	}
}
