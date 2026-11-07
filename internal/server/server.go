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
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/scrobbler"
	"github.com/simonhull/kitsune/internal/store"
)

// ErrStarting is what a [Config] Ready func returns while the server is still
// coming up. Clients poll through it, so it is logged at Debug, not as a fault.
var ErrStarting = errors.New("initial scan not finished")

// Config holds the server's dependencies.
type Config struct {
	Auth          *auth.Service               // Required; New panics if nil.
	Logger        *slog.Logger                // Nil discards.
	Now           func() time.Time            // Nil means time.Now. Used by the login limiter and to bound a play's time.
	ClientIP      func(*http.Request) string  // Nil means the host part of RemoteAddr.
	SecureCookies bool                        // Mark cookies Secure even without TLS (HTTPS proxy in front).
	BasePath      string                      // "" or a path such as "/music": serve under it. New panics on a malformed one.
	Ready         func(context.Context) error // Nil means always ready. Backs GET /readyz.
	Artwork       *artwork.Cache              // Nil means every GET /artwork/{hash} is a 404.
	Library       *store.Store                // Required; New panics if nil. Backs the browse routes.
	Roots         int                         // The number of configured library roots, reported by GET /library.
	TagWrites     bool                        // Administrators may edit tags. Off leaves every file alone.
	Scanner       *library.Scanner            // Required; New panics if nil. Applies tag edits and reports scans.
	// StartScan claims a scan and runs it in the background; required, New panics if
	// nil. It returns [library.ErrScanRunning] while one runs, and another error when
	// the server is stopping.
	StartScan func(library.Mode) error
	Genres    *library.Genres // Nil means no album shows a genre.
	// Context bounds tag edits. Nil means context.Background. An edit runs on it and
	// not on the request, so a client that goes away can't stop an album midway.
	Context context.Context //nolint:containedctx // The server's lifetime, not a request's.
	// Web is the built client, served for what isn't the API. Nil means none. A
	// BasePath leaves it out: the client's paths are fixed to the root of a host.
	Web fs.FS
	// Lastfm sends plays, now playing and loves to Last.fm. Nil, or one with no
	// client, means Last.fm is not set up here.
	Lastfm *scrobbler.Scrobbler
	// PublicURL is the address browsers reach the server at, base path included,
	// such as "https://example.com/music": Last.fm sends them back there. "" means
	// the host each request names.
	PublicURL string
}

// Server serves the API. It is safe for concurrent use.
type Server struct {
	mux     *http.ServeMux
	handler http.Handler
	outside http.Handler // Answers what is outside the API.

	auth          *auth.Service
	log           *slog.Logger
	now           func() time.Time
	clientIP      func(*http.Request) string
	secureCookies bool
	basePath      string
	ready         func(context.Context) error
	artwork       *artwork.Cache
	library       *store.Store
	roots         int
	tagWrites     bool
	scanner       *library.Scanner
	startScan     func(library.Mode) error
	genres        *library.Genres
	editContext   context.Context //nolint:containedctx // Config.Context: the server's lifetime, not a request's.
	limiter       *loginLimiter
	lastfm        *scrobbler.Scrobbler
	publicURL     string
	lastfmStates  lastfmStates
}

// route is one contract operation: its ServeMux pattern, which is the spec's
// method and path, whether it skips authentication (security: []), and whether
// only administrators may call it.
type route struct {
	pattern string
	public  bool
	admin   bool
	handle  http.HandlerFunc
}

// New returns a server for cfg.
//
// New panics if cfg.Auth, cfg.Library, cfg.Scanner or cfg.StartScan is nil or
// cfg.BasePath is malformed, which are programming errors.
func New(cfg Config) *Server { //nolint:gocritic // hugeParam: Config is built once at startup; a value keeps every caller a plain literal.
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
		library:       cfg.Library,
		roots:         cfg.Roots,
		tagWrites:     cfg.TagWrites,
		scanner:       cfg.Scanner,
		startScan:     cfg.StartScan,
		genres:        cfg.Genres,
		editContext:   cfg.Context,
		limiter:       newLoginLimiter(maxLimiterKeys),
		lastfm:        cfg.Lastfm,
		publicURL:     cfg.PublicURL,
		lastfmStates:  lastfmStates{byUser: map[int64]pendingState{}},
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
	if s.editContext == nil {
		s.editContext = context.Background()
	}
	if s.genres == nil {
		s.genres = library.NewGenres(cfg.Library)
	}
	s.outside = http.NotFoundHandler()
	if cfg.Web != nil && cfg.BasePath == "" {
		s.outside = webHandler(cfg.Web)
	}
	for _, rt := range s.routes() {
		s.mux.Handle(rt.pattern, s.guarded(&rt))
	}
	s.handler = withRequestID(s.withAccessLog(withProblems(s.withBasePath(s.mux))))
	return s
}

// checkConfig panics on a Config that New can't use.
func checkConfig(cfg *Config) {
	if cfg.Auth == nil {
		panic("server: Config.Auth is required")
	}
	if cfg.Library == nil {
		panic("server: Config.Library is required")
	}
	if cfg.Scanner == nil {
		panic("server: Config.Scanner is required")
	}
	if cfg.StartScan == nil {
		panic("server: Config.StartScan is required")
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

// guarded returns rt's handler behind the checks its route asks for.
func (s *Server) guarded(rt *route) http.Handler {
	switch {
	case rt.public:
		return rt.handle
	case rt.admin:
		return s.authenticate(requireAdmin(rt.handle))
	}
	return s.authenticate(rt.handle)
}

// routes lists every operation in the contract.
func (s *Server) routes() []route {
	return []route{
		{pattern: "GET /healthz", public: true, handle: s.getHealth},
		{pattern: "GET /readyz", public: true, handle: s.getReady},
		{pattern: "POST /auth/login", public: true, handle: s.login},
		{pattern: "POST /auth/logout", handle: s.logout},
		{pattern: "GET /admin/library/issues", admin: true, handle: s.getIssues},
		{pattern: "POST /admin/library/scan", admin: true, handle: s.postScan},
		{pattern: "GET /me", handle: s.getMe},
		{pattern: "POST /me/plays", handle: s.postPlay},
		{pattern: "PUT /me/hearts/{kind}/{id}", handle: s.putHeart},
		{pattern: "DELETE /me/hearts/{kind}/{id}", handle: s.deleteHeart},
		{pattern: "PUT /me/ratings/tracks/{id}", handle: s.putRating},
		{pattern: "DELETE /me/ratings/tracks/{id}", handle: s.deleteRating},
		{pattern: "GET /me/lastfm", handle: s.getLastfm},
		{pattern: "PATCH /me/lastfm", handle: s.patchLastfm},
		{pattern: "DELETE /me/lastfm", handle: s.deleteLastfm},
		{pattern: "POST /me/lastfm/connect", handle: s.connectLastfm},
		{pattern: "GET /lastfm/callback", public: true, handle: s.lastfmCallback},
		{pattern: "POST /me/now-playing", handle: s.postNowPlaying},
		{pattern: "GET /artwork/{hash}", handle: s.getArtwork},
		{pattern: "GET /albums", handle: s.getAlbums},
		{pattern: "GET /albums/{id}", handle: s.getAlbum},
		{pattern: "POST /albums/{id}/edits", admin: true, handle: s.postAlbumEdit},
		{pattern: "GET /albums/{id}/tags", admin: true, handle: s.getAlbumTags},
		{pattern: "GET /artists", handle: s.getArtists},
		{pattern: "GET /artists/{id}", handle: s.getArtist},
		{pattern: "GET /library", handle: s.getLibrary},
		{pattern: "GET /library/index", handle: s.getLibraryIndex},
		{pattern: "GET /search", handle: s.getSearch},
		{pattern: "GET /tracks/{id}", handle: s.getTrack},
		{pattern: "GET /tracks/{id}/stream", handle: s.getTrackStream},
	}
}

// getHealth reports that the process is up.
func (*Server) getHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// getReady reports whether the server is ready to serve. A failure is logged
// but never described to the client: as a warning, unless it is [ErrStarting].
func (s *Server) getReady(w http.ResponseWriter, r *http.Request) {
	if s.ready != nil {
		if err := s.ready(r.Context()); err != nil {
			level := slog.LevelWarn
			if errors.Is(err, ErrStarting) {
				level = slog.LevelDebug
			}
			s.log.Log(r.Context(), level, "not ready", "requestId", requestID(r.Context()), "err", err)
			writeProblem(w, r, http.StatusServiceUnavailable, "not ready")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
