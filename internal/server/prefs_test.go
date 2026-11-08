package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	columnsPath = "/me/prefs/library.columns"
	columnsBody = `{"order":["genre","artist","plays"],"artistWhenDifferent":false}`
	themePath   = "/me/prefs/ui.theme"
)

// putPrefCall sets a preference as the holder of token.
func putPrefCall(token, path, body string) call {
	return call{method: http.MethodPut, path: path, body: body, header: jsonHeader(token)}
}

// pref reads a preference as the holder of token.
func (ts *testServer) pref(t *testing.T, token, path string) response {
	t.Helper()
	return ts.send(t, call{method: http.MethodGet, path: path, header: bearer(token)})
}

func TestPrefsKeepTheCallersValue(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	other := ts.tokenFor(t, store.RoleUser)
	ts.pref(t, sess.token, columnsPath).problem(t, http.StatusNotFound)

	wantNoContent(t, ts.send(t, putPrefCall(sess.token, columnsPath, columnsBody)))
	got := ts.pref(t, sess.token, columnsPath)
	if got.Status != http.StatusOK || string(got.Body) != columnsBody {
		t.Fatalf("GET = %d %s, want 200 %s", got.Status, got.Body, columnsBody)
	}
	if cc := got.Header.Get("Cache-Control"); cc != noStore {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	ts.pref(t, other, columnsPath).problem(t, http.StatusNotFound)

	// A second write replaces the first, spaced however the client likes.
	wantNoContent(t, ts.send(t, putPrefCall(sess.token, columnsPath, "{ \"order\": [],\n \"artistWhenDifferent\": true }")))
	if got = ts.pref(t, sess.token, columnsPath); string(got.Body) != `{"order":[],"artistWhenDifferent":true}` {
		t.Errorf("after a replace, GET = %s", got.Body)
	}
}

func TestPrefsKeepTheTheme(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	ts.pref(t, sess.token, themePath).problem(t, http.StatusNotFound)
	for _, theme := range []string{`"light"`, `"dark"`, `"system"`} {
		wantNoContent(t, ts.send(t, putPrefCall(sess.token, themePath, theme)))
		if got := ts.pref(t, sess.token, themePath); got.Status != http.StatusOK || string(got.Body) != theme {
			t.Errorf("GET after PUT %s = %d %s", theme, got.Status, got.Body)
		}
	}
	// Each key keeps its own value.
	wantNoContent(t, ts.send(t, putPrefCall(sess.token, columnsPath, columnsBody)))
	if got := ts.pref(t, sess.token, themePath); string(got.Body) != `"system"` {
		t.Errorf("theme after a columns write = %s", got.Body)
	}
	for name, body := range map[string]string{
		"unknown":    `"sepia"`,
		"case":       `"Dark"`,
		"columns":    columnsBody,
		"not a name": `true`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := putPrefCall(sess.token, themePath, body)
			c.invalid = name != "columns" // A columns value is valid by the contract, and not for this key.
			ts.send(t, c).problem(t, http.StatusBadRequest)
		})
	}
}

func TestPrefsRefuseUnknownKeys(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	for _, key := range []string{"server.roots", "library.Columns", "ui.Theme"} {
		ts.pref(t, sess.token, "/me/prefs/"+key).problem(t, http.StatusNotFound)
		ts.send(t, putPrefCall(sess.token, "/me/prefs/"+key, columnsBody)).problem(t, http.StatusNotFound)
	}
}

func TestPrefsRefuseBadValues(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	for name, body := range map[string]string{
		"unknown column": `{"order":["skips"],"artistWhenDifferent":true}`,
		"always shown":   `{"order":["title"],"artistWhenDifferent":true}`,
		"twice":          `{"order":["genre","genre"],"artistWhenDifferent":true}`,
		"no switch":      `{"order":["genre"]}`,
		"no order":       `{"artistWhenDifferent":true}`,
		"null order":     `{"order":null,"artistWhenDifferent":true}`,
		"extra":          `{"order":[],"artistWhenDifferent":true,"widths":{}}`,
		"not an object":  `["genre"]`,
		"truncated":      "{",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := putPrefCall(sess.token, columnsPath, body)
			c.invalid = true
			ts.send(t, c).problem(t, http.StatusBadRequest)
		})
	}
	t.Run("too long", func(t *testing.T) {
		t.Parallel()
		// Valid by the contract's schema, and still more than a value may take.
		long := `{"order":[],` + strings.Repeat(" ", 4096) + `"artistWhenDifferent":true}`
		ts.send(t, putPrefCall(sess.token, columnsPath, long)).problem(t, http.StatusBadRequest)
	})
	t.Run("not JSON", func(t *testing.T) {
		t.Parallel()
		c := putPrefCall(sess.token, columnsPath, columnsBody)
		c.header.Set(contentType, "text/plain")
		c.invalid = true
		ts.send(t, c).problem(t, http.StatusBadRequest)
	})
	ts.pref(t, sess.token, columnsPath).problem(t, http.StatusNotFound)
}

func TestPrefsForgetAValueTheirRuleNoLongerTakes(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	// As if a column had since been taken out.
	if err := ts.Store.SetPref(t.Context(), sess.user.ID, "library.columns", []byte(`{"order":["skips"],"artistWhenDifferent":true}`)); err != nil {
		t.Fatal(err)
	}
	ts.pref(t, sess.token, columnsPath).problem(t, http.StatusNotFound)
	if !strings.Contains(ts.Log.String(), "stored preference fails its rule") {
		t.Errorf("log = %s, want the bad value noted", ts.Log)
	}
}

func TestPrefsNeedASession(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	for _, c := range []call{
		{method: http.MethodGet, path: columnsPath},
		{method: http.MethodPut, path: columnsPath, body: columnsBody, header: http.Header{contentType: {api.JSONMediaType}}},
	} {
		c.invalid = true
		ts.send(t, c).problem(t, http.StatusUnauthorized)
	}
}

func TestPrefsByCookieNeedTheRequestHeader(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	c := call{method: http.MethodPut, path: columnsPath, body: columnsBody, header: cookieOnly(sess.token)}
	c.header.Set(contentType, api.JSONMediaType)
	ts.send(t, c).problem(t, http.StatusForbidden)
	c.header.Set(requestHeader, "1")
	wantNoContent(t, ts.send(t, c))
}
