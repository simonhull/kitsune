package library

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
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
	artistMBID, albumArtistMBID, compilation, albumMBID, trackMBID string
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
				compilation: "COMPILATION", albumMBID: "MUSICBRAINZ_ALBUMID", trackMBID: "MUSICBRAINZ_TRACKID",
			},
			artistIDs: []string{mbArtist1, "mb-artist-2"},
		},
		{
			name: "MP3 v2.4", sample: "sample_v24.mp3",
			keys: rawKeys{
				titleSort: "TSOT", artistSort: "TSOP", albumSort: "TSOA",
				albumArtist: "TPE2", albumArtistSort: "TSO2",
				artistMBID: keyID3ArtistID, albumArtistMBID: "MUSICBRAINZ ALBUM ARTIST ID",
				compilation: "TCMP", albumMBID: "MUSICBRAINZ ALBUM ID", trackMBID: "MUSICBRAINZ TRACK ID",
			},
			artistIDs: []string{mbArtist1},
		},
		{
			name: formatM4A, sample: "sample.m4a",
			keys: rawKeys{
				titleSort: "sonm", artistSort: "soar", albumSort: "soal",
				albumArtist: "aART", albumArtistSort: "soaa",
				artistMBID: keyID3ArtistID, albumArtistMBID: "MUSICBRAINZ ALBUM ARTIST ID",
				compilation: "cpil", albumMBID: "MUSICBRAINZ ALBUM ID", trackMBID: "MUSICBRAINZ TRACK ID",
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
				f.Tags.Set(tt.keys.trackMBID, "mb-track-1")
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
			wantSingle("track MBID", keysTrackMBID, "mb-track-1")
		})
	}
}

// TestDiscSubtitleKeysPerFormat writes a disc's name under each format's own key
// with audiometa and reads it back as a scan does, through the raw key and
// through the candidates. Only ID3v2.4 has a frame for it.
func TestDiscSubtitleKeysPerFormat(t *testing.T) {
	t.Parallel()
	const name = "The Electric Set"
	tests := []struct{ sample, key string }{
		{sampleFLAC, "DISCSUBTITLE"},
		{sampleMP3, "TSST"},
		{sampleMP3v23, "DISCSUBTITLE"}, // A TXXX: audiometa writes no TSST to ID3v2.3.
		{sampleM4A, "DISCSUBTITLE"},    // The freeform ----:com.apple.iTunes:DISCSUBTITLE.
	}
	for _, tt := range tests {
		t.Run(tt.sample, func(t *testing.T) {
			t.Parallel()
			path := copySample(t, tt.sample)
			if err := audiometa.Write(path, func(f *audiometa.File) error {
				f.Tags.Set(tt.key, name)
				return nil
			}); err != nil {
				t.Fatalf("Write: %v", err)
			}
			got := reread(t, path)
			if got.DiscSubtitle != name {
				t.Errorf("DiscSubtitle = %q, want %q", got.DiscSubtitle, name)
			}
			if v := rawValues(&got, tt.key); !slices.Equal(v, []string{name}) {
				t.Errorf("raw %s = %q, want %q; tags %+v", tt.key, v, name, got.Tags)
			}
		})
	}
}

const (
	rootMusic = "/music"
	formatM4A = "M4A"
)

// mapSample tags a copy of a sample through audiometa's typed fields, reopens
// it and maps it as a file in /music/X.
func mapSample(t *testing.T, sample, compilationKey string) store.TrackWrite {
	t.Helper()
	path := copySample(t, sample)
	err := audiometa.Write(path, func(f *audiometa.File) error {
		f.Tags.Title = "Round Trip"
		f.Tags.Artists = []string{"First Artist", "Second Artist"}
		f.Tags.Album = "The Album"
		f.Tags.AlbumArtist = "The Album Artist"
		f.Tags.TrackNumber, f.Tags.TrackTotal = 3, 12
		f.Tags.DiscNumber, f.Tags.DiscTotal = 1, 2
		f.Tags.Year = 2001
		f.Tags.Set(compilationKey, "1")
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
	return NewTrackWrite(f, &FileInfo{Root: rootMusic, RelPath: "X/" + sample, Size: f.Size})
}

func TestNewTrackWriteFromFiles(t *testing.T) {
	t.Parallel()
	samples := []struct{ name, sample, compilationKey, format string }{
		{"FLAC", "sample.flac", "COMPILATION", formatFLAC},
		{"MP3 v2.4", "sample_v24.mp3", "TCMP", "MP3"},
		{formatM4A, "sample.m4a", "cpil", formatM4A},
	}
	tws := make([]store.TrackWrite, len(samples))
	for i, tt := range samples {
		tws[i] = mapSample(t, tt.sample, tt.compilationKey)
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checkRoundTrip(t, &tws[i], tt.format)
		})
	}
	for i := 1; i < len(tws); i++ {
		if got, want := tws[i].Album.GroupKey, tws[0].Album.GroupKey; got != want {
			t.Errorf("GroupKey of %s = %q, want %q: the format must not change the album", samples[i].name, got, want)
		}
	}
}

func checkRoundTrip(t *testing.T, tw *store.TrackWrite, format string) {
	t.Helper()
	if tw.Format != format || tw.Title != "Round Trip" || !tw.Compilation {
		t.Errorf("Format, Title, Compilation = %q, %q, %t", tw.Format, tw.Title, tw.Compilation)
	}
	checkRoundTripNumbers(t, tw)
	checkRoundTripPeople(t, tw)
}

func checkRoundTripNumbers(t *testing.T, tw *store.TrackWrite) {
	t.Helper()
	if tw.TrackNo != 3 || tw.TrackTotal != 12 || tw.DiscNo != 1 || tw.DiscTotal != 2 || tw.Year != 2001 {
		t.Errorf("numbers wrong: %+v", tw)
	}
	if len(tw.Tags) == 0 || tw.Duration <= 0 {
		t.Errorf("Tags = %d, Duration = %v, want both non-empty", len(tw.Tags), tw.Duration)
	}
}

func checkRoundTripPeople(t *testing.T, tw *store.TrackWrite) {
	t.Helper()
	if len(tw.Artists) != 2 || tw.Artists[0].MatchKey != "first artist" || tw.Artists[1].Name != "Second Artist" {
		t.Errorf("Artists = %+v", tw.Artists)
	}
	if len(tw.AlbumArtists) != 1 || tw.AlbumArtists[0].MatchKey != "the album artist" {
		t.Errorf("AlbumArtists = %+v", tw.AlbumArtists)
	}
	if a := tw.Album; a.Name != "The Album" || a.Year != 2001 || !a.Compilation || a.GroupKey == "" {
		t.Errorf("Album = %+v", a)
	}
}
