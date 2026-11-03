package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/store"
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
}

// testServer is a running API over a real store and auth service.
type testServer struct {
	Server *httptest.Server
	URL    string
	Clock  *testClock
	Log    *syncBuffer
	Auth   *auth.Service
	Handle *Server
}

// newTestServer starts a server over a temporary store.
func newTestServer(t *testing.T, opts ...testServerOptions) *testServer {
	t.Helper()
	var o testServerOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Params == (auth.Params{}) {
		o.Params = auth.Params{Time: 1, MemoryKiB: 64, Threads: 1}
	}
	clock := newTestClock()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), store.FileName), store.Options{Now: clock.Now})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := st.Close(); closeErr != nil {
			t.Errorf("close store: %v", closeErr)
		}
	})
	svc, err := auth.New(st, auth.Options{Params: o.Params, Now: clock.Now})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	logs := &syncBuffer{}
	h := New(Config{
		Auth:          svc,
		Logger:        slog.New(slog.NewTextHandler(logs, nil)),
		Now:           clock.Now,
		ClientIP:      testClientIP,
		SecureCookies: o.SecureCookies,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &testServer{Server: srv, URL: srv.URL, Clock: clock, Log: logs, Auth: svc, Handle: h}
}

// testClientIP reads the client IP from testClientIPHeader, falling back to
// the connection's address.
func testClientIP(r *http.Request) string {
	if v := r.Header.Get(testClientIPHeader); v != "" {
		return v
	}
	return remoteHost(r)
}
