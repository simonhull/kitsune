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
		want     string // A substring of the reported violation; "" for none.
	}{
		{"health", healthPath, nil, canned{http.StatusNoContent, "", ""}, ""},
		{"undeclared status", healthPath, nil, canned{http.StatusOK, "", ""}, "response 200 to GET"},
		{"unknown path", "/nope", nil, canned{http.StatusNotFound, "application/problem+json", problem}, "request GET"},
		{"valid request expected invalid", healthPath, openapitest.ExpectInvalidRequest, canned{http.StatusNoContent, "", ""}, "expected to break the contract"},
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
			switch {
			case tt.want == "" && len(rec.errs) > 0:
				t.Errorf("reports = %q, want none", rec.errs)
			case tt.want != "" && !slices.ContainsFunc(rec.errs, func(e string) bool { return strings.Contains(e, tt.want) }):
				t.Errorf("reports = %q, want one containing %q", rec.errs, tt.want)
			}
		})
	}
}

// get sends a GET for url through client and closes the body.
func get(t *testing.T, client *http.Client, url string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
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
}

func TestCoverage(t *testing.T) {
	t.Parallel()
	c := openapitest.Load(t)
	want := []string{"GET /healthz 204"}
	if got := c.Unexercised(); !slices.Equal(got, want) {
		t.Errorf("before any request, Unexercised() = %q, want %q", got, want)
	}

	// A response outside the contract does not count.
	offContract := &http.Client{Transport: c.Transport(&recorder{TB: t}, canned{http.StatusNoContent, "", ""})}
	get(t, offContract, "http://kitsune.test/nope")
	if got := c.Unexercised(); !slices.Equal(got, want) {
		t.Errorf("after an off-contract response, Unexercised() = %q, want %q", got, want)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	get(t, c.Client(t), srv.URL+healthPath)
	if got := c.Unexercised(); len(got) != 0 {
		t.Errorf("after GET /healthz, Unexercised() = %q, want none", got)
	}
}
