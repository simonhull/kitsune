package prefs_test

import (
	"bytes"
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

// The default columns, as a value reads back: the number and the title first.
const defaults = `{"order":["number","title","artist","genre","rating","plays","lastPlayed","format"],"artistWhenDifferent":true}`

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
			"saved before number and title moved", `{"order":["artist","genre","rating","plays","lastPlayed","format"],"artistWhenDifferent":true}`,
			defaults,
		},
		{
			"defaults", defaults, defaults,
		},
		{
			"moved", `{"order":["artist","title","genre","number"],"artistWhenDifferent":false}`,
			`{"order":["artist","title","genre","number"],"artistWhenDifferent":false}`,
		},
		{"title only", `{"order":["genre","title"],"artistWhenDifferent":true}`, `{"order":["number","genre","title"],"artistWhenDifferent":true}`},
		{"number only", `{"order":["genre","number"],"artistWhenDifferent":true}`, `{"order":["genre","number","title"],"artistWhenDifferent":true}`},
		{"none shown", `{"artistWhenDifferent":false,"order":[]}`, `{"order":["number","title"],"artistWhenDifferent":false}`},
		{
			"widths", `{"order":["genre"],"artistWhenDifferent":true,"widths":{"rating":96,"genre":180}}`,
			`{"order":["number","title","genre"],"artistWhenDifferent":true,"widths":{"genre":180,"rating":96}}`,
		},
		{"no widths", `{"order":[],"artistWhenDifferent":true,"widths":{}}`, `{"order":["number","title"],"artistWhenDifferent":true}`},
		{
			"saved with the heart", `{"order":["number","title","heart","rating"],"artistWhenDifferent":true,"widths":{"heart":40,"rating":96}}`,
			`{"order":["number","title","rating"],"artistWhenDifferent":true,"widths":{"rating":96}}`,
		},
		{"width of nothing", `{"order":[],"artistWhenDifferent":true,"widths":{"skips":80}}`, ""},
		{"width of the title", `{"order":[],"artistWhenDifferent":true,"widths":{"title":300}}`, ""},
		{"width of the number", `{"order":[],"artistWhenDifferent":true,"widths":{"number":30}}`, ""},
		{"too narrow", `{"order":[],"artistWhenDifferent":true,"widths":{"genre":15}}`, ""},
		{"too wide", `{"order":[],"artistWhenDifferent":true,"widths":{"genre":961}}`, ""},
		{"not a width", `{"order":[],"artistWhenDifferent":true,"widths":{"genre":"wide"}}`, ""},
		{"spaced", "{ \"order\" : [ \"bitrate\" ] , \"artistWhenDifferent\" : true }", `{"order":["number","title","bitrate"],"artistWhenDifferent":true}`},
		{"unknown", `{"order":["skips"],"artistWhenDifferent":true}`, ""},
		{"twice", `{"order":["year","year"],"artistWhenDifferent":true}`, ""},
		{"title twice", `{"order":["title","year","title"],"artistWhenDifferent":true}`, ""},
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
	if len(seen) != 13 {
		t.Errorf("%d columns, want the spec's 13, the heart gone", len(seen))
	}
	placed := prefs.Placed()
	if len(placed) != 15 || placed[0] != prefs.ColumnNumber || placed[1] != prefs.ColumnTitle {
		t.Errorf("Placed() = %v, want number, title and the 13", placed)
	}
}

// A checked value checks to itself: the way out accepts what the way in wrote.
func TestColumnsCheckIsStable(t *testing.T) {
	t.Parallel()
	key, lookupErr := prefs.Lookup(prefs.LibraryColumns)
	if lookupErr != nil {
		t.Fatal(lookupErr)
	}
	for _, in := range []string{
		`{"order":["artist"],"artistWhenDifferent":true}`,
		`{"order":["title","artist","number"],"artistWhenDifferent":false,"widths":{"artist":150}}`,
	} {
		once, err := key.Check([]byte(in))
		if err != nil {
			t.Fatalf("Check(%s) = %v", in, err)
		}
		twice, err := key.Check(once)
		if err != nil || !bytes.Equal(twice, once) {
			t.Errorf("Check(%s) = %s, %v, want %s", once, twice, err, once)
		}
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
