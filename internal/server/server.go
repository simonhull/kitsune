// Package server is Kitsune's native HTTP API, written by hand against
// api/openapi.yaml. The kitsune-server binary serves it.
//
// Routing uses [http.ServeMux] method and path patterns, which share the
// contract's {name} placeholder syntax, so a route's pattern is the spec's
// method and path verbatim. A test asserts that every operation in the
// contract has a route registered under exactly that pattern.
package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/auth"
)

// Config holds the server's dependencies.
type Config struct {
	Auth          *auth.Service               // Required; New panics if nil.
	Logger        *slog.Logger                // Nil discards.
	Now           func() time.Time            // Nil means time.Now. Used by the login limiter.
	ClientIP      func(*http.Request) string  // Nil means the host part of RemoteAddr.
	SecureCookies bool                        // Mark cookies Secure even without TLS (HTTPS proxy in front).
	BasePath      string                      // "" or a path such as "/music": serve under it. New panics on a malformed one.
	Ready         func(context.Context) error // Nil means always ready. Backs GET /readyz.
	Artwork       *artwork.Cache              // Nil means every GET /artwork/{hash} is a 404.
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
	basePath      string
	ready         func(context.Context) error
	artwork       *artwork.Cache
	limiter       *loginLimiter
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
// New panics if cfg.Auth is nil or cfg.BasePath is malformed, which are
// programming errors.
func New(cfg Config) *Server {
	checkConfig(&cfg)
	s := &Server{
		mux:           http.NewServeMux(),
		auth:          cfg.Auth,
		log:           cfg.Logger,
		now:           cfg.Now,
		clientIP:      cfg.ClientIP,
		secureCookies: cfg.SecureCookies,
		basePath:      cfg.BasePath,
		ready:         cfg.Ready,
		artwork:       cfg.Artwork,
		limiter:       newLoginLimiter(maxLimiterKeys),
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
	s.handler = withRequestID(s.withAccessLog(withProblems(s.withBasePath(s.mux))))
	return s
}

// checkConfig panics on a Config that New can't use.
func checkConfig(cfg *Config) {
	if cfg.Auth == nil {
		panic("server: Config.Auth is required")
	}
	if b := cfg.BasePath; b != "" && (!strings.HasPrefix(b, "/") || strings.HasSuffix(b, "/")) {
		panic("server: Config.BasePath must be empty, or start with / and not end with /")
	}
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
		{pattern: "GET /readyz", public: true, handle: s.getReady},
		{pattern: "POST /auth/login", public: true, handle: s.login},
		{pattern: "POST /auth/logout", handle: s.logout},
		{pattern: "GET /me", handle: s.getMe},
		{pattern: "GET /artwork/{hash}", handle: s.getArtwork},
	}
}

// getHealth reports that the process is up.
func (*Server) getHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// getReady reports whether the server is ready to serve. A failure is logged
// but never described to the client.
func (s *Server) getReady(w http.ResponseWriter, r *http.Request) {
	if s.ready != nil {
		if err := s.ready(r.Context()); err != nil {
			s.log.WarnContext(r.Context(), "not ready", "requestId", requestID(r.Context()), "err", err)
			writeProblem(w, r, http.StatusServiceUnavailable, "not ready")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
