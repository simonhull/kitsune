// Package openapitest enforces api/openapi.yaml in tests: it validates every
// request a test sends and every response it receives, and lists the
// contract's operations for route-coverage checks.
//
// The contract defaults to the embedded [api.Spec]. Set KITSUNE_OPENAPI_SPEC
// to a file path to test against another copy, for example to see what a
// drifted spec breaks.
package openapitest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/simonhull/kitsune/api"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/config"
	valerrors "github.com/pb33f/libopenapi-validator/errors"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// SpecEnv names the environment variable that overrides the contract file.
const SpecEnv = "KITSUNE_OPENAPI_SPEC"

// eventStreamMediaType is the media type of server-sent events, whose bodies
// are validated on Close rather than read up front.
const eventStreamMediaType = "text/event-stream"

// errInvalidContract reports a contract that is not valid OpenAPI.
var errInvalidContract = errors.New("contract is not valid OpenAPI")

// Contract is a loaded OpenAPI document and its validator. It is safe for
// concurrent use, so one Contract can serve every test in a package.
type Contract struct {
	model     *v3.Document
	validator validator.Validator
	paths     *config.ValidationOptions // For resolving request paths; see [Contract.record].

	mu   sync.Mutex
	seen map[string]bool // Response keys exercised so far; see [Contract.Unexercised].
}

// Operation is one method and path from the contract.
type Operation struct {
	Method string // Upper case, as in an http.ServeMux pattern.
	Path   string // With {name} placeholders, as in the spec.
	// Public reports whether the operation opts out of the document's
	// security requirement with `security: []`.
	Public bool
}

// Load parses the contract and builds a validator for it, failing tb on any
// error.
func Load(tb testing.TB) *Contract {
	tb.Helper()
	c, err := New()
	if err != nil {
		tb.Fatal(err)
	}
	return c
}

// New reads the contract, from [SpecEnv] if set, and builds its validator.
// It is Load for TestMain, which has no testing.TB.
func New() (*Contract, error) {
	spec := api.Spec
	if path := os.Getenv(SpecEnv); path != "" {
		b, err := os.ReadFile(path) //nolint:gosec // G304, G703: the developer names this file to test against it.
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", SpecEnv, err)
		}
		spec = b
	}
	doc, err := libopenapi.NewDocument(spec)
	if err != nil {
		return nil, fmt.Errorf("parse contract: %w", err)
	}
	model, err := doc.BuildV3Model()
	if err != nil {
		return nil, fmt.Errorf("build contract model: %w", err)
	}
	v := validator.NewValidatorFromV3Model(&model.Model,
		// Strict mode reports undeclared properties, so a renamed or extra
		// field fails even where the schema allows additional properties.
		config.WithStrictMode(),
		config.WithFormatAssertions(),
	)
	v.SetDocument(doc)
	if ok, errs := v.ValidateDocument(); !ok {
		return nil, fmt.Errorf("%w:\n%s", errInvalidContract, describe(errs))
	}
	return &Contract{
		model:     &model.Model,
		validator: v,
		paths:     config.NewValidationOptions(),
		seen:      make(map[string]bool),
	}, nil
}

// Operations lists every operation in the contract.
func (c *Contract) Operations() []Operation {
	var ops []Operation
	for path, item := range c.model.Paths.PathItems.FromOldest() {
		for method, op := range item.GetOperations().FromOldest() {
			ops = append(ops, Operation{
				Method: strings.ToUpper(method),
				Path:   path,
				Public: op.Security != nil && len(op.Security) == 0,
			})
		}
	}
	return ops
}

// Client returns an HTTP client whose every request and response is checked
// against the contract. See [Transport].
func (c *Contract) Client(tb testing.TB) *http.Client {
	tb.Helper()
	return &http.Client{Transport: c.Transport(tb, http.DefaultTransport)}
}

// Transport returns a [Transport] over base that reports violations to tb. It
// also fails t at cleanup if any response it returned was never validated,
// which happens only to a streaming response whose body was not closed.
func (c *Contract) Transport(tb testing.TB, base http.RoundTripper) *Transport {
	tb.Helper()
	tr := &Transport{tb: tb, contract: c, base: base}
	tb.Cleanup(func() {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		if tr.validated != tr.received {
			tb.Errorf("openapitest: %d of %d responses were never validated; close every response body",
				tr.received-tr.validated, tr.received)
		}
	})
	return tr
}

// expectInvalidKey marks a request context whose request should break the
// contract.
type expectInvalidKey struct{}

// ExpectInvalidRequest returns a context for a request that deliberately
// breaks the contract, such as limit=0 or a missing token. [Transport] then
// fails the test if the request validates, and still validates the response.
func ExpectInvalidRequest(ctx context.Context) context.Context {
	return context.WithValue(ctx, expectInvalidKey{}, true)
}

// Transport is an [http.RoundTripper] that validates each request and
// response against the contract and reports every violation to its test.
// Response bodies are read in full and replaced, except for
// text/event-stream bodies, which are validated on Close with whatever was
// read.
type Transport struct {
	tb       testing.TB
	contract *Contract
	base     http.RoundTripper

	mu        sync.Mutex
	received  int
	validated int
}

// RoundTrip implements [http.RoundTripper].
func (tr *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.checkRequest(req)
	resp, err := tr.base.RoundTrip(req)
	if err != nil {
		return nil, err //nolint:wrapcheck // A RoundTripper passes its transport's errors through.
	}
	tr.mu.Lock()
	tr.received++
	tr.mu.Unlock()

	if strings.HasPrefix(resp.Header.Get("Content-Type"), eventStreamMediaType) {
		resp.Body = &validateOnClose{ReadCloser: resp.Body, tr: tr, resp: resp}
		return resp, nil
	}
	body, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("openapitest: read response: %w", err)
	}
	tr.checkResponse(resp, body)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

// checkRequest validates req and reports a mismatch with the expectation set
// by [ExpectInvalidRequest].
func (tr *Transport) checkRequest(req *http.Request) {
	wantInvalid, _ := req.Context().Value(expectInvalidKey{}).(bool) // Absent means false.
	ok, errs := tr.contract.validator.ValidateHttpRequestSync(req)
	switch {
	case ok && wantInvalid:
		tr.tb.Errorf("openapitest: %s %s was expected to break the contract but is valid", req.Method, req.URL)
	case !ok && !wantInvalid:
		tr.tb.Errorf("openapitest: request %s %s breaks the contract:\n%s", req.Method, req.URL, describe(errs))
	}
}

// checkResponse validates resp with the given body.
func (tr *Transport) checkResponse(resp *http.Response, body []byte) {
	tr.mu.Lock()
	tr.validated++
	tr.mu.Unlock()

	clone := *resp
	clone.Body = io.NopCloser(bytes.NewReader(body))
	if ok, errs := tr.contract.validator.ValidateHttpResponse(resp.Request, &clone); !ok {
		tr.tb.Errorf("openapitest: response %d to %s %s breaks the contract:\n%s",
			resp.StatusCode, resp.Request.Method, resp.Request.URL, describe(errs))
		return
	}
	tr.contract.record(resp.Request, resp.StatusCode)
}

// validateOnClose records a streaming body and validates the response once
// the caller closes it.
type validateOnClose struct {
	io.ReadCloser

	tr   *Transport
	resp *http.Response
	seen bytes.Buffer
	once sync.Once
}

// Read implements [io.Reader].
func (v *validateOnClose) Read(p []byte) (int, error) {
	n, err := v.ReadCloser.Read(p)
	v.seen.Write(p[:n])
	return n, err //nolint:wrapcheck // A Reader returns io.EOF unwrapped.
}

// Close implements [io.Closer].
func (v *validateOnClose) Close() error {
	err := v.ReadCloser.Close()
	v.once.Do(func() { v.tr.checkResponse(v.resp, v.seen.Bytes()) })
	return err //nolint:wrapcheck // Pass the body's own error through.
}

// describe formats validation errors, one per line with their schema
// failures indented.
func describe(errs []*valerrors.ValidationError) string {
	var b strings.Builder
	for _, e := range errs {
		fmt.Fprintf(&b, "  - %s: %s\n", e.Message, e.Reason)
		for _, f := range e.SchemaValidationErrors {
			fmt.Fprintf(&b, "      %s: %s\n", f.FieldPath, f.Reason)
		}
	}
	return b.String()
}
