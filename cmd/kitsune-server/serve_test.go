package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/server"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	apiPrefix      = "/api/v1"
	serveCmd       = "serve"
	listenAnyPort  = "127.0.0.1:0"
	stopDeadline   = 10 * time.Second
	pollDeadline   = 5 * time.Second
	pollInterval   = 10 * time.Millisecond
	loginURLPath   = apiPrefix + "/auth/login"
	healthURLPath  = apiPrefix + "/healthz"
	readyURLPath   = apiPrefix + "/readyz"
	forwardedFor   = "X-Forwarded-For"
	proxiedAddr    = "198.51.100.7"
	otherAddr      = "198.51.100.8"
	publicBaseURL  = "https://example.com/music"
	contentTypeKey = "Content-Type"
	jsonMediaType  = "application/json"
	hashSessionLen = 32
	wrongLoginPass = "not the password"
	failuresToCap  = 7
)

// syncBuffer is a bytes.Buffer that the server's goroutines and the test can
// use at the same time.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p) //nolint:wrapcheck // bytes.Buffer never fails; pass it through.
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// serveEnv is a config file for serve, with a data_dir of its own.
type serveEnv struct {
	config  string
	dataDir string
}

// newServeEnv writes a config that listens on a free local port. extra is
// more TOML, placed in the [server] table.
func newServeEnv(t *testing.T, extra string) serveEnv {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "data")
	body := fmt.Sprintf("[server]\nlisten = %q\n%s\n[storage]\ndata_dir = %q\n", listenAnyPort, extra, dataDir)
	return serveEnv{config: writeFile(t, "server.toml", body), dataDir: dataDir}
}

func (e serveEnv) load(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load(e.config, config.DefaultPaths(noEnv))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

func (e serveEnv) serveCLI() cliResult {
	var out, errOut bytes.Buffer
	code := cli([]string{serveCmd, "-config", e.config}, noEnv, strings.NewReader(""), &out, &errOut)
	return cliResult{code: code, stdout: out.String(), stderr: errOut.String()}
}

func TestServeRefusesWithoutDatabase(t *testing.T) {
	t.Parallel()
	e := newServeEnv(t, "")
	r := e.serveCLI()
	want := "kitsune-server users add -config " + shellQuote(e.config) + " -admin NAME"
	if r.code != exitFailure || !strings.Contains(r.stderr, "no users yet") || !strings.Contains(r.stderr, want) {
		t.Errorf("result = %+v, want exit 1 and a message with %q", r, want)
	}
	if _, err := os.Stat(filepath.Join(e.dataDir, store.FileName)); err == nil {
		t.Error("serve created the database")
	}
}

func TestServeMigrates(t *testing.T) {
	t.Parallel()
	e := newServeEnv(t, "")
	if err := os.MkdirAll(e.dataDir, dataDirMode); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.dataDir, store.FileName)
	st, err := store.Open(t.Context(), path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.CreateUser(t.Context(), store.NewUser{Name: "simon", PasswordHash: "h", Role: store.RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	markUnmigrated(t, path)

	cfg := e.load(t)
	a, err := openApp(t.Context(), &cfg, e.config, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("openApp: %v", err)
	}
	if err = a.close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(t.Context(), path, store.Options{}) // Refused if anything were still pending.
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServeRefusesEmptyDatabase(t *testing.T) {
	t.Parallel()
	e := newServeEnv(t, "")
	if err := os.MkdirAll(e.dataDir, dataDirMode); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.Context(), filepath.Join(e.dataDir, store.FileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	r := e.serveCLI()
	want := "kitsune-server users add -config " + shellQuote(e.config) + " -admin NAME"
	if r.code != exitFailure || !strings.Contains(r.stderr, "no users yet") || !strings.Contains(r.stderr, want) {
		t.Errorf("result = %+v, want exit 1 and a message with %q", r, want)
	}
}

func TestNoUsersErrorWithoutConfigFlag(t *testing.T) {
	t.Parallel()
	msg := (&noUsersError{}).Error()
	if !strings.Contains(msg, "users add -admin NAME") || strings.Contains(msg, "-config") {
		t.Errorf("message = %q, want the command without -config", msg)
	}
}

func TestShellQuote(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"/etc/kitsune.toml":  "/etc/kitsune.toml",
		"/tmp/my dir/s.toml": "'/tmp/my dir/s.toml'",
		"it's":               `'it'\''s'`,
		"":                   "''",
	}
	for in, want := range tests {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestServeListenError(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	held, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", listenAnyPort)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Close() })
	e := newServeEnv(t, "")
	if err = createUser(e.dataDir, simonName, goodPassword, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	e.config = writeFile(t, "server.toml",
		fmt.Sprintf("[server]\nlisten = %q\n[storage]\ndata_dir = %q\n", held.Addr().String(), e.dataDir))
	r := e.serveCLI()
	if r.code != exitFailure || !strings.Contains(r.stderr, "listen:") {
		t.Errorf("result = %+v, want exit 1 and a listen error", r)
	}
}

// runningApp is an app serving on a local port.
type runningApp struct {
	app    *app
	ln     net.Listener
	url    string
	logs   *syncBuffer
	client *http.Client
	cancel context.CancelFunc
	// finished is closed once serve has returned; err is its result.
	finished chan struct{}
	err      error
}

// reply is the part of a response the serve tests check.
type reply struct {
	status  int
	cookies []string
}

// newApp opens an app over a config with a user simon. It does not serve.
func newApp(t *testing.T, e serveEnv) (*app, *syncBuffer) {
	t.Helper()
	logs := &syncBuffer{}
	log := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := e.load(t)
	a, err := openApp(t.Context(), &cfg, e.config, log)
	if err != nil {
		t.Fatalf("openApp: %v", err)
	}
	return a, logs
}

// startApp creates simon, opens the app and serves it until stop.
func startApp(t *testing.T, extra string) *runningApp {
	t.Helper()
	e := newServeEnv(t, extra)
	if err := createUser(e.dataDir, simonName, goodPassword, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	return serveApp(t, e)
}

func serveApp(t *testing.T, e serveEnv) *runningApp {
	t.Helper()
	a, logs := newApp(t, e)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", listenAnyPort)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	ra := &runningApp{
		app: a, ln: ln, url: "http://" + ln.Addr().String(), logs: logs, cancel: cancel,
		client: &http.Client{Transport: &http.Transport{}}, finished: make(chan struct{}),
	}
	go func() {
		defer close(ra.finished)
		ra.err = a.serve(ctx, ln)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-ra.finished:
		case <-time.After(stopDeadline):
			t.Error("serve did not return after cleanup")
		}
		_ = a.close()
	})
	return ra
}

// stop shuts the app down and returns serve's result, failing the test if it
// takes too long.
func (ra *runningApp) stop(t *testing.T) error {
	t.Helper()
	ra.client.CloseIdleConnections()
	ra.cancel()
	select {
	case <-ra.finished:
		return ra.err
	case <-time.After(stopDeadline):
		t.Fatalf("serve did not return within %v", stopDeadline)
		return nil
	}
}

// do sends one request.
func (ra *runningApp) do(t *testing.T, method, path, body string, header http.Header) reply {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, ra.url+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(req.Header, header)
	resp, err := ra.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	if err = resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	return reply{status: resp.StatusCode, cookies: resp.Header.Values("Set-Cookie")}
}

// login posts a login for name with the given forwarded address and path prefix.
func (ra *runningApp) login(t *testing.T, prefix, name, password, forwarded string, cookie bool) reply {
	t.Helper()
	body, err := json.Marshal(api.LoginRequest{Name: name, Password: password, Cookie: cookie})
	if err != nil {
		t.Fatal(err)
	}
	h := http.Header{contentTypeKey: {jsonMediaType}}
	if forwarded != "" {
		h.Set(forwardedFor, forwarded)
	}
	return ra.do(t, http.MethodPost, prefix+loginURLPath, string(body), h)
}

// logEntries parses the JSON log lines.
func logEntries(t *testing.T, logs *syncBuffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		entries = append(entries, m)
	}
	return entries
}

func TestServeRunsAndStops(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	ra := startApp(t, "")
	ra.expectStatus(t, ra.do(t, http.MethodGet, healthURLPath, "", nil), http.StatusNoContent)
	ra.waitReady(t)
	ra.expectStatus(t, ra.login(t, "", simonName, goodPassword, "", false), http.StatusOK)
	if err := ra.stop(t); err != nil {
		t.Errorf("serve = %v, want nil", err)
	}
	if err := ra.app.close(); err != nil {
		t.Errorf("close = %v, want nil", err)
	}
	seen := logMessages(t, ra.logs)
	for _, msg := range []string{"listening", "shutting down", "stopped"} {
		if seen[msg] == nil {
			t.Errorf("no %q log line in %s", msg, ra.logs)
		}
	}
	if got := seen["listening"]["addr"]; got != ra.ln.Addr().String() {
		t.Errorf("listening addr = %v, want %s", got, ra.ln.Addr())
	}
}

// waitReady polls /readyz until it answers 204, which happens once the first
// scan has finished.
func (ra *runningApp) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(pollDeadline)
	for {
		got := ra.do(t, http.MethodGet, readyURLPath, "", nil)
		if got.status == http.StatusNoContent {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("/readyz = %d after %v, want 204; log: %s", got.status, pollDeadline, ra.logs)
		}
		time.Sleep(pollInterval)
	}
}

// expectStatus fails the test unless r has the given status.
func (*runningApp) expectStatus(t *testing.T, r reply, want int) {
	t.Helper()
	if r.status != want {
		t.Errorf("status = %d, want %d", r.status, want)
	}
}

// logMessages maps each log message to its (last) entry.
func logMessages(t *testing.T, logs *syncBuffer) map[string]map[string]any {
	t.Helper()
	seen := map[string]map[string]any{}
	for _, m := range logEntries(t, logs) {
		if msg, ok := m["msg"].(string); ok {
			seen[msg] = m
		}
	}
	return seen
}

func TestServeBehindProxy(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	ra := startApp(t, fmt.Sprintf("base_url = %q\ntrusted_proxies = [\"127.0.0.1\"]\n", publicBaseURL))
	ra.expectStatus(t, ra.do(t, http.MethodGet, "/music"+healthURLPath, "", nil), http.StatusNoContent)
	ra.expectStatus(t, ra.do(t, http.MethodGet, healthURLPath, "", nil), http.StatusNotFound)
	ra.checkProxyCookie(t)
	ra.checkForwardedLimits(t)
}

// checkProxyCookie checks the cookie of a login under the base path.
func (ra *runningApp) checkProxyCookie(t *testing.T) {
	t.Helper()
	r := ra.login(t, "/music", simonName, goodPassword, "", true)
	if r.status != http.StatusOK || len(r.cookies) != 1 {
		t.Fatalf("cookie login = %d with cookies %q, want 200 and one cookie", r.status, r.cookies)
	}
	c, err := http.ParseSetCookie(r.cookies[0])
	if err != nil {
		t.Fatal(err)
	}
	if !c.Secure || c.Path != "/music" {
		t.Errorf("cookie = %+v, want Secure and Path /music", c)
	}
}

// checkForwardedLimits checks that login failures are counted per forwarded
// address, on the real clock.
func (ra *runningApp) checkForwardedLimits(t *testing.T) {
	t.Helper()
	for i := 1; i <= failuresToCap; i++ {
		want := http.StatusUnauthorized
		if i == failuresToCap {
			want = http.StatusTooManyRequests
		}
		got := ra.login(t, "/music", fmt.Sprintf("ghost%d", i), wrongLoginPass, proxiedAddr, false)
		if got.status != want {
			t.Fatalf("failed login %d = %d, want %d", i, got.status, want)
		}
	}
	ra.expectStatus(t, ra.login(t, "/music", simonName, goodPassword, otherAddr, false), http.StatusOK)
}

func TestServeWarnsWithoutTrustedProxies(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newServeEnv(t, fmt.Sprintf("base_url = %q\n", publicBaseURL))
	if err := createUser(e.dataDir, simonName, goodPassword, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	a, logs := newApp(t, e)
	t.Cleanup(func() { _ = a.close() })
	if !strings.Contains(logs.String(), "trusted_proxies is empty") {
		t.Errorf("log = %s, want the trusted_proxies warning", logs)
	}
}

func TestServePurgesExpiredSessionsAtStart(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newServeEnv(t, "")
	if err := createUser(e.dataDir, simonName, goodPassword, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	hash := bytes.Repeat([]byte{1}, hashSessionLen)
	st, err := store.Open(t.Context(), filepath.Join(e.dataDir, store.FileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.UserByName(t.Context(), simonName)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	err = st.CreateSession(t.Context(), &store.NewSession{
		TokenHash: hash, UserID: u.ID, CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}

	ra := serveApp(t, e)
	deadline := time.Now().Add(pollDeadline)
	for !strings.Contains(ra.logs.String(), "purged expired sessions") {
		if time.Now().After(deadline) {
			t.Fatalf("no purge logged within %v; log: %s", pollDeadline, ra.logs)
		}
		time.Sleep(pollInterval)
	}
	if _, err = ra.app.store.SessionByTokenHash(t.Context(), hash); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("SessionByTokenHash = %v, want store.ErrNotFound", err)
	}
}

func TestServeStopsOnServeError(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newServeEnv(t, "")
	if err := createUser(e.dataDir, simonName, goodPassword, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	a, _ := newApp(t, e)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", listenAnyPort)
	if err != nil {
		t.Fatal(err)
	}
	if err = ln.Close(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.serve(t.Context(), ln) }()
	select {
	case err = <-done:
		if err == nil || !strings.Contains(err.Error(), "serve:") {
			t.Errorf("serve = %v, want an error containing %q", err, "serve:")
		}
	case <-time.After(pollDeadline):
		t.Fatal("serve did not return after its listener failed")
	}
	if err = a.close(); err != nil {
		t.Errorf("close = %v, want nil", err)
	}
}

// libraryFixture is a config with a library root, and that root.
type libraryFixture struct {
	env  serveEnv
	root string
}

// libraryEnv returns a config whose library has one root holding n copies of
// the sample FLAC, and a user simon.
func libraryEnv(t *testing.T, n int) libraryFixture {
	t.Helper()
	sample, err := os.ReadFile(filepath.Join("..", "..", "internal", "library", "testdata", "sample.flac"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close() //nolint:errcheck // The files are written; nothing to report.
	for i := range n {
		if err = dir.WriteFile(fmt.Sprintf("%02d.flac", i), sample, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e := newServeEnv(t, fmt.Sprintf("[library]\nroots = [%q]\n", root))
	if err = createUser(e.dataDir, simonName, goodPassword, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	return libraryFixture{env: e, root: root}
}

func TestServeScansLibrary(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	lib := libraryEnv(t, 1)
	ra := serveApp(t, lib.env)
	ra.waitReady(t)
	states, err := ra.app.store.FileStates(t.Context(), lib.root)
	if err != nil {
		t.Fatal(err)
	}
	if st := states["00.flac"]; st.TrackID == 0 || len(states) != 1 {
		t.Errorf("states = %+v, want the one track", states)
	}
	entry := logMessages(t, ra.logs)["scan finished"]
	if entry == nil {
		t.Fatalf("no \"scan finished\" log line in %s", ra.logs)
	}
	if added, ok := entry["added"].(float64); !ok || added != 1 {
		t.Errorf("scan finished = %v, want added 1", entry)
	}
}

func TestServeNotReadyUntilScanned(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newServeEnv(t, "")
	if err := createUser(e.dataDir, simonName, goodPassword, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	a, _ := newApp(t, e)
	t.Cleanup(func() { _ = a.close() })
	if err := a.ready(t.Context()); !errors.Is(err, server.ErrStarting) {
		t.Errorf("ready before the scan = %v, want server.ErrStarting", err)
	}
	a.scanned.Store(true)
	if err := a.ready(t.Context()); err != nil {
		t.Errorf("ready after the scan = %v, want nil", err)
	}
}

func TestServeStopsDuringScan(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	ra := serveApp(t, libraryEnv(t, 40).env)
	if err := ra.stop(t); err != nil {
		t.Errorf("serve = %v, want nil", err)
	}
	if err := ra.app.close(); err != nil {
		t.Errorf("close = %v, want nil", err)
	}
}

func TestServeRendersAndServesArtwork(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	lib := libraryEnv(t, 1)
	ra := serveApp(t, lib.env)
	ra.waitReady(t)

	dir := filepath.Join(lib.env.dataDir, "artwork")
	st, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("the artwork cache was not created: %v", err)
	}
	if mode := st.Mode().Perm(); mode != 0o700 {
		t.Errorf("artwork dir mode = %v, want 0700", mode)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*", "*-128.jpg"))
	if err != nil || len(files) != 1 {
		t.Fatalf("renditions = %v, %v; want one cover rendered by the scan", files, err)
	}
	if covers, ok := logMessages(t, ra.logs)["scan finished"]["covers"].(float64); !ok || covers != 1 {
		t.Errorf("scan finished covers = %v, want 1", covers)
	}
	hash, _, _ := strings.Cut(filepath.Base(files[0]), "-")
	got := ra.getArtwork(t, hash)
	if got.status != http.StatusOK || got.contentType != "image/jpeg" {
		t.Errorf("GET artwork = %d %q, want 200 image/jpeg", got.status, got.contentType)
	}
}

// stalledDrain is the drain a stalled client runs out.
const stalledDrain = 200 * time.Millisecond

// stalledFileSize is far more than the socket buffers hold, so a client that
// stops reading keeps the request active.
const stalledFileSize = 64 << 20

// streamURLPath returns the URL path of the only track in the library.
func (ra *runningApp) streamURLPath(t *testing.T) string {
	t.Helper()
	page, err := ra.app.store.Albums(t.Context(), store.SortAdded, 1, "")
	if err != nil || len(page.Albums) != 1 {
		t.Fatalf("Albums = %+v, %v; want one", page.Albums, err)
	}
	album, err := ra.app.store.Album(t.Context(), page.Albums[0].ID)
	if err != nil || len(album.Tracks) != 1 {
		t.Fatalf("Album = %+v, %v; want one track", album, err)
	}
	return apiPrefix + "/tracks/" + album.Tracks[0].ID + "/stream"
}

// openStream starts streaming path as simon and returns the response, with its
// body unread and open.
func (ra *runningApp) openStream(t *testing.T, path string) *http.Response {
	t.Helper()
	res, err := ra.app.auth.Login(t.Context(), simonName, goodPassword, "test/1")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ra.url+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+res.Token)
	resp, err := ra.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestServeStopsWithAStalledStream(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	lib := libraryEnv(t, 1)
	ra := serveApp(t, lib.env)
	ra.waitReady(t)
	// Stat comes from the open file, so growing it needs no new scan.
	if err := os.Truncate(filepath.Join(lib.root, "00.flac"), stalledFileSize); err != nil {
		t.Fatal(err)
	}
	ra.app.drain = stalledDrain

	resp := ra.openStream(t, ra.streamURLPath(t))
	defer resp.Body.Close() //nolint:errcheck // Read-only.
	if resp.StatusCode != http.StatusOK || resp.ContentLength != stalledFileSize {
		t.Fatalf("stream = %d of %d bytes, want 200 of %d", resp.StatusCode, resp.ContentLength, stalledFileSize)
	}
	if err := ra.stop(t); err != nil {
		t.Errorf("serve = %v, want nil", err)
	}
	entry := logMessages(t, ra.logs)["closed connections still open after the drain"]
	if entry == nil {
		t.Fatalf("no line for the closed connections in %s", ra.logs)
	}
	if entry["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", entry["level"])
	}
}

// errCloseListener is what [failingListener] returns from Close.
var errCloseListener = errors.New("close listener failed")

// failingListener fails to close, which fails the shutdown.
type failingListener struct{ net.Listener }

func (l failingListener) Close() error {
	return errors.Join(l.Listener.Close(), errCloseListener)
}

func TestServeReportsOtherShutdownErrors(t *testing.T) { //nolint:paralleltest // overrides authParams.
	useCheapParams(t)
	e := newServeEnv(t, "")
	if err := createUser(e.dataDir, simonName, goodPassword, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	a, _ := newApp(t, e)
	t.Cleanup(func() { _ = a.close() })
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", listenAnyPort)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- a.serve(ctx, failingListener{ln}) }()
	// A shutdown only closes the listener once Serve has taken it.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+healthURLPath, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err = resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err = <-done:
		if !errors.Is(err, errCloseListener) || !strings.Contains(err.Error(), "shutdown:") {
			t.Errorf("serve = %v, want a shutdown error wrapping %v", err, errCloseListener)
		}
	case <-time.After(stopDeadline):
		t.Fatal("serve did not return")
	}
}

// getArtwork fetches the 128 px rendition of hash as simon.
func (ra *runningApp) getArtwork(t *testing.T, hash string) artworkReply {
	t.Helper()
	res, err := ra.app.auth.Login(t.Context(), simonName, goodPassword, "test/1")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ra.url+apiPrefix+"/artwork/"+hash+"?size=128", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+res.Token)
	resp, err := ra.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck // Read-only.
	return artworkReply{status: resp.StatusCode, contentType: resp.Header.Get(contentTypeKey)}
}

// artworkReply is the status and media type of an artwork response.
type artworkReply struct {
	status      int
	contentType string
}
