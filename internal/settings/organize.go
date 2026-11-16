package settings

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"path/filepath"

	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/organize"
)

// The organizer's keys. server.toml doesn't carry them: the Organize page sets
// them, and a KITSUNE_ variable or -set flag may seed them.
const (
	OrganizeTemplate         Key = "organize.template"
	OrganizeCompilations     Key = "organize.compilations"
	OrganizeRoots            Key = "organize.roots"
	OrganizeOnEdit           Key = "organize.on_edit"
	OrganizeCompanions       Key = "organize.companions"
	OrganizeRewritePlaylists Key = "organize.rewrite_playlists"
)

var (
	// errTemplate never quotes the template: a key's error never holds its value.
	errTemplate   = errors.New("isn't a template the organizer reads; the Organize page says where it goes wrong")
	errRootPath   = errors.New("each folder must be an absolute path")
	errWantObject = errors.New(`want a JSON object of folders, each {"template": …, "compilations": …}`)
)

// RootTemplates are the templates of one library folder that doesn't use the
// main pair.
type RootTemplates struct {
	Template     string `json:"template"`
	Compilations string `json:"compilations"`
}

// organizeDefs are the rules of the organizer's keys.
func organizeDefs() map[Key]def {
	return map[Key]def{
		OrganizeTemplate: field[string]{
			get:       func(v *Values) string { return v.OrganizeTemplate },
			set:       func(v *Values, w string) error { return setTemplate(&v.OrganizeTemplate, w) },
			parseText: identity,
			fromFile:  func(*config.Config) string { return organize.DefaultTemplate },
		},
		OrganizeCompilations: field[string]{
			get:       func(v *Values) string { return v.OrganizeCompilations },
			set:       func(v *Values, w string) error { return setTemplate(&v.OrganizeCompilations, w) },
			parseText: identity,
			fromFile:  func(*config.Config) string { return organize.DefaultCompilations },
		},
		OrganizeRoots: rootsField{},
		OrganizeOnEdit: field[bool]{
			get:       func(v *Values) bool { return v.OrganizeOnEdit },
			set:       func(v *Values, w bool) error { v.OrganizeOnEdit = w; return nil },
			parseText: parseBool,
			fromFile:  func(*config.Config) bool { return false },
		},
		OrganizeCompanions: field[bool]{
			get:       func(v *Values) bool { return v.OrganizeCompanions },
			set:       func(v *Values, w bool) error { v.OrganizeCompanions = w; return nil },
			parseText: parseBool,
			fromFile:  func(*config.Config) bool { return true },
		},
		OrganizeRewritePlaylists: field[bool]{
			get:       func(v *Values) bool { return v.OrganizeRewritePlaylists },
			set:       func(v *Values, w bool) error { v.OrganizeRewritePlaylists = w; return nil },
			parseText: parseBool,
			fromFile:  func(*config.Config) bool { return false },
		},
	}
}

// setTemplate sets dst to w when it parses as a template.
func setTemplate(dst *string, w string) error {
	if _, err := organize.Parse(w); err != nil {
		return errTemplate
	}
	*dst = w
	return nil
}

// rootsField is organize.roots: a JSON object of library folders, each with
// its pair of templates. Empty is none, which isn't stored.
type rootsField struct{}

func (rootsField) decode(v *Values, raw []byte) error {
	var w map[string]RootTemplates
	if json.Unmarshal(raw, &w) != nil {
		return errWantObject
	}
	if len(w) > maxList {
		return errTooMany
	}
	out := make(map[string]RootTemplates, len(w))
	for root, ts := range w {
		if len(root) > maxPath {
			return errTooLong
		}
		if !filepath.IsAbs(root) {
			return errRootPath
		}
		if _, err := organize.Parse(ts.Template); err != nil {
			return errTemplate
		}
		if _, err := organize.Parse(ts.Compilations); err != nil {
			return errTemplate
		}
		out[filepath.Clean(root)] = ts
	}
	v.OrganizeRoots = out
	return nil
}

func (rootsField) encode(v *Values) []byte {
	m := v.OrganizeRoots
	if m == nil {
		m = map[string]RootTemplates{}
	}
	b, err := json.Marshal(m, json.Deterministic(true))
	if err != nil { // A map of strings always encodes.
		panic("settings: encode: " + err.Error())
	}
	return b
}

func (rootsField) reset(v, _ *Values) { v.OrganizeRoots = map[string]RootTemplates{} }

func (f rootsField) text(s string) ([]byte, error) {
	var v Values
	if err := f.decode(&v, []byte(s)); err != nil {
		return nil, err
	}
	return f.encode(&v), nil
}

func (rootsField) file(*config.Config) []byte { return []byte("{}") }

func (rootsField) empty(raw []byte) bool {
	t := bytes.TrimSpace(raw)
	return string(t) == "null" || string(t) == "{}"
}
