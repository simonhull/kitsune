package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// requestLines returns the log lines that record a request.
func requestLines(logs string) []string {
	var lines []string
	for line := range strings.SplitSeq(logs, "\n") {
		if strings.Contains(line, "msg=request") {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestAccessLogDebug(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{LogLevel: slog.LevelDebug})
	resp := ts.send(t, call{method: http.MethodGet, path: "/healthz?p=hunter2", invalid: true})
	lines := requestLines(ts.Log.String())
	if len(lines) != 1 {
		t.Fatalf("request lines = %q, want exactly one", lines)
	}
	for _, want := range []string{"method=GET", "path=/api/v1/healthz", "status=204", "requestId=" + resp.Header.Get("X-Request-ID")} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("line %q does not contain %q", lines[0], want)
		}
	}
	if strings.Contains(ts.Log.String(), "hunter2") {
		t.Errorf("log %q contains the query", ts.Log.String())
	}
}

func TestAccessLogProblemStatus(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{LogLevel: slog.LevelDebug})
	ts.send(t, call{method: http.MethodGet, path: nopePath, client: plainClient()})
	lines := requestLines(ts.Log.String())
	if len(lines) != 1 || !strings.Contains(lines[0], "status=404") {
		t.Errorf("request lines = %q, want one with status=404", lines)
	}
}

func TestAccessLogOffAtInfo(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.send(t, call{method: http.MethodGet, path: healthPath})
	if lines := requestLines(ts.Log.String()); len(lines) != 0 {
		t.Errorf("request lines = %q, want none at info level", lines)
	}
}

func TestStatusRecorderUnwrap(t *testing.T) {
	t.Parallel()
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	if err := http.NewResponseController(rec).Flush(); err != nil {
		t.Errorf("Flush through statusRecorder: %v", err)
	}
}
