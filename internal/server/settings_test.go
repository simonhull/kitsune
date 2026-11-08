package server

import (
	"encoding/json/v2"
	"net/http"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	settingsPath = "/admin/settings"
	// secretValue is a Last.fm shared secret no response, header or log may hold.
	secretValue = "f00dfacecafe5ecre7"
)

// getSettings reads the settings as the holder of token.
func (ts *testServer) getSettings(t *testing.T, token string) response {
	t.Helper()
	return ts.send(t, call{method: http.MethodGet, path: settingsPath, header: bearer(token)})
}

// patchSettings changes settings as the holder of token.
func (ts *testServer) patchSettings(t *testing.T, token, body string, invalid bool) response {
	t.Helper()
	return ts.send(t, call{method: http.MethodPatch, path: settingsPath, body: body, header: jsonHeader(token), invalid: invalid})
}

// decodeSettings decodes a 200 settings response.
func decodeSettings(t *testing.T, r response) api.ServerSettings {
	t.Helper()
	if r.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", r.Status, r.Body)
	}
	var s api.ServerSettings
	if err := json.Unmarshal(r.Body, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// noSecret fails if r or the server's log holds the shared secret.
func (ts *testServer) noSecret(t *testing.T, r response) {
	t.Helper()
	if strings.Contains(string(r.Body), secretValue) {
		t.Errorf("body holds the secret: %s", r.Body)
	}
	for k, vs := range r.Header {
		for _, v := range vs {
			if strings.Contains(v, secretValue) {
				t.Errorf("header %s holds the secret", k)
			}
		}
	}
	if strings.Contains(ts.Log.String(), secretValue) {
		t.Errorf("log holds the secret: %s", ts.Log)
	}
}

func TestSettingsReadsEveryKey(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	r := ts.getSettings(t, sess.token)
	if cc := r.Header.Get("Cache-Control"); cc != noStore {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	got := decodeSettings(t, r)
	if len(got.Settings) != 9 || got.Listen != testListen || got.ListenSource != api.SettingDefault || got.Version != testVersion {
		t.Errorf("settings = %+v", got)
	}
	checkSecret(t, got, false, api.SettingDefault)
	if base := got.Settings[baseURLKey]; !base.RestartRequired || base.PendingRestart {
		t.Errorf("base_url = %+v, want restartRequired and nothing pending", base)
	}
	checkSetting(t, got, "log.level", `"info"`, api.SettingDefault)
}

const (
	secretKey  = "lastfm.shared_secret" //nolint:gosec // G101: a setting's name, not a credential.
	baseURLKey = "server.base_url"
)

// checkSetting fails unless key holds the JSON value from source.
func checkSetting(t *testing.T, got api.ServerSettings, key, value, source string) {
	t.Helper()
	if s := got.Settings[key]; string(s.Value) != value || s.Source != source {
		t.Errorf("%s = %s from %s, want %s from %s", key, s.Value, s.Source, value, source)
	}
}

// checkSecret fails unless the shared secret is set or not, from source, and
// carries no value.
func checkSecret(t *testing.T, got api.ServerSettings, set bool, source string) {
	t.Helper()
	s := got.Settings[secretKey]
	if s.Set == nil || *s.Set != set || s.Value != nil || s.Source != source {
		t.Errorf("secret = %+v, want set %v from %s and no value", s, set, source)
	}
}

func TestSettingsAreForAdministrators(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	user := ts.tokenFor(t, store.RoleUser)
	ts.getSettings(t, user).problem(t, http.StatusForbidden)
	ts.patchSettings(t, user, `{"log.level":"debug"}`, false).problem(t, http.StatusForbidden)
	ts.send(t, call{method: http.MethodGet, path: settingsPath, invalid: true}).problem(t, http.StatusUnauthorized)
	ts.send(t, call{method: http.MethodPatch, path: settingsPath, body: `{}`, header: http.Header{contentType: {api.JSONMediaType}}, invalid: true}).
		problem(t, http.StatusUnauthorized)
}

func TestSettingsChangeAndKeepTheSecret(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	r := ts.patchSettings(t, sess.token,
		`{"lastfm.api_key":"abc123","lastfm.shared_secret":"`+secretValue+`","library.roots":["/srv/music/"],"server.base_url":"https://example.com"}`, false)
	got := decodeSettings(t, r)
	ts.noSecret(t, r)
	checkSecret(t, got, true, api.SettingWeb)
	checkSetting(t, got, "library.roots", `["/srv/music"]`, api.SettingWeb)
	if base := got.Settings[baseURLKey]; !base.PendingRestart {
		t.Errorf("base_url = %+v, want it pending a restart", base)
	}
	if logs := ts.Log.String(); !strings.Contains(logs, "settings changed") || !strings.Contains(logs, secretKey) {
		t.Errorf("log = %s, want the changed keys", logs)
	}
	ts.noSecret(t, ts.getSettings(t, sess.token))
	if snap := ts.Settings.Snapshot(); snap.LastfmSharedSecret != secretValue {
		t.Error("the secret was not kept as written")
	}

	// Clearing the secret leaves it unset.
	got = decodeSettings(t, ts.patchSettings(t, sess.token, `{"lastfm.shared_secret":null}`, false))
	checkSecret(t, got, false, api.SettingDefault)
}

func TestSettingsChangeIsAllOrNothing(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	r := ts.patchSettings(t, sess.token, `{"log.level":"debug","lastfm.shared_secret":"`+secretValue+`","library.roots":["music"]}`, false)
	if p := r.problem(t, http.StatusBadRequest); !strings.HasPrefix(p.Detail, "library.roots: ") {
		t.Errorf("detail = %q, want it to name library.roots", p.Detail)
	}
	ts.noSecret(t, r)
	got := decodeSettings(t, ts.getSettings(t, sess.token))
	checkSetting(t, got, "log.level", `"info"`, api.SettingDefault)
	checkSecret(t, got, false, api.SettingDefault)
}

func TestSettingsRefuseBadBodies(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	for name, body := range map[string]string{
		"unknown key":   `{"server.listen":":80"}`,
		"wrong type":    `{"library.tag_writes":"yes"}`,
		"not an object": `["log.level"]`,
		"null":          `null`,
		"truncated":     `{`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ts.patchSettings(t, sess.token, body, true).problem(t, http.StatusBadRequest)
		})
	}
	h := bearer(sess.token)
	h.Set(contentType, "text/plain")
	ts.send(t, call{method: http.MethodPatch, path: settingsPath, body: `{}`, header: h, invalid: true}).problem(t, http.StatusBadRequest)
}

func TestSettingsApplyNoChange(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	sess := ts.sessionFor(t)
	if got := decodeSettings(t, ts.patchSettings(t, sess.token, `{}`, false)); len(got.Settings) != 9 {
		t.Errorf("settings = %+v", got)
	}
}
