package server

import (
	"encoding/json/v2"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/lastfm/lastfmtest"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	lastfmPath     = "/me/lastfm"
	connectPath    = "/me/lastfm/connect"
	nowPlayingPath = "/me/now-playing"
	fakeUser       = "fakeuser"
	scrobbleMethod = "track.scrobble"
	albumOne       = "Album 1"
	trackOne       = "Track 1"
	trackTwo       = "Track 2"
)

// lastfmFixture is a server whose sender talks to a fake Last.fm, with one
// album of two tracks and simon signed in by cookie.
type lastfmFixture struct {
	ts   *testServer
	fake *lastfmtest.Server
	sess session
}

func newLastfmFixture(t *testing.T) *lastfmFixture {
	t.Helper()
	fake := lastfmtest.New(t)
	ts := newTestServer(t, testServerOptions{Lastfm: fake, LogLevel: slog.LevelDebug})
	ts.seedAlbums(t, browseAlbum{n: 1, sortName: "a", tracks: 2})
	return &lastfmFixture{ts: ts, fake: fake, sess: ts.sessionFor(t)}
}

// noRedirects is a contract client that hands back redirects instead of
// following them.
func noRedirects(t *testing.T) *http.Client {
	t.Helper()
	c := contract.Client(t)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// cookies is a Cookie header with the session cookie and any others.
func cookies(token string, more ...string) http.Header {
	return http.Header{cookieHeader: {strings.Join(append([]string{sessionCookie + "=" + token}, more...), "; ")}}
}

// started is a connect in progress: the last.fm page and the state cookie.
type started struct {
	authURL *url.URL
	state   string
}

// connectStart posts the connect as the holder of token and returns the
// last.fm URL and the state cookie.
func (f *lastfmFixture) connectStart(t *testing.T, token string) started {
	t.Helper()
	h := cookies(token)
	h.Set(requestHeader, "1")
	resp := f.ts.send(t, call{method: http.MethodPost, path: connectPath, header: h})
	if resp.Status != http.StatusOK {
		t.Fatalf("connect = %d: %s", resp.Status, resp.Body)
	}
	var body api.LastfmConnect
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(body.URL)
	if err != nil {
		t.Fatal(err)
	}
	stateCookie := stateCookieOf(t, resp)
	return started{authURL: u, state: stateCookie}
}

// stateCookieOf returns the state cookie resp sets, after checking how it is set.
func stateCookieOf(t *testing.T, resp response) string {
	t.Helper()
	for _, c := range (&http.Response{Header: resp.Header}).Cookies() {
		if c.Name != lastfmStateCookie {
			continue
		}
		if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge != 600 {
			t.Errorf("state cookie = %+v, want HttpOnly, Lax, path / and 10 minutes", c)
		}
		return c.Value
	}
	t.Fatal("connect set no state cookie")
	return ""
}

// approve visits the fake's approval page and returns the callback, as a path
// under the API, that last.fm sends the browser to.
func (f *lastfmFixture) approve(t *testing.T, authURL *url.URL) string {
	t.Helper()
	client := f.fake.Client().HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, authURL.String(), http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(loc, f.ts.URL+lastfmCallbackPath+"?") {
		t.Fatalf("approval = %d to %q, want a redirect to the callback", resp.StatusCode, loc)
	}
	return strings.TrimPrefix(loc, f.ts.URL)
}

// callback follows Last.fm back with the cookies in h and returns the result
// the You page is told.
func (f *lastfmFixture) callback(t *testing.T, path string, h http.Header) string {
	t.Helper()
	resp := f.ts.send(t, call{method: http.MethodGet, path: path, header: h, client: noRedirects(t)})
	if resp.Status != http.StatusSeeOther || len(resp.Body) != 0 {
		t.Fatalf("callback = %d %q, want an empty 303", resp.Status, resp.Body)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || loc.Path != youPath {
		t.Fatalf("callback Location = %q, want %s", resp.Header.Get("Location"), youPath)
	}
	if !strings.Contains(resp.Header.Get("Set-Cookie"), lastfmStateCookie+"=;") {
		t.Errorf("callback Set-Cookie = %q, want the state cookie cleared", resp.Header.Get("Set-Cookie"))
	}
	return loc.Query().Get("lastfm")
}

// connect runs the whole round trip for simon.
func (f *lastfmFixture) connect(t *testing.T) {
	t.Helper()
	c := f.connectStart(t, f.sess.token)
	path := f.approve(t, c.authURL)
	if got := f.callback(t, path, cookies(f.sess.token, lastfmStateCookie+"="+c.state)); got != lastfmConnected {
		t.Fatalf("callback result = %q, want connected; log:\n%s", got, f.ts.Log)
	}
}

func (f *lastfmFixture) status(t *testing.T) api.LastfmStatus {
	t.Helper()
	var s api.LastfmStatus
	f.ts.getJSON(t, f.sess.token, lastfmPath, &s)
	return s
}

// eventually waits up to a few seconds for ok, which the sender makes true.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLastfmNotSetUp(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	var s api.LastfmStatus
	ts.getJSON(t, sess.token, lastfmPath, &s)
	if s != (api.LastfmStatus{State: api.LastfmOK}) {
		t.Errorf("status = %+v, want not configured, not connected", s)
	}
	ts.send(t, call{method: http.MethodPost, path: connectPath, header: bearer(sess.token)}).problem(t, http.StatusConflict)
	resp := ts.send(t, call{method: http.MethodGet, path: lastfmCallbackPath + "?token=x&state=y", header: cookies(sess.token), client: noRedirects(t)})
	if resp.Status != http.StatusSeeOther || resp.Header.Get("Location") != "/you?lastfm=failed" {
		t.Errorf("callback = %d to %q, want 303 to failed", resp.Status, resp.Header.Get("Location"))
	}
	wantNoContent(t, ts.send(t, call{method: http.MethodPost, path: nowPlayingPath, body: `{"trackId":"x"}`, header: jsonHeader(sess.token)}))
}

func TestLastfmConnectRoundTrip(t *testing.T) {
	t.Parallel()
	f := newLastfmFixture(t)
	if s := f.status(t); !s.Configured || s.Connected {
		t.Fatalf("before = %+v, want configured and not connected", s)
	}
	c := f.connectStart(t, f.sess.token)
	if c.authURL.Query().Get("api_key") != lastfmtest.APIKey || !strings.HasPrefix(c.authURL.String(), f.fake.URL+"/api/auth/") {
		t.Errorf("auth URL = %s", c.authURL)
	}
	if cb := c.authURL.Query().Get("cb"); cb != f.ts.URL+lastfmCallbackPath+"?state="+c.state {
		t.Errorf("cb = %s, want the callback with the state", cb)
	}
	path := f.approve(t, c.authURL)
	if got := f.callback(t, path, cookies(f.sess.token, lastfmStateCookie+"="+c.state)); got != lastfmConnected {
		t.Fatalf("result = %q, want connected; log:\n%s", got, f.ts.Log)
	}
	want := api.LastfmStatus{Configured: true, Connected: true, Name: fakeUser, Scrobble: true, NowPlaying: true, Loves: true, State: api.LastfmOK}
	if s := f.status(t); s != want {
		t.Errorf("after = %+v, want %+v", s, want)
	}
	// The state is used up: the same callback again fails.
	if got := f.callback(t, path, cookies(f.sess.token, lastfmStateCookie+"="+c.state)); got != lastfmFailed {
		t.Errorf("replayed callback = %q, want failed", got)
	}
}

func TestLastfmCallbackRefusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(t *testing.T, f *lastfmFixture) string
		want string
	}{
		{name: "wrong state", want: lastfmFailed, run: func(t *testing.T, f *lastfmFixture) string {
			t.Helper()
			c := f.connectStart(t, f.sess.token)
			path := strings.Replace(f.approve(t, c.authURL), url.QueryEscape(c.state), "nope", 1)
			return f.callback(t, path, cookies(f.sess.token, lastfmStateCookie+"="+c.state))
		}},
		{name: "cookie does not match", want: lastfmFailed, run: func(t *testing.T, f *lastfmFixture) string {
			t.Helper()
			c := f.connectStart(t, f.sess.token)
			return f.callback(t, f.approve(t, c.authURL), cookies(f.sess.token, lastfmStateCookie+"=other"))
		}},
		{name: "expired state", want: lastfmFailed, run: func(t *testing.T, f *lastfmFixture) string {
			t.Helper()
			c := f.connectStart(t, f.sess.token)
			path := f.approve(t, c.authURL)
			f.ts.Clock.Advance(lastfmStateLife + time.Second)
			return f.callback(t, path, cookies(f.sess.token, lastfmStateCookie+"="+c.state))
		}},
		{name: "another user's state", want: lastfmFailed, run: func(t *testing.T, f *lastfmFixture) string {
			t.Helper()
			c := f.connectStart(t, f.sess.token)
			other := f.ts.tokenFor(t, store.RoleUser)
			return f.callback(t, f.approve(t, c.authURL), cookies(other, lastfmStateCookie+"="+c.state))
		}},
		{name: "no session", want: lastfmFailed, run: func(t *testing.T, f *lastfmFixture) string {
			t.Helper()
			c := f.connectStart(t, f.sess.token)
			return f.callback(t, f.approve(t, c.authURL), http.Header{cookieHeader: {lastfmStateCookie + "=" + c.state}})
		}},
		{name: "said no", want: lastfmRefused, run: func(t *testing.T, f *lastfmFixture) string {
			t.Helper()
			c := f.connectStart(t, f.sess.token)
			path := lastfmCallbackPath + "?" + url.Values{"state": {c.state}}.Encode()
			return f.callback(t, path, cookies(f.sess.token, lastfmStateCookie+"="+c.state))
		}},
		{name: "unknown token", want: lastfmFailed, run: func(t *testing.T, f *lastfmFixture) string {
			t.Helper()
			c := f.connectStart(t, f.sess.token)
			path := lastfmCallbackPath + "?" + url.Values{"state": {c.state}, "token": {"forged"}}.Encode()
			return f.callback(t, path, cookies(f.sess.token, lastfmStateCookie+"="+c.state))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newLastfmFixture(t)
			if got := tt.run(t, f); got != tt.want {
				t.Errorf("result = %q, want %q", got, tt.want)
			}
			if s := f.status(t); s.Connected {
				t.Errorf("status = %+v, want still not connected", s)
			}
		})
	}
}

// playsBody is a play of track n.
func playsBody(n int, at time.Time) string {
	return `{"trackId":"` + trackOf(n) + `","playedAt":"` + at.Format(time.RFC3339) + `","source":"web","clientId":"c` + strconv.Itoa(n) + `"}`
}

func trackOf(n int) string { return "01J00000000000000000001" + strconv.Itoa(1000 + n)[1:] }

func TestLastfmPlayIsScrobbled(t *testing.T) {
	t.Parallel()
	f := newLastfmFixture(t)
	f.connect(t)
	at := f.ts.Clock.Now().Add(-4 * time.Minute)
	wantNoContent(t, f.ts.send(t, call{method: http.MethodPost, path: playsPath, body: playsBody(1, at), header: jsonHeader(f.sess.token)}))
	eventually(t, "the scrobble", func() bool { return len(f.fake.CallsTo(scrobbleMethod)) == 1 })
	p := f.fake.CallsTo(scrobbleMethod)[0].Params
	for k, v := range map[string]string{
		"artist[0]": annName, "track[0]": trackOne, "album[0]": albumOne, "albumArtist[0]": annName,
		"timestamp[0]": strconv.FormatInt(at.Unix(), 10), "duration[0]": "60", "trackNumber[0]": "1",
		"sk": "sk-token-1", "api_key": lastfmtest.APIKey,
	} {
		if got := p.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if p.Get("api_sig") == "" {
		t.Error("the scrobble is not signed") // The fake checked the signature itself: the call succeeded.
	}
	eventually(t, "the send recorded", func() bool { return !f.status(t).LastSentAt.IsZero() })
	if s := f.status(t); s.Waiting != 0 || !s.LastSentAt.Equal(f.ts.Clock.Now()) {
		t.Errorf("status = %+v, want nothing waiting, sent now", s)
	}
}

func TestLastfmHeartIsLoved(t *testing.T) {
	t.Parallel()
	f := newLastfmFixture(t)
	f.connect(t)
	wantNoContent(t, f.ts.send(t, heartCall(http.MethodPut, f.sess.token, kindTracks, firstTrack)))
	wantNoContent(t, f.ts.send(t, heartCall(http.MethodPut, f.sess.token, kindAlbums, albumULID(1)))) // Sends nothing.
	eventually(t, "the love", func() bool { return len(f.fake.CallsTo("track.love")) == 1 })
	if p := f.fake.CallsTo("track.love")[0].Params; p.Get("artist") != annName || p.Get("track") != trackOne {
		t.Errorf("love = %v", p)
	}
	wantNoContent(t, f.ts.send(t, heartCall(http.MethodDelete, f.sess.token, kindTracks, firstTrack)))
	eventually(t, "the unlove", func() bool { return len(f.fake.CallsTo("track.unlove")) == 1 })
}

func TestLastfmOutageWaitsThenSends(t *testing.T) {
	t.Parallel()
	f := newLastfmFixture(t)
	f.connect(t)
	f.fake.Fail(11)
	at := f.ts.Clock.Now()
	wantNoContent(t, f.ts.send(t, call{method: http.MethodPost, path: playsPath, body: playsBody(1, at), header: jsonHeader(f.sess.token)}))
	eventually(t, "a wait", func() bool { return f.status(t).State == api.LastfmWaiting })
	if s := f.status(t); s.Waiting != 1 || !s.RetryAt.Equal(at.Add(time.Minute)) {
		t.Errorf("status = %+v, want 1 waiting, retried in a minute", s)
	}
	f.ts.Clock.Advance(time.Minute)
	wantNoContent(t, f.ts.send(t, heartCall(http.MethodPut, f.sess.token, kindTracks, firstTrack))) // Wakes the sender.
	eventually(t, "the scrobble", func() bool { return len(f.fake.Scrobbles()) == 1 })
	eventually(t, "an empty outbox", func() bool { s := f.status(t); return s.Waiting == 0 && s.State == api.LastfmOK })
}

func TestLastfmRejectedSessionNeedsReconnecting(t *testing.T) {
	t.Parallel()
	f := newLastfmFixture(t)
	f.connect(t)
	f.fake.Fail(9)
	wantNoContent(t, f.ts.send(t, call{method: http.MethodPost, path: playsPath, body: playsBody(1, f.ts.Clock.Now()), header: jsonHeader(f.sess.token)}))
	eventually(t, "reconnect", func() bool { return f.status(t).State == api.LastfmReconnect })
	if s := f.status(t); s.Waiting != 1 || !s.Connected {
		t.Errorf("status = %+v, want the play kept", s)
	}
	f.connect(t) // Reconnecting sends what waited.
	eventually(t, "the scrobble", func() bool { return len(f.fake.Scrobbles()) == 1 })
}

func TestLastfmSwitchesAndDisconnect(t *testing.T) {
	t.Parallel()
	f := newLastfmFixture(t)
	patch := func(body string) response {
		return f.ts.send(t, call{method: http.MethodPatch, path: lastfmPath, body: body, header: jsonHeader(f.sess.token)})
	}
	patch(`{"scrobble":false}`).problem(t, http.StatusNotFound)
	f.connect(t)
	resp := patch(`{"scrobble":false,"loves":false}`)
	var s api.LastfmStatus
	if err := json.Unmarshal(resp.Body, &s); err != nil || resp.Status != http.StatusOK {
		t.Fatalf("patch = %d %s", resp.Status, resp.Body)
	}
	if s.Scrobble || s.Loves || !s.NowPlaying {
		t.Errorf("switches = %+v, want only now playing on", s)
	}
	c := call{method: http.MethodPatch, path: lastfmPath, body: `{"scrobble":"yes"}`, header: jsonHeader(f.sess.token), invalid: true}
	f.ts.send(t, c).problem(t, http.StatusBadRequest)
	byCookie := cookies(f.sess.token)
	byCookie.Set(contentType, api.JSONMediaType)
	f.ts.send(t, call{method: http.MethodPatch, path: lastfmPath, body: `{}`, header: byCookie}).problem(t, http.StatusForbidden)

	wantNoContent(t, f.ts.send(t, call{method: http.MethodPost, path: playsPath, body: playsBody(1, f.ts.Clock.Now()), header: jsonHeader(f.sess.token)}))
	if s = f.status(t); s.Waiting != 0 {
		t.Errorf("waiting = %d with scrobbling off, want 0", s.Waiting)
	}
	patch(`{"scrobble":true}`)
	f.fake.SetDown(true)
	wantNoContent(t, f.ts.send(t, call{method: http.MethodPost, path: playsPath, body: playsBody(2, f.ts.Clock.Now()), header: jsonHeader(f.sess.token)}))
	eventually(t, "a wait", func() bool { return f.status(t).Waiting == 1 })
	f.ts.send(t, call{method: http.MethodDelete, path: lastfmPath, header: cookies(f.sess.token)}).problem(t, http.StatusForbidden)
	wantNoContent(t, f.ts.send(t, call{method: http.MethodDelete, path: lastfmPath, header: bearer(f.sess.token)}))
	if s = f.status(t); s.Connected || s.Waiting != 0 {
		t.Errorf("after disconnect = %+v, want not connected, nothing waiting", s)
	}
}

func TestLastfmNowPlaying(t *testing.T) {
	t.Parallel()
	f := newLastfmFixture(t)
	f.connect(t)
	post := func(h http.Header, body string, invalid bool) response {
		return f.ts.send(t, call{method: http.MethodPost, path: nowPlayingPath, body: body, header: h, invalid: invalid})
	}
	wantNoContent(t, post(jsonHeader(f.sess.token), `{"trackId":"`+trackOf(2)+`"}`, false))
	eventually(t, "now playing", func() bool { return len(f.fake.CallsTo("track.updateNowPlaying")) == 1 })
	if p := f.fake.CallsTo("track.updateNowPlaying")[0].Params; p.Get("track") != trackTwo || p.Get("artist") != annName {
		t.Errorf("now playing = %v", p)
	}
	post(jsonHeader(f.sess.token), `{}`, true).problem(t, http.StatusBadRequest)
	byCookie := cookies(f.sess.token)
	byCookie.Set(contentType, api.JSONMediaType)
	post(byCookie, `{"trackId":"x"}`, false).problem(t, http.StatusForbidden)
}

func TestLastfmNeedsAuthentication(t *testing.T) {
	t.Parallel()
	f := newLastfmFixture(t)
	for _, c := range []call{
		{method: http.MethodGet, path: lastfmPath},
		{method: http.MethodPatch, path: lastfmPath, body: `{}`, header: http.Header{contentType: {api.JSONMediaType}}},
		{method: http.MethodDelete, path: lastfmPath},
		{method: http.MethodPost, path: connectPath},
		{method: http.MethodPost, path: nowPlayingPath, body: `{"trackId":"x"}`, header: http.Header{contentType: {api.JSONMediaType}}},
	} {
		c.invalid = true
		f.ts.send(t, c).problem(t, http.StatusUnauthorized)
	}
	byCookie := cookies(f.sess.token)
	f.ts.send(t, call{method: http.MethodPost, path: connectPath, header: byCookie}).problem(t, http.StatusForbidden)
}

// TestLastfmKeepsSecrets runs every Last.fm route, failures included, and
// finds neither the shared secret nor a session key in any response or log.
func TestLastfmKeepsSecrets(t *testing.T) {
	t.Parallel()
	f := newLastfmFixture(t)
	f.connect(t)
	f.fake.Fail(11, 9)
	wantNoContent(t, f.ts.send(t, call{method: http.MethodPost, path: playsPath, body: playsBody(1, f.ts.Clock.Now()), header: jsonHeader(f.sess.token)}))
	eventually(t, "a wait", func() bool { return f.status(t).State == api.LastfmWaiting })
	f.ts.Clock.Advance(time.Minute)
	wantNoContent(t, f.ts.send(t, call{method: http.MethodPost, path: nowPlayingPath, body: `{"trackId":"` + trackOf(1) + `"}`, header: jsonHeader(f.sess.token)}))
	wantNoContent(t, f.ts.send(t, heartCall(http.MethodPut, f.sess.token, kindTracks, firstTrack)))
	eventually(t, "reconnect", func() bool { return f.status(t).State == api.LastfmReconnect })
	var bodies strings.Builder
	for _, c := range []call{
		{method: http.MethodGet, path: lastfmPath, header: bearer(f.sess.token)},
		{method: http.MethodPatch, path: lastfmPath, body: `{"loves":false}`, header: jsonHeader(f.sess.token)},
	} {
		resp := f.ts.send(t, c)
		bodies.Write(resp.Body)
		for k, v := range resp.Header {
			bodies.WriteString(k + ": " + strings.Join(v, ",") + "\n")
		}
	}
	if f.fake.HasSecret(bodies.String()) {
		t.Errorf("a response holds a secret:\n%s", bodies.String())
	}
	log := f.ts.Log.String()
	if !strings.Contains(log, "last.fm unavailable") || !strings.Contains(log, "needs reconnecting") {
		t.Errorf("the log lacks the failures:\n%s", log)
	}
	if f.fake.HasSecret(log) || strings.Contains(log, "token-") {
		t.Errorf("the log holds a secret or a token:\n%s", log)
	}
}
