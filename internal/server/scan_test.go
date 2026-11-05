package server

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	scanPath = "/admin/library/scan"
	fullScan = `{"mode":"full"}`
)

// jsonHeader returns the headers of a JSON request by the holder of token.
func jsonHeader(token string) http.Header {
	h := bearer(token)
	h.Set(contentType, api.JSONMediaType)
	return h
}

// postScan sends POST /admin/library/scan with body.
func (ts *testServer) postScan(t *testing.T, token, body string, invalid bool) response {
	t.Helper()
	return ts.send(t, call{method: http.MethodPost, path: scanPath, body: body, header: jsonHeader(token), invalid: invalid})
}

// getLibrary returns GET /library as the holder of token sees it.
func (ts *testServer) getLibrary(t *testing.T, token string) api.LibraryFacts {
	t.Helper()
	resp := ts.send(t, call{method: http.MethodGet, path: libraryPath, header: bearer(token)})
	if resp.Status != http.StatusOK {
		t.Fatalf("GET /library = %d: %s", resp.Status, resp.Body)
	}
	var facts api.LibraryFacts
	if err := json.Unmarshal(resp.Body, &facts); err != nil {
		t.Fatalf("decode %q: %v", resp.Body, err)
	}
	return facts
}

// wantScan checks the state and mode of scan.
func wantScan(t *testing.T, scan *api.ScanStatus, state, mode string) {
	t.Helper()
	if scan.State != state || scan.Mode != mode || (scan.StartedAt != nil) != (state == api.ScanRunning) {
		t.Errorf("scan = %+v, want state %q and mode %q", scan, state, mode)
	}
}

func TestLibraryShowsARunningScan(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{LibraryRoot: t.TempDir()})
	token := ts.tokenFor(t, store.RoleUser)
	run, err := ts.Scanner.Claim(library.Full)
	if err != nil {
		t.Fatal(err)
	}
	running := ts.getLibrary(t, token).Scan
	wantScan(t, &running, api.ScanRunning, library.Full.String())
	if _, err = run(t.Context()); err != nil {
		t.Fatal(err)
	}
	idle := ts.getLibrary(t, token).Scan
	wantScan(t, &idle, api.ScanIdle, "")
	if last := idle.Last; last == nil || last.Outcome != api.ScanFinished || last.Mode != library.Full.String() {
		t.Errorf("last = %+v, want a finished Full scan", last)
	}
}

func TestOnlyAdminsSeeWhyAScanFailed(t *testing.T) {
	t.Parallel()
	const reason = "scan: the disk went away"
	failed := library.Progress{Last: &library.ScanResult{Err: reason}}
	for _, admin := range []bool{true, false} {
		want := ""
		if admin {
			want = reason
		}
		if last := toAPIScan(failed, admin).Last; last == nil || last.Outcome != api.ScanFailed || last.Error != want {
			t.Errorf("admin %t: last = %+v, want failed, with error %q", admin, last, want)
		}
	}
}

func TestPostScanRunsAFullScan(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{LibraryRoot: t.TempDir()})
	token := ts.tokenFor(t, store.RoleAdmin)
	resp := ts.postScan(t, token, fullScan, false)
	if resp.Status != http.StatusAccepted {
		t.Fatalf("POST = %d: %s", resp.Status, resp.Body)
	}
	const deadline, poll = 5 * time.Second, time.Millisecond
	for start := time.Now(); time.Since(start) < deadline; time.Sleep(poll) {
		scan := ts.getLibrary(t, token).Scan
		if scan.Last != nil {
			if scan.Last.Mode != library.Full.String() || scan.Last.Outcome != api.ScanFinished {
				t.Errorf("last = %+v, want a finished Full scan", scan.Last)
			}
			return
		}
	}
	t.Error("the scan never ended")
}

func TestPostScanAnswersWithTheRunningScan(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{StartScan: func(library.Mode) error { return nil }})
	token := ts.tokenFor(t, store.RoleAdmin)
	if _, err := ts.Scanner.Claim(library.Quick); err != nil { // What StartScan would have done.
		t.Fatal(err)
	}
	resp := ts.postScan(t, token, `{"mode":"quick"}`, false)
	var scan api.ScanStatus
	if err := json.Unmarshal(resp.Body, &scan); err != nil {
		t.Fatalf("decode %q: %v", resp.Body, err)
	}
	wantScan(t, &scan, api.ScanRunning, library.Quick.String())
}

func TestPostScanIsRefusedWhileOneRuns(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{LibraryRoot: t.TempDir()})
	token := ts.tokenFor(t, store.RoleAdmin)
	run, err := ts.Scanner.Claim(library.Quick)
	if err != nil {
		t.Fatal(err)
	}
	ts.postScan(t, token, fullScan, false).problem(t, http.StatusConflict)
	if _, err = run(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPostScanIsRefusedWhileStopping(t *testing.T) {
	t.Parallel()
	stopping := errors.New("stopping")
	ts := newTestServer(t, testServerOptions{StartScan: func(library.Mode) error { return stopping }})
	ts.postScan(t, ts.tokenFor(t, store.RoleAdmin), fullScan, false).problem(t, http.StatusServiceUnavailable)
}

func TestPostScanIsForAdmins(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{StartScan: func(library.Mode) error {
		t.Error("a scan started")
		return nil
	}})
	ts.postScan(t, ts.tokenFor(t, store.RoleUser), fullScan, false).problem(t, http.StatusForbidden)
	resp := ts.send(t, call{method: http.MethodPost, path: scanPath, body: fullScan, header: http.Header{contentType: {api.JSONMediaType}}, invalid: true})
	resp.problem(t, http.StatusUnauthorized)
}

func TestPostScanRefusesABadBody(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{StartScan: func(library.Mode) error {
		t.Error("a scan started")
		return nil
	}})
	token := ts.tokenFor(t, store.RoleAdmin)
	for _, body := range []string{"", "{}", `{"mode":"slow"}`, `{"mode":"full","extra":1}`, "full"} {
		ts.postScan(t, token, body, true).problem(t, http.StatusBadRequest)
	}
}
