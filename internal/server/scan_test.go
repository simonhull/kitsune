package server

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"net/http"
	"reflect"
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

func TestLibraryShowsTheFingerprintPass(t *testing.T) {
	t.Parallel()
	if got := toAPIFingerprints(library.FingerprintProgress{Done: 3, Total: 40}); got != (api.FingerprintStatus{Done: 3, Total: 40}) {
		t.Errorf("toAPIFingerprints = %+v, want 3 of 40", got)
	}
	ts := newTestServer(t, testServerOptions{LibraryRoot: t.TempDir()})
	if got := ts.getLibrary(t, ts.tokenFor(t, store.RoleUser)).Fingerprints; got != (api.FingerprintStatus{}) {
		t.Errorf("fingerprints with no pass = %+v, want zero", got)
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

const (
	issuesPath  = "/admin/library/issues"
	issuesRoot  = "/srv/music"
	warnedTrack = "01J00000000000000000000001"
)

// seedIssues writes an unsupported file, a corrupted one and a track with a
// warning, each mentioning the root.
func (ts *testServer) seedIssues(t *testing.T) {
	t.Helper()
	batch := store.Batch{
		Issues: []store.ScanIssue{
			{Root: issuesRoot, RelPath: "a/1.wav", Kind: store.IssueUnsupported, Message: issuesRoot + "/a/1.wav: unsupported format: wav"},
			{Root: issuesRoot, RelPath: "a/2.flac", Kind: store.IssueCorrupted, Message: "corrupted: " + issuesRoot + "/a/2.flac"},
			{Root: issuesRoot, RelPath: "a/3.wav", Kind: store.IssueUnsupported, Message: issuesRoot + "/a/3.wav: unsupported format: wav"},
		},
		Tracks: []store.TrackWrite{{
			ULID: warnedTrack, Root: issuesRoot, RelPath: "b/1.flac", Format: flacFormat, Codec: flacFormat, Title: "T",
			Album:    store.AlbumWrite{GroupKey: "g", ULID: albumULID(1), Name: "A", SortName: "a", Artist: annName},
			Warnings: []string{"odd tag in " + issuesRoot + "/b/1.flac"},
		}},
	}
	if err := ts.Store.WriteBatch(t.Context(), &batch); err != nil {
		t.Fatal(err)
	}
}

func TestIssuesListsDetailWithoutTheRoot(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedIssues(t)
	resp := ts.send(t, call{method: http.MethodGet, path: issuesPath, header: bearer(ts.tokenFor(t, store.RoleAdmin))})
	if bytes.Contains(resp.Body, []byte(issuesRoot)) {
		t.Errorf("the response holds the root: %s", resp.Body)
	}
	var got api.ScanIssues
	if err := json.Unmarshal(resp.Body, &got); err != nil {
		t.Fatalf("decode %q: %v", resp.Body, err)
	}
	want := api.ScanIssues{
		Issues: []api.ScanIssue{
			{Path: "a/1.wav", Kind: "unsupported", Message: "a/1.wav: unsupported format: wav"},
			{Path: "a/2.flac", Kind: "corrupted", Message: "corrupted: a/2.flac"},
			{Path: "a/3.wav", Kind: "unsupported", Message: "a/3.wav: unsupported format: wav"},
		},
		Warnings: []api.TrackWarned{{TrackID: warnedTrack, Path: "b/1.flac", Messages: []string{"odd tag in b/1.flac"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("issues = %+v, want %+v", got, want)
	}
}

func TestIssuesListsEmptyArrays(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	var got api.ScanIssues
	ts.getJSON(t, ts.tokenFor(t, store.RoleAdmin), issuesPath, &got)
	if got.Issues == nil || got.Warnings == nil || len(got.Issues)+len(got.Warnings) != 0 {
		t.Errorf("issues = %+v, want two empty lists", got)
	}
}

func TestIssuesAreForAdminsAndCountsForEveryone(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.seedIssues(t)
	token := ts.tokenFor(t, store.RoleUser)
	ts.send(t, call{method: http.MethodGet, path: issuesPath, header: bearer(token)}).problem(t, http.StatusForbidden)
	ts.send(t, call{method: http.MethodGet, path: issuesPath, invalid: true}).problem(t, http.StatusUnauthorized)
	facts := ts.getLibrary(t, token)
	if want := (api.IssueCounts{Corrupted: 1, Unsupported: 2, Warnings: 1}); facts.IssueCounts != want {
		t.Errorf("counts = %+v, want %+v", facts.IssueCounts, want)
	}
	if facts.Issues != 3 {
		t.Errorf("issues = %d, want 3: a warned track plays", facts.Issues)
	}
}
