package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
)

const searchPath = "/search"

// searchFor returns the path of a search for q.
func searchFor(q string) string { return searchPath + "?q=" + url.QueryEscape(q) }

func TestSearchTracks(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 2})

	var got api.SearchResults
	ts.getJSON(t, token, searchFor("track 2"), &got)
	if len(got.Tracks) != 1 {
		t.Fatalf("tracks = %+v, want one", got.Tracks)
	}
	hit := got.Tracks[0]
	if hit.Title != "Track 2" || hit.AlbumID != albumULID(1) || hit.Album != "Album 1" ||
		hit.DurationMs != time.Minute.Milliseconds() || len(hit.Artists) != 1 || hit.Artists[0] != annName {
		t.Errorf("hit = %+v", hit)
	}
}

func TestSearchNeedsCredentials(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.send(t, call{method: http.MethodGet, path: searchFor("x"), invalid: true}).problem(t, http.StatusUnauthorized)
}

func TestSearchWithNoQueryFindsNothing(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 1})
	for _, path := range []string{searchPath, searchFor(""), searchFor("  "), searchFor("!?")} {
		var got api.SearchResults
		ts.getJSON(t, token, path, &got)
		if got.Artists == nil || got.Albums == nil || got.Tracks == nil || len(got.Tracks) != 0 {
			t.Errorf("%s = %+v, want empty groups", path, got)
		}
	}
}

// TestSearchHostileQueries sends what took a read connection down in review, all
// at once, and then checks that an ordinary search still works.
func TestSearchHostileQueries(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	token := ts.sessionFor(t).token
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 1})
	queries := []string{
		strings.Repeat("a ", 5000), strings.Repeat("a", 40_000), "\x00", `foo"bar`, `"`, "*", "AND", "!",
	}
	var wg sync.WaitGroup
	for _, q := range queries {
		wg.Go(func() {
			resp, err := ts.trySend(t, call{method: http.MethodGet, path: searchFor(q), header: bearer(token)})
			if err != nil {
				t.Error(err)
			} else if resp.Status != http.StatusOK {
				t.Errorf("q = %.20q: %d: %s", q, resp.Status, resp.Body)
			}
		})
	}
	wg.Wait()
	var got api.SearchResults
	ts.getJSON(t, token, searchFor("track"), &got)
	if len(got.Tracks) != 1 {
		t.Errorf("after the attack: %+v", got)
	}
}

func TestSearchAbortedIsNotAnError(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, searchFor("track"), http.NoBody)
	ts.Handle.getSearch(httptest.NewRecorder(), req)
	if logs := ts.Log.String(); strings.Contains(logs, "level=ERROR") {
		t.Errorf("an aborted search was logged as a fault: %s", logs)
	}
}
