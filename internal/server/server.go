// Package server is Kitsune's native HTTP API, written by hand against
// api/openapi.yaml. The kitsune-server binary does not serve it yet.
//
// Routing uses [http.ServeMux] method and path patterns, which share the
// contract's {name} placeholder syntax, so a route's pattern is the spec's
// method and path verbatim. A test asserts that every operation in the
// contract has a route registered under exactly that pattern.
package server

import (
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/simonhull/kitsune/internal/auth"
)

// Config holds the server's dependencies.
type Config struct {
	Auth          *auth.Service              // Required; New panics if nil.
	Logger        *slog.Logger               // Nil discards.
	Now           func() time.Time           // Nil means time.Now. Used by the login limiter.
	ClientIP      func(*http.Request) string // Nil means the host part of RemoteAddr.
	SecureCookies bool                       // Mark cookies Secure even without TLS (HTTPS proxy in front).
}

// Server serves the API. It is safe for concurrent use.
type Server struct {
	mux     *http.ServeMux
	handler http.Handler

	auth          *auth.Service
	log           *slog.Logger
	now           func() time.Time
	clientIP      func(*http.Request) string
	secureCookies bool
}

// route is one contract operation: its ServeMux pattern, which is the spec's
// method and path, and whether it skips authentication (security: []).
type route struct {
	pattern string
	public  bool
	handle  http.HandlerFunc
}

// New returns a server for cfg.
//
// New panics if cfg.Auth is nil, which is a programming error.
func New(cfg Config) *Server {
	if cfg.Auth == nil {
		panic("server: Config.Auth is required")
	}
	s := &Server{
		mux:           http.NewServeMux(),
		auth:          cfg.Auth,
		log:           cfg.Logger,
		now:           cfg.Now,
		clientIP:      cfg.ClientIP,
		secureCookies: cfg.SecureCookies,
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.clientIP == nil {
		s.clientIP = remoteHost
	}
	for _, rt := range s.routes() {
		if rt.public {
			s.mux.Handle(rt.pattern, rt.handle)
			continue
		}
		s.mux.Handle(rt.pattern, s.authenticate(rt.handle))
	}
	s.handler = withRequestID(withProblems(s.mux))
	return s
}

// remoteHost returns the host part of r.RemoteAddr, or all of it when it has
// no port.
func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ServeHTTP implements [http.Handler].
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// routes lists every operation in the contract.
func (s *Server) routes() []route {
	return []route{
		{pattern: "GET /healthz", public: true, handle: s.getHealth},
		{pattern: "GET /me", handle: s.getMe},
	}
}

// getHealth reports that the process is up.
func (*Server) getHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
