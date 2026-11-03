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
		Type:   aboutBlank,
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

func TestWithProblemsKeepsTheConvertedStatus(t *testing.T) {
	t.Parallel()
	rec := serveWithProblems(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.WriteHeader(http.StatusOK)
	})
	if p := decodeProblem(t, rec); rec.Code != http.StatusNotFound || p.Status != http.StatusNotFound {
		t.Errorf("got %d %+v, want a 404 problem", rec.Code, p)
	}
}

func TestWithProblemsConvertsAfterInformational(t *testing.T) {
	t.Parallel()
	rec := serveWithProblems(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusEarlyHints)
		http.Error(w, "nope", http.StatusNotFound)
	})
	if p := decodeProblem(t, rec); p.Status != http.StatusNotFound || p.Detail != "nope" {
		t.Errorf("got %+v, want a 404 problem with detail \"nope\"", p)
	}
}

func TestWithProblemsPassesParameterizedProblemsThrough(t *testing.T) {
	t.Parallel()
	const body = `{"type":"about:blank","title":"Not Found","status":404}`
	rec := serveWithProblems(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", api.ProblemMediaType+"; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(body))
	})
	if rec.Body.String() != body {
		t.Errorf("body = %s, want the handler's problem unchanged", rec.Body)
	}
}

func TestWithProblemsDropsBodyHeaders(t *testing.T) {
	t.Parallel()
	rec := serveWithProblems(t, func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h.Set("Content-Encoding", "gzip")
		h.Set("Content-Disposition", "attachment")
		h.Set("ETag", `"x"`)
		h.Set("Last-Modified", "Mon, 02 Nov 2026 00:00:00 GMT")
		http.Error(w, "nope", http.StatusNotFound)
	})
	decodeProblem(t, rec)
	for _, k := range []string{"Content-Encoding", "Content-Disposition", "ETag", "Last-Modified"} {
		if v := rec.Header().Get(k); v != "" {
			t.Errorf("%s = %q on the problem, want it removed", k, v)
		}
	}
}

func TestWithProblemsUnwrapsForResponseController(t *testing.T) {
	t.Parallel()
	rec := serveWithProblems(t, func(w http.ResponseWriter, _ *http.Request) {
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("Flush: %v", err)
		}
	})
	if !rec.Flushed {
		t.Error("the underlying writer was not flushed")
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

func TestProblemTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, "urn:kitsune:problem:invalid-request"},
		{http.StatusUnauthorized, "urn:kitsune:problem:unauthorized"},
		{http.StatusForbidden, "urn:kitsune:problem:forbidden"},
		{http.StatusNotFound, "urn:kitsune:problem:not-found"},
		{http.StatusConflict, "urn:kitsune:problem:conflict"},
		{http.StatusTooManyRequests, "urn:kitsune:problem:rate-limited"},
		{http.StatusInternalServerError, "urn:kitsune:problem:internal"},
		{http.StatusServiceUnavailable, "urn:kitsune:problem:unavailable"},
		{http.StatusMethodNotAllowed, aboutBlank},
		{http.StatusRequestEntityTooLarge, aboutBlank},
		{http.StatusRequestedRangeNotSatisfiable, aboutBlank},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			writeProblem(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody), tt.status, "x")
			if p := decodeProblem(t, rec); p.Type != tt.want || p.Status != tt.status {
				t.Errorf("type = %q, want %q", p.Type, tt.want)
			}
		})
	}
}
