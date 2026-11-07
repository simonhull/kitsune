package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	webIndex = "<!doctype html><title>Kitsune</title>"
	webChunk = "export const a = 1;"
	webHTML  = "text/html; charset=utf-8"
)

// webFiles is a built client in miniature.
func webFiles() fstest.MapFS {
	return fstest.MapFS{
		"index.html":                  {Data: []byte(webIndex)},
		"robots.txt":                  {Data: []byte("User-agent: *")},
		"_app/version.json":           {Data: []byte(`{"version":"1"}`)},
		"_app/immutable/chunks/a.js":  {Data: []byte(webChunk)},
		"_app/immutable/assets/a.css": {Data: []byte("a{}")},
	}
}

// webReply is what the server said.
type webReply struct {
	status int
	header http.Header
	body   string
}

// webGet sends method to path and reads the reply.
func webGet(t *testing.T, ts *testServer, method, path string) webReply {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, ts.Server.URL+path, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := plainClient().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return webReply{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

func TestWebServesTheClient(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{Web: webFiles()})
	tests := []struct {
		name, path, body, contentType, cache string
	}{
		{"root", "/", webIndex, webHTML, webCache},
		{"a page", "/library", webIndex, webHTML, webCache},
		{"a deep page", "/albums/01JABCDEFGHJKMNPQRSTVWXYZ0", webIndex, webHTML, webCache},
		{"the page itself", "/index.html", webIndex, webHTML, webCache},
		{"a hashed chunk", "/_app/immutable/chunks/a.js", webChunk, "text/javascript", webHashedCache},
		{"a hashed stylesheet", "/_app/immutable/assets/a.css", "a{}", "text/css", webHashedCache},
		{"an unhashed file", "/_app/version.json", `{"version":"1"}`, "application/json", webCache},
		{"a static file", "/robots.txt", "User-agent: *", "text/plain; charset=utf-8", webCache},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reply := webGet(t, ts, http.MethodGet, tt.path)
			if reply.status != http.StatusOK {
				t.Fatalf("status = %d, want 200", reply.status)
			}
			if reply.body != tt.body {
				t.Errorf("body = %q, want %q", reply.body, tt.body)
			}
			if got := reply.header.Get("Content-Type"); !strings.HasPrefix(got, tt.contentType) {
				t.Errorf("Content-Type = %q, want %q", got, tt.contentType)
			}
			if got := reply.header.Get("Cache-Control"); got != tt.cache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.cache)
			}
			if got := reply.header.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
	}
}

func TestWebHead(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{Web: webFiles()})
	reply := webGet(t, ts, http.MethodHead, "/library")
	if reply.status != http.StatusOK || reply.body != "" {
		t.Errorf("status = %d, body %q, want 200 and none", reply.status, reply.body)
	}
}

// A stale chunk must fail, or the browser parses a page as JavaScript. An API
// path that matched nothing must stay a problem, or clients parse a page as JSON.
func TestWebMisses(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{Web: webFiles()})
	tests := []struct{ name, method, path string }{
		{"a stale chunk", http.MethodGet, "/_app/immutable/chunks/gone.js"},
		{"a folder of the build", http.MethodGet, "/_app/immutable"},
		{"a missing file", http.MethodGet, "/favicon.ico"},
		{"an unknown API path", http.MethodGet, apiPrefix + "/nope"},
		{"another API version", http.MethodGet, "/api/v2/albums"},
		{"the API's parent", http.MethodGet, "/api"},
		{"an escaped API path", http.MethodGet, "/api%2Fv2/albums"},
		{"a write", http.MethodPost, "/library"},
		{"readiness at the root", http.MethodGet, readyPath},
		{"health at the root", http.MethodGet, healthPath},
		{"a HEAD of readiness at the root", http.MethodHead, readyPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reply := webGet(t, ts, tt.method, tt.path)
			if reply.status != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", reply.status)
			}
			if !isProblem(reply.header.Get("Content-Type")) {
				t.Errorf("Content-Type = %q, want a problem", reply.header.Get("Content-Type"))
			}
			if strings.Contains(reply.body, webIndex) {
				t.Errorf("body is the client: %q", reply.body)
			}
		})
	}
}

func TestWebLeavesTheAPIAlone(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{Web: webFiles()})
	for _, path := range []string{healthPath, readyPath} {
		if resp := ts.send(t, call{method: http.MethodGet, path: path}); resp.Status != http.StatusNoContent {
			t.Errorf("%s: status = %d, want 204", path, resp.Status)
		}
	}
}

// The client is built for the root of a host: its asset and API paths are fixed.
func TestWebNotUnderABasePath(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{Web: webFiles(), BasePath: musicBase})
	for _, path := range []string{"/", musicBase + "/", musicBase + "/library"} {
		if reply := webGet(t, ts, http.MethodGet, path); reply.status != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, reply.status)
		}
	}
}

func TestNoWebIsNotFound(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	if reply := webGet(t, ts, http.MethodGet, "/library"); reply.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", reply.status)
	}
}
