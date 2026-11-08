package server

import (
	"encoding/json/v2"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	rulesPath    = "/me/shuffle/rules"
	runsPath     = "/me/shuffle/runs"
	excludedPath = "/me/shuffle/excluded"
	thirdTrack   = "01J00000000000000000001003"
)

// shuffleFixture is a server with one three-track album and a signed-in admin.
func shuffleFixture(t *testing.T) (*testServer, session) {
	t.Helper()
	ts := newTestServer(t)
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 3})
	sess := ts.sessionFor(t)
	return ts, sess
}

// indexMarks returns the shuffle marks the index gives the holder of token, by track.
func (ts *testServer) indexMarks(t *testing.T, token string) map[string]*api.ShuffleMark {
	t.Helper()
	var index api.LibraryIndex
	ts.getJSON(t, token, indexPath+"?fields=shuffle", &index)
	marks := map[string]*api.ShuffleMark{}
	for _, tr := range index.Artists[0].Albums[0].Tracks {
		if tr.TrackFields != nil {
			marks[tr.ID] = tr.Shuffle
		}
	}
	return marks
}

func tracksBody(ids ...string) string {
	return `{"tracks":["` + strings.Join(ids, `","`) + `"]}`
}

func (ts *testServer) rulesOf(t *testing.T, token string, ids ...string) api.ShuffleRules {
	t.Helper()
	var out api.ShuffleRules
	ts.getJSON(t, token, rulesPath+"?tracks="+strings.Join(ids, ","), &out)
	return out
}

func TestCombineSongsIntoARun(t *testing.T) {
	t.Parallel()
	ts, sess := shuffleFixture(t)
	resp := ts.send(t, call{method: http.MethodPut, path: runsPath, body: tracksBody(thirdTrack, firstTrack), header: jsonHeader(sess.token)})
	var run api.ShuffleRun
	resp.decode(t, &run)
	if len(run.ID) != 26 || !slices.Equal(run.Tracks, []string{thirdTrack, firstTrack}) {
		t.Errorf("run = %+v", run)
	}
	rules := ts.rulesOf(t, sess.token, firstTrack)
	if len(rules.Runs) != 1 || rules.Runs[0].ID != run.ID || !slices.Equal(rules.Runs[0].Tracks, run.Tracks) {
		t.Errorf("rules = %+v, want the whole run", rules)
	}
	if m := ts.indexMarks(t, sess.token)[thirdTrack]; m == nil || m.Run != run.ID || m.Position != 1 || m.Length != 2 {
		t.Errorf("index mark of the run's first = %+v", m)
	}
}

func TestARunIsItsPersonsOwn(t *testing.T) {
	t.Parallel()
	ts, sess := shuffleFixture(t)
	var run api.ShuffleRun
	ts.send(t, call{method: http.MethodPut, path: runsPath, body: tracksBody(firstTrack, secondTrack), header: jsonHeader(sess.token)}).decode(t, &run)
	other := ts.tokenFor(t, store.RoleUser)
	if got := ts.rulesOf(t, other, firstTrack); len(got.Runs) != 0 {
		t.Errorf("another person's rules = %+v, want none", got)
	}
	ts.send(t, call{method: http.MethodDelete, path: runsPath + "/" + run.ID, header: bearer(other)}).problem(t, http.StatusNotFound)
	if resp := ts.send(t, call{method: http.MethodDelete, path: runsPath + "/" + run.ID, header: bearer(sess.token)}); resp.Status != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", resp.Status, resp.Body)
	}
	if got := ts.rulesOf(t, sess.token, firstTrack); len(got.Runs) != 0 {
		t.Errorf("rules after delete = %+v", got)
	}
}

func TestRunRefusals(t *testing.T) {
	t.Parallel()
	ts, sess := shuffleFixture(t)
	tests := []struct {
		name, body string
		want       int
		invalid    bool
	}{
		{"one song", tracksBody(firstTrack), http.StatusBadRequest, false},
		{"a song twice", tracksBody(firstTrack, firstTrack), http.StatusBadRequest, false},
		{"no songs", `{"tracks":[]}`, http.StatusBadRequest, true},
		{"an unknown song", tracksBody(firstTrack, missingID), http.StatusNotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := call{method: http.MethodPut, path: runsPath, body: tt.body, header: jsonHeader(sess.token), invalid: tt.invalid}
			ts.send(t, c).problem(t, tt.want)
		})
	}
	ts.send(t, call{method: http.MethodGet, path: rulesPath + "?tracks=", header: bearer(sess.token)}).problem(t, http.StatusBadRequest)
	ts.send(t, call{method: http.MethodGet, path: rulesPath + "?tracks=x", header: bearer("nope")}).problem(t, http.StatusUnauthorized)
	ts.send(t, call{method: http.MethodPut, path: runsPath, body: tracksBody(firstTrack, secondTrack), header: jsonHeader("nope")}).problem(t, http.StatusUnauthorized)
	ts.send(t, call{method: http.MethodDelete, path: runsPath + "/x", header: bearer("nope")}).problem(t, http.StatusUnauthorized)
}

func TestExcludeSongs(t *testing.T) {
	t.Parallel()
	ts, sess := shuffleFixture(t)
	put := func(method, body string) response {
		return ts.send(t, call{method: method, path: excludedPath, body: body, header: jsonHeader(sess.token)})
	}
	if resp := put(http.MethodPut, tracksBody(firstTrack, secondTrack)); resp.Status != http.StatusNoContent {
		t.Fatalf("exclude = %d: %s", resp.Status, resp.Body)
	}
	if resp := put(http.MethodDelete, tracksBody(secondTrack)); resp.Status != http.StatusNoContent {
		t.Fatalf("let back = %d: %s", resp.Status, resp.Body)
	}
	if got := ts.rulesOf(t, sess.token, firstTrack, secondTrack, thirdTrack); !slices.Equal(got.Excluded, []string{firstTrack}) {
		t.Errorf("excluded = %q, want the first alone", got.Excluded)
	}
	put(http.MethodPut, tracksBody(missingID)).problem(t, http.StatusNotFound)
	put(http.MethodDelete, tracksBody(missingID)).problem(t, http.StatusNotFound)
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		c := call{method: method, path: excludedPath, body: `{"tracks":[]}`, header: jsonHeader(sess.token), invalid: true}
		ts.send(t, c).problem(t, http.StatusBadRequest)
		ts.send(t, call{method: method, path: excludedPath, body: tracksBody(firstTrack), header: jsonHeader("nope")}).problem(t, http.StatusUnauthorized)
	}
}

func TestShuffleRulesBodyIsJSON(t *testing.T) {
	t.Parallel()
	ts, sess := shuffleFixture(t)
	resp := ts.send(t, call{method: http.MethodGet, path: rulesPath + "?tracks=" + firstTrack, header: bearer(sess.token)})
	var raw map[string]any
	if err := json.Unmarshal(resp.Body, &raw); err != nil || raw["runs"] == nil || raw["excluded"] == nil {
		t.Errorf("body = %s, want empty lists, not nulls", resp.Body)
	}
}
