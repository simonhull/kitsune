package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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

// errUnknownKey reports config keys Kitsune does not know.
var errUnknownKey = errors.New("not a config key")

// knownKeys lists every key a config file may hold, tables included. Keys
// match exactly, case included.
func knownKeys() []string {
	return []string{
		"server", "server.listen", "server.base_url", "server.trusted_proxies",
		"library", "library.roots", "library.missing_grace", "library.quick_scan", "library.tag_writes",
		"storage", "storage.data_dir",
		"log", "log.level",
	}
}

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
	// TrustedProxies are the networks of the reverse proxies whose
	// X-Forwarded-For header is believed. Empty means none.
	TrustedProxies []Network `toml:"trusted_proxies"`
}

// BasePath is BaseURL's path without its trailing slash, such as "/music",
// or "" when the server is at the root.
func (s *Server) BasePath() string {
	u, err := url.Parse(s.BaseURL)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(u.Path, "/")
}

// HTTPS reports whether BaseURL is an https URL.
func (s *Server) HTTPS() bool {
	u, err := url.Parse(s.BaseURL)
	return err == nil && u.Scheme == "https"
}

// Library configures the music library.
type Library struct {
	// Roots are the absolute directories to scan. No two overlap.
	Roots []string `toml:"roots"`
	// MissingGrace is how long a vanished file keeps its track before removal.
	MissingGrace time.Duration `toml:"missing_grace"`
	// QuickScan is the daily local time of the quick scan.
	QuickScan ClockTime `toml:"quick_scan"`
	// TagWrites lets an administrator edit tags, which rewrites audio files
	// under Roots. Off by default: nothing else stops a write, so a read-only
	// library stays safe only while this stays false.
	TagWrites bool `toml:"tag_writes"`
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
		Server: Server{Listen: defaultListen, TrustedProxies: []Network{}}, // Not nil, so the config shows trusted_proxies = [].
		Library: Library{
			Roots:        []string{}, // Not nil, so `kitsune-server config` shows roots = [].
			MissingGrace: defaultMissingGrace,
			QuickScan:    ClockTime{Hour: defaultQuickScanHour, Minute: defaultQuickScanMinute},
		},
		Log: Log{Level: slog.LevelInfo},
	}
}

// Load reads the configuration. With path "", it reads paths.ConfigFile when
// that exists and uses the defaults otherwise; a named path must exist, and a
// default file that exists but can't be read or is invalid is an error too. Keys a
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

// decodeFile decodes the TOML file name into cfg, rejecting unknown keys and
// values the decoder would misread.
func decodeFile(name string, cfg *Config) error {
	b, err := os.ReadFile(name) //nolint:gosec // G304: the operator names the config file.
	if err != nil {
		return fmt.Errorf("read config: %w", err) // The error already names the file.
	}
	md, err := toml.Decode(string(b), cfg)
	if err != nil {
		return fmt.Errorf("config %s: %w", name, err)
	}
	if unknown := unknownKeys(&md); len(unknown) > 0 {
		return fmt.Errorf("config %s: %w: %s", name, errUnknownKey, strings.Join(unknown, ", "))
	}
	// The decoder reads an integer duration as nanoseconds, so 86400 would
	// mean 86.4µs, not a day.
	if md.IsDefined("library", "missing_grace") && md.Type("library", "missing_grace") != "String" {
		return fmt.Errorf("config %s: %w", name,
			&invalidError{key: "library.missing_grace", reason: `want a duration string, such as "24h"`})
	}
	return nil
}

// unknownKeys lists the keys in md that are not in [knownKeys], leaving out
// those under an unknown table. The decoder matches keys without regard to
// case, so this exact check is what keeps "LISTEN" or a second "Listen" out.
func unknownKeys(md *toml.MetaData) []string {
	known := knownKeys()
	var unknown []string
	for _, k := range md.Keys() {
		key := k.String()
		under := slices.ContainsFunc(unknown, func(u string) bool { return strings.HasPrefix(key, u+".") })
		if !under && !slices.Contains(known, key) {
			unknown = append(unknown, key)
		}
	}
	return unknown
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
	return portProblem(port)
}

// portProblem describes what is wrong with a port number, or returns "".
func portProblem(port string) string {
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return "port must be a number from 0 to 65535"
	}
	return ""
}

// baseURLProblem describes what is wrong with a base URL, or returns "".
func baseURLProblem(s string) string {
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || !isAbsoluteHTTP(u) {
		return "want an absolute http or https URL"
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(s, "#") {
		return "must not have user info, a query or a fragment"
	}
	if reason := pathProblem(u); reason != "" {
		return reason
	}
	if port := u.Port(); port != "" {
		return portProblem(port)
	}
	return ""
}

// pathProblem describes what is wrong with the path of a base URL, or returns
// "". The path must be plain segments, with at most one trailing slash.
func pathProblem(u *url.URL) string {
	const reason = "path must be plain segments of letters, digits and - . _ ~"
	if u.RawPath != "" {
		return reason
	}
	path := strings.TrimSuffix(u.Path, "/")
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		return reason
	}
	for seg := range strings.SplitSeq(path[1:], "/") {
		if seg == "" || seg == "." || seg == ".." || strings.ContainsFunc(seg, notPlain) {
			return reason
		}
	}
	return ""
}

// notPlain reports whether r is outside A-Z a-z 0-9 - . _ ~.
func notPlain(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	}
	return !strings.ContainsRune("-._~", r)
}

// isAbsoluteHTTP reports whether u is an http or https URL with a host.
func isAbsoluteHTTP(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
