package fanarttv_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/remote"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv/fanarttvtest"
)

const mbid = "5b1c0d2e-0000-4000-8000-0000000000e2"

// personal is a person's key, which must never appear in an error either.
const personal = "fanart-personal-key-of-someone"

var keys = fanarttv.Keys{API: fanarttvtest.Key, Client: personal} //nolint:gochecknoglobals // A test's fixed keys.

func lowTide() fanarttvtest.Artist {
	return fanarttvtest.Artist{
		MBID:        mbid,
		Thumbs:      []fanarttvtest.Image{{ID: "11", Lang: "en", Likes: 3}},
		Backgrounds: []fanarttvtest.Image{{ID: "21", Likes: 1}, {ID: "22", Likes: 5}},
		HDLogos:     []fanarttvtest.Image{{ID: "31", Lang: "en"}},
	}
}

func TestArtist(t *testing.T) {
	t.Parallel()
	ft := fanarttvtest.New(t, lowTide())
	c := fanarttv.New(ft.Options())
	a, err := c.Artist(t.Context(), keys, mbid)
	if err != nil {
		t.Fatal(err)
	}
	checkArtist(t, &a)
	body, err := c.Download(t.Context(), a.HDLogos[0].URL)
	if err != nil || len(body) == 0 {
		t.Fatalf("download = %d bytes, %v", len(body), err)
	}
	checkCalls(t, ft.Calls())
}

// checkArtist holds a to lowTide's images, their languages, likes and sizes.
func checkArtist(t *testing.T, a *fanarttv.Artist) {
	t.Helper()
	if th := a.Thumbs; len(th) != 1 || th[0].Lang != "en" || th[0].Likes != 3 || th[0].Width != 1000 {
		t.Errorf("thumbs = %+v", th)
	}
	if bg := a.Backgrounds; len(bg) != 2 || bg[0].Lang != "" || bg[1].Likes != 5 || bg[1].Height != 1080 {
		t.Errorf("backgrounds = %+v", bg)
	}
}

// checkCalls holds calls to the keys in headers, never in a path, and
// Kitsune's User-Agent.
func checkCalls(t *testing.T, calls []fanarttvtest.Call) {
	t.Helper()
	if len(calls) != 2 || calls[0].APIKey != fanarttvtest.Key || calls[0].ClientKey != personal {
		t.Errorf("calls = %+v, want the keys in headers", calls)
	}
	for _, call := range calls {
		if strings.Contains(call.Path, fanarttvtest.Key) || call.UserAgent != remote.UserAgent("test") {
			t.Errorf("call %+v", call)
		}
	}
}

func TestLatest(t *testing.T) {
	t.Parallel()
	ft := fanarttvtest.New(t)
	ft.SetLatest(mbid)
	got, err := fanarttv.New(ft.Options()).Latest(t.Context(), keys, time.Unix(1_700_000_000, 0))
	if err != nil || len(got) != 1 || got[0] != mbid {
		t.Errorf("latest = %v, %v", got, err)
	}
}

// TestNoErrorQuotesAKey answers every way fanart.tv can fail and holds every
// error to naming no key.
func TestNoErrorQuotesAKey(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		set  func(*fanarttvtest.Server)
		want error
	}{
		{"busy", func(s *fanarttvtest.Server) { s.Fail("1", http.StatusTooManyRequests, 429, 429, 429) }, remote.ErrUnavailable},
		{"down", func(s *fanarttvtest.Server) { s.Fail("", 503, 503, 503, 503) }, remote.ErrUnavailable},
		{"refused", func(s *fanarttvtest.Server) { s.Want("another") }, remote.ErrRefused},
		{"forbidden", func(s *fanarttvtest.Server) { s.Fail("", http.StatusForbidden) }, remote.ErrRefused},
		{"bare names", func(s *fanarttvtest.Server) { s.BareNames(true) }, fanarttv.ErrBadAnswer},
		{"huge", func(s *fanarttvtest.Server) { s.Huge(true) }, remote.ErrTooLarge},
		{"slow", func(s *fanarttvtest.Server) { s.Slow(time.Second) }, remote.ErrUnavailable},
		{"none", func(*fanarttvtest.Server) {}, remote.ErrNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ft := fanarttvtest.New(t, lowTide())
			c.set(ft)
			o := ft.Options()
			o.HTTP.Timeout = 200 * time.Millisecond
			ask := mbid
			if c.name == "none" {
				ask = "00000000-0000-4000-8000-000000000000"
			}
			_, err := fanarttv.New(o).Artist(t.Context(), keys, ask)
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
			if err != nil && (strings.Contains(err.Error(), fanarttvtest.Key) || strings.Contains(err.Error(), personal)) {
				t.Errorf("%q quotes a key", err)
			}
		})
	}
}

func TestNoKeyAsksNothing(t *testing.T) {
	t.Parallel()
	ft := fanarttvtest.New(t, lowTide())
	if _, err := fanarttv.New(ft.Options()).Artist(t.Context(), fanarttv.Keys{Client: personal}, mbid); !errors.Is(err, fanarttv.ErrNoKey) {
		t.Errorf("err = %v, want ErrNoKey", err)
	}
	if n := len(ft.Calls()); n != 0 {
		t.Errorf("%d calls without a project key", n)
	}
}

func TestAnImageElsewhereIsNeverFetched(t *testing.T) {
	t.Parallel()
	ft := fanarttvtest.New(t, lowTide())
	c := fanarttv.New(ft.Options())
	for _, u := range []string{"11.jpg", "http://example.com/fanart/a.jpg", ft.URL + "/fanart/../v3.2/music/" + mbid} {
		if _, err := c.Download(t.Context(), u); !errors.Is(err, fanarttv.ErrBadAnswer) {
			t.Errorf("%s: %v, want ErrBadAnswer", u, err)
		}
	}
	if n := len(ft.Calls()); n != 0 {
		t.Errorf("%d calls for images elsewhere", n)
	}
}
