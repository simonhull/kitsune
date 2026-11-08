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

const listensPath = "/me/listens"

// listenBody returns a report of a listen to firstTrack.
func listenBody(t *testing.T, r *api.ListenReport) string {
	t.Helper()
	if r.TrackID == "" {
		r.TrackID = firstTrack
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// postListen sends POST /me/listens as the holder of token.
func (ts *testServer) postListen(t *testing.T, token, body string) response {
	t.Helper()
	return ts.send(t, call{method: http.MethodPost, path: listensPath, body: body, header: jsonHeader(token)})
}

// listens returns a user's listens, newest first.
func (ts *testServer) listens(t *testing.T, userID int64) []store.Listen {
	t.Helper()
	got, err := ts.Store.RecentListens(t.Context(), userID, 100)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPostListenJoinsItsStartAndEnd(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	at := ts.Clock.Now().Add(-time.Minute)
	start := listenBody(t, &api.ListenReport{ClientID: "l1", StartedAt: at, Shuffle: "s1"})
	end := listenBody(t, &api.ListenReport{ClientID: "l1", StartedAt: at, Shuffle: "s1", Ended: "skipped", ListenedMs: 9000})

	for _, body := range []string{start, start, end, end, start} {
		wantNoContent(t, ts.postListen(t, sess.token, body))
	}
	got := ts.listens(t, sess.user.ID)
	want := store.Listen{
		TrackID: firstTrack, ClientID: "l1", Shuffle: "s1", StartedAt: at,
		Listened: 9 * time.Second, Duration: got[0].Duration, Ended: store.Skipped,
	}
	if len(got) != 1 || got[0] != want {
		t.Errorf("listens = %+v, want [%+v]", got, want)
	}
	if plays := ts.plays(t, sess.user.ID); len(plays) != 0 {
		t.Errorf("plays = %+v: a listen is not a play", plays)
	}
}

func TestPostListenClampsTheFuture(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	body := listenBody(t, &api.ListenReport{ClientID: "l1", StartedAt: ts.Clock.Now().Add(time.Hour), Ended: "completed"})
	wantNoContent(t, ts.postListen(t, sess.token, body))
	if got := ts.listens(t, sess.user.ID); len(got) != 1 || !got[0].StartedAt.Equal(ts.Clock.Now()) {
		t.Errorf("listens = %+v, want one at %s", got, ts.Clock.Now())
	}
}

func TestPostListenIsTheCallersOwn(t *testing.T) {
	t.Parallel()
	ts, admin := playFixture(t)
	other := ts.seedUser(t, "ada", testPassword, store.RoleUser)
	res, err := ts.Auth.Login(t.Context(), "ada", testPassword, "test/1")
	if err != nil {
		t.Fatal(err)
	}
	body := listenBody(t, &api.ListenReport{ClientID: "same", StartedAt: ts.Clock.Now()})
	wantNoContent(t, ts.postListen(t, res.Token, body))
	wantNoContent(t, ts.postListen(t, admin.token, body))
	if a, b := len(ts.listens(t, admin.user.ID)), len(ts.listens(t, other.ID)); a != 1 || b != 1 {
		t.Errorf("listens: admin %d, user %d, want one each: a client ID is the caller's own", a, b)
	}
}

func TestPostListenUnknownTrack(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	body := listenBody(t, &api.ListenReport{TrackID: missingID, ClientID: "l1", StartedAt: ts.Clock.Now()})
	ts.postListen(t, sess.token, body).problem(t, http.StatusNotFound)
}

func TestPostListenRefuses(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	at := ts.Clock.Now().Format(time.RFC3339)
	long := strings.Repeat("x", maxPlayField+1)
	for name, body := range map[string]string{
		"cut short":     "{",
		"no time":       fmt.Sprintf(`{"trackId":%q,"clientId":"c"}`, firstTrack),
		"no track":      fmt.Sprintf(`{"clientId":"c","startedAt":%q}`, at),
		"no client ID":  fmt.Sprintf(`{"trackId":%q,"startedAt":%q}`, firstTrack, at),
		"long clientID": fmt.Sprintf(`{"trackId":%q,"clientId":%q,"startedAt":%q}`, firstTrack, long, at),
		"long shuffle":  fmt.Sprintf(`{"trackId":%q,"clientId":"c","startedAt":%q,"shuffle":%q}`, firstTrack, at, long),
		"bad ending":    fmt.Sprintf(`{"trackId":%q,"clientId":"c","startedAt":%q,"ended":"paused"}`, firstTrack, at),
		"negative":      fmt.Sprintf(`{"trackId":%q,"clientId":"c","startedAt":%q,"listenedMs":-1}`, firstTrack, at),
		"names a user":  fmt.Sprintf(`{"trackId":%q,"clientId":"c","startedAt":%q,"userId":2}`, firstTrack, at),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ts.send(t, call{method: http.MethodPost, path: listensPath, body: body, header: jsonHeader(sess.token), invalid: true}).problem(t, http.StatusBadRequest)
		})
	}
	if got := ts.listens(t, sess.user.ID); len(got) != 0 {
		t.Errorf("listens = %+v, want none", got)
	}
}

func TestPostListenNeedsASession(t *testing.T) {
	t.Parallel()
	ts, _ := playFixture(t)
	h := http.Header{contentType: {api.JSONMediaType}}
	body := listenBody(t, &api.ListenReport{ClientID: "l1", StartedAt: ts.Clock.Now()})
	ts.send(t, call{method: http.MethodPost, path: listensPath, body: body, header: h, invalid: true}).problem(t, http.StatusUnauthorized)
}

func TestPostListenByCookieNeedsTheRequestHeader(t *testing.T) {
	t.Parallel()
	ts, sess := playFixture(t)
	h := cookieOnly(sess.token)
	h.Set(contentType, api.JSONMediaType)
	body := listenBody(t, &api.ListenReport{ClientID: "l1", StartedAt: ts.Clock.Now()})
	ts.send(t, call{method: http.MethodPost, path: listensPath, body: body, header: h}).problem(t, http.StatusForbidden)
}
