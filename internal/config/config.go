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
		"lastfm", "lastfm.api_key", "lastfm.shared_secret",
	}
}

// Config is kitsune-server's configuration.
//
// Only Server.Listen and Storage.DataDir apply at every start. Every other key
// is a setting the database keeps: the file only seeds one that has no value
// there yet (see internal/settings).
type Config struct {
	Server  Server  `toml:"server"`
	Library Library `toml:"library"`
	Storage Storage `toml:"storage"`
	Log     Log     `toml:"log"`
	LastFM  LastFM  `toml:"lastfm"`
}

// Keys are the keys a config file set, such as "lastfm.api_key". A key the
// file leaves out has its [Default] value, which seeds nothing.
type Keys []string

// Has reports whether the file set key.
func (k Keys) Has(key string) bool { return slices.Contains(k, key) }

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

// LastFM configures sending plays and loves to Last.fm. Both keys come from
// an API account at last.fm/api; with neither set, the feature is off.
type LastFM struct {
	// APIKey identifies the server to Last.fm.
	APIKey string `toml:"api_key"`
	// SharedSecret signs every call. It is never logged or sent to a client.
	SharedSecret Secret `toml:"shared_secret"`
}

// Enabled reports whether Last.fm is set up.
func (l LastFM) Enabled() bool { return l.APIKey != "" && l.SharedSecret != "" }

// Secret is a config value that is never printed: it formats and encodes as
// "(set)", or "" when empty. Convert it with string() to use it.
type Secret string

// redacted is how a set [Secret] prints.
const redacted = "(set)"

// String returns "(set)", or "" for an empty secret.
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return redacted
}

// MarshalText encodes the secret as [Secret.String] does, so
// `kitsune-server config` never prints it.
func (s Secret) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText reads the secret as written.
func (s *Secret) UnmarshalText(b []byte) error {
	*s = Secret(b)
	return nil
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
	cfg, _, err := LoadKeys(path, paths)
	return cfg, err
}

// LoadKeys is [Load] that also returns the keys the file set: those are the
// settings it seeds.
func LoadKeys(path string, paths Paths) (Config, Keys, error) {
	cfg := Default()
	name := path
	if name == "" {
		name = paths.ConfigFile
	}
	var keys Keys
	read := false
	if name != "" {
		var err error
		keys, err = decodeFile(name, &cfg)
		switch {
		case err == nil:
			read = true
		case path != "" || !errors.Is(err, fs.ErrNotExist):
			return Config{}, nil, err
		}
	}
	if cfg.Storage.DataDir == "" {
		cfg.Storage.DataDir = paths.DataDir
	}
	if err := cfg.validate(); err != nil {
		if read {
			return Config{}, nil, fmt.Errorf("config %s: %w", name, err)
		}
		return Config{}, nil, fmt.Errorf("config: %w", err)
	}
	return cfg, keys, nil
}

// decodeFile decodes the TOML file name into cfg, rejecting unknown keys and
// values the decoder would misread, and returns the keys it set.
func decodeFile(name string, cfg *Config) (Keys, error) {
	b, err := os.ReadFile(name) //nolint:gosec // G304: the operator names the config file.
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err) // The error already names the file.
	}
	md, err := toml.Decode(string(b), cfg)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", name, err)
	}
	if unknown := unknownKeys(&md); len(unknown) > 0 {
		return nil, fmt.Errorf("config %s: %w: %s", name, errUnknownKey, strings.Join(unknown, ", "))
	}
	keys := make(Keys, 0, len(md.Keys()))
	for _, k := range md.Keys() {
		keys = append(keys, k.String())
	}
	// The decoder reads an integer duration as nanoseconds, so 86400 would
	// mean 86.4µs, not a day.
	if md.IsDefined("library", "missing_grace") && md.Type("library", "missing_grace") != "String" {
		return nil, fmt.Errorf("config %s: %w", name,
			&invalidError{key: "library.missing_grace", reason: `want a duration string, such as "24h"`})
	}
	return keys, nil
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

// WriteTOML writes c to w as a config file that Load reads back to c, except
// that a set lastfm.shared_secret is written as "(set)".
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
	if (c.LastFM.APIKey == "") != (c.LastFM.SharedSecret == "") {
		if c.LastFM.APIKey == "" {
			invalid("lastfm.api_key", "not set, but lastfm.shared_secret is: set both or neither")
		} else {
			invalid("lastfm.shared_secret", "not set, but lastfm.api_key is: set both or neither")
		}
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
	for _, p := range rootProblems(c.Library.Roots) {
		invalid(rootKey(p.index), p.reason)
	}
}

// rootProblem is what is wrong with the root at index.
type rootProblem struct {
	index  int
	reason string
}

// rootProblems cleans each absolute root in place and describes the roots that
// are relative or that equal or contain an earlier one.
func rootProblems(roots []string) []rootProblem {
	var problems []rootProblem
	for i, root := range roots {
		if !filepath.IsAbs(root) {
			problems = append(problems, rootProblem{i, "must be an absolute path"})
			continue
		}
		roots[i] = filepath.Clean(root)
	}
	for j, b := range roots {
		for i, a := range roots[:j] {
			if filepath.IsAbs(a) && filepath.IsAbs(b) && (contains(a, b) || contains(b, a)) {
				problems = append(problems, rootProblem{j, "overlaps " + rootKey(i)})
				break
			}
		}
	}
	return problems
}

// ErrInvalid means a value is not one its key takes. The error wrapping it
// says why.
var ErrInvalid = errors.New("invalid value")

// CleanRoots returns a cleaned copy of roots, or an error wrapping [ErrInvalid]
// when one is relative or overlaps another.
func CleanRoots(roots []string) ([]string, error) {
	out := slices.Clone(roots)
	if p := rootProblems(out); len(p) > 0 {
		return nil, fmt.Errorf("%w: %s: %s", ErrInvalid, rootKey(p[0].index), p[0].reason)
	}
	return out, nil
}

// Overlaps reports whether the cleaned absolute paths a and b are the same
// folder or one holds the other.
func Overlaps(a, b string) bool { return contains(a, b) || contains(b, a) }

// CheckBaseURL returns an error wrapping [ErrInvalid] when s is not a value
// server.base_url takes. "" is one: it means the server is not behind a proxy.
func CheckBaseURL(s string) error {
	if reason := baseURLProblem(s); reason != "" {
		return fmt.Errorf("%w: %s", ErrInvalid, reason)
	}
	return nil
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
