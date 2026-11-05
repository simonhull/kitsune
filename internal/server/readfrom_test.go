package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
)

// probePath is a route only the tests in this file add.
const probePath = "/probe"

// probeBody is what the probe route copies to its writer.
const probeBody = "not a file, but it copies like one"

// errProbe is the failure of a source that can't be read.
var errProbe = errors.New("probe read failed")

// readerFromWriter is the connection's writer in miniature: it takes what
// [io.Copy] hands to ReadFrom, and counts the calls.
type readerFromWriter struct {
	*httptest.ResponseRecorder

	calls int
}

func (w *readerFromWriter) ReadFrom(src io.Reader) (int64, error) {
	w.calls++
	return w.Body.ReadFrom(src) //nolint:wrapcheck // A stand-in for the connection; the test compares this error.
}

// probeSource returns probeBody as a plain reader, with no WriterTo for
// [io.Copy] to prefer, as a file behind a limit is.
func probeSource() io.Reader { return struct{ io.Reader }{strings.NewReader(probeBody)} }

func TestHandlerWriterIsReaderFrom(t *testing.T) {
	t.Parallel()
	for _, level := range []slog.Level{slog.LevelInfo, slog.LevelDebug} {
		t.Run(level.String(), func(t *testing.T) {
			t.Parallel()
			ts := newTestServer(t, testServerOptions{LogLevel: level})
			isReaderFrom := make(chan bool, 1)
			ts.Handle.mux.HandleFunc("GET "+probePath, func(w http.ResponseWriter, _ *http.Request) {
				_, ok := w.(io.ReaderFrom)
				isReaderFrom <- ok
				_, _ = io.Copy(w, probeSource()) // The test reads what arrived.
			})
			resp := ts.send(t, call{method: http.MethodGet, path: probePath, client: plainClient()})
			if !<-isReaderFrom {
				t.Error("the handler's writer is not an io.ReaderFrom")
			}
			if resp.Status != http.StatusOK || string(resp.Body) != probeBody {
				t.Errorf("got %d %q, want 200 %q", resp.Status, resp.Body, probeBody)
			}
		})
	}
}

func TestReadFromReachesTheConnection(t *testing.T) {
	t.Parallel()
	conn := &readerFromWriter{ResponseRecorder: httptest.NewRecorder()}
	rec := &statusRecorder{ResponseWriter: conn}
	pw := &problemWriter{ResponseWriter: rec}
	n, err := io.CopyN(pw, probeSource(), int64(len(probeBody)))
	if err != nil || n != int64(len(probeBody)) {
		t.Fatalf("CopyN = %d, %v; want %d, nil", n, err, len(probeBody))
	}
	if conn.calls != 1 {
		t.Errorf("the connection's ReadFrom ran %d times, want 1", conn.calls)
	}
	if got := conn.Body.String(); got != probeBody {
		t.Errorf("body = %q, want %q", got, probeBody)
	}
	if rec.status != http.StatusOK || rec.bytes != n {
		t.Errorf("recorded %d and %d bytes, want 200 and %d", rec.status, rec.bytes, n)
	}
}

func TestProblemWriterReadFromSendsImplicitOK(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	pw := &problemWriter{ResponseWriter: rr}
	if _, err := pw.ReadFrom(probeSource()); err != nil {
		t.Fatal(err)
	}
	if !pw.wroteHeader || pw.status != 0 {
		t.Errorf("wroteHeader = %v, status = %d; want true, 0", pw.wroteHeader, pw.status)
	}
	if rr.Code != http.StatusOK || rr.Body.String() != probeBody {
		t.Errorf("got %d %q, want 200 %q", rr.Code, rr.Body, probeBody)
	}
}

func TestProblemWriterReadFromKeepsConversion(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	pw := &problemWriter{ResponseWriter: rr}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, probePath, http.NoBody)
	pw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	pw.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
	n, err := pw.ReadFrom(probeSource())
	if err != nil || n != int64(len(probeBody)) {
		t.Fatalf("ReadFrom = %d, %v; want the whole source consumed", n, err)
	}
	pw.finish(r)
	resp := response{Status: rr.Code, Header: rr.Header(), Body: rr.Body.Bytes()}
	resp.problem(t, http.StatusRequestedRangeNotSatisfiable)
	if strings.Contains(rr.Body.String(), probeBody) {
		t.Errorf("body %q leaks the discarded text", rr.Body)
	}
}

func TestReadFromReportsSourceErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		w    io.ReaderFrom
	}{
		{"problemWriter", &problemWriter{ResponseWriter: httptest.NewRecorder()}},
		{"statusRecorder", &statusRecorder{ResponseWriter: httptest.NewRecorder()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tt.w.ReadFrom(iotest.ErrReader(errProbe)); !errors.Is(err, errProbe) {
				t.Errorf("ReadFrom err = %v, want %v", err, errProbe)
			}
		})
	}
}

func TestAccessLogCountsReadFrom(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, testServerOptions{LogLevel: slog.LevelDebug})
	ts.Handle.mux.HandleFunc("GET "+probePath, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.Copy(w, probeSource()) // The test reads the log.
	})
	ts.send(t, call{method: http.MethodGet, path: probePath, client: plainClient()})
	lines := requestLines(ts.Log.String())
	want := fmt.Sprintf("bytes=%d", len(probeBody))
	if len(lines) != 1 || !strings.Contains(lines[0], "status=200") || !strings.Contains(lines[0], want) {
		t.Errorf("request lines = %q, want one with status=200 and %s", lines, want)
	}
}
