package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

const readyPath = "/readyz"

var errDiskOnFire = errors.New("disk on fire")

func TestReadyDefault(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	if resp := ts.send(t, call{method: http.MethodGet, path: readyPath}); resp.Status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.Status)
	}
}

func TestReadyOK(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{Ready: func(context.Context) error { return nil }})
	if resp := ts.send(t, call{method: http.MethodGet, path: readyPath}); resp.Status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.Status)
	}
}

func TestReadyFailing(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{Ready: func(context.Context) error { return errDiskOnFire }})
	resp := ts.send(t, call{method: http.MethodGet, path: readyPath})
	p := resp.problem(t, http.StatusServiceUnavailable)
	if p.Detail != "not ready" {
		t.Errorf("detail = %q, want %q", p.Detail, "not ready")
	}
	if strings.Contains(string(resp.Body), "disk on fire") {
		t.Errorf("body %q leaks the error text", resp.Body)
	}
	logs := ts.Log.String()
	if !strings.Contains(logs, "level=WARN") || !strings.Contains(logs, "not ready") || !strings.Contains(logs, "disk on fire") {
		t.Errorf("log = %q, want the warning with the error", logs)
	}
}

func TestReadyStartingIsQuiet(t *testing.T) {
	t.Parallel()
	ready := func(context.Context) error { return fmt.Errorf("scan: %w", ErrStarting) }
	ts := newTestServer(t, testServerOptions{Ready: ready, LogLevel: slog.LevelDebug})
	resp := ts.send(t, call{method: http.MethodGet, path: readyPath})
	if p := resp.problem(t, http.StatusServiceUnavailable); p.Detail != "not ready" {
		t.Errorf("detail = %q, want %q", p.Detail, "not ready")
	}
	logs := ts.Log.String()
	if strings.Contains(logs, "level=WARN") {
		t.Errorf("log = %q, want no warning while starting", logs)
	}
	if !strings.Contains(logs, "level=DEBUG") || !strings.Contains(logs, "not ready") {
		t.Errorf("log = %q, want the debug line", logs)
	}
}
