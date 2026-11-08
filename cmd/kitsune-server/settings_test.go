package main

import (
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/lastfm/lastfmtest"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	settingsURLPath = apiPrefix + "/admin/settings"
	// envSecret and webSecret are shared secrets no log or response may hold.
	envSecret = "env-5ecret-never-logged"
	webSecret = "web-5ecret-never-logged"

	quickScanKey = "library.quick_scan"
	quickScanEnv = "KITSUNE_LIBRARY_QUICK_SCAN"
	logLevelKey  = "log.level"
	resetCmd     = "reset"
)

// token logs simon in and returns a bearer token.
func (ra *runningApp) token(t *testing.T) string {
	t.Helper()
	res, err := ra.app.auth.Login(t.Context(), simonName, goodPassword, "test/1")
	if err != nil {
		t.Fatal(err)
	}
	return res.Token
}

// api sends a request as simon, with a JSON body when body isn't "".
func (ra *runningApp) api(t *testing.T, method, path, body string) reply {
	t.Helper()
	h := http.Header{"Authorization": {"Bearer " + ra.token(t)}}
	if body != "" {
		h.Set(contentTypeKey, jsonMediaType)
	}
	return ra.do(t, method, path, body, h)
}

// settings reads GET /admin/settings.
func (ra *runningApp) settings(t *testing.T) api.ServerSettings {
	t.Helper()
	var s api.ServerSettings
	decodeInto(t, ra.api(t, http.MethodGet, settingsURLPath, ""), &s)
	return s
}

// patch changes settings and fails the test unless the server took them.
func (ra *runningApp) patch(t *testing.T, body string) {
	t.Helper()
	var s api.ServerSettings
	decodeInto(t, ra.api(t, http.MethodPatch, settingsURLPath, body), &s)
}

// decodeInto decodes a 200 reply into v.
func decodeInto(t *testing.T, r reply, v any) {
	t.Helper()
	if r.status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", r.status, r.body)
	}
	if err := json.Unmarshal([]byte(r.body), v); err != nil {
		t.Fatal(err)
	}
}

// wantSetting fails unless key holds the JSON value from source.
func wantSetting(t *testing.T, got api.ServerSettings, key, value, source string) {
	t.Helper()
	if s := got.Settings[key]; string(s.Value) != value || s.Source != source {
		t.Errorf("%s = %s from %s, want %s from %s", key, s.Value, s.Source, value, source)
	}
}

// warnings counts the log's warnings that a seed didn't apply.
func warnings(t *testing.T, logs *syncBuffer) int {
	t.Helper()
	n := 0
	for _, entry := range logEntries(t, logs) {
		if entry["level"] == "WARN" && strings.Contains(fmt.Sprint(entry["msg"]), "which wins") {
			n++
		}
	}
	return n
}

// noSecret fails if any of texts holds either secret.
func noSecret(t *testing.T, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if strings.Contains(text, envSecret) || strings.Contains(text, webSecret) {
			t.Errorf("a secret leaked: %s", text)
		}
	}
}

// eventually polls ok until it holds, failing the test after pollDeadline.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(pollDeadline)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("%s: not after %v", what, pollDeadline)
		}
		time.Sleep(pollInterval)
	}
}

// startEnvApp is startApp with the environment env.
func startEnvApp(t *testing.T, env map[string]string) (*runningApp, serveEnv) {
	t.Helper()
	e := newServeEnv(t, "")
	e.env = env
	if err := createUser(e.dataDir, simonName, goodPassword, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	return serveApp(t, e), e
}

// TestSettingsSeedOnceThenTheWebWins is the life of a setting across restarts:
// the environment seeds it, the browser changes it, and a restart with the same
// environment keeps the browser's value and warns, naming the key and never the
// value.
func TestSettingsSeedOnceThenTheWebWins(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	ra, e := startEnvApp(t, map[string]string{
		quickScanEnv:                   "04:15",
		"KITSUNE_LASTFM_API_KEY":       "envkey",
		"KITSUNE_LASTFM_SHARED_SECRET": envSecret,
	})
	got := ra.settings(t)
	wantSetting(t, got, quickScanKey, `"04:15"`, api.SettingEnv)
	if s := got.Settings[quickScanKey]; s.From != quickScanEnv {
		t.Errorf("quick_scan from %q, want the variable", s.From)
	}
	if s := got.Settings["lastfm.shared_secret"]; s.Set == nil || !*s.Set || s.Value != nil {
		t.Errorf("secret = %+v, want set and no value", s)
	}
	ra.patch(t, `{"library.quick_scan":"05:00","lastfm.shared_secret":"`+webSecret+`"}`)
	if err := ra.stop(t); err != nil {
		t.Fatal(err)
	}

	again := serveApp(t, e) // A restart with the same environment.
	wantSetting(t, again.settings(t), quickScanKey, `"05:00"`, api.SettingWeb)
	if again.app.live.Snapshot().LastfmSharedSecret != webSecret {
		t.Error("after a restart, the secret is not the web's")
	}
	if n := warnings(t, again.logs); n != 2 {
		t.Errorf("restart warned %d times, want once per key the web changed: %s", n, again.logs)
	}
	noSecret(t, ra.logs.String(), again.logs.String(), again.api(t, http.MethodGet, settingsURLPath, "").body)
}

// lastfmConfigured reads whether Last.fm is set up, as the You page does.
func (ra *runningApp) lastfmConfigured(t *testing.T) bool {
	t.Helper()
	var status api.LastfmStatus
	decodeInto(t, ra.api(t, http.MethodGet, apiPrefix+"/me/lastfm", ""), &status)
	return status.Configured
}

// TestSettingsLastfmKeyTurnsConnectOn is an administrator adding a Last.fm key
// in the browser: Connect appears on the You page without a restart.
func TestSettingsLastfmKeyTurnsConnectOn(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	fake := lastfmtest.New(t)
	e := newServeEnv(t, "")
	e.lastfmURL = fake.URL
	if err := createUser(e.dataDir, simonName, goodPassword, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	ra := serveApp(t, e)
	if ra.lastfmConfigured(t) {
		t.Fatal("Last.fm is set up before any key")
	}
	ra.patch(t, `{"lastfm.api_key":"`+lastfmtest.APIKey+`","lastfm.shared_secret":"`+lastfmtest.Secret+`"}`)
	eventually(t, "Last.fm configured", func() bool { return ra.lastfmConfigured(t) })
	var connect api.LastfmConnect
	decodeInto(t, ra.api(t, http.MethodPost, apiPrefix+"/me/lastfm/connect", `{}`), &connect)
	if !strings.HasPrefix(connect.URL, fake.URL+"/api/auth/") || !strings.Contains(connect.URL, "api_key="+lastfmtest.APIKey) {
		t.Errorf("connect URL = %q, want the stand-in's approval page with the new key", connect.URL)
	}
	if strings.Contains(ra.logs.String(), lastfmtest.Secret) {
		t.Errorf("the log holds the secret: %s", ra.logs)
	}

	ra.patch(t, `{"lastfm.shared_secret":null}`)
	eventually(t, "Last.fm off again", func() bool { return !ra.lastfmConfigured(t) })
}

// TestSettingsNewRootStartsAScan is an administrator adding a library folder:
// the scan starts at once and the songs appear.
func TestSettingsNewRootStartsAScan(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	ra := startApp(t, "")
	ra.waitReady(t)
	sample, err := os.ReadFile(filepath.Join("..", "..", "internal", "library", "testdata", "sample.flac"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close() //nolint:errcheck // The file is written; nothing to report.
	if err = dir.WriteFile("00.flac", sample, 0o600); err != nil {
		t.Fatal(err)
	}
	ra.patch(t, fmt.Sprintf(`{"library.roots":[%q]}`, root))
	eventually(t, "the new folder's song", func() bool {
		var facts api.LibraryFacts
		decodeInto(t, ra.api(t, http.MethodGet, apiPrefix+"/library", ""), &facts)
		return facts.Tracks == 1 && facts.Roots == 1
	})
	if logMessages(t, ra.logs)["library folders changed: scanning"] == nil {
		t.Errorf("no scan logged for the new folder: %s", ra.logs)
	}
	waitFingerprinted(t, ra.app.store)
}

// TestSettingsLogLevelChangesLive turns debug logging on in the browser.
func TestSettingsLogLevelChangesLive(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newServeEnv(t, "")
	e.level = new(slog.LevelVar)
	if err := createUser(e.dataDir, simonName, goodPassword, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	ra := serveApp(t, e)
	if e.level.Level() != slog.LevelInfo {
		t.Fatalf("level = %v, want the default info", e.level.Level())
	}
	ra.patch(t, `{"log.level":"debug"}`)
	eventually(t, "debug logging", func() bool { return e.level.Level() == slog.LevelDebug })
}

// TestSettingsTagWritesTurnOnLive lets an administrator edit tags once the
// switch is on, without a restart.
func TestSettingsTagWritesTurnOnLive(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	ra := startApp(t, "")
	canEdit := func() bool {
		var me api.Me
		decodeInto(t, ra.api(t, http.MethodGet, apiPrefix+"/me", ""), &me)
		return me.CanEditTags
	}
	if canEdit() {
		t.Fatal("tag writes are on by default")
	}
	ra.patch(t, `{"library.tag_writes":true}`)
	eventually(t, "tag writes on", canEdit)
}

func TestSetFlags(t *testing.T) {
	t.Parallel()
	var s setFlags
	if err := s.Set(logLevelKey + "=debug"); err != nil || len(s) != 1 || s.String() != logLevelKey {
		t.Errorf("Set = %v, %v", s, err)
	}
	for _, bad := range []string{logLevelKey, "nope=1", logLevelKey + "=loud", "lastfm.shared_secret=x"} {
		if err := s.Set(bad); err == nil {
			t.Errorf("Set(%q) took it", bad)
		} else if strings.Contains(err.Error(), "=x") {
			t.Errorf("Set(%q) = %v holds the value", bad, err)
		}
	}
}

func TestSeedsForPrefersFlagThenEnvThenFile(t *testing.T) {
	t.Parallel()
	e := newServeEnv(t, "")
	body := fmt.Sprintf("[log]\nlevel = \"warn\"\n[library]\nquick_scan = \"01:00\"\ntag_writes = true\n[storage]\ndata_dir = %q\n", e.dataDir)
	e.config = writeFile(t, "server.toml", body)
	e.env = map[string]string{"KITSUNE_LOG_LEVEL": "error", quickScanEnv: "02:00"}
	var flags setFlags
	if err := flags.Set(logLevelKey + "=debug"); err != nil {
		t.Fatal(err)
	}
	cfg, keys, err := config.LoadKeys(e.config, config.DefaultPaths(noEnv))
	if err != nil {
		t.Fatal(err)
	}
	seeds, err := seedsFor(&cfg, keys, e.lookup, flags)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range seeds {
		got[string(s.Key)] = string(s.Source)
	}
	want := map[string]string{logLevelKey: "flag", quickScanKey: "env", "library.tag_writes": "server.toml"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("seeds = %v, want %v", got, want)
	}
	e.env = map[string]string{"KITSUNE_LIBRARY_TAG_WRITES": "maybe"}
	if _, err = seedsFor(&cfg, keys, e.lookup, nil); err == nil || !strings.Contains(err.Error(), "KITSUNE_LIBRARY_TAG_WRITES") {
		t.Errorf("bad env = %v, want it named", err)
	}
}

// settingsCLI runs kitsune-server settings with args.
func settingsCLI(args ...string) cliResult {
	var out, errOut strings.Builder
	code := cli(append([]string{"settings"}, args...), noEnv, strings.NewReader(""), &out, &errOut)
	return cliResult{code: code, stdout: out.String(), stderr: errOut.String()}
}

func TestSettingsReset(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	ra, e := startEnvApp(t, map[string]string{"KITSUNE_LOG_LEVEL": "warn"})
	ra.patch(t, `{"log.level":"error"}`)
	if err := ra.stop(t); err != nil {
		t.Fatal(err)
	}
	if r := settingsCLI(resetCmd, "-config", e.config, logLevelKey); r.code != exitOK || !strings.HasPrefix(r.stdout, "cleared log.level") {
		t.Errorf("reset = %+v", r)
	}
	if r := settingsCLI(resetCmd, "-config", e.config, logLevelKey); r.code != exitOK || !strings.Contains(r.stdout, "no value") {
		t.Errorf("second reset = %+v", r)
	}
	again := serveApp(t, e) // The environment seeds it again.
	wantSetting(t, again.settings(t), logLevelKey, `"warn"`, api.SettingEnv)
}

func TestSettingsCLIRefusesBadCommands(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{}, {resetCmd}, {resetCmd, "storage.data_dir"}, {"nope"}, {resetCmd, "-x"}} {
		if r := settingsCLI(args...); r.code != exitUsage {
			t.Errorf("settings %v = %+v, want a usage error", args, r)
		}
	}
	if r := settingsCLI(resetCmd, "-config", newServeEnv(t, "").config, logLevelKey); r.code != exitFailure {
		t.Errorf("reset without a database = %+v, want a failure", r)
	}
}
