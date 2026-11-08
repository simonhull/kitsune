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
	if len(seen) != 13 {
		t.Errorf("%d columns, want the spec's 13", len(seen))
	}
}
