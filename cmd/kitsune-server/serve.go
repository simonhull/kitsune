package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/events"
	"github.com/simonhull/kitsune/internal/lastfm"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/scrobbler"
	"github.com/simonhull/kitsune/internal/server"
	"github.com/simonhull/kitsune/internal/settings"
	"github.com/simonhull/kitsune/internal/store"
	"github.com/simonhull/kitsune/internal/webui"
)

// serveName is the name of the serve command.
const serveName = "serve"

const (
	// shutdownTimeout is how long in-flight requests get to finish after a
	// signal, after which the connections left are closed. Docker sends SIGKILL
	// 10 s after SIGTERM.
	shutdownTimeout = 8 * time.Second
	// purgeInterval is how often expired sessions are deleted.
	purgeInterval = time.Hour
	// readHeaderTimeout bounds how long a client may take to send its headers.
	readHeaderTimeout = 10 * time.Second
	// idleTimeout closes keep-alive connections that sit unused.
	idleTimeout = 2 * time.Minute
)

// errStopping means a scan was asked for while the app is not serving: it has
// not started, or shutdown has begun.
var errStopping = errors.New("the server is stopping")

// newLogger returns a JSON logger on w at level, which the Server page changes.
func newLogger(w io.Writer, level *slog.LevelVar) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

// runServe runs the server until SIGINT or SIGTERM.
func runServe(args []string, env config.Env, stderr io.Writer) error {
	flags := flag.NewFlagSet(serveName, flag.ContinueOnError)
	flags.SetOutput(io.Discard) // cli prints the usage itself.
	path := flags.String("config", "", "config file")
	var sets setFlags
	flags.Var(&sets, "set", "seed a setting: KEY=VALUE")
	if err := flags.Parse(args); err != nil {
		return &usageError{msg: err.Error()}
	}
	if err := noArgs(serveName, flags.Args()); err != nil {
		return err
	}
	cfg, keys, err := config.LoadKeys(*path, config.DefaultPaths(env))
	if err != nil {
		return err //nolint:wrapcheck // config's errors already name the file and key.
	}
	seeds, err := seedsFor(&cfg, keys, env, sets)
	if err != nil {
		return err
	}
	// Until the database says otherwise, log at the level the seeds ask for.
	level := new(slog.LevelVar)
	level.Set(cfg.Log.Level)
	log := newLogger(stderr, level)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// After the first signal, restore the default handling so a second one
	// kills a slow drain.
	context.AfterFunc(ctx, stop)

	lastfmURL, _ := env(lastfmURLEnv)
	l := launch{
		lastfmURL: lastfmURL, seeds: seeds, level: level, listenFromFile: keys.Has("server.listen"),
		configDir: configDir(*path, config.DefaultPaths(env)),
	}
	return serveConfig(ctx, &cfg, l, log)
}

// configDir is the folder of the config file serve reads, named or default, or
// "" when there is none.
func configDir(named string, paths config.Paths) string {
	if name := cmp.Or(named, paths.ConfigFile); name != "" {
		if abs, err := filepath.Abs(name); err == nil {
			return filepath.Dir(abs)
		}
	}
	return ""
}

// lastfmURLEnv names the root of a fake Last.fm to talk to in place of the
// real one, serving the API at /2.0/ and the approval page at /api/auth/. Only
// tests and manual checks set it; production never needs it.
const lastfmURLEnv = "KITSUNE_LASTFM_URL"

// errBadLastfmURL means KITSUNE_LASTFM_URL is not an absolute http URL.
var errBadLastfmURL = errors.New(lastfmURLEnv + " must be an absolute http or https URL")

// launch is what serve takes besides the config: the fake Last.fm to use, if
// any, the seeds for settings the database has no value for, and the logger's
// level, which the database's log.level sets.
type launch struct {
	lastfmURL      string // "" means the real Last.fm.
	seeds          []settings.Seed
	level          *slog.LevelVar // Nil means the level can't change.
	listenFromFile bool           // server.toml set listen.
	configDir      string         // The config file's folder, which the folder picker won't offer.
}

// serveConfig opens the app, listens and serves until ctx ends.
func serveConfig(ctx context.Context, cfg *config.Config, l launch, log *slog.Logger) (err error) {
	a, err := openApp(ctx, cfg, l, log)
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

// app is a server with the state it needs to run.
type app struct {
	log      *slog.Logger
	store    *store.Store
	auth     *auth.Service
	scanner  *library.Scanner
	watcher  *library.Watcher // Follows the roots and scans what changes.
	events   *events.Hub      // What every client hears.
	lastfm   *scrobbler.Scrobbler
	scanned  atomic.Bool // Set when the first scan has ended, whatever its outcome.
	http     *http.Server
	drain    time.Duration // How long shutdown waits for in-flight requests.
	basePath string
	dataDir  string
	baseURL  string
	// setupCode makes the first account from outside the server's network.
	// It is "" when the server started with accounts.
	setupCode string
	covers    *artwork.Cache
	genres    *library.Genres

	// live holds the settings, and the owners of the ones that apply at once
	// follow it (follow.go).
	live      *settings.Service
	level     *slog.LevelVar // Nil means the level can't change.
	lastfmURL string         // A stand-in Last.fm's root, or "".
	changes   ownerChanges

	// Background work outlives requests. Shutdown cancels it and waits, so a
	// pending scan batch is written before the store closes.
	work    sync.WaitGroup
	workMu  sync.Mutex      // Orders a scan joining work against shutdown's wait.
	workCtx context.Context //nolint:containedctx // The work's lifetime, not a request's. Nil unless serving.
}

// openApp creates data_dir if needed, opens and migrates the database in it,
// seeds the settings it has no value for, and builds the HTTP server.
func openApp(ctx context.Context, cfg *config.Config, l launch, log *slog.Logger) (*app, error) {
	if err := os.MkdirAll(cfg.Storage.DataDir, dataDirMode); err != nil {
		return nil, fmt.Errorf("create data_dir: %w", err)
	}
	dbPath := filepath.Join(cfg.Storage.DataDir, store.FileName)
	st, err := store.Open(ctx, dbPath, store.Options{Migrate: true, Logger: log})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	live, err := openSettings(ctx, st, l.seeds, log)
	if err != nil {
		return nil, errors.Join(err, st.Close())
	}
	if l.level != nil {
		l.level.Set(live.Snapshot().LogLevel)
	}
	a, err := buildApp(ctx, cfg, &l, st, live, log)
	if err != nil {
		return nil, errors.Join(err, st.Close())
	}
	return a, nil
}

// openSettings stores the seeds the database has no value for and opens the
// settings.
func openSettings(ctx context.Context, st *store.Store, seeds []settings.Seed, log *slog.Logger) (*settings.Service, error) {
	if err := settings.Apply(ctx, st, seeds, log); err != nil {
		return nil, err //nolint:wrapcheck // It says it is the settings.
	}
	return settings.Open(ctx, st) //nolint:wrapcheck // As above, and it names the key.
}

// firstRunCode returns a fresh setup code when st has no accounts, so the first
// can be made in a browser, and "" when it has some.
func firstRunCode(ctx context.Context, st *store.Store) (string, error) {
	n, err := st.CountUsers(ctx)
	if err != nil {
		return "", fmt.Errorf("count users: %w", err)
	}
	if n > 0 {
		return "", nil
	}
	return newSetupCode(), nil
}

// buildApp wires the auth service and the HTTP server over st, with the settings
// in live. ctx ends tag edits: a shutdown stops one between files, inside the
// drain.
func buildApp(ctx context.Context, cfg *config.Config, l *launch, st *store.Store, live *settings.Service, log *slog.Logger) (*app, error) {
	snap := live.Snapshot()
	svc, err := auth.New(st, auth.Options{Params: authParams()})
	if err != nil {
		return nil, fmt.Errorf("start auth: %w", err)
	}
	covers, err := openArtworkCache(cfg.Storage.DataDir)
	if err != nil {
		return nil, err
	}
	hub := events.New(events.Options{})
	sender, err := newScrobbler(lastfmConfig(snap), l.lastfmURL, st, hub, log)
	if err != nil {
		return nil, err
	}
	genres, err := readGenres(ctx, st)
	if err != nil {
		return nil, err
	}
	setupCode, err := firstRunCode(ctx, st)
	if err != nil {
		return nil, err
	}
	// base_url and trusted_proxies apply as the server started: changing them
	// live could cut off the page that changed them.
	public := config.Server{BaseURL: snap.BaseURL}
	a := &app{
		log: log, store: st, auth: svc, lastfm: sender, drain: shutdownTimeout,
		scanner: newScanner(snap, st, covers, genres, hub, log), events: hub,
		basePath: public.BasePath(), dataDir: cfg.Storage.DataDir, baseURL: snap.BaseURL,
		setupCode: setupCode, covers: covers, genres: genres,
		live: live, level: l.level, lastfmURL: l.lastfmURL, changes: subscribe(live),
	}
	a.watcher = library.NewWatcher(a.scanner, library.WatchOptions{OnChange: server.FoldersChanged(hub)})
	a.http = newHTTPServer(a.handler(ctx, cfg, l, &public), log)
	if snap.BaseURL != "" && len(snap.TrustedProxies) == 0 {
		log.Warn("base_url is set but trusted_proxies is empty; every client will share the proxy's address for login rate limits")
	}
	return a, nil
}

// handler returns the API and web client over a's parts. ctx ends tag edits.
func (a *app) handler(ctx context.Context, cfg *config.Config, l *launch, public *config.Server) *server.Server {
	return server.New(server.Config{
		Auth:           a.auth,
		Logger:         a.log,
		ClientIP:       server.ForwardedClientIP(a.live.Snapshot().TrustedProxies),
		SecureCookies:  public.HTTPS(),
		BasePath:       public.BasePath(),
		Ready:          a.ready,
		Artwork:        a.covers,
		Library:        a.store,
		Scanner:        a.scanner,
		StartScan:      a.startScan,
		Genres:         a.genres,
		Context:        ctx,
		Web:            webClient(public.BasePath(), a.log),
		Lastfm:         a.lastfm,
		PublicURL:      public.BaseURL,
		SetupCode:      a.setupCode,
		Settings:       a.live,
		Listen:         cfg.Server.Listen,
		ListenFromFile: l.listenFromFile,
		DataDir:        cfg.Storage.DataDir,
		ConfigDir:      l.configDir,
		Version:        version(),
		Watcher:        a.watcher,
		Events:         a.events,
	})
}

// lastfmConfig is the Last.fm part of snap, as [lastfmClient] takes it.
func lastfmConfig(snap *settings.Snapshot) config.LastFM {
	return config.LastFM{APIKey: snap.LastfmAPIKey, SharedSecret: config.Secret(snap.LastfmSharedSecret)}
}

// readGenres returns the library's genres, read from st.
func readGenres(ctx context.Context, st *store.Store) (*library.Genres, error) {
	genres := library.NewGenres(st)
	if err := genres.Refresh(ctx); err != nil {
		return nil, fmt.Errorf("read genres: %w", err)
	}
	return genres, nil
}

// newScanner returns the library scanner over st that snap sets up.
func newScanner(snap *settings.Snapshot, st *store.Store, covers *artwork.Cache, genres *library.Genres, hub *events.Hub, log *slog.Logger) *library.Scanner {
	return library.NewScanner(st, library.Options{
		Roots:        snap.Roots,
		MissingGrace: snap.MissingGrace,
		Logger:       log,
		Artwork:      covers,
		TagWrites:    snap.TagWrites,
		Genres:       genres,
		Notify:       server.NewLibraryEvents(hub),
		QuickScan:    library.Daily{Hour: snap.QuickScan.Hour, Minute: snap.QuickScan.Minute},
	})
}

// newScrobbler returns the sender for st, over the Last.fm client cfg sets up.
func newScrobbler(cfg config.LastFM, fakeURL string, st *store.Store, hub *events.Hub, log *slog.Logger) (*scrobbler.Scrobbler, error) {
	client, err := lastfmClient(cfg, fakeURL, log)
	if err != nil {
		return nil, err
	}
	return scrobbler.New(st, scrobbler.Options{
		Client: client, MBIDKeys: library.TrackMBIDKeys(), Logger: log, OnChange: server.LastfmChanged(hub),
	}), nil
}

// lastfmClient returns the Last.fm client cfg sets up, aimed at the fake at
// fakeURL when that is set, or nil when Last.fm is not set up.
func lastfmClient(cfg config.LastFM, fakeURL string, log *slog.Logger) (*lastfm.Client, error) {
	if !cfg.Enabled() {
		return nil, nil //nolint:nilnil // No client is how Last.fm being off reads.
	}
	client := lastfm.New(cfg.APIKey, string(cfg.SharedSecret))
	if fakeURL == "" {
		return client, nil
	}
	u, err := url.Parse(fakeURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errBadLastfmURL
	}
	root := strings.TrimSuffix(fakeURL, "/")
	client.APIBase, client.AuthBase = root+"/2.0/", root+"/api/auth/"
	log.Warn("talking to a stand-in Last.fm, not the real one", "env", lastfmURLEnv, "url", root)
	return client, nil
}

// webClient returns the client built into this binary, and says so when the
// server will give the API alone.
func webClient(basePath string, log *slog.Logger) fs.FS {
	web := webui.Files()
	switch {
	case web == nil:
		log.Info("this build has no web client: serving the API alone")
	case basePath != "":
		log.Warn("the web client only runs at the root of a host: serving the API alone", "basePath", basePath)
	}
	return web
}

// artworkDir is the cover cache's folder inside data_dir.
const artworkDir = "artwork"

// privateDirMode is the mode of the cover cache: only the server reads it.
const privateDirMode = 0o700

// openArtworkCache returns the cover cache under dataDir, creating its folder.
func openArtworkCache(dataDir string) (*artwork.Cache, error) {
	dir := filepath.Join(dataDir, artworkDir)
	if err := os.MkdirAll(dir, privateDirMode); err != nil {
		return nil, fmt.Errorf("create artwork cache: %w", err)
	}
	return artwork.NewCache(dir), nil
}

// ready backs /readyz: it fails with [server.ErrStarting] until the first scan
// has finished, then with the database's error if it can't be reached.
func (a *app) ready(ctx context.Context) error {
	if !a.scanned.Load() {
		return server.ErrStarting
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
// returns an error if the listener fails or the shutdown does. A drain that
// runs out of time is not a failure: see [app.shutdown].
func (a *app) serve(ctx context.Context, ln net.Listener) error {
	a.log.InfoContext(ctx, "listening",
		"addr", ln.Addr().String(), "version", version(), "basePath", a.basePath, "dataDir", a.dataDir)
	if a.setupCode != "" {
		// The one secret the server ever logs, once: whoever reads the log runs
		// the server, and the code dies with the first account.
		a.log.WarnContext(ctx, "no accounts yet: open Kitsune in a browser to make the first one",
			"setupCode", a.setupCode, "url", setupURL(a.baseURL, ln.Addr()))
	}

	bgCtx, cancelBackground := context.WithCancel(ctx)
	a.workMu.Lock()
	a.workCtx = bgCtx
	a.workMu.Unlock()
	a.work.Go(func() { a.purgeSessions(bgCtx) })
	a.work.Go(func() { a.lastfm.Run(bgCtx) })
	a.work.Go(func() { a.scanner.Run(bgCtx, func() { a.scanned.Store(true) }) })
	a.work.Go(func() { a.followLibrary(bgCtx) })
	a.work.Go(func() { a.followRest(bgCtx) })
	a.work.Go(func() { a.scanner.RunFingerprints(bgCtx) }) // Readiness never waits for it.
	a.work.Go(func() { a.watcher.Run(bgCtx) })

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
	a.workMu.Lock()
	a.workCtx = nil
	a.workMu.Unlock()
	cancelBackground()
	a.work.Wait()
	a.log.InfoContext(context.WithoutCancel(ctx), "stopped")
	return result
}

// startScan claims a scan and runs it as background work. It returns
// [library.ErrScanRunning] while another scan runs, and [errStopping] when the
// app is not serving.
func (a *app) startScan(mode library.Mode) error {
	a.workMu.Lock()
	defer a.workMu.Unlock()
	ctx := a.workCtx
	if ctx == nil {
		return errStopping
	}
	run, err := a.scanner.Claim(mode)
	if err != nil {
		return fmt.Errorf("start scan: %w", err)
	}
	a.work.Go(func() { _, _ = run(ctx) }) // The scanner logs how it went.
	return nil
}

// shutdown drains the HTTP server and waits for Serve to return. Streams are
// expected to outlast a drain, since a client may hold one open for as long as
// it likes, so the connections left at the deadline are closed and that is a
// clean stop.
func (a *app) shutdown(ctx context.Context, served <-chan error) error {
	logCtx := context.WithoutCancel(ctx)
	a.log.InfoContext(logCtx, "shutting down")
	drain, cancel := context.WithTimeout(logCtx, a.drain)
	defer cancel()
	var result error
	if err := a.http.Shutdown(drain); err != nil {
		_ = a.http.Close() // Cut off what the drain could not finish.
		if errors.Is(err, context.DeadlineExceeded) {
			a.log.InfoContext(logCtx, "closed connections still open after the drain", "drain", a.drain)
		} else {
			result = fmt.Errorf("shutdown: %w", err)
		}
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
