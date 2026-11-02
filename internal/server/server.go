// Package server is Kitsune's native HTTP API, written by hand against
// api/openapi.yaml. The kitsune-server binary does not serve it yet.
//
// Routing uses [http.ServeMux] method and path patterns, which share the
// contract's {name} placeholder syntax, so a route's pattern is the spec's
// method and path verbatim. A test asserts that every operation in the
// contract has a route registered under exactly that pattern.
package server

import "net/http"

// Config holds the server's dependencies. There are none yet; they arrive
// with the operations that need them.
type Config struct{}

// Server serves the API. It is safe for concurrent use.
type Server struct {
	mux     *http.ServeMux
	handler http.Handler
}

// route is one contract operation: its ServeMux pattern, which is the spec's
// method and path, and whether it skips bearer auth (security: []).
type route struct {
	pattern string
	public  bool
	handle  http.HandlerFunc
}

// New returns a server for cfg.
//
// New panics if a route needs bearer auth, which is not implemented yet.
func New(Config) *Server {
	s := &Server{mux: http.NewServeMux()}
	for _, rt := range s.routes() {
		if !rt.public {
			panic("server: route " + rt.pattern + " needs auth, which is not implemented yet")
		}
		s.mux.Handle(rt.pattern, rt.handle)
	}
	s.handler = withRequestID(withProblems(s.mux))
	return s
}

// ServeHTTP implements [http.Handler].
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// routes lists every operation in the contract.
func (s *Server) routes() []route {
	return []route{
		{pattern: "GET /healthz", public: true, handle: s.getHealth},
	}
}

// getHealth reports that the process is up.
func (*Server) getHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
