package server

import (
	"bytes"
	"encoding/json/v2"
	"io"
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
	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/openapitest"
	"github.com/simonhull/kitsune/internal/store"
)

// authorizationHeader is the credentials request header.
const authorizationHeader = "Authorization"

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
func newTestServer(t *testing.T) *testServer {
	t.Helper()
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
	svc, err := auth.New(st, auth.Options{Params: auth.Params{Time: 1, MemoryKiB: 64, Threads: 1}, Now: clock.Now})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	logs := &syncBuffer{}
	h := New(Config{
		Auth:     svc,
		Logger:   slog.New(slog.NewTextHandler(logs, nil)),
		Now:      clock.Now,
		ClientIP: testClientIP,
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

// seedUser creates a user and returns it.
func (ts *testServer) seedUser(t *testing.T, name, password string, role store.Role) store.User {
	t.Helper()
	u, err := ts.Auth.CreateUser(t.Context(), name, password, role)
	if err != nil {
		t.Fatalf("seed user %q: %v", name, err)
	}
	return u
}

// call describes one request to [testServer.send].
type call struct {
	method, path string
	body         string
	header       http.Header
	client       *http.Client // Nil means a fresh contract client.
	invalid      bool         // The request deliberately breaks the contract.
}

// response is what [testServer.send] got back.
type response struct {
	Status int
	Header http.Header
	Body   []byte
}

// send sends c through a contract client and returns the response.
func (ts *testServer) send(t *testing.T, c call) response {
	t.Helper()
	ctx := t.Context()
	if c.invalid {
		ctx = openapitest.ExpectInvalidRequest(ctx)
	}
	req, err := http.NewRequestWithContext(ctx, c.method, ts.URL+c.path, strings.NewReader(c.body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	maps.Copy(req.Header, c.header)
	client := c.client
	if client == nil {
		client = contract.Client(t)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", c.method, c.path, err)
	}
	body, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return response{Status: resp.StatusCode, Header: resp.Header, Body: body}
}

// problem decodes r as a problem and checks its status and type.
func (r response) problem(t *testing.T, status int) {
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
}

// bearer returns headers carrying token as a bearer credential.
func bearer(token string) http.Header {
	return http.Header{authorizationHeader: {"Bearer " + token}}
}
