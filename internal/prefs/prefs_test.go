package prefs_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/prefs"
)

func TestLookup(t *testing.T) {
	t.Parallel()
	if k, err := prefs.Lookup(prefs.LibraryColumns); err != nil || k.Name() != prefs.LibraryColumns {
		t.Errorf("Lookup(%q) = %v, %v", prefs.LibraryColumns, k.Name(), err)
	}
	if k, err := prefs.Lookup(prefs.UITheme); err != nil || k.Name() != prefs.UITheme {
		t.Errorf("Lookup(%q) = %v, %v", prefs.UITheme, k.Name(), err)
	}
	if _, err := prefs.Lookup("server.roots"); !errors.Is(err, prefs.ErrUnknownKey) {
		t.Errorf("Lookup(server.roots) = %v, want ErrUnknownKey", err)
	}
}

func TestColumnsCheck(t *testing.T) {
	t.Parallel()
	key, lookupErr := prefs.Lookup(prefs.LibraryColumns)
	if lookupErr != nil {
		t.Fatal(lookupErr)
	}
	tests := []struct {
		name, in, want string // An empty want is a refusal.
	}{
		{
			"defaults", `{"order":["artist","genre","rating","plays","lastPlayed","format"],"artistWhenDifferent":true}`,
			`{"order":["artist","genre","rating","plays","lastPlayed","format"],"artistWhenDifferent":true}`,
		},
		{"none shown", `{"artistWhenDifferent":false,"order":[]}`, `{"order":[],"artistWhenDifferent":false}`},
		{
			"widths", `{"order":["genre"],"artistWhenDifferent":true,"widths":{"rating":96,"genre":180}}`,
			`{"order":["genre"],"artistWhenDifferent":true,"widths":{"genre":180,"rating":96}}`,
		},
		{"no widths", `{"order":[],"artistWhenDifferent":true,"widths":{}}`, `{"order":[],"artistWhenDifferent":true}`},
		{"width of nothing", `{"order":[],"artistWhenDifferent":true,"widths":{"skips":80}}`, ""},
		{"too narrow", `{"order":[],"artistWhenDifferent":true,"widths":{"genre":15}}`, ""},
		{"too wide", `{"order":[],"artistWhenDifferent":true,"widths":{"genre":961}}`, ""},
		{"not a width", `{"order":[],"artistWhenDifferent":true,"widths":{"genre":"wide"}}`, ""},
		{"spaced", "{ \"order\" : [ \"bitrate\" ] , \"artistWhenDifferent\" : true }", `{"order":["bitrate"],"artistWhenDifferent":true}`},
		{"unknown", `{"order":["skips"],"artistWhenDifferent":true}`, ""},
		{"twice", `{"order":["year","year"],"artistWhenDifferent":true}`, ""},
		{"missing", `{"order":[]}`, ""},
		{"null", `{"order":null,"artistWhenDifferent":true}`, ""},
		{"extra", `{"order":[],"artistWhenDifferent":true,"x":1}`, ""},
		{"wrong type", `{"order":"artist","artistWhenDifferent":true}`, ""},
		{"too long", `{"order":[],"artistWhenDifferent":true}` + strings.Repeat(" ", prefs.MaxValue), ""},
		{"not JSON", "{", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := key.Check([]byte(tt.in))
			if tt.want == "" {
				if !errors.Is(err, prefs.ErrBadValue) {
					t.Errorf("Check = %s, %v, want ErrBadValue", got, err)
				}
				return
			}
			if err != nil || string(got) != tt.want {
				t.Errorf("Check = %s, %v, want %s", got, err, tt.want)
			}
		})
	}
}

func TestColumnsListsEachOnce(t *testing.T) {
	t.Parallel()
	seen := map[prefs.Column]bool{}
	for _, c := range prefs.Columns() {
		if seen[c] {
			t.Errorf("%s is listed twice", c)
		}
		seen[c] = true
	}
	if len(seen) != 14 {
		t.Errorf("%d columns, want the spec's 14", len(seen))
	}
}

func TestThemeCheck(t *testing.T) {
	t.Parallel()
	key, lookupErr := prefs.Lookup(prefs.UITheme)
	if lookupErr != nil {
		t.Fatal(lookupErr)
	}
	quoted := func(theme prefs.Theme) string { return `"` + string(theme) + `"` }
	tests := []struct {
		name, in, want string // An empty want is a refusal.
	}{
		{"system", quoted(prefs.ThemeSystem), quoted(prefs.ThemeSystem)},
		{"light", quoted(prefs.ThemeLight), quoted(prefs.ThemeLight)},
		{"dark", quoted(prefs.ThemeDark), quoted(prefs.ThemeDark)},
		{"spaced", " " + quoted(prefs.ThemeDark) + "\n", quoted(prefs.ThemeDark)},
		{"unknown", `"sepia"`, ""},
		{"case", `"Light"`, ""},
		{"empty", `""`, ""},
		{"nothing", "null", ""},
		{"object", `{"theme":"dark"}`, ""},
		{"bare word", string(prefs.ThemeDark), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := key.Check([]byte(tt.in))
			if tt.want == "" {
				if !errors.Is(err, prefs.ErrBadValue) {
					t.Errorf("Check = %s, %v, want ErrBadValue", got, err)
				}
				return
			}
			if err != nil || string(got) != tt.want {
				t.Errorf("Check = %s, %v, want %s", got, err, tt.want)
			}
		})
	}
}

func TestShuffleChecks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key, in, want string // An empty want is a refusal.
	}{
		{prefs.ShuffleRatings, "0.5", "0.5"},
		{prefs.ShuffleRatings, " 1 ", "1"},
		{prefs.ShuffleFlow, "0", "0"},
		{prefs.ShuffleFlow, "1.5", ""},
		{prefs.ShuffleFlow, "-0.1", ""},
		{prefs.ShuffleRatings, `"0.5"`, ""},
		{prefs.ShuffleMemory, "false", "false"},
		{prefs.ShuffleMemory, "1", ""},
		{prefs.PlaybackUntaggedGain, "-8", "-8"},
		{prefs.PlaybackUntaggedGain, "0", "0"},
		{prefs.PlaybackUntaggedGain, "-20.5", ""},
		{prefs.PlaybackUntaggedGain, "3", ""},
	}
	for _, tt := range tests {
		t.Run(tt.key+" "+tt.in, func(t *testing.T) {
			t.Parallel()
			key, err := prefs.Lookup(tt.key)
			if err != nil {
				t.Fatal(err)
			}
			got, err := key.Check([]byte(tt.in))
			if tt.want == "" {
				if !errors.Is(err, prefs.ErrBadValue) {
					t.Errorf("Check = %s, %v, want ErrBadValue", got, err)
				}
				return
			}
			if err != nil || string(got) != tt.want {
				t.Errorf("Check = %s, %v, want %s", got, err, tt.want)
			}
		})
	}
}
