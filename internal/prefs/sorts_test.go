package prefs_test

import (
	"errors"
	"testing"

	"github.com/simonhull/kitsune/internal/prefs"
)

func TestSortChecks(t *testing.T) {
	t.Parallel()
	const (
		library = `{"artists":{"by":"name","dir":"asc"},"albums":{"by":"year","dir":"desc"}}`
		rating  = `{"by":"rating","dir":"desc"}`
	)
	tests := []struct {
		key, in, want string // An empty want is a refusal.
	}{
		{prefs.LibrarySort, library, library},
		{
			prefs.LibrarySort, `{ "albums" : {"dir":"asc","by":"added"}, "artists": {"dir":"desc","by":"plays"} }`,
			`{"artists":{"by":"plays","dir":"desc"},"albums":{"by":"added","dir":"asc"}}`,
		},
		{prefs.LibrarySort, `{"artists":{"by":"year","dir":"asc"},"albums":{"by":"year","dir":"desc"}}`, ""},
		{prefs.LibrarySort, `{"artists":{"by":"name","dir":"asc"},"albums":{"by":"plays","dir":"desc"}}`, ""},
		{prefs.LibrarySort, `{"artists":{"by":"name","dir":"asc"}}`, ""},
		{prefs.LibrarySort, `{"artists":{"by":"name"},"albums":{"by":"year","dir":"desc"}}`, ""},
		{prefs.LibrarySort, `{"artists":{"by":"name","dir":"asc"},"albums":{"by":"year","dir":"desc"},"x":1}`, ""},
		{prefs.LibrarySort, `{"artists":null,"albums":{"by":"year","dir":"desc"}}`, ""},
		{prefs.AlbumsSort, rating, rating},
		{prefs.AlbumsSort, `{"by":"artist","dir":"asc"}`, `{"by":"artist","dir":"asc"}`},
		{prefs.AlbumsSort, `{"by":"albums","dir":"asc"}`, ""},
		{prefs.AlbumsSort, `{"by":"name","dir":"up"}`, ""},
		{prefs.AlbumsSort, `{"by":"name","dir":"ASC"}`, ""},
		{prefs.AlbumsSort, `{"by":"name"}`, ""},
		{prefs.AlbumsSort, `"name"`, ""},
		{prefs.ArtistsSort, `{"by":"albums","dir":"desc"}`, `{"by":"albums","dir":"desc"}`},
		{prefs.ArtistsSort, `{"by":"plays","dir":"asc"}`, `{"by":"plays","dir":"asc"}`},
		{prefs.ArtistsSort, rating, ""},
		{prefs.ArtistsSort, `{"by":"name","dir":"asc","then":"id"}`, ""},
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
