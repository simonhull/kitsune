package settings_test

import (
	"bytes"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/settings"
	"github.com/simonhull/kitsune/internal/store"
)

// secretValue is a shared secret no log or error may hold.
const (
	secretValue = "s3cr3t-never-shown"
	debugJSON   = `"debug"`
	trueJSON    = `true`
)

// syncBuffer is a bytes.Buffer a logger and the test can share.
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

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), store.FileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func open(t *testing.T, st *store.Store) *settings.Service {
	t.Helper()
	svc, err := settings.Open(t.Context(), st)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func textSeed(t *testing.T, k settings.Key, text string, src settings.Source) settings.Seed {
	t.Helper()
	s, ok, err := settings.TextSeed(k, text, src)
	if err != nil || !ok {
		t.Fatalf("TextSeed(%s, %q) = %v, %v", k, text, ok, err)
	}
	return s
}

func apply(t *testing.T, st *store.Store, seeds ...settings.Seed) string {
	t.Helper()
	logs := &syncBuffer{}
	if err := settings.Apply(t.Context(), st, seeds, slog.New(slog.NewJSONHandler(logs, nil))); err != nil {
		t.Fatal(err)
	}
	return logs.String()
}

func change(t *testing.T, svc *settings.Service, values map[settings.Key]string) *settings.Snapshot {
	t.Helper()
	snap, err := svc.Change(t.Context(), raw(values))
	if err != nil {
		t.Fatalf("Change(%v): %v", values, err)
	}
	return snap
}

func raw(values map[settings.Key]string) map[string][]byte {
	out := make(map[string][]byte, len(values))
	for k, v := range values {
		out[string(k)] = []byte(v)
	}
	return out
}

func TestOpenEmptyHasTheDefaults(t *testing.T) {
	t.Parallel()
	snap := open(t, openStore(t)).Snapshot()
	d := config.Default()
	if snap.QuickScan != d.Library.QuickScan || snap.MissingGrace != d.Library.MissingGrace ||
		snap.LogLevel != slog.LevelInfo || len(snap.Roots) != 0 || snap.TagWrites || snap.LastfmEnabled() {
		t.Errorf("snapshot = %+v, want the defaults", snap.Values)
	}
	for _, k := range settings.Keys() {
		if src := snap.Source(k); src != settings.SourceDefault {
			t.Errorf("source of %s = %q, want default", k, src)
		}
	}
}

// checkLogs fails unless logs hold every one of want and none of never.
func checkLogs(t *testing.T, logs string, want, never []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(logs, w) {
			t.Errorf("logs lack %q: %s", w, logs)
		}
	}
	for _, n := range never {
		if strings.Contains(logs, n) {
			t.Errorf("logs hold %q: %s", n, logs)
		}
	}
}

// TestSeedThenWebThenRestart is the life of a setting: the environment seeds
// it, the browser changes it, and a restart with the same environment keeps
// the browser's value and warns, without the value.
func TestSeedThenWebThenRestart(t *testing.T) {
	t.Parallel()
	st := openStore(t)
	seeds := []settings.Seed{
		textSeed(t, settings.LastfmAPIKey, "envkey", settings.SourceEnv),
		textSeed(t, settings.LastfmSharedSecret, secretValue, settings.SourceEnv),
	}
	checkLogs(t, apply(t, st, seeds...), []string{"setting seeded"}, []string{secretValue, "envkey"})
	svc := open(t, st)
	snap := svc.Snapshot()
	if snap.LastfmAPIKey != "envkey" || snap.LastfmSharedSecret != secretValue || !snap.LastfmEnabled() {
		t.Fatalf("seeded = %+v", snap.Values)
	}
	if src := snap.Source(settings.LastfmSharedSecret); src.From(settings.LastfmSharedSecret) != "KITSUNE_LASTFM_SHARED_SECRET" {
		t.Errorf("source = %q, want env", src)
	}

	snap = change(t, svc, map[settings.Key]string{settings.LastfmAPIKey: `"webkey"`, settings.LastfmSharedSecret: `"websecret"`})
	if snap.Source(settings.LastfmAPIKey) != settings.SourceWeb {
		t.Errorf("after the web = %q from %q", snap.LastfmAPIKey, snap.Source(settings.LastfmAPIKey))
	}

	logs := apply(t, st, seeds...) // A restart with the same environment.
	if got := strings.Count(logs, `"level":"WARN"`); got != 2 {
		t.Errorf("restart logs %d warnings, want one per key: %s", got, logs)
	}
	checkLogs(t, logs, []string{`"key":"lastfm.shared_secret"`, "KITSUNE_LASTFM_SHARED_SECRET"}, []string{secretValue, "websecret"})
	if snap = open(t, st).Snapshot(); snap.LastfmAPIKey != "webkey" || snap.LastfmSharedSecret != "websecret" {
		t.Errorf("after restart = %+v, want the web's values", snap.Values)
	}
}

func TestSeedEqualToTheDatabaseIsQuiet(t *testing.T) {
	t.Parallel()
	st := openStore(t)
	seed := textSeed(t, settings.LibraryQuickScan, "04:15", settings.SourceEnv)
	apply(t, st, seed)
	if logs := apply(t, st, seed); strings.Contains(logs, "WARN") {
		t.Errorf("same value again logs %s, want nothing", logs)
	}
}

func TestClearSeedsAgain(t *testing.T) {
	t.Parallel()
	st := openStore(t)
	seed := textSeed(t, settings.LibraryRoots, "/srv/music", settings.SourceFile)
	apply(t, st, seed)
	svc := open(t, st)
	snap := change(t, svc, map[settings.Key]string{settings.LibraryRoots: `[]`})
	if len(snap.Roots) != 0 || snap.Source(settings.LibraryRoots) != settings.SourceDefault {
		t.Fatalf("cleared = %v from %q", snap.Roots, snap.Source(settings.LibraryRoots))
	}
	apply(t, st, seed)
	if snap = open(t, st).Snapshot(); !slices.Equal(snap.Roots, []string{"/srv/music"}) {
		t.Errorf("after the next start = %v, want the seed again", snap.Roots)
	}
}

func TestChangeIsAllOrNothing(t *testing.T) {
	t.Parallel()
	st := openStore(t)
	svc := open(t, st)
	before := svc.Snapshot()
	_, err := svc.Change(t.Context(), raw(map[settings.Key]string{
		settings.LibraryQuickScan:    `"05:00"`,
		settings.LibraryMissingGrace: `"-1h"`,
		settings.LastfmSharedSecret:  `"` + secretValue + `"`,
	}))
	ke, ok := errors.AsType[*settings.KeyError](err)
	if !ok || ke.Key != settings.LibraryMissingGrace || !errors.Is(err, settings.ErrBadValue) {
		t.Fatalf("err = %v, want a KeyError for library.missing_grace", err)
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Errorf("err = %v holds the secret", err)
	}
	if svc.Snapshot() != before {
		t.Error("the live snapshot changed")
	}
	if snap := open(t, st).Snapshot(); snap.QuickScan != before.QuickScan || snap.LastfmSharedSecret != "" {
		t.Errorf("stored = %+v, want nothing stored", snap.Values)
	}
}

func TestChangeRefusesUnknownKeys(t *testing.T) {
	t.Parallel()
	svc := open(t, openStore(t))
	_, err := svc.Change(t.Context(), raw(map[settings.Key]string{"server.listen": `":80"`, settings.LogLevel: debugJSON}))
	if ke, ok := errors.AsType[*settings.KeyError](err); !ok || ke.Key != "server.listen" || !errors.Is(err, settings.ErrUnknownKey) {
		t.Errorf("err = %v, want unknown server.listen", err)
	}
	if svc.Snapshot().LogLevel != slog.LevelInfo {
		t.Error("log.level changed with an unknown key beside it")
	}
}

func TestChangeChecksEveryKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key       settings.Key
		good, bad string
	}{
		{settings.LastfmAPIKey, `" 3f9c1e0b "`, `"two words"`},
		{settings.LastfmSharedSecret, `"abc"`, `42`},
		{settings.LibraryRoots, `["/a/", "/b"]`, `["/a", "/a/b"]`},
		{settings.LibraryRoots, `["/c"]`, `["relative"]`},
		{settings.LibraryTagWrites, trueJSON, `"yes"`},
		{settings.LibraryQuickScan, `"03:00"`, `"25:00"`},
		{settings.LibraryMissingGrace, `"720h"`, `"forever"`},
		{settings.LogLevel, `"WARN"`, `"loud"`},
		{settings.ServerBaseURL, `"https://example.com/music"`, `"example.com"`},
		{settings.ServerTrustedProxies, `["10.0.0.2", "172.17.0.0/16"]`, `["not an ip"]`},
	}
	for _, tt := range tests {
		t.Run(string(tt.key)+" "+tt.bad, func(t *testing.T) {
			t.Parallel()
			svc := open(t, openStore(t))
			change(t, svc, map[settings.Key]string{tt.key: tt.good})
			if _, err := svc.Change(t.Context(), raw(map[settings.Key]string{tt.key: tt.bad})); !errors.Is(err, settings.ErrBadValue) {
				t.Errorf("bad %s = %v, want ErrBadValue", tt.bad, err)
			}
		})
	}
}

func TestChangeKeepsCanonicalValues(t *testing.T) {
	t.Parallel()
	svc := open(t, openStore(t))
	snap := change(t, svc, map[settings.Key]string{
		settings.LastfmAPIKey: `" abc "`, settings.LibraryRoots: `["/a/"]`, settings.LogLevel: `"WARN"`,
		settings.ServerTrustedProxies: `["10.0.0.2", "172.17.0.9/16"]`, settings.LibraryMissingGrace: `"30m"`,
	})
	for k, want := range map[settings.Key]string{
		settings.LastfmAPIKey: `"abc"`, settings.LibraryRoots: `["/a"]`, settings.LogLevel: `"warn"`,
		settings.ServerTrustedProxies: `["10.0.0.2/32","172.17.0.0/16"]`, settings.LibraryMissingGrace: `"30m0s"`,
	} {
		if got := string(snap.JSON(k)); got != want {
			t.Errorf("%s = %s, want %s", k, got, want)
		}
	}
}

func TestNullResetsToTheDefault(t *testing.T) {
	t.Parallel()
	svc := open(t, openStore(t))
	change(t, svc, map[settings.Key]string{settings.LibraryTagWrites: trueJSON, settings.LogLevel: debugJSON})
	snap := change(t, svc, map[settings.Key]string{settings.LibraryTagWrites: `null`, settings.LogLevel: `null`})
	if snap.TagWrites || snap.LogLevel != slog.LevelInfo || snap.Source(settings.LogLevel) != settings.SourceDefault {
		t.Errorf("after null = %+v", snap.Values)
	}
}

func TestSubscribeHearsOnlyItsKeys(t *testing.T) {
	t.Parallel()
	svc := open(t, openStore(t))
	lastfm := svc.Subscribe(settings.LastfmAPIKey, settings.LastfmSharedSecret)
	level := svc.Subscribe(settings.LogLevel)
	change(t, svc, map[settings.Key]string{settings.LogLevel: debugJSON})
	select {
	case <-level:
	default:
		t.Error("the log level's owner was not told")
	}
	select {
	case <-lastfm:
		t.Error("Last.fm's owner was told of a change it doesn't own")
	default:
	}
	change(t, svc, map[settings.Key]string{settings.LogLevel: debugJSON}) // No change.
	select {
	case <-level:
		t.Error("told of a value that didn't change")
	default:
	}
}

func TestPendingRestart(t *testing.T) {
	t.Parallel()
	svc := open(t, openStore(t))
	if svc.PendingRestart(settings.ServerBaseURL) {
		t.Error("pending before any change")
	}
	change(t, svc, map[settings.Key]string{settings.ServerBaseURL: `"https://example.com"`, settings.LogLevel: debugJSON})
	if !svc.PendingRestart(settings.ServerBaseURL) || svc.PendingRestart(settings.LogLevel) {
		t.Error("want base_url pending and log.level live")
	}
}

func TestOpenRefusesAStoredValueItCantRead(t *testing.T) {
	t.Parallel()
	st := openStore(t)
	if err := st.ChangeSettings(t.Context(), []store.SettingChange{{Key: "log.level", Value: []byte(`"loud"`), Source: "web"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Open(t.Context(), st); !errors.Is(err, settings.ErrBadValue) || !strings.Contains(err.Error(), "log.level") {
		t.Errorf("Open = %v, want a bad log.level", err)
	}
}

func TestOpenIgnoresKeysFromALaterVersion(t *testing.T) {
	t.Parallel()
	st := openStore(t)
	if err := st.ChangeSettings(t.Context(), []store.SettingChange{{Key: "future.key", Value: []byte(`1`), Source: "web"}}); err != nil {
		t.Fatal(err)
	}
	open(t, st)
}

func TestTextSeeds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key  settings.Key
		text string
		want string
	}{
		{settings.LibraryRoots, "/music:/audiobooks/", `["/music","/audiobooks"]`},
		{settings.LibraryTagWrites, "true", trueJSON},
		{settings.LibraryMissingGrace, "48h", `"48h0m0s"`},
		{settings.ServerTrustedProxies, "10.0.0.2, 172.17.0.0/16", `["10.0.0.2/32","172.17.0.0/16"]`},
		{settings.LogLevel, "debug", debugJSON},
	}
	for _, tt := range tests {
		st := openStore(t)
		apply(t, st, textSeed(t, tt.key, tt.text, settings.SourceFlag))
		snap := open(t, st).Snapshot()
		if got := string(snap.JSON(tt.key)); got != tt.want || snap.Source(tt.key) != settings.SourceFlag {
			t.Errorf("%s %q = %s from %s, want %s", tt.key, tt.text, got, snap.Source(tt.key), tt.want)
		}
	}
	if _, _, err := settings.TextSeed(settings.LibraryTagWrites, "maybe", settings.SourceEnv); !errors.Is(err, settings.ErrBadValue) {
		t.Errorf("bad bool = %v, want ErrBadValue", err)
	}
	if _, ok, err := settings.TextSeed(settings.LastfmAPIKey, "", settings.SourceEnv); ok || err != nil {
		t.Errorf("empty = %v, %v, want no seed", ok, err)
	}
}

func TestFileSeedsOnlyWhatTheFileSet(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Library.Roots = []string{"/srv/music"}
	cfg.Library.MissingGrace = 90 * time.Minute
	seeds := settings.FileSeeds(&cfg, config.Keys{"library", "library.roots", "storage.data_dir"})
	if len(seeds) != 1 || seeds[0].Key != settings.LibraryRoots || seeds[0].Source != settings.SourceFile {
		t.Errorf("seeds = %+v, want library.roots alone", seeds)
	}
}

func TestEveryKeyIsListedOnce(t *testing.T) {
	t.Parallel()
	keys := settings.Keys()
	for _, k := range keys {
		if got, err := settings.Lookup(string(k)); err != nil || got != k {
			t.Errorf("Lookup(%s) = %v, %v", k, got, err)
		}
	}
	if len(slices.Compact(slices.Sorted(slices.Values(keys)))) != len(keys) {
		t.Error("a key is listed twice")
	}
	if _, err := settings.Lookup("storage.data_dir"); !errors.Is(err, settings.ErrUnknownKey) {
		t.Errorf("data_dir = %v, want unknown: it stays in the file", err)
	}
}
