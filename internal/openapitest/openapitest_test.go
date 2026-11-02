package openapitest_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/simonhull/kitsune/internal/openapitest"
)

// recorder captures Errorf calls instead of failing the test.
type recorder struct {
	testing.TB

	mu   sync.Mutex
	errs []string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

// canned is a RoundTripper that answers every request with one response.
type canned struct {
	status      int
	contentType string
	body        string
}

func (c canned) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: c.status,
		Header:     http.Header{"Content-Type": {c.contentType}},
		Body:       io.NopCloser(strings.NewReader(c.body)),
		Request:    req,
	}, nil
}

const (
	healthPath = "/healthz"
	problem    = `{"type":"about:blank","title":"Not Found","status":404}`
)

// TestTransportCatches records which contract breaks the validator reports.
func TestTransportCatches(t *testing.T) {
	t.Parallel()
	contract := openapitest.Load(t)

	tests := []struct {
		name     string
		path     string
		ctx      func(context.Context) context.Context
		response canned
		want     bool // Whether the validator reports a violation.
	}{
		{"health", healthPath, nil, canned{http.StatusNoContent, "", ""}, false},
		{"undeclared status", healthPath, nil, canned{http.StatusOK, "", ""}, true},
		{"unknown path", "/nope", nil, canned{http.StatusNotFound, "application/problem+json", problem}, true},
		{"valid request expected invalid", healthPath, openapitest.ExpectInvalidRequest, canned{http.StatusNoContent, "", ""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			if tt.ctx != nil {
				ctx = tt.ctx(ctx)
			}
			rec := &recorder{TB: t}
			client := &http.Client{Transport: contract.Transport(rec, tt.response)}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://kitsune.test"+tt.path, http.NoBody)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("close: %v", closeErr)
			}

			rec.mu.Lock()
			defer rec.mu.Unlock()
			if got := len(rec.errs) > 0; got != tt.want {
				t.Errorf("violation reported = %t, want %t; reports: %q", got, tt.want, rec.errs)
			}
			t.Logf("reports: %q", rec.errs)
		})
	}
}

func TestCoverage(t *testing.T) {
	t.Parallel()
	c := openapitest.Load(t)
	if got := c.Unexercised(); !slices.Equal(got, []string{"GET /healthz 204"}) {
		t.Errorf("before any request, Unexercised() = %q, want [GET /healthz 204]", got)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+healthPath, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := c.Client(t).Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}

	if got := c.Unexercised(); len(got) != 0 {
		t.Errorf("after GET /healthz, Unexercised() = %q, want none", got)
	}
}
