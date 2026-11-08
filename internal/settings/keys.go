package settings

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/netip"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/simonhull/kitsune/internal/config"
)

// The keys there are, as server.toml spells them.
const (
	LastfmAPIKey         Key = "lastfm.api_key"       //nolint:gosec // G101: a key's name, not a credential.
	LastfmSharedSecret   Key = "lastfm.shared_secret" //nolint:gosec // G101: as above.
	LibraryRoots         Key = "library.roots"
	LibraryTagWrites     Key = "library.tag_writes"
	LibraryQuickScan     Key = "library.quick_scan"
	LibraryMissingGrace  Key = "library.missing_grace"
	LogLevel             Key = "log.level"
	ServerBaseURL        Key = "server.base_url"
	ServerTrustedProxies Key = "server.trusted_proxies"
)

// Limits on what a value may hold, so a request can't store anything large.
const (
	maxToken = 256                   // Bytes in a Last.fm key or secret.
	maxList  = 64                    // Roots or trusted proxies.
	maxPath  = 4096                  // Bytes in a root.
	maxGrace = 3650 * 24 * time.Hour // Ten years.
)

var (
	// ErrUnknownKey means no setting has that key.
	ErrUnknownKey = errors.New("unknown setting")
	// ErrBadValue means a value isn't one its key takes.
	ErrBadValue = errors.New("bad setting value")

	errToken    = errors.New("must be letters and digits, with no spaces")
	errTooMany  = fmt.Errorf("must list at most %d", maxList)
	errTooLong  = errors.New("is too long")
	errLevel    = errors.New(`want "debug", "info", "warn" or "error"`)
	errGrace    = errors.New(`want a duration from "0s" to "87600h", such as "720h"`)
	errDuration = errors.New(`want a duration, such as "720h"`)
	errBool     = errors.New(`want "true" or "false"`)

	errWantString = errors.New("want a JSON string")
	errWantBool   = errors.New("want true or false")
	errWantList   = errors.New("want a JSON list of strings")
)

// Key is a server setting.
type Key string

// String returns the key as server.toml spells it.
func (k Key) String() string { return string(k) }

// Keys lists every setting, in the order the Server page shows them.
func Keys() []Key {
	return []Key{
		LibraryRoots, LibraryTagWrites, LastfmAPIKey, LastfmSharedSecret,
		LibraryQuickScan, LibraryMissingGrace, ServerBaseURL, ServerTrustedProxies, LogLevel,
	}
}

// Lookup returns the key named name, or an error wrapping [ErrUnknownKey].
func Lookup(name string) (Key, error) {
	k := Key(name)
	if _, ok := defs()[k]; !ok {
		return "", fmt.Errorf("%q: %w", name, ErrUnknownKey)
	}
	return k, nil
}

// Secret reports whether k is write-only: its value never leaves the server.
func (k Key) Secret() bool { return k == LastfmSharedSecret }

// RestartRequired reports whether a change to k waits for a restart, because
// changing it live could cut off the page making the change.
func (k Key) RestartRequired() bool { return k == ServerBaseURL || k == ServerTrustedProxies }

// EnvName is the environment variable that seeds k, such as
// KITSUNE_LASTFM_API_KEY.
func (k Key) EnvName() string {
	return "KITSUNE_" + strings.ToUpper(strings.ReplaceAll(string(k), ".", "_"))
}

// Values are what every setting holds. A value of [Snapshot] is shared: never
// change its slices.
type Values struct {
	LastfmAPIKey       string
	LastfmSharedSecret string // Never logged or sent to a client.
	Roots              []string
	TagWrites          bool
	QuickScan          config.ClockTime
	MissingGrace       time.Duration
	LogLevel           slog.Level
	BaseURL            string
	TrustedProxies     []netip.Prefix
}

// LastfmEnabled reports whether both Last.fm values are set.
func (v *Values) LastfmEnabled() bool { return v.LastfmAPIKey != "" && v.LastfmSharedSecret != "" }

// Defaults are the values of keys nothing has set: [config.Default]'s.
func Defaults() Values {
	d := config.Default()
	return Values{
		Roots: []string{}, QuickScan: d.Library.QuickScan, MissingGrace: d.Library.MissingGrace,
		LogLevel: d.Log.Level, TrustedProxies: []netip.Prefix{},
	}
}

// def is how one key's value is read, checked and written. A value is JSON:
// decode checks it and sets it on Values, encode writes the canonical form
// back, text reads an environment variable or flag, and file reads the value
// from a config file. A clearable key's empty value is no value at all: it
// isn't stored, and the next start seeds it again.
type def interface {
	decode(v *Values, raw []byte) error
	encode(v *Values) []byte
	reset(v, defaults *Values)
	text(s string) ([]byte, error)
	file(c *config.Config) []byte
	empty(raw []byte) bool
}

// field is a [def] whose JSON form is W.
type field[W string | bool | []string] struct {
	get       func(v *Values) W
	set       func(v *Values, w W) error
	parseText func(s string) (W, error)
	fromFile  func(c *config.Config) W
	clearable bool
}

func (f field[W]) decode(v *Values, raw []byte) error {
	var w W
	if json.Unmarshal(raw, &w) != nil {
		// The decoder's error may quote the value, which can be a secret.
		switch any(w).(type) {
		case bool:
			return errWantBool
		case []string:
			return errWantList
		}
		return errWantString
	}
	return f.set(v, w)
}

func (f field[W]) encode(v *Values) []byte {
	b, err := json.Marshal(f.get(v))
	if err != nil { // A string, a bool or a list of strings always encodes.
		panic("settings: encode: " + err.Error())
	}
	return b
}

func (f field[W]) reset(v, defaults *Values) {
	_ = f.set(v, f.get(defaults)) // A default always passes its own rule.
}

func (f field[W]) text(s string) ([]byte, error) {
	w, err := f.parseText(s)
	if err != nil {
		return nil, err
	}
	var v Values
	if setErr := f.set(&v, w); setErr != nil {
		return nil, setErr
	}
	return f.encode(&v), nil
}

func (f field[W]) file(c *config.Config) []byte {
	b, err := json.Marshal(f.fromFile(c))
	if err != nil { // As in encode.
		panic("settings: encode: " + err.Error())
	}
	return b
}

func (f field[W]) empty(raw []byte) bool {
	if string(raw) == "null" {
		return true
	}
	if !f.clearable {
		return false
	}
	var w W
	if json.Unmarshal(raw, &w) != nil {
		return false
	}
	switch w := any(w).(type) {
	case string:
		return strings.TrimSpace(w) == ""
	case []string:
		return len(w) == 0
	}
	return false
}

// defs are every key's rules, made once.
//
//nolint:gochecknoglobals // A read-only table of the allowed keys, like prefs'.
var defs = sync.OnceValue(func() map[Key]def {
	all := libraryDefs()
	maps.Copy(all, timeDefs())
	return all
})

// libraryDefs are the rules of the library and Last.fm keys.
func libraryDefs() map[Key]def {
	return map[Key]def{
		LastfmAPIKey: field[string]{
			get:       func(v *Values) string { return v.LastfmAPIKey },
			set:       func(v *Values, w string) error { return setToken(&v.LastfmAPIKey, w) },
			parseText: identity,
			fromFile:  func(c *config.Config) string { return c.LastFM.APIKey },
			clearable: true,
		},
		LastfmSharedSecret: field[string]{
			get:       func(v *Values) string { return v.LastfmSharedSecret },
			set:       func(v *Values, w string) error { return setToken(&v.LastfmSharedSecret, w) },
			parseText: identity,
			fromFile:  func(c *config.Config) string { return string(c.LastFM.SharedSecret) },
			clearable: true,
		},
		LibraryRoots: field[[]string]{
			get:       func(v *Values) []string { return v.Roots },
			set:       setRoots,
			parseText: splitPaths,
			fromFile:  func(c *config.Config) []string { return c.Library.Roots },
			clearable: true,
		},
		LibraryTagWrites: field[bool]{
			get:       func(v *Values) bool { return v.TagWrites },
			set:       func(v *Values, w bool) error { v.TagWrites = w; return nil },
			parseText: parseBool,
			fromFile:  func(c *config.Config) bool { return c.Library.TagWrites },
		},
	}
}

// timeDefs are the rules of the keys that say when things happen, and of the
// server's own.
func timeDefs() map[Key]def {
	return map[Key]def{
		LibraryQuickScan: field[string]{
			get:       func(v *Values) string { return v.QuickScan.String() },
			set:       func(v *Values, w string) error { return v.QuickScan.UnmarshalText([]byte(strings.TrimSpace(w))) },
			parseText: identity,
			fromFile:  func(c *config.Config) string { return c.Library.QuickScan.String() },
		},
		LibraryMissingGrace: field[string]{
			get:       func(v *Values) string { return v.MissingGrace.String() },
			set:       setGrace,
			parseText: identity,
			fromFile:  func(c *config.Config) string { return c.Library.MissingGrace.String() },
		},
		LogLevel: field[string]{
			get:       func(v *Values) string { return levelName(v.LogLevel) },
			set:       setLevel,
			parseText: identity,
			fromFile:  func(c *config.Config) string { return levelName(c.Log.Level) },
		},
		ServerBaseURL: field[string]{
			get:       func(v *Values) string { return v.BaseURL },
			set:       setBaseURL,
			parseText: identity,
			fromFile:  func(c *config.Config) string { return c.Server.BaseURL },
			clearable: true,
		},
		ServerTrustedProxies: field[[]string]{
			get:       proxyStrings,
			set:       setProxies,
			parseText: splitCommas,
			fromFile: func(c *config.Config) []string {
				out := make([]string, len(c.Server.TrustedProxies))
				for i, n := range c.Server.TrustedProxies {
					out[i] = n.String()
				}
				return out
			},
			clearable: true,
		},
	}
}

func identity(s string) (string, error) { return s, nil }

func parseBool(s string) (bool, error) {
	on, err := strconv.ParseBool(strings.TrimSpace(s))
	if err != nil {
		return false, errBool
	}
	return on, nil
}

// splitPaths splits a list of paths joined as $PATH joins them, such as
// "/music:/audiobooks".
func splitPaths(s string) ([]string, error) { return filepath.SplitList(s), nil }

// splitCommas splits a list such as "10.0.0.2, 172.17.0.0/16", dropping
// empty items.
func splitCommas(s string) ([]string, error) {
	out := []string{}
	for item := range strings.SplitSeq(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out, nil
}

// setToken sets a Last.fm key or secret: printable, without spaces once the
// ends are trimmed, since a pasted value often carries a stray space.
func setToken(dst *string, w string) error {
	w = strings.TrimSpace(w)
	if len(w) > maxToken {
		return errTooLong
	}
	if strings.ContainsFunc(w, func(r rune) bool { return r > unicode.MaxASCII || !unicode.IsPrint(r) || r == ' ' }) {
		return errToken
	}
	*dst = w
	return nil
}

func setRoots(v *Values, w []string) error {
	if len(w) > maxList {
		return errTooMany
	}
	if slices.ContainsFunc(w, func(p string) bool { return len(p) > maxPath }) {
		return errTooLong
	}
	roots, err := config.CleanRoots(w)
	if err != nil {
		return err //nolint:wrapcheck // config's error says which root and why.
	}
	v.Roots = roots
	return nil
}

func setGrace(v *Values, w string) error {
	d, err := time.ParseDuration(strings.TrimSpace(w))
	if err != nil {
		return errDuration
	}
	if d < 0 || d > maxGrace {
		return errGrace
	}
	v.MissingGrace = d
	return nil
}

// levelName is the name of the level at or below l: debug, info, warn or
// error. A config file may hold one between them, such as "INFO+2".
func levelName(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	}
	return "error"
}

func setLevel(v *Values, w string) error {
	switch strings.ToLower(strings.TrimSpace(w)) {
	case "debug":
		v.LogLevel = slog.LevelDebug
	case "info":
		v.LogLevel = slog.LevelInfo
	case "warn":
		v.LogLevel = slog.LevelWarn
	case "error":
		v.LogLevel = slog.LevelError
	default:
		return errLevel
	}
	return nil
}

func setBaseURL(v *Values, w string) error {
	w = strings.TrimSpace(w)
	if err := config.CheckBaseURL(w); err != nil {
		return err //nolint:wrapcheck // config's error says why.
	}
	v.BaseURL = w
	return nil
}

func proxyStrings(v *Values) []string {
	out := make([]string, len(v.TrustedProxies))
	for i, p := range v.TrustedProxies {
		out[i] = p.String()
	}
	return out
}

func setProxies(v *Values, w []string) error {
	if len(w) > maxList {
		return errTooMany
	}
	out := make([]netip.Prefix, 0, len(w))
	for _, s := range w {
		var n config.Network
		if err := n.UnmarshalText([]byte(strings.TrimSpace(s))); err != nil {
			return err //nolint:wrapcheck // config's error names the value and says why.
		}
		if !slices.Contains(out, n.Prefix) {
			out = append(out, n.Prefix)
		}
	}
	v.TrustedProxies = out
	return nil
}
