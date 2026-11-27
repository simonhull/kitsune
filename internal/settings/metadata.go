package settings

import (
	"errors"
	"strings"

	"github.com/simonhull/kitsune/internal/config"
)

// The artist pass's keys (plan 072). server.toml doesn't carry them: the
// Server page sets them, and a KITSUNE_ variable seeds them; a -set flag may
// seed every one but the keys, which are secrets.
const (
	// MetadataRemote is the master switch for every lookup on the internet,
	// off until whoever runs the server turns it on.
	MetadataRemote Key = "metadata.remote"
	// MetadataFanart and MetadataWikipedia switch one source each, under it.
	MetadataFanart    Key = "metadata.fanarttv"
	MetadataWikipedia Key = "metadata.wikipedia"
	// MetadataLastfmBios asks Last.fm for the bio of an artist that has none
	// from its folder or Wikipedia, through lastfm.api_key: without that key it
	// is off whatever it says.
	MetadataLastfmBios Key = "metadata.lastfm_bios"
	// MetadataArchive asks the Cover Art Archive for the front cover of an
	// album whose files have none, and for a back cover its folder lacks.
	MetadataArchive Key = "metadata.coverartarchive"
	// MetadataLanguage is the two-letter language bios and art are wanted in.
	MetadataLanguage Key = "metadata.language"
	// FanartAPIKey is the fanart.tv project key, which fanart.tv needs: Kitsune
	// ships none, so fanart.tv is off until it is set.
	FanartAPIKey Key = "fanarttv.api_key" //nolint:gosec // G101: a key's name, not a credential.
	// FanartClientKey is a person's own fanart.tv key, which brings new art
	// sooner. Optional.
	FanartClientKey Key = "fanarttv.client_key"
)

// defaultLanguage is the language bios and art are wanted in until set.
const defaultLanguage = "en"

// errLanguage never quotes the value.
var errLanguage = errors.New("want a two-letter language code, such as \"en\"")

// metadataDefs are the rules of the artist pass's keys.
func metadataDefs() map[Key]def {
	return map[Key]def{
		MetadataRemote: field[bool]{
			get:       func(v *Values) bool { return v.MetadataRemote },
			set:       func(v *Values, w bool) error { v.MetadataRemote = w; return nil },
			parseText: parseBool,
			fromFile:  func(*config.Config) bool { return false },
		},
		MetadataFanart: field[bool]{
			get:       func(v *Values) bool { return v.MetadataFanart },
			set:       func(v *Values, w bool) error { v.MetadataFanart = w; return nil },
			parseText: parseBool,
			fromFile:  func(*config.Config) bool { return true },
		},
		MetadataWikipedia: field[bool]{
			get:       func(v *Values) bool { return v.MetadataWikipedia },
			set:       func(v *Values, w bool) error { v.MetadataWikipedia = w; return nil },
			parseText: parseBool,
			fromFile:  func(*config.Config) bool { return true },
		},
		MetadataLastfmBios: field[bool]{
			get:       func(v *Values) bool { return v.MetadataLastfmBios },
			set:       func(v *Values, w bool) error { v.MetadataLastfmBios = w; return nil },
			parseText: parseBool,
			fromFile:  func(*config.Config) bool { return true },
		},
		MetadataArchive: field[bool]{
			get:       func(v *Values) bool { return v.MetadataArchive },
			set:       func(v *Values, w bool) error { v.MetadataArchive = w; return nil },
			parseText: parseBool,
			fromFile:  func(*config.Config) bool { return true },
		},
		MetadataLanguage: field[string]{
			get:       func(v *Values) string { return v.MetadataLanguage },
			set:       setLanguage,
			parseText: identity,
			fromFile:  func(*config.Config) string { return defaultLanguage },
		},
		FanartAPIKey: field[string]{
			get:       func(v *Values) string { return v.FanartAPIKey },
			set:       func(v *Values, w string) error { return setToken(&v.FanartAPIKey, w) },
			parseText: identity,
			fromFile:  func(*config.Config) string { return "" },
			clearable: true,
		},
		FanartClientKey: field[string]{
			get:       func(v *Values) string { return v.FanartClientKey },
			set:       func(v *Values, w string) error { return setToken(&v.FanartClientKey, w) },
			parseText: identity,
			fromFile:  func(*config.Config) string { return "" },
			clearable: true,
		},
	}
}

// setLanguage sets a two-letter code, lower-cased.
func setLanguage(v *Values, w string) error {
	w = strings.ToLower(strings.TrimSpace(w))
	if len(w) != 2 || strings.ContainsFunc(w, func(r rune) bool { return r < 'a' || r > 'z' }) {
		return errLanguage
	}
	v.MetadataLanguage = w
	return nil
}
