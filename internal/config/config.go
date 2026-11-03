package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Defaults for the keys a config file leaves out.
const (
	defaultListen          = ":4533"
	defaultMissingGrace    = 24 * time.Hour
	defaultQuickScanHour   = 3
	defaultQuickScanMinute = 30
)

// errUnknownKey reports a config key Kitsune does not know.
var errUnknownKey = errors.New("unknown keys")

// Config is kitsune-server's configuration.
type Config struct {
	Server  Server  `toml:"server"`
	Library Library `toml:"library"`
	Storage Storage `toml:"storage"`
	Log     Log     `toml:"log"`
}

// Server configures the HTTP server.
type Server struct {
	// Listen is the TCP address to listen on, as host:port.
	Listen string `toml:"listen"`
	// BaseURL is the server's public URL when it is behind a reverse proxy,
	// such as "https://example.com/music"; "" when it is not.
	BaseURL string `toml:"base_url"`
}

// Library configures the music library.
type Library struct {
	// Roots are the absolute directories to scan. No two overlap.
	Roots []string `toml:"roots"`
	// MissingGrace is how long a vanished file keeps its track before removal.
	MissingGrace time.Duration `toml:"missing_grace"`
	// QuickScan is the daily local time of the quick scan.
	QuickScan ClockTime `toml:"quick_scan"`
}

// Storage configures where the server keeps its state.
type Storage struct {
	// DataDir holds the database and the artwork cache.
	DataDir string `toml:"data_dir"`
}

// Log configures logging.
type Log struct {
	// Level is the minimum level logged.
	Level slog.Level `toml:"level"`
}

// Default returns the value of every key a config file leaves out.
// Storage.DataDir is empty; [Load] fills it from [Paths].
func Default() Config {
	return Config{
		Server: Server{Listen: defaultListen},
		Library: Library{
			Roots:        []string{}, // Not nil, so `kitsune-server config` shows roots = [].
			MissingGrace: defaultMissingGrace,
			QuickScan:    ClockTime{Hour: defaultQuickScanHour, Minute: defaultQuickScanMinute},
		},
		Log: Log{Level: slog.LevelInfo},
	}
}

// Load reads the configuration. With path "", it reads paths.ConfigFile when
// that exists and uses the defaults otherwise; a named path must exist. Keys a
// file leaves out keep their [Default] values, and an empty data_dir becomes
// paths.DataDir. Roots and data_dir are cleaned.
//
// Load returns an error that wraps [fs.ErrNotExist] for a missing named
// file, and an error if the file is not valid TOML, has a key Kitsune does
// not know, or has invalid values. Every invalid value is reported, as
// "key: reason" joined with [errors.Join].
func Load(path string, paths Paths) (Config, error) {
	cfg := Default()
	name := path
	if name == "" {
		name = paths.ConfigFile
	}
	read := false
	if name != "" {
		err := decodeFile(name, &cfg)
		switch {
		case err == nil:
			read = true
		case path != "" || !errors.Is(err, fs.ErrNotExist):
			return Config{}, err
		}
	}
	if cfg.Storage.DataDir == "" {
		cfg.Storage.DataDir = paths.DataDir
	}
	if err := cfg.validate(); err != nil {
		if read {
			return Config{}, fmt.Errorf("config %s: %w", name, err)
		}
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

// decodeFile decodes the TOML file name into cfg, rejecting unknown keys.
func decodeFile(name string, cfg *Config) error {
	md, err := toml.DecodeFile(name, cfg)
	if _, ok := errors.AsType[*fs.PathError](err); ok {
		return fmt.Errorf("read config: %w", err) // The error already names the file.
	}
	if err != nil {
		return fmt.Errorf("config %s: %w", name, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return fmt.Errorf("config %s: %w: %s", name, errUnknownKey, strings.Join(keys, ", "))
	}
	return nil
}

// WriteTOML writes c to w as a config file that Load reads back to c.
func (c *Config) WriteTOML(w io.Writer) error {
	if err := toml.NewEncoder(w).Encode(c); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// invalidError reports an invalid value for one key.
type invalidError struct {
	key, reason string
}

func (e *invalidError) Error() string { return e.key + ": " + e.reason }

// validate checks every value, cleaning roots and data_dir in place, and
// returns every problem joined.
func (c *Config) validate() error {
	var errs []error
	invalid := func(key, reason string) {
		errs = append(errs, &invalidError{key: key, reason: reason})
	}
	if reason := listenProblem(c.Server.Listen); reason != "" {
		invalid("server.listen", reason)
	}
	if reason := baseURLProblem(c.Server.BaseURL); reason != "" {
		invalid("server.base_url", reason)
	}
	c.checkRoots(invalid)
	if c.Library.MissingGrace < 0 {
		invalid("library.missing_grace", "must not be negative")
	}
	switch dir := c.Storage.DataDir; {
	case dir == "":
		invalid("storage.data_dir", "not set, and neither $XDG_DATA_HOME nor $HOME is an absolute path")
	case !filepath.IsAbs(dir):
		invalid("storage.data_dir", "must be an absolute path")
	default:
		c.Storage.DataDir = filepath.Clean(dir)
	}
	return errors.Join(errs...)
}

// checkRoots cleans each absolute root and reports roots that are relative
// or that equal or contain an earlier one.
func (c *Config) checkRoots(invalid func(key, reason string)) {
	roots := c.Library.Roots
	for i, root := range roots {
		if !filepath.IsAbs(root) {
			invalid(rootKey(i), "must be an absolute path")
			continue
		}
		roots[i] = filepath.Clean(root)
	}
	for j, b := range roots {
		for i, a := range roots[:j] {
			if filepath.IsAbs(a) && filepath.IsAbs(b) && (contains(a, b) || contains(b, a)) {
				invalid(rootKey(j), "overlaps "+rootKey(i))
				break
			}
		}
	}
}

func rootKey(i int) string { return "library.roots[" + strconv.Itoa(i) + "]" }

// contains reports whether the cleaned path child is parent or lies under it.
func contains(parent, child string) bool {
	sep := string(filepath.Separator)
	return child == parent || strings.HasPrefix(child, strings.TrimSuffix(parent, sep)+sep)
}

// listenProblem describes what is wrong with a listen address, or returns "".
func listenProblem(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return `want host:port, such as ":4533"`
	}
	if _, parseErr := strconv.ParseUint(port, 10, 16); parseErr != nil {
		return "port must be 0 to 65535"
	}
	return ""
}

// baseURLProblem describes what is wrong with a base URL, or returns "".
func baseURLProblem(s string) string {
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	switch {
	case err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "":
		return "want an absolute http or https URL"
	case u.User != nil || u.RawQuery != "" || u.Fragment != "":
		return "must not have user info, a query or a fragment"
	}
	return ""
}
