package server

import (
	"encoding/json/v2"
	"net/http"
	"testing"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/store"
)

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
