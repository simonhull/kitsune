package wikimedia_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/remote"
	"github.com/simonhull/kitsune/internal/scrape/wikimedia"
	"github.com/simonhull/kitsune/internal/scrape/wikimedia/wikimediatest"
)

const (
	item    = "Q2831"
	band    = "Low Tide Choir"
	bandDE  = "Low Tide Choir (Band)"
	picture = "Low Tide Choir 2019.jpg"
)

func standIn(t *testing.T) *wikimediatest.Server {
	t.Helper()
	wm := wikimediatest.New(t)
	wm.Add(
		wikimediatest.Item{ID: item, Articles: map[string]string{"en": band, "de": bandDE}, Image: picture},
		[]wikimediatest.Article{
			{Lang: "en", Title: band, Extract: "Low Tide Choir are a band.\nThey formed in 2009."},
			{Lang: "de", Title: bandDE, Disambiguation: true},
		},
		&wikimediatest.File{Name: picture, Author: `<a href="//commons.wikimedia.org/wiki/User:Ann">Ann &amp; Bo</a>`, License: "CC BY-SA 4.0", LicenseURL: "https://creativecommons.org/licenses/by-sa/4.0"},
	)
	return wm
}

func TestAnArtistsBio(t *testing.T) {
	t.Parallel()
	c := wikimedia.New(standIn(t).Options())
	articles, err := c.Articles(t.Context(), item, []string{"de", "en", "not a language"})
	if err != nil || articles["en"] != band || articles["de"] != bandDE {
		t.Fatalf("articles = %v, %v", articles, err)
	}
	sum, err := c.Summary(t.Context(), "en", articles["en"])
	if err != nil || sum.Extract == "" || sum.PageURL != "https://en.wikipedia.org/wiki/Low_Tide_Choir" || sum.Lang != "en" {
		t.Errorf("summary = %+v, %v", sum, err)
	}
	if _, err = c.Summary(t.Context(), "de", articles["de"]); !errors.Is(err, remote.ErrNotFound) {
		t.Errorf("a disambiguation page: %v, want none", err)
	}
}

func TestAnArtistsPortrait(t *testing.T) {
	t.Parallel()
	wm := standIn(t)
	c := wikimedia.New(wm.Options())
	name, err := c.Portrait(t.Context(), item)
	if err != nil || name != picture {
		t.Fatalf("portrait = %q, %v", name, err)
	}
	f, err := c.File(t.Context(), name, 1024)
	if err != nil {
		t.Fatal(err)
	}
	checkFile(t, &f)
	if body, derr := c.Download(t.Context(), f.URL); derr != nil || len(body) == 0 {
		t.Errorf("download: %d bytes, %v", len(body), derr)
	}
	for _, call := range wm.Calls() {
		if call.UserAgent != remote.UserAgent("test") {
			t.Errorf("User-Agent %q", call.UserAgent)
		}
	}
}

// checkFile holds f to the stand-in's portrait and its credit, as plain text.
func checkFile(t *testing.T, f *wikimedia.File) {
	t.Helper()
	if f.Author != "Ann & Bo" || f.License != "CC BY-SA 4.0" || f.LicenseURL == "" || f.PageURL == "" || f.Width != 300 {
		t.Errorf("file = %+v", f)
	}
}

func TestMissingThings(t *testing.T) {
	t.Parallel()
	c := wikimedia.New(standIn(t).Options())
	if _, err := c.Articles(t.Context(), "Q1", []string{"en"}); !errors.Is(err, remote.ErrNotFound) {
		t.Errorf("an item it lacks: %v", err)
	}
	if name, err := c.Portrait(t.Context(), "Q1"); name != "" || err != nil {
		t.Errorf("no image: %q, %v", name, err)
	}
	if _, err := c.File(t.Context(), "Nothing.jpg", 1024); !errors.Is(err, remote.ErrNotFound) {
		t.Errorf("a file it lacks: %v", err)
	}
	if _, err := c.Summary(t.Context(), "en", "Nobody"); !errors.Is(err, remote.ErrNotFound) {
		t.Errorf("an article it lacks: %v", err)
	}
}

func TestFailures(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		set  func(*wikimediatest.Server)
		want error
	}{
		{"busy", func(s *wikimediatest.Server) { s.Fail("1", 429, 429, 429, 429) }, remote.ErrUnavailable},
		{"down", func(s *wikimediatest.Server) { s.Fail("", 503, 503, 503, 503) }, remote.ErrUnavailable},
		{"refused", func(s *wikimediatest.Server) { s.Fail("", http.StatusForbidden) }, remote.ErrRefused},
		{"bare name", func(s *wikimediatest.Server) { s.BareNames(true) }, wikimedia.ErrBadAnswer},
		{"huge", func(s *wikimediatest.Server) { s.Huge(true) }, remote.ErrTooLarge},
		{"slow", func(s *wikimediatest.Server) { s.Slow(time.Second) }, remote.ErrUnavailable},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			wm := standIn(t)
			c.set(wm)
			o := wm.Options()
			o.HTTP.Timeout = timeoutFor(c.name)
			if _, err := wikimedia.New(o).File(t.Context(), picture, 1024); !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestAnImageElsewhereIsNeverFetched(t *testing.T) {
	t.Parallel()
	wm := standIn(t)
	c := wikimedia.New(wm.Options())
	for _, u := range []string{picture, "https://example.com/upload/a.jpg", wm.URL + "/upload/../wikidata/w/api.php"} {
		if _, err := c.Download(t.Context(), u); !errors.Is(err, wikimedia.ErrBadAnswer) {
			t.Errorf("%s: %v, want ErrBadAnswer", u, err)
		}
	}
	if n := len(wm.Calls()); n != 0 {
		t.Errorf("%d calls for images elsewhere", n)
	}
}

func TestPlainText(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		`<a href="x">Ann</a> and <span>Bo</span>`: "Ann and Bo",
		"Caf&eacute;\n  Ann":                      "Café Ann",
		"":                                        "",
	} {
		if got := wikimedia.PlainText(in); got != want {
			t.Errorf("PlainText(%q) = %q, want %q", in, got, want)
		}
	}
}

// timeoutFor is how long a request may take in the failure named: a slow
// stand-in outlasts it, and nothing else comes near it, however busy the
// machine running the tests.
func timeoutFor(name string) time.Duration {
	if name == "slow" {
		return 200 * time.Millisecond
	}
	return time.Minute
}
