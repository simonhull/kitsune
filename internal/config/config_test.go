package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

// writeConfig writes body to a server.toml in a new temp dir and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "server.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Error prefixes that several cases expect.
const (
	listenKey = "server.listen:"
	root1Key  = "library.roots[1]:"
)

// Values that several cases share.
const (
	musicRoot  = "/srv/music"
	dataPath   = "/data"
	baseURLKey = "server.base_url:"

	baseURLPathProblem = "server.base_url: path must be plain segments"
)

// testDataDir is the default data directory in testPaths.
const testDataDir = "/var/lib/kitsune"

// testPaths are default paths whose config file does not exist.
func testPaths(t *testing.T) Paths {
	t.Helper()
	return Paths{ConfigFile: filepath.Join(t.TempDir(), "absent.toml"), DataDir: testDataDir}
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()
	got, err := Load("", testPaths(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Default()
	want.Storage.DataDir = testDataDir
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
	if want.Server.Listen != defaultListen || want.Library.MissingGrace != 24*time.Hour ||
		want.Library.QuickScan != (ClockTime{Hour: 3, Minute: 30}) || want.Log.Level != slog.LevelInfo {
		t.Errorf("Default() = %+v, want the spec's defaults", want)
	}
}

// TestTagWritesDefaultOff pins the one thing that keeps a read-only library safe.
func TestTagWritesDefaultOff(t *testing.T) {
	t.Parallel()
	if Default().Library.TagWrites {
		t.Error("Default().Library.TagWrites = true, want false")
	}
	got, err := Load(writeConfig(t, "[library]\nmissing_grace = \"1h\"\n"), testPaths(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Library.TagWrites {
		t.Error("a config that leaves tag_writes out turned it on")
	}
}

func TestLoadWithoutDefaultFileLocation(t *testing.T) {
	t.Parallel()
	got, err := Load("", Paths{DataDir: testDataDir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Server.Listen != defaultListen {
		t.Errorf("Load() = %+v, want the defaults", got)
	}
}

func TestLoadDefaultFile(t *testing.T) {
	t.Parallel()
	paths := Paths{ConfigFile: writeConfig(t, "[log]\nlevel = \"error\"\n"), DataDir: testDataDir}
	got, err := Load("", paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Log.Level != slog.LevelError {
		t.Errorf("Log.Level = %v, want ERROR from the default file", got.Log.Level)
	}
}

func TestLoadFile(t *testing.T) {
	t.Parallel()
	path := writeConfig(t, `
[server]
listen = "127.0.0.1:8080"
base_url = "https://example.com/music"
trusted_proxies = ["172.18.0.0/16", "192.168.1.5"]

[library]
roots = ["/srv/music/", "/srv/audiobooks", "/srv/musicals"]
missing_grace = "1h30m"
quick_scan = "04:15"
tag_writes = true

[storage]
data_dir = "/data/kitsune/"

[log]
level = "debug"
`)
	got, err := Load(path, testPaths(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		Server: Server{
			Listen:         "127.0.0.1:8080",
			BaseURL:        "https://example.com/music",
			TrustedProxies: []Network{{netip.MustParsePrefix("172.18.0.0/16")}, {netip.MustParsePrefix("192.168.1.5/32")}},
		},
		Library: Library{
			Roots:        []string{musicRoot, "/srv/audiobooks", "/srv/musicals"},
			MissingGrace: 90 * time.Minute,
			QuickScan:    ClockTime{Hour: 4, Minute: 15},
			TagWrites:    true,
		},
		Storage: Storage{DataDir: "/data/kitsune"},
		Log:     Log{Level: slog.LevelDebug},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestLoadPartialFileKeepsDefaults(t *testing.T) {
	t.Parallel()
	got, err := Load(writeConfig(t, "[library]\nroots = [\"/srv/music\"]\n"), testPaths(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Server.Listen != defaultListen || got.Library.QuickScan != (ClockTime{Hour: 3, Minute: 30}) ||
		got.Storage.DataDir != testDataDir {
		t.Errorf("Load() = %+v, want defaults outside library.roots", got)
	}
}

func TestLoadMissingNamedFile(t *testing.T) {
	t.Parallel()
	_, err := Load(filepath.Join(t.TempDir(), "nope.toml"), testPaths(t))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load(missing) error = %v, want fs.ErrNotExist", err)
	}
}

func TestLoadBrokenDefaultFile(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"invalid file": writeConfig(t, "bogus = 1\n"),
		"directory":    t.TempDir(),
	}
	for name, file := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Load("", Paths{ConfigFile: file, DataDir: testDataDir}); err == nil {
				t.Errorf("Load with a default file that is a %s succeeded, want an error", name)
			}
		})
	}
}

// TestKnownKeysMatchTags keeps the key allowlist in step with the struct
// tags: a fully populated config must write exactly the known keys.
func TestKnownKeysMatchTags(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Server.BaseURL = "https://example.com"
	cfg.Library.Roots = []string{musicRoot}
	cfg.Storage.DataDir = dataPath
	cfg.LastFM = LastFM{APIKey: "key", SharedSecret: "secret"}
	var b strings.Builder
	if err := cfg.WriteTOML(&b); err != nil {
		t.Fatalf("WriteTOML: %v", err)
	}
	md, err := toml.Decode(b.String(), new(Config))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	keys := md.Keys()
	written := make([]string, 0, len(keys))
	for _, k := range keys {
		written = append(written, k.String())
	}
	got, want := slices.Sorted(slices.Values(written)), slices.Sorted(slices.Values(knownKeys()))
	if !slices.Equal(got, want) {
		t.Errorf("WriteTOML keys = %q, knownKeys() = %q; want equal", got, want)
	}
}

func TestBaseURLAccepts(t *testing.T) {
	t.Parallel()
	for _, u := range []string{
		"https://h", "https://h/", "https://h/music", "https://h/music/", "http://h:8080/a/b-c_d.e~f",
	} {
		t.Run(u, func(t *testing.T) {
			t.Parallel()
			body := "[server]\nbase_url = " + strconv.Quote(u) + "\n"
			if _, err := Load(writeConfig(t, body), testPaths(t)); err != nil {
				t.Errorf("Load(base_url %q): %v", u, err)
			}
		})
	}
}

func TestServerBasePathAndHTTPS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		baseURL   string
		wantPath  string
		wantHTTPS bool
	}{
		{baseURL: "", wantPath: "", wantHTTPS: false},
		{baseURL: "http://h", wantPath: "", wantHTTPS: false},
		{baseURL: "https://h/", wantPath: "", wantHTTPS: true},
		{baseURL: "https://h/music/", wantPath: "/music", wantHTTPS: true},
		{baseURL: "HTTPS://h/music", wantPath: "/music", wantHTTPS: true},
		{baseURL: "://bad", wantPath: "", wantHTTPS: false},
	}
	for _, tt := range tests {
		t.Run(tt.baseURL, func(t *testing.T) {
			t.Parallel()
			s := Server{BaseURL: tt.baseURL}
			if got := s.BasePath(); got != tt.wantPath {
				t.Errorf("BasePath() = %q, want %q", got, tt.wantPath)
			}
			if got := s.HTTPS(); got != tt.wantHTTPS {
				t.Errorf("HTTPS() = %t, want %t", got, tt.wantHTTPS)
			}
		})
	}
}

func TestLoadRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		body    string
		noPaths bool     // Use Paths{} instead of testPaths, so no data_dir default.
		want    []string // Substrings the error must contain.
	}{
		{name: "unknown key", body: "[library]\nmissing_grase = \"1h\"\n", want: []string{"library.missing_grase"}},
		{name: "unknown section", body: "[media]\nroots = []\n", want: []string{"not a config key: media"}},
		{name: "upper-case keys", body: "[Server]\nLISTEN = \":1\"\n", want: []string{"not a config key: Server"}},
		{name: "keys differing in case", body: "[server]\nlisten = \":1\"\nListen = \":2\"\n", want: []string{"server.Listen"}},
		{name: "bad TOML", body: "[server\n", want: []string{"toml: line"}},
		{name: "integer grace", body: "[library]\nmissing_grace = 86400\n", want: []string{`library.missing_grace: want a duration string, such as "24h"`}},
		{name: "wrong type", body: "[server]\nlisten = 4533\n", want: []string{"listen"}},
		{name: "bad duration", body: "[library]\nmissing_grace = \"a day\"\n", want: []string{"missing_grace"}},
		{name: "bad clock", body: "[library]\nquick_scan = \"25:00\"\n", want: []string{"quick_scan", "HH:MM"}},
		{name: "bad level", body: "[log]\nlevel = \"loud\"\n", want: []string{"level"}},
		{name: "listen without port", body: "[server]\nlisten = \"localhost\"\n", want: []string{listenKey}},
		{name: "listen bad port", body: "[server]\nlisten = \":99999\"\n", want: []string{listenKey}},
		{name: "relative base_url", body: "[server]\nbase_url = \"/music\"\n", want: []string{baseURLKey}},
		{name: "base_url with query", body: "[server]\nbase_url = \"https://x.test/?a=1\"\n", want: []string{baseURLKey}},
		{name: "base_url with empty query", body: "[server]\nbase_url = \"https://x.test/?\"\n", want: []string{baseURLKey}},
		{name: "base_url with user info", body: "[server]\nbase_url = \"https://user@x.test/\"\n", want: []string{baseURLKey}},
		{name: "base_url with fragment", body: "[server]\nbase_url = \"https://x.test/#a\"\n", want: []string{baseURLKey}},
		{name: "base_url with empty fragment", body: "[server]\nbase_url = \"https://x.test/#\"\n", want: []string{baseURLKey}},
		{name: "base_url bad port", body: "[server]\nbase_url = \"https://x.test:99999/\"\n", want: []string{"server.base_url: port must be a number"}},
		{name: "bad trusted_proxies", body: "[server]\ntrusted_proxies = [\"nope\"]\n", want: []string{"trusted_proxies", "want an IP address or CIDR network"}},
		{name: "base_url empty segment", body: "[server]\nbase_url = \"https://h/a//b\"\n", want: []string{baseURLPathProblem}},
		{name: "base_url dot-dot", body: "[server]\nbase_url = \"https://h/a/../b\"\n", want: []string{baseURLPathProblem}},
		{name: "base_url escaped space", body: "[server]\nbase_url = \"https://h/a%20b\"\n", want: []string{baseURLPathProblem}},
		{name: "base_url escaped UTF-8", body: "[server]\nbase_url = \"https://h/caf%C3%A9\"\n", want: []string{baseURLPathProblem}},
		{name: "base_url double trailing slash", body: "[server]\nbase_url = \"https://h/music//\"\n", want: []string{baseURLPathProblem}},
		{name: "relative root", body: "[library]\nroots = [\"/ok\", \"music\"]\n", want: []string{root1Key}},
		{
			name: "duplicate root",
			body: "[library]\nroots = [\"/srv/music\", \"/srv/music/\"]\n",
			want: []string{root1Key, "library.roots[0]"},
		},
		{name: "nested root", body: "[library]\nroots = [\"/srv/music/jazz\", \"/srv/music\"]\n", want: []string{root1Key}},
		{name: "root slash", body: "[library]\nroots = [\"/\", \"/srv\"]\n", want: []string{root1Key}},
		{name: "negative grace", body: "[library]\nmissing_grace = \"-1h\"\n", want: []string{"library.missing_grace:"}},
		{name: "lastfm key alone", body: "[lastfm]\napi_key = \"k\"\n", want: []string{"lastfm.shared_secret: not set, but lastfm.api_key is"}},
		{name: "lastfm secret alone", body: "[lastfm]\nshared_secret = \"s\"\n", want: []string{"lastfm.api_key: not set, but lastfm.shared_secret is"}},
		{name: "relative data_dir", body: "[storage]\ndata_dir = \"data\"\n", want: []string{"storage.data_dir:"}},
		{name: "no data_dir anywhere", body: "", noPaths: true, want: []string{"storage.data_dir:", "XDG_DATA_HOME"}},
		{
			name: "every problem at once",
			body: "[server]\nlisten = \"x\"\n[library]\nroots = [\"rel\"]\nmissing_grace = \"-1s\"\n",
			want: []string{"server.toml: ", listenKey, "library.roots[0]:", "library.missing_grace:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			paths := testPaths(t)
			if tt.noPaths {
				paths = Paths{}
			}
			_, err := Load(writeConfig(t, tt.body), paths)
			if err == nil {
				t.Fatal("Load succeeded, want an error")
			}
			for _, s := range tt.want {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("error %q does not mention %q", err, s)
				}
			}
		})
	}
}

func TestWriteTOMLRoundTrips(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Library.Roots = []string{musicRoot}
	cfg.Storage.DataDir = dataPath
	cfg.Log.Level = slog.LevelWarn
	cfg.Server.TrustedProxies = []Network{{netip.MustParsePrefix("192.168.1.5/32")}, {netip.MustParsePrefix("fd00::/8")}}
	var b strings.Builder
	if err := cfg.WriteTOML(&b); err != nil {
		t.Fatalf("WriteTOML: %v", err)
	}
	got, err := Load(writeConfig(t, b.String()), testPaths(t))
	if err != nil {
		t.Fatalf("Load(WriteTOML output): %v\n%s", err, b.String())
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("round trip = %+v, want %+v\n%s", got, cfg, b.String())
	}
}

func TestLastFM(t *testing.T) {
	t.Parallel()
	got, err := Load(writeConfig(t, "[lastfm]\napi_key = \"abc\"\nshared_secret = \"hush-123\"\n"), testPaths(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.LastFM.APIKey != "abc" || string(got.LastFM.SharedSecret) != "hush-123" || !got.LastFM.Enabled() {
		t.Errorf("LastFM = %+v, want abc and the secret, enabled", got.LastFM)
	}
	var b strings.Builder
	if err = got.WriteTOML(&b); err != nil {
		t.Fatalf("WriteTOML: %v", err)
	}
	if strings.Contains(b.String(), "hush-123") || !strings.Contains(b.String(), `shared_secret = "(set)"`) {
		t.Errorf("WriteTOML = %s, want the secret shown as (set)", b.String())
	}
	if s := fmt.Sprintf("%v %+v %s", got.LastFM, got.LastFM, got.LastFM.SharedSecret); strings.Contains(s, "hush-123") {
		t.Errorf("formatted config %q shows the secret", s)
	}
}

func TestLastFMDefaultOff(t *testing.T) {
	t.Parallel()
	if Default().LastFM.Enabled() {
		t.Error("Default().LastFM.Enabled() = true, want false")
	}
	if Secret("").String() != "" {
		t.Error(`Secret("").String() != ""`)
	}
}
