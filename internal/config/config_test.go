package config

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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

[library]
roots = ["/srv/music/", "/srv/audiobooks"]
missing_grace = "1h30m"
quick_scan = "04:15"

[storage]
data_dir = "/data/kitsune"

[log]
level = "debug"
`)
	got, err := Load(path, testPaths(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		Server: Server{Listen: "127.0.0.1:8080", BaseURL: "https://example.com/music"},
		Library: Library{
			Roots:        []string{"/srv/music", "/srv/audiobooks"},
			MissingGrace: 90 * time.Minute,
			QuickScan:    ClockTime{Hour: 4, Minute: 15},
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

func TestLoadRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		body    string
		noPaths bool     // Use Paths{} instead of testPaths, so no data_dir default.
		want    []string // Substrings the error must contain.
	}{
		{name: "unknown key", body: "[library]\nmissing_grase = \"1h\"\n", want: []string{"library.missing_grase"}},
		{name: "unknown section", body: "[media]\nroots = []\n", want: []string{"media"}},
		{name: "bad TOML", body: "[server\n", want: []string{"server.toml"}},
		{name: "wrong type", body: "[server]\nlisten = 4533\n", want: []string{"listen"}},
		{name: "bad duration", body: "[library]\nmissing_grace = \"a day\"\n", want: []string{"missing_grace"}},
		{name: "bad clock", body: "[library]\nquick_scan = \"25:00\"\n", want: []string{"quick_scan", "HH:MM"}},
		{name: "bad level", body: "[log]\nlevel = \"loud\"\n", want: []string{"level"}},
		{name: "listen without port", body: "[server]\nlisten = \"localhost\"\n", want: []string{listenKey}},
		{name: "listen bad port", body: "[server]\nlisten = \":99999\"\n", want: []string{listenKey}},
		{name: "relative base_url", body: "[server]\nbase_url = \"/music\"\n", want: []string{"server.base_url:"}},
		{name: "base_url with query", body: "[server]\nbase_url = \"https://x.test/?a=1\"\n", want: []string{"server.base_url:"}},
		{name: "relative root", body: "[library]\nroots = [\"/ok\", \"music\"]\n", want: []string{root1Key}},
		{
			name: "duplicate root",
			body: "[library]\nroots = [\"/srv/music\", \"/srv/music/\"]\n",
			want: []string{root1Key, "library.roots[0]"},
		},
		{name: "nested root", body: "[library]\nroots = [\"/srv/music/jazz\", \"/srv/music\"]\n", want: []string{root1Key}},
		{name: "root slash", body: "[library]\nroots = [\"/\", \"/srv\"]\n", want: []string{root1Key}},
		{name: "negative grace", body: "[library]\nmissing_grace = \"-1h\"\n", want: []string{"library.missing_grace:"}},
		{name: "relative data_dir", body: "[storage]\ndata_dir = \"data\"\n", want: []string{"storage.data_dir:"}},
		{name: "no data_dir anywhere", body: "", noPaths: true, want: []string{"storage.data_dir:", "XDG_DATA_HOME"}},
		{
			name: "every problem at once",
			body: "[server]\nlisten = \"x\"\n[library]\nroots = [\"rel\"]\nmissing_grace = \"-1s\"\n",
			want: []string{listenKey, "library.roots[0]:", "library.missing_grace:"},
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
	cfg.Library.Roots = []string{"/srv/music"}
	cfg.Storage.DataDir = "/data"
	cfg.Log.Level = slog.LevelWarn
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
