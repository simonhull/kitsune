package server

import (
	"context"
	"errors"
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
	if logs := ts.Log.String(); !strings.Contains(logs, "not ready") || !strings.Contains(logs, "disk on fire") {
		t.Errorf("log = %q, want the warning with the error", logs)
	}
}
