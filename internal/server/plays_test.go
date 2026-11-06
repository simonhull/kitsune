package server

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	playsPath  = "/me/plays"
	firstTrack = "01J00000000000000000001001"
	playClient = "c1"
	playSource = "web"
)

// playBody returns a report of firstTrack at the given time.
func playBody(t *testing.T, at time.Time) string {
	t.Helper()
	b, err := json.Marshal(&api.PlayReport{TrackID: firstTrack, PlayedAt: at, Source: playSource, ClientID: playClient})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// postPlay sends POST /me/plays as the holder of token.
func (ts *testServer) postPlay(t *testing.T, token, body string) response {
	t.Helper()
	return ts.send(t, call{method: http.MethodPost, path: playsPath, body: body, header: jsonHeader(token)})
}

// wantNoContent fails unless r is a 204.
func wantNoContent(t *testing.T, r response) {
	t.Helper()
	if r.Status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", r.Status, r.Body)
	}
}

// plays returns everything a user has played.
func (ts *testServer) plays(t *testing.T, userID int64) []store.Play {
	t.Helper()
	plays, err := ts.Store.RecentPlays(t.Context(), userID, 100)
	if err != nil {
		t.Fatal(err)
	}
	return plays
}

// playFixture is a server with one album and a signed-in admin.
func playFixture(t *testing.T) (*testServer, session) {
	t.Helper()
	ts := newTestServer(t)
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 1})
	sess := ts.sessionFor(t)
	return ts, sess
}

func TestPostPlayRecordsIt(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	at := ts.Clock.Now().Add(-time.Hour)

	wantNoContent(t, ts.postPlay(t, sess.token, playBody(t, at)))

	got := ts.plays(t, sess.user.ID)
	if len(got) != 1 || got[0].TrackID != firstTrack || !got[0].PlayedAt.Equal(at) || got[0].Source != playSource {
		t.Errorf("plays = %+v, want one of %s at %s", got, firstTrack, at)
	}
}

func TestPostPlayCountsARetryOnce(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	body := playBody(t, ts.Clock.Now())
	for range 3 {
		wantNoContent(t, ts.postPlay(t, sess.token, body))
	}
	if got := len(ts.plays(t, sess.user.ID)); got != 1 {
		t.Errorf("plays = %d, want 1", got)
	}
}

func TestPostPlayClampsTheFuture(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	wantNoContent(t, ts.postPlay(t, sess.token, playBody(t, ts.Clock.Now().Add(24*time.Hour))))
	if got := ts.plays(t, sess.user.ID); len(got) != 1 || !got[0].PlayedAt.Equal(ts.Clock.Now()) {
		t.Errorf("plays = %+v, want one at %s", got, ts.Clock.Now())
	}
}

func TestPostPlayIsTheCallersOwn(t *testing.T) {
	t.Parallel()
	ts, admin := playFixture(t)
	other := ts.seedUser(t, "ada", testPassword, store.RoleUser)
	res, err := ts.Auth.Login(t.Context(), "ada", testPassword, "test/1")
	if err != nil {
		t.Fatal(err)
	}
	body := playBody(t, ts.Clock.Now())

	wantNoContent(t, ts.postPlay(t, res.Token, body))
	if got := len(ts.plays(t, admin.user.ID)); got != 0 {
		t.Errorf("the admin has %d of another user's plays", got)
	}
	wantNoContent(t, ts.postPlay(t, admin.token, body))
	if got := len(ts.plays(t, other.ID)); got != 1 {
		t.Errorf("the user's plays = %d, want 1: the admin's report is the admin's", got)
	}
	if got := len(ts.plays(t, admin.user.ID)); got != 1 {
		t.Errorf("the admin's plays = %d, want 1", got)
	}

	naming := strings.Replace(body, "{", fmt.Sprintf(`{"userId":%d,`, other.ID), 1)
	ts.send(t, call{method: http.MethodPost, path: playsPath, body: naming, header: jsonHeader(admin.token), invalid: true}).problem(t, http.StatusBadRequest)
}

func TestPostPlayFindsAMissingTrack(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	album, err := ts.Store.Album(t.Context(), albumULID(1))
	if err != nil {
		t.Fatal(err)
	}
	files, err := ts.Store.FileStates(t.Context(), "/music")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(files))
	for _, f := range files {
		ids = append(ids, f.TrackID)
	}
	if err = ts.Store.MarkMissing(t.Context(), ids, ts.Clock.Now()); err != nil {
		t.Fatal(err)
	}
	wantNoContent(t, ts.postPlay(t, sess.token, playBody(t, ts.Clock.Now())))
	if got := ts.plays(t, sess.user.ID); len(got) != 1 || got[0].TrackID != album.Tracks[0].ID {
		t.Errorf("plays = %+v, want one of %s", got, album.Tracks[0].ID)
	}
}

func TestPostPlayUnknownTrack(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	body := strings.Replace(playBody(t, ts.Clock.Now()), firstTrack, missingID, 1)
	ts.postPlay(t, sess.token, body).problem(t, http.StatusNotFound)
}

func TestPostPlayRefuses(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	at := ts.Clock.Now().Format(time.RFC3339)
	long := strings.Repeat("x", maxPlayField+1)
	report := func(track, played, source, client string) string {
		return fmt.Sprintf(`{"trackId":%q,"playedAt":%q,"source":%q,"clientId":%q}`, track, played, source, client)
	}
	for name, body := range map[string]string{
		"garbage":       "{",
		"no time":       fmt.Sprintf(`{"trackId":%q,"source":"web","clientId":"c"}`, firstTrack),
		"bad time":      report(firstTrack, "yesterday", playSource, playClient),
		"no track":      report("", at, playSource, playClient),
		"no source":     report(firstTrack, at, "", playClient),
		"no client ID":  report(firstTrack, at, playSource, ""),
		"long source":   report(firstTrack, at, long, playClient),
		"long clientID": report(firstTrack, at, playSource, long),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ts.send(t, call{method: http.MethodPost, path: playsPath, body: body, header: jsonHeader(sess.token), invalid: true}).problem(t, http.StatusBadRequest)
		})
	}
	if got := len(ts.plays(t, sess.user.ID)); got != 0 {
		t.Errorf("plays = %d, want 0", got)
	}
}

func TestPostPlayNeedsASession(t *testing.T) {
	t.Parallel()
	ts, _ := playFixture(t)
	h := http.Header{contentType: {api.JSONMediaType}}
	ts.send(t, call{method: http.MethodPost, path: playsPath, body: playBody(t, ts.Clock.Now()), header: h, invalid: true}).problem(t, http.StatusUnauthorized)
}

func TestPostPlayByCookieNeedsTheRequestHeader(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	h := cookieOnly(sess.token)
	h.Set(contentType, api.JSONMediaType)
	body := playBody(t, ts.Clock.Now())
	ts.send(t, call{method: http.MethodPost, path: playsPath, body: body, header: h}).problem(t, http.StatusForbidden)

	h.Set(requestHeader, "1")
	wantNoContent(t, ts.send(t, call{method: http.MethodPost, path: playsPath, body: body, header: h}))
}
