package library

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/audiometa"
)

// copySample copies testdata/name into a new temp dir and returns its path.
func copySample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close() //nolint:errcheck // Read-only use is done; nothing to report.
	if err = root.WriteFile(name, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, name)
}

const mbArtist1 = "mb-artist-1"

// rawKeys is the key a format uses for each field the candidate tables cover.
type rawKeys struct {
	titleSort, artistSort, albumSort, albumArtist, albumArtistSort string
	artistMBID, albumArtistMBID, compilation, albumMBID            string
}

func TestRawKeysPerFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, sample string
		keys         rawKeys
		artistIDs    []string // Written and expected for the artist MBID.
	}{
		{
			name: formatFLAC, sample: "sample.flac",
			keys: rawKeys{
				titleSort: "TITLESORT", artistSort: keyArtistSort, albumSort: "ALBUMSORT",
				albumArtist: "ALBUMARTIST", albumArtistSort: "ALBUMARTISTSORT",
				artistMBID: keyArtistMBID, albumArtistMBID: "MUSICBRAINZ_ALBUMARTISTID",
				compilation: "COMPILATION", albumMBID: "MUSICBRAINZ_ALBUMID",
			},
			artistIDs: []string{mbArtist1, "mb-artist-2"},
		},
		{
			name: "MP3 v2.4", sample: "sample_v24.mp3",
			keys: rawKeys{
				titleSort: "TSOT", artistSort: "TSOP", albumSort: "TSOA",
				albumArtist: "TPE2", albumArtistSort: "TSO2",
				artistMBID: keyID3ArtistID, albumArtistMBID: "MUSICBRAINZ ALBUM ARTIST ID",
				compilation: "TCMP", albumMBID: "MUSICBRAINZ ALBUM ID",
			},
			artistIDs: []string{mbArtist1},
		},
		{
			name: "M4A", sample: "sample.m4a",
			keys: rawKeys{
				titleSort: "sonm", artistSort: "soar", albumSort: "soal",
				albumArtist: "aART", albumArtistSort: "soaa",
				artistMBID: keyID3ArtistID, albumArtistMBID: "MUSICBRAINZ ALBUM ARTIST ID",
				compilation: "cpil", albumMBID: "MUSICBRAINZ ALBUM ID",
			},
			artistIDs: []string{mbArtist1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, tt.sample)
			albumArtists := []string{"Album Artist X", "Album Artist Y"}
			artistSorts := []string{"Sort A", "Sort B"}
			err := audiometa.Write(path, func(f *audiometa.File) error {
				f.Tags.Set(tt.keys.titleSort, "Title Sort")
				f.Tags.Set(tt.keys.artistSort, artistSorts...)
				f.Tags.Set(tt.keys.albumSort, "Album Sort")
				f.Tags.Set(tt.keys.albumArtist, albumArtists...)
				f.Tags.Set(tt.keys.albumArtistSort, "Album Artist Sort")
				f.Tags.Set(tt.keys.artistMBID, tt.artistIDs...)
				f.Tags.Set(tt.keys.albumArtistMBID, "mb-albumartist-1")
				f.Tags.Set(tt.keys.compilation, "1")
				f.Tags.Set(tt.keys.albumMBID, "mb-album-1")
				return nil
			})
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			f, err := audiometa.Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			t.Cleanup(func() {
				if cerr := f.Close(); cerr != nil {
					t.Error(cerr)
				}
			})
			dump := func() string {
				var sb strings.Builder
				for k, v := range f.Tags.All() {
					fmt.Fprintf(&sb, "\n\t%q = %q", k, v)
				}
				return sb.String()
			}
			wantMulti := func(field string, keys, want []string) {
				t.Helper()
				if got := multi(&f.Tags, keys); !slices.Equal(got, want) {
					t.Errorf("%s = %q, want %q; stored:%s", field, got, want, dump())
				}
			}
			wantSingle := func(field string, keys []string, want string) {
				t.Helper()
				if got := single(&f.Tags, keys); got != want {
					t.Errorf("%s = %q, want %q; stored:%s", field, got, want, dump())
				}
			}
			wantSingle("title sort", keysTitleSort, "Title Sort")
			wantMulti("artist sort", keysArtistSort, artistSorts)
			wantSingle("album sort", keysAlbumSort, "Album Sort")
			wantMulti("album artist", keysAlbumArtist, albumArtists)
			wantMulti("album artist sort", keysAlbumArtistSort, []string{"Album Artist Sort"})
			wantMulti("artist MBID", keysArtistMBID, tt.artistIDs)
			wantMulti("album artist MBID", keysAlbumArtistMBID, []string{"mb-albumartist-1"})
			wantSingle("compilation", keysCompilation, "1")
			wantSingle("album MBID", keysAlbumMBID, "mb-album-1")
		})
	}
}
