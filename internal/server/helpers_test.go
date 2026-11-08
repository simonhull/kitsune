package server

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/lastfm/lastfmtest"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/openapitest"
	"github.com/simonhull/kitsune/internal/scrobbler"
	"github.com/simonhull/kitsune/internal/settings"
	"github.com/simonhull/kitsune/internal/store"
)

// authorizationHeader is the credentials request header.
const authorizationHeader = "Authorization"

// Paths that several tests request.
const (
	healthPath = "/healthz"
	nopePath   = "/nope"
)

// testClientIPHeader (X-Test-Client-IP, in canonical form) lets a test choose the client IP the server sees. Only
// newTestServer installs the hook that reads it.
const testClientIPHeader = "X-Test-Client-Ip"

// testClock is a settable clock shared by the store, the auth service and the
// server.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.UnixMilli(1_762_000_000_000).UTC()}
}

// Now returns the clock's current time.
func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// syncBuffer is a bytes.Buffer that the server's goroutines and the test can
// use at the same time.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p) //nolint:wrapcheck // bytes.Buffer never fails; pass it through.
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// testServerOptions configures newTestServer.
type testServerOptions struct {
	SecureCookies bool
	Params        auth.Params // Zero means cheap parameters.
	BasePath      string
	Ready         func(context.Context) error
	LogLevel      slog.Level // The zero value is Info.
	Artwork       *artwork.Cache
	Roots         int // Empty library roots to configure, beside LibraryRoot.
	TagWrites     bool
	LibraryRoot   string                   // "" means a scanner with no roots.
	Write         library.WriteFunc        // Nil means real writes.
	StartScan     func(library.Mode) error // Nil means claim and run on the scanner.
	Web           fs.FS                    // Nil means no web client.
	Lastfm        *lastfmtest.Server       // Nil means Last.fm is not set up; else the sender runs against it.
	SetupCode     string                   // "" means the server started with accounts.
	FolderRoot    string                   // What the folder picker calls /. "" means the real /.
	DataDir       string                   // Kitsune's data folder, as the picker sees it.
	Watch         bool                     // Run a folder watcher over the roots.
}

// testServer is a running API over a real store and auth service.
type testServer struct {
	Server   *httptest.Server
	URL      string // The API root: the origin, the base path and /api/v1.
	Clock    *testClock
	Log      *syncBuffer
	Auth     *auth.Service
	Handle   *Server
	Store    *store.Store
	Settings *settings.Service
	Scanner  *library.Scanner
}

// newTestServer starts a server over a temporary store.
func newTestServer(tb testing.TB, opts ...testServerOptions) *testServer {
	tb.Helper()
	var o testServerOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Params == (auth.Params{}) {
		o.Params = auth.Params{Time: 1, MemoryKiB: 64, Threads: 1}
	}
	clock := newTestClock()
	st, err := store.Open(tb.Context(), filepath.Join(tb.TempDir(), store.FileName), store.Options{Now: clock.Now})
	if err != nil {
		tb.Fatalf("open store: %v", err)
	}
	tb.Cleanup(func() {
		if closeErr := st.Close(); closeErr != nil {
			tb.Errorf("close store: %v", closeErr)
		}
	})
	svc, err := auth.New(st, auth.Options{Params: o.Params, Now: clock.Now})
	if err != nil {
		tb.Fatalf("auth.New: %v", err)
	}
	roots := testRoots(tb, &o)
	scanner := library.NewScanner(st, library.Options{
		Roots: roots, MissingGrace: time.Hour, Workers: 2, TagWrites: o.TagWrites, Write: o.Write,
	})
	if o.StartScan == nil {
		var scans sync.WaitGroup
		tb.Cleanup(scans.Wait) // Before the store closes.
		o.StartScan = func(mode library.Mode) error {
			run, claimErr := scanner.Claim(mode)
			if claimErr != nil {
				return claimErr //nolint:wrapcheck // Tests compare it.
			}
			scans.Go(func() { _, _ = run(context.WithoutCancel(tb.Context())) })
			return nil
		}
	}
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: o.LogLevel}))
	var sender *scrobbler.Scrobbler
	if o.Lastfm != nil {
		sender = scrobbler.New(st, scrobbler.Options{
			Client: o.Lastfm.Client(), MBIDKeys: library.TrackMBIDKeys(), Logger: logger, Now: clock.Now,
		})
		ctx, cancel := context.WithCancel(context.WithoutCancel(tb.Context()))
		var sending sync.WaitGroup
		sending.Go(func() { sender.Run(ctx) })
		tb.Cleanup(func() { cancel(); sending.Wait() }) // Before the store closes.
	}
	live := openSettings(tb, st)
	var watcher *library.Watcher
	if o.Watch {
		watcher = library.NewWatcher(scanner, library.WatchOptions{})
		ctx, cancel := context.WithCancel(context.WithoutCancel(tb.Context()))
		var watching sync.WaitGroup
		watching.Go(func() { watcher.Run(ctx) })
		tb.Cleanup(func() { cancel(); watching.Wait() }) // Before the store closes.
	}
	h := New(Config{
		Watcher:       watcher,
		Settings:      live,
		Listen:        testListen,
		Version:       testVersion,
		Auth:          svc,
		Logger:        logger,
		Now:           clock.Now,
		ClientIP:      testClientIP,
		SecureCookies: o.SecureCookies,
		BasePath:      o.BasePath,
		Ready:         o.Ready,
		Artwork:       o.Artwork,
		Library:       st,
		Scanner:       scanner,
		StartScan:     o.StartScan,
		Web:           o.Web,
		Lastfm:        sender,
		SetupCode:     o.SetupCode,
		FolderRoot:    o.FolderRoot,
		DataDir:       o.DataDir,
	})
	srv := httptest.NewServer(h)
	tb.Cleanup(srv.Close)
	return &testServer{Server: srv, URL: srv.URL + o.BasePath + apiPrefix, Clock: clock, Log: logs, Auth: svc, Handle: h, Store: st, Scanner: scanner, Settings: live}
}

// What GET /admin/settings reports of a test server.
const (
	testListen  = ":4533"
	testVersion = "v0.0.0-test"
)

// testRoots returns the library roots o asks for: its LibraryRoot, then
// o.Roots empty folders.
func testRoots(tb testing.TB, o *testServerOptions) []string {
	tb.Helper()
	var roots []string
	if o.LibraryRoot != "" {
		roots = append(roots, o.LibraryRoot)
	}
	for range o.Roots {
		roots = append(roots, tb.TempDir())
	}
	return roots
}

// openSettings opens the settings kept in st.
func openSettings(tb testing.TB, st *store.Store) *settings.Service {
	tb.Helper()
	live, err := settings.Open(tb.Context(), st)
	if err != nil {
		tb.Fatalf("open settings: %v", err)
	}
	return live
}

// testClientIP reads the client IP from testClientIPHeader, falling back to
// the connection's address.
func testClientIP(r *http.Request) string {
	if v := r.Header.Get(testClientIPHeader); v != "" {
		return v
	}
	return remoteHost(r)
}

// seedUser creates a user and returns it.
func (ts *testServer) seedUser(tb testing.TB, name, password string, role store.Role) store.User {
	tb.Helper()
	u, err := ts.Auth.CreateUser(tb.Context(), name, password, role)
	if err != nil {
		tb.Fatalf("seed user %q: %v", name, err)
	}
	return u
}

// tokenFor seeds a user with role and returns a session token for it.
func (ts *testServer) tokenFor(tb testing.TB, role store.Role) string {
	tb.Helper()
	name, password := "user-"+string(role), "a password"
	ts.seedUser(tb, name, password, role)
	res, err := ts.Auth.Login(tb.Context(), name, password, "test/1")
	if err != nil {
		tb.Fatalf("login: %v", err)
	}
	return res.Token
}

// call describes one request to [testServer.send].
type call struct {
	method, path string
	body         string
	header       http.Header
	client       *http.Client // Nil means a fresh contract client.
	raw          bool         // The path is from the origin, outside the base path and /api/v1.
	invalid      bool         // The request deliberately breaks the contract.
}

// response is what [testServer.send] got back.
type response struct {
	Status int
	Header http.Header
	Body   []byte
}

// send sends c through a contract client and returns the response. It fails
// the test on a transport error, so call it only from the test goroutine.
func (ts *testServer) send(t *testing.T, c call) response {
	t.Helper()
	resp, err := ts.trySend(t, c)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// trySend is send for other goroutines: it reports a transport error instead
// of failing the test.
func (ts *testServer) trySend(t *testing.T, c call) (response, error) {
	t.Helper()
	ctx := t.Context()
	if c.invalid {
		ctx = openapitest.ExpectInvalidRequest(ctx)
	}
	base := ts.URL
	if c.raw {
		base = ts.Server.URL
	}
	req, err := http.NewRequestWithContext(ctx, c.method, base+c.path, strings.NewReader(c.body))
	if err != nil {
		return response{}, fmt.Errorf("new request: %w", err)
	}
	maps.Copy(req.Header, c.header)
	client := c.client
	if client == nil {
		client = contract.Client(t)
	}
	resp, err := client.Do(req)
	if err != nil {
		return response{}, fmt.Errorf("%s %s: %w", c.method, c.path, err)
	}
	body, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return response{}, fmt.Errorf("read response: %w", err)
	}
	return response{Status: resp.StatusCode, Header: resp.Header, Body: body}, nil
}

// problem decodes r as a problem and checks its status and type.
func (r response) problem(t *testing.T, status int) api.Problem {
	t.Helper()
	if got := r.Header.Get("Content-Type"); got != api.ProblemMediaType {
		t.Errorf("Content-Type = %q, want %q", got, api.ProblemMediaType)
	}
	var p api.Problem
	if err := json.Unmarshal(r.Body, &p); err != nil {
		t.Fatalf("decode problem %q: %v", r.Body, err)
	}
	if r.Status != status || p.Status != status || p.Type != problemType(status) {
		t.Errorf("got %d %+v, want a %d problem of type %s", r.Status, p, status, problemType(status))
	}
	return p
}

// bearer returns headers carrying token as a bearer credential.
func bearer(token string) http.Header {
	return http.Header{authorizationHeader: {"Bearer " + token}}
}
