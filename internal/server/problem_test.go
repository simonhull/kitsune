package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/api"
)

// serveWithProblems runs h behind withProblems and returns what it wrote.
func serveWithProblems(t *testing.T, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	withProblems(h).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody))
	return rec
}

func TestWithProblemsConvertsPlainErrors(t *testing.T) {
	t.Parallel()
	rec := serveWithProblems(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusRequestedRangeNotSatisfiable)
	})
	p := decodeProblem(t, rec)
	want := api.Problem{
		Type:   "about:blank",
		Title:  http.StatusText(http.StatusRequestedRangeNotSatisfiable),
		Status: http.StatusRequestedRangeNotSatisfiable,
		Detail: "nope",
	}
	if rec.Code != want.Status || p != want {
		t.Errorf("got %d %+v, want %d %+v", rec.Code, p, want.Status, want)
	}
}

func TestWithProblemsPassesProblemsThrough(t *testing.T) {
	t.Parallel()
	rec := serveWithProblems(t, func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, http.StatusNotFound, "no such track")
	})
	p := decodeProblem(t, rec)
	if rec.Code != http.StatusNotFound || p.Detail != "no such track" {
		t.Errorf("got %d %+v, want the handler's 404 problem unchanged", rec.Code, p)
	}
}

func TestWithProblemsPassesSuccessThrough(t *testing.T) {
	t.Parallel()
	rec := serveWithProblems(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/flac")
		_, _ = w.Write([]byte("fLaC"))
	})
	if rec.Code != http.StatusOK || rec.Body.String() != "fLaC" {
		t.Errorf("got %d %q, want 200 \"fLaC\"", rec.Code, rec.Body)
	}
}

func TestProblemWriterUnwraps(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	pw := &problemWriter{ResponseWriter: rec}
	if got := pw.Unwrap(); got != rec {
		t.Errorf("Unwrap() = %v, want the wrapped writer", got)
	}
}

// TestMarshalReplacesInvalidUTF8 checks that a raw tag with invalid UTF-8
// does not fail the response, as it would under json/v2's defaults.
func TestMarshalReplacesInvalidUTF8(t *testing.T) {
	t.Parallel()
	b, err := marshal(api.Problem{Title: "bad \xff byte"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), "bad � byte") {
		t.Errorf("marshal = %s, want U+FFFD in place of the invalid byte", b)
	}
}
