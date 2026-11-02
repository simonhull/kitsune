package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

// placeholder matches a {name} path template segment.
var placeholder = regexp.MustCompile(`\{[^}]+\}`)

// TestRoutesMatchContract is the drift check for routing. Every operation in
// the contract must reach a route registered under exactly the spec's method
// and path (placeholder names included, so r.PathValue keys match the spec's
// parameter names), with bearer auth unless the operation has security: [].
// Every route must be in the contract.
func TestRoutesMatchContract(t *testing.T) {
	t.Parallel()
	s := New(Config{})

	routes := make(map[string]route)
	for _, rt := range s.routes() {
		routes[rt.pattern] = rt
	}
	inContract := make(map[string]bool)

	for _, op := range contract.Operations() {
		pattern := op.Method + " " + op.Path
		inContract[pattern] = true

		req := httptest.NewRequestWithContext(t.Context(), op.Method, placeholder.ReplaceAllString(op.Path, "x"), http.NoBody)
		if _, got := s.mux.Handler(req); got != pattern {
			t.Errorf("%s: ServeMux routes it to %q", pattern, got)
		}
		rt, ok := routes[pattern]
		if !ok {
			t.Errorf("%s: no route", pattern)
			continue
		}
		if rt.public != op.Public {
			t.Errorf("%s: route public = %t, contract public = %t", pattern, rt.public, op.Public)
		}
	}
	for pattern := range routes {
		if !inContract[pattern] {
			t.Errorf("%s: route is not in the contract", pattern)
		}
	}
}
