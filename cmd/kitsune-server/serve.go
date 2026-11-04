package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/server"
	"github.com/simonhull/kitsune/internal/store"
)

// serveName is the name of the serve command.
const serveName = "serve"

const (
	// shutdownTimeout is how long in-flight requests get to finish after a
	// signal. Docker sends SIGKILL 10 s after SIGTERM.
	shutdownTimeout = 8 * time.Second
	// purgeInterval is how often expired sessions are deleted.
	purgeInterval = time.Hour
	// readHeaderTimeout bounds how long a client may take to send its headers.
	readHeaderTimeout = 10 * time.Second
	// idleTimeout closes keep-alive connections that sit unused.
	idleTimeout = 2 * time.Minute
)

// plainShellWord matches a string that a shell reads as one word.
var plainShellWord = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

// shellQuote quotes s for a POSIX shell, leaving plain words alone.
func shellQuote(s string) string {
	if plainShellWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// noUsersError means the database has no users, so nobody could sign in.
type noUsersError struct{ configPath string }

func (e *noUsersError) Error() string {
	cmd := "kitsune-server users add"
	if e.configPath != "" {
		cmd += " -config " + shellQuote(e.configPath)
	}
	return "no users yet; create an administrator first:\n  " + cmd + " -admin NAME"
}

// newLogger returns a JSON logger on w at the given level.
func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

// runServe runs the server until SIGINT or SIGTERM.
func runServe(args []string, env config.Env, stderr io.Writer) error {
	flags := flag.NewFlagSet(serveName, flag.ContinueOnError)
	flags.SetOutput(io.Discard) // cli prints the usage itself.
	path := flags.String("config", "", "config file")
	if err := flags.Parse(args); err != nil {
		return &usageError{msg: err.Error()}
	}
	if err := noArgs(serveName, flags.Args()); err != nil {
		return err
	}
	cfg, err := config.Load(*path, config.DefaultPaths(env))
	if err != nil {
		return err //nolint:wrapcheck // config's errors already name the file and key.
	}
	log := newLogger(stderr, cfg.Log.Level)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// After the first signal, restore the default handling so a second one
	// kills a slow drain.
	context.AfterFunc(ctx, stop)

	return serveConfig(ctx, &cfg, *path, log)
}

// serveConfig opens the app, listens and serves until ctx ends.
func serveConfig(ctx context.Context, cfg *config.Config, configPath string, log *slog.Logger) (err error) {
	a, err := openApp(ctx, cfg, configPath, log)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := a.close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.Server.Listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	return a.serve(ctx, ln)
}

// errScanning is why /readyz fails until the first library scan has finished.
var errScanning = errors.New("initial scan not finished")

// app is a server with the state it needs to run.
type app struct {
	log       *slog.Logger
	store     *store.Store
	auth      *auth.Service
	scanner   *library.Scanner
	quickScan config.ClockTime
	scanned   atomic.Bool // Set when the first scan has ended, whatever its outcome.
	http      *http.Server
	basePath  string
	dataDir   string
}

// openApp opens the database and builds the HTTP server. It refuses to start
// when there are no users, without creating anything.
func openApp(ctx context.Context, cfg *config.Config, configPath string, log *slog.Logger) (*app, error) {
	noUsers := &noUsersError{configPath: configPath}
	dbPath := filepath.Join(cfg.Storage.DataDir, store.FileName)
	if _, err := os.Stat(dbPath); errors.Is(err, fs.ErrNotExist) {
		return nil, noUsers
	}
	st, err := store.Open(ctx, dbPath, store.Options{})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err = requireUsers(ctx, st, noUsers); err != nil {
		return nil, errors.Join(err, st.Close())
	}
	a, err := buildApp(cfg, st, log)
	if err != nil {
		return nil, errors.Join(err, st.Close())
	}
	return a, nil
}

// requireUsers returns noUsers when st has no users.
func requireUsers(ctx context.Context, st *store.Store, noUsers *noUsersError) error {
	n, err := st.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if n == 0 {
		return noUsers
	}
	return nil
}

// buildApp wires the auth service and the HTTP server over st.
func buildApp(cfg *config.Config, st *store.Store, log *slog.Logger) (*app, error) {
	svc, err := auth.New(st, auth.Options{Params: authParams()})
	if err != nil {
		return nil, fmt.Errorf("start auth: %w", err)
	}
	trusted := make([]netip.Prefix, 0, len(cfg.Server.TrustedProxies))
	for _, n := range cfg.Server.TrustedProxies {
		trusted = append(trusted, n.Prefix)
	}
	a := &app{
		log:   log,
		store: st,
		auth:  svc,
		scanner: library.NewScanner(st, library.Options{
			Roots:        cfg.Library.Roots,
			MissingGrace: cfg.Library.MissingGrace,
			Logger:       log,
		}),
		quickScan: cfg.Library.QuickScan,
		basePath:  cfg.Server.BasePath(),
		dataDir:   cfg.Storage.DataDir,
	}
	handler := server.New(server.Config{
		Auth:          svc,
		Logger:        log,
		ClientIP:      server.ForwardedClientIP(trusted),
		SecureCookies: cfg.Server.HTTPS(),
		BasePath:      cfg.Server.BasePath(),
		Ready:         a.ready,
	})
	a.http = newHTTPServer(handler, log)
	if cfg.Server.BaseURL != "" && len(trusted) == 0 {
		log.Warn("base_url is set but trusted_proxies is empty; every client will share the proxy's address for login rate limits")
	}
	return a, nil
}

// ready backs /readyz: it fails with errScanning until the first scan has
// finished, then with the database's error if it can't be reached.
func (a *app) ready(ctx context.Context) error {
	if !a.scanned.Load() {
		return errScanning
	}
	return a.store.Ping(ctx) //nolint:wrapcheck // The store's error already says what failed.
}

// newHTTPServer returns the HTTP server for h. It sets no ReadTimeout or
// WriteTimeout, because streams and event feeds run long.
func newHTTPServer(h http.Handler, log *slog.Logger) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}

// close closes the database.
func (a *app) close() error {
	if err := a.store.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}

// serve serves on ln until ctx ends, then drains in-flight requests. It
// returns an error if the listener fails or the drain overruns.
func (a *app) serve(ctx context.Context, ln net.Listener) error {
	a.log.InfoContext(ctx, "listening",
		"addr", ln.Addr().String(), "version", version(), "basePath", a.basePath, "dataDir", a.dataDir)

	// The purge loop and the scanner stop together, and shutdown waits for both,
	// so a pending scan batch is written before the store closes.
	bgCtx, cancelBackground := context.WithCancel(ctx)
	var background sync.WaitGroup
	background.Go(func() { a.purgeSessions(bgCtx) })
	background.Go(func() {
		a.scanner.Run(bgCtx, a.quickScan.Hour, a.quickScan.Minute, func() { a.scanned.Store(true) })
	})

	served := make(chan error, 1)
	go func() { served <- a.http.Serve(ln) }()

	var result error
	select {
	case err := <-served:
		// Serve returned before shutdown: the listener failed. Stop the
		// handlers before the caller closes the store.
		result = fmt.Errorf("serve: %w", err)
		_ = a.http.Close() // Nothing more to report: the serve error is the cause.
	case <-ctx.Done():
		result = a.shutdown(ctx, served)
	}
	cancelBackground()
	background.Wait()
	a.log.InfoContext(context.WithoutCancel(ctx), "stopped")
	return result
}

// shutdown drains the HTTP server and waits for Serve to return.
func (a *app) shutdown(ctx context.Context, served <-chan error) error {
	a.log.InfoContext(context.WithoutCancel(ctx), "shutting down")
	drain, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	var result error
	if err := a.http.Shutdown(drain); err != nil {
		_ = a.http.Close() // Cut off what the drain could not finish.
		result = fmt.Errorf("shutdown: %w", err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		result = errors.Join(result, fmt.Errorf("serve: %w", err))
	}
	return result
}

// purgeSessions deletes expired sessions at start and then every
// purgeInterval, until ctx ends.
func (a *app) purgeSessions(ctx context.Context) {
	a.purgeOnce(ctx)
	ticker := time.NewTicker(purgeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.purgeOnce(ctx)
		}
	}
}

// purgeOnce deletes expired sessions and logs the outcome.
func (a *app) purgeOnce(ctx context.Context) {
	n, err := a.auth.PurgeExpiredSessions(ctx)
	switch {
	case err != nil && ctx.Err() == nil:
		a.log.WarnContext(ctx, "purge expired sessions", "err", err)
	case err == nil && n > 0:
		a.log.InfoContext(ctx, "purged expired sessions", "count", n)
	}
}
