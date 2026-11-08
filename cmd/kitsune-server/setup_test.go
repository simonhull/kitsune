package main

import (
	"encoding/json/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	setupURLPath = apiPrefix + "/setup"
	firstRunMsg  = "no accounts yet: open Kitsune in a browser to make the first one"
)

// setupCodeShape is what every setup code looks like.
var setupCodeShape = regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{4}(-[0-9a-hjkmnp-tv-z]{4}){3}$`)

// firstRunLog is what the first-run log line says.
type firstRunLog struct {
	url  any
	code string
}

// setupLine returns what the first-run log line says, failing the test unless
// there is exactly one such line and the code appears nowhere else.
func setupLine(t *testing.T, ra *runningApp) firstRunLog {
	t.Helper()
	var found []map[string]any
	for _, e := range logEntries(t, ra.logs) {
		if e["msg"] == firstRunMsg {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d first-run log lines, want 1; log: %s", len(found), ra.logs)
	}
	code, _ := found[0]["setupCode"].(string) // Checked by its shape next.
	if !setupCodeShape.MatchString(code) {
		t.Fatalf("setupCode = %q, want four groups of four Crockford base32 characters", code)
	}
	if found[0]["level"] != "WARN" {
		t.Errorf("first-run line level = %v, want WARN", found[0]["level"])
	}
	if n := strings.Count(ra.logs.String(), code); n != 1 {
		t.Errorf("the code appears %d times in the log, want once", n)
	}
	return firstRunLog{url: found[0]["url"], code: code}
}

// postSetup posts a setup request with the given forwarded address.
func (ra *runningApp) postSetup(t *testing.T, req api.SetupRequest, forwarded string) reply {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	h := http.Header{contentTypeKey: {jsonMediaType}}
	if forwarded != "" {
		h.Set(forwardedFor, forwarded)
	}
	return ra.do(t, http.MethodPost, setupURLPath, string(body), h)
}

// setupStatus gets GET /setup with the given forwarded address.
func (ra *runningApp) setupStatus(t *testing.T, forwarded string) api.SetupStatus {
	t.Helper()
	h := http.Header{}
	if forwarded != "" {
		h.Set(forwardedFor, forwarded)
	}
	r := ra.do(t, http.MethodGet, setupURLPath, "", h)
	var st api.SetupStatus
	if err := json.Unmarshal([]byte(r.body), &st); r.status != http.StatusOK || err != nil {
		t.Fatalf("GET /setup = %d %q, want 200 and a SetupStatus", r.status, r.body)
	}
	return st
}

func TestServeStartsWithoutAccounts(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newServeEnv(t, "") // Not even data_dir exists yet.
	ra := serveApp(t, e)
	ra.waitReady(t)
	if info, err := os.Stat(e.dataDir); err != nil || info.Mode().Perm() != dataDirMode {
		t.Errorf("data_dir = %v, %v; want it made with mode 0700", info, err)
	}
	line := setupLine(t, ra)
	code := line.code
	if want := "http://" + ra.ln.Addr().String(); line.url != want {
		t.Errorf("url = %v, want %s", line.url, want)
	}
	if got := ra.setupStatus(t, ""); got != (api.SetupStatus{Needed: true}) {
		t.Errorf("status = %+v, want needed without the code from loopback", got)
	}
	r := ra.postSetup(t, api.SetupRequest{Name: simonName, Password: goodPassword}, "")
	if r.status != http.StatusOK || len(r.cookies) != 1 || strings.Contains(r.body, code) {
		t.Fatalf("POST /setup = %d %q with cookies %q, want 200, one cookie and no code", r.status, r.body, r.cookies)
	}
	if got := ra.setupStatus(t, ""); got.Needed {
		t.Errorf("status after setup = %+v, want not needed", got)
	}
	ra.expectStatus(t, ra.login(t, "", simonName, goodPassword, "", false), http.StatusOK)
	ra.checkCodeLoggedOnce(t, code)
}

// checkCodeLoggedOnce stops the app and checks that code appears exactly once
// in everything it logged.
func (ra *runningApp) checkCodeLoggedOnce(t *testing.T, code string) {
	t.Helper()
	if err := ra.stop(t); err != nil {
		t.Errorf("serve = %v, want nil", err)
	}
	if n := strings.Count(ra.logs.String(), code); n != 1 {
		t.Errorf("the code appears %d times in the log after setup, want once", n)
	}
}

func TestServeStartsWithEmptyDatabase(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newServeEnv(t, "")
	if err := os.MkdirAll(e.dataDir, dataDirMode); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.Context(), filepath.Join(e.dataDir, store.FileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	ra := serveApp(t, e)
	ra.waitReady(t)
	setupLine(t, ra)
}

func TestServeSetupBehindProxy(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newServeEnv(t, "base_url = \"https://music.example.com\"\ntrusted_proxies = [\"127.0.0.1\"]\n")
	ra := serveApp(t, e)
	ra.waitReady(t)
	line := setupLine(t, ra)
	if line.url != "https://music.example.com" {
		t.Errorf("url = %v, want base_url", line.url)
	}
	if got := ra.setupStatus(t, proxiedAddr); got != (api.SetupStatus{Needed: true, CodeRequired: true}) {
		t.Errorf("status from outside = %+v, want needed with the code", got)
	}
	simon := api.SetupRequest{Name: simonName, Password: goodPassword}
	ra.expectStatus(t, ra.postSetup(t, simon, proxiedAddr), http.StatusForbidden)
	simon.Code = "0000-0000-0000-0000"
	ra.expectStatus(t, ra.postSetup(t, simon, proxiedAddr), http.StatusForbidden)
	simon.Code = strings.ToUpper(line.code)
	ra.expectStatus(t, ra.postSetup(t, simon, proxiedAddr), http.StatusOK)
	ra.expectStatus(t, ra.postSetup(t, simon, proxiedAddr), http.StatusConflict)
}

func TestServeWithAccountsLogsNoCode(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	ra := startApp(t, "")
	ra.waitReady(t)
	if logMessages(t, ra.logs)[firstRunMsg] != nil {
		t.Errorf("a server with accounts logged the first-run line: %s", ra.logs)
	}
	if got := ra.setupStatus(t, ""); got.Needed {
		t.Errorf("status = %+v, want not needed", got)
	}
}

func TestSetupURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		baseURL, listen, want string
	}{
		{"https://music.example.com/k", "0.0.0.0:4533", "https://music.example.com/k"},
		{"", "127.0.0.1:4533", "http://127.0.0.1:4533"},
		{"", "0.0.0.0:4533", "http://localhost:4533"},
		{"", "[::]:4533", "http://localhost:4533"},
		{"", "[2001:db8::1]:4533", "http://[2001:db8::1]:4533"},
	}
	for _, tt := range tests {
		addr, err := net.ResolveTCPAddr("tcp", tt.listen)
		if err != nil {
			t.Fatal(err)
		}
		if got := setupURL(tt.baseURL, addr); got != tt.want {
			t.Errorf("setupURL(%q, %s) = %q, want %q", tt.baseURL, tt.listen, got, tt.want)
		}
	}
}
