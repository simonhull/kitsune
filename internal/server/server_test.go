package server

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/simonhull/kitsune/api"
)

func TestHealth(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(New(Config{}))
	t.Cleanup(srv.Close)

	status, header := get(t, contract.Client(t), srv.URL+"/healthz")
	if status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", status)
	}
	if header.Get("X-Request-ID") == "" {
		t.Error("X-Request-ID is missing")
	}
}

// TestOffContractErrorsAreProblems covers responses the contract cannot
// declare (unknown paths and methods), so they bypass the validator.
func TestOffContractErrorsAreProblems(t *testing.T) {
	t.Parallel()
	tests := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/nope", http.StatusNotFound},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			New(Config{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, http.NoBody))
			p := decodeProblem(t, rec)
			if rec.Code != tt.want || p.Status != tt.want || p.Detail == "" || p.RequestID == "" {
				t.Errorf("got %d %+v, want a %d problem with a detail and a request ID", rec.Code, p, tt.want)
			}
			if got := rec.Header().Get("X-Request-ID"); got != p.RequestID {
				t.Errorf("X-Request-ID = %q, problem requestId = %q; want equal", got, p.RequestID)
			}
		})
	}
}

// get sends a GET for url through client, closes the response body and
// returns the status and header.
func get(t *testing.T, client *http.Client, url string) (int, http.Header) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Fatalf("close body: %v", closeErr)
	}
	return resp.StatusCode, resp.Header
}

// decodeProblem checks that rec holds a problem response and decodes it.
func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) api.Problem {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); got != api.ProblemMediaType {
		t.Errorf("Content-Type = %q, want %q", got, api.ProblemMediaType)
	}
	var p api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode problem %q: %v", rec.Body, err)
	}
	return p
}
